//    Copyright 2018 Immutable Systems, Inc.
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package aws

import (
	"context"
	"encoding/base64"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/immutable/metavisor-cli/pkg/logging"
)

const (
	tagSpecInstance = "instance"
	tagSpecVolume   = "volume"
)

type instance struct {
	resource
	instanceType    string
	rootDeviceName  string
	deviceMapping   map[string]string
	pubIP, privIP   string
	zone            string
	sriovNetSupport string
	enaSupport      bool
}

func (i *instance) InstanceType() string             { return i.instanceType }
func (i *instance) RootDeviceName() string           { return i.rootDeviceName }
func (i *instance) DeviceMapping() map[string]string { return i.deviceMapping }
func (i *instance) PublicIP() string                 { return i.pubIP }
func (i *instance) PrivateIP() string                { return i.privIP }
func (i *instance) AvailabilityZone() string         { return i.zone }
func (i *instance) SriovNetSupport() string          { return i.sriovNetSupport }
func (i *instance) ENASupport() bool                 { return i.enaSupport }

func (a *awsService) GetInstance(ctx context.Context, instanceID string) (Instance, error) {
	if strings.TrimSpace(instanceID) == "" {
		return nil, ErrInstanceNonExisting
	}
	input := &ec2.DescribeInstancesInput{
		InstanceIds: aws.StringSlice([]string{instanceID}),
	}
	out, err := a.client.DescribeInstancesWithContext(ctx, input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return nil, ErrNotAllowed
		}
		if ok && strings.Contains(aerr.Code(), instanceIDErrorCode) {
			return nil, ErrInstanceNonExisting
		}
		return nil, err
	}
	for _, reservation := range out.Reservations {
		for _, inst := range reservation.Instances {
			var puIP, prIP string
			if inst.PublicIpAddress != nil {
				puIP = *inst.PublicIpAddress
			}
			if inst.PrivateIpAddress != nil {
				prIP = *inst.PrivateIpAddress
			}
			var zone string
			if inst.Placement != nil && inst.Placement.AvailabilityZone != nil {
				zone = *inst.Placement.AvailabilityZone
			}
			var sriovSupport string
			if inst.SriovNetSupport != nil {
				sriovSupport = *inst.SriovNetSupport
			}
			var enaSupport bool
			if inst.EnaSupport != nil {
				enaSupport = *inst.EnaSupport
			}
			res := &instance{
				resource: resource{
					id: *inst.InstanceId,
				},
				instanceType:    *inst.InstanceType,
				rootDeviceName:  *inst.RootDeviceName,
				deviceMapping:   blockToMap(inst.BlockDeviceMappings),
				pubIP:           puIP,
				privIP:          prIP,
				zone:            zone,
				sriovNetSupport: sriovSupport,
				enaSupport:      enaSupport,
			}
			return res, nil
		}
	}
	// If we got this far, the instance doesn't exist
	return nil, ErrInstanceNonExisting
}

func (a *awsService) LaunchInstance(ctx context.Context, imageID, instanceType, userData, keyName, subnetID string, tags map[string]string, extraDevices ...NewDevice) (Instance, error) {
	if !IsAMIID(imageID) {
		return nil, ErrInvalidAMIID
	}
	if strings.TrimSpace(keyName) != "" {
		if keyExist, err := a.KeyPairExist(ctx, keyName); (err != nil && err != ErrNotAllowed) || (!keyExist && err == nil) {
			// If we don't have permission to list keys, we assume it exist and continue
			if err == nil {
				err = ErrKeyNonExisting
			}
			return nil, err
		}
	}
	blockDeviceMapping := []*ec2.BlockDeviceMapping{}
	for _, dev := range extraDevices {
		blockDevice := &ec2.BlockDeviceMapping{
			DeviceName: aws.String(dev.DeviceName),
		}
		ebsDevice := &ec2.EbsBlockDevice{
			DeleteOnTermination: aws.Bool(true),
			VolumeType:          aws.String(genericVolumeType),
			SnapshotId:          aws.String(dev.SnapshotID),
		}
		blockDevice.Ebs = ebsDevice
		blockDeviceMapping = append(blockDeviceMapping, blockDevice)
	}
	input := &ec2.RunInstancesInput{
		EbsOptimized: aws.Bool(false),
		ImageId:      aws.String(imageID),
		InstanceType: aws.String(instanceType),
		MinCount:     aws.Int64(1),
		MaxCount:     aws.Int64(1),
	}
	if strings.TrimSpace(subnetID) != "" {
		input.SubnetId = aws.String(subnetID)
	}
	if strings.TrimSpace(userData) != "" {
		input.UserData = aws.String(base64.StdEncoding.EncodeToString([]byte(userData)))
	}
	if len(blockDeviceMapping) > 0 {
		input.BlockDeviceMappings = blockDeviceMapping
	}
	if strings.TrimSpace(keyName) != "" {
		input.KeyName = aws.String(keyName)
	} else {
		logging.Debug("Launching instance without key pair")
	}
	if tags != nil {
		// Created tag specification so that tags apply to both the
		// launched instance and its associated volume
		awsTags := []*ec2.Tag{}
		if _, exist := tags[cliResourceTagKey]; !exist {
			tags[cliResourceTagKey] = cliResourceTagValue
		}
		for key, val := range tags {
			awsTags = append(awsTags, &ec2.Tag{
				Key:   aws.String(key),
				Value: aws.String(val),
			})
			logging.Debugf("Launching instance with tag \"%s: %s\"", key, val)
		}
		tagSpec := []*ec2.TagSpecification{
			&ec2.TagSpecification{
				ResourceType: aws.String(tagSpecInstance),
				Tags:         awsTags,
			},
			&ec2.TagSpecification{
				ResourceType: aws.String(tagSpecVolume),
				Tags:         awsTags,
			},
		}
		input.TagSpecifications = tagSpec
	}

	out, err := a.client.RunInstancesWithContext(ctx, input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return nil, ErrNotAllowed
		} else if ok && strings.Contains(aerr.Code(), vpcNotFoundErrorCode) {
			return nil, ErrRequiresSubnet
		} else if ok && aerr.Code() == subnetNotFoundErrorCode {
			return nil, ErrInvalidSubnetID
		}
		return nil, err
	}
	for _, inst := range out.Instances {
		var puIP, prIP string
		if inst.PublicIpAddress != nil {
			puIP = *inst.PublicIpAddress
		}
		if inst.PrivateIpAddress != nil {
			prIP = *inst.PrivateIpAddress
		}
		var zone string
		if inst.Placement != nil && inst.Placement.AvailabilityZone != nil {
			zone = *inst.Placement.AvailabilityZone
		}
		var sriovSupport string
		if inst.SriovNetSupport != nil {
			sriovSupport = *inst.SriovNetSupport
		}
		var enaSupport bool
		if inst.EnaSupport != nil {
			enaSupport = *inst.EnaSupport
		}
		res := &instance{
			resource: resource{
				id: *inst.InstanceId,
			},
			instanceType:    *inst.InstanceType,
			rootDeviceName:  *inst.RootDeviceName,
			deviceMapping:   blockToMap(inst.BlockDeviceMappings),
			pubIP:           puIP,
			privIP:          prIP,
			zone:            zone,
			sriovNetSupport: sriovSupport,
			enaSupport:      enaSupport,
		}
		return res, nil
	}
	// If this is reached, we never launched any instance
	return nil, ErrFailedLaunchingInstance
}

func (a *awsService) StopInstance(ctx context.Context, instanceID string) error {
	if strings.TrimSpace(instanceID) == "" {
		return ErrInvalidName
	}
	input := &ec2.StopInstancesInput{
		InstanceIds: aws.StringSlice([]string{instanceID}),
	}
	_, err := a.client.StopInstancesWithContext(ctx, input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return ErrNotAllowed
		} else if ok && strings.Contains(aerr.Code(), instanceIDErrorCode) {
			logging.Error("Attempted to stop non-existing instance")
			return ErrInstanceNonExisting
		}
		return err
	}
	return nil
}

func (a *awsService) StartInstance(ctx context.Context, instanceID string) error {
	if strings.TrimSpace(instanceID) == "" {
		return ErrInvalidName
	}
	input := &ec2.StartInstancesInput{
		InstanceIds: aws.StringSlice([]string{instanceID}),
	}
	_, err := a.client.StartInstancesWithContext(ctx, input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return ErrNotAllowed
		} else if ok && strings.Contains(aerr.Code(), instanceIDErrorCode) {
			logging.Error("Attempted to start non-existing instance")
			return ErrInstanceNonExisting
		}
		return err
	}
	return nil
}

func (a *awsService) TerminateInstance(ctx context.Context, instanceID string) error {
	if strings.TrimSpace(instanceID) == "" {
		return ErrInvalidName
	}
	input := &ec2.TerminateInstancesInput{
		InstanceIds: aws.StringSlice([]string{instanceID}),
	}
	_, err := a.client.TerminateInstancesWithContext(ctx, input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return ErrNotAllowed
		} else if ok && strings.Contains(aerr.Code(), instanceIDErrorCode) {
			logging.Debug("Attempted to terminate non-existing instance")
			return nil
		}
		return err
	}
	return nil
}

func (a *awsService) ModifyInstanceAttribute(ctx context.Context, instanceID string, attr instanceAttribute, value interface{}) error {
	if strings.TrimSpace(instanceID) == "" {
		return ErrInvalidInstanceID
	}
	input := &ec2.ModifyInstanceAttributeInput{
		InstanceId: aws.String(instanceID),
	}
	switch attr {
	case AttrSriovNetSupport:
		valueStr, ok := value.(string)
		if !ok {
			logging.Error("Expected sriovNetSupport value to be a string")
			return ErrInvalidInstanceAttrValue
		}
		input.SriovNetSupport = &ec2.AttributeValue{
			Value: aws.String(valueStr),
		}
		break
	case AttrENASupport:
		valueBool, ok := value.(bool)
		if !ok {
			logging.Error("Expected ENA support value to be a bool")
			return ErrInvalidInstanceAttrValue
		}
		input.EnaSupport = &ec2.AttributeBooleanValue{
			Value: aws.Bool(valueBool),
		}
		break
	case AttrUserData:
		valueStr, ok := value.(string)
		if !ok {
			logging.Error("Expected userdata value to be a string")
		}
		input.UserData = &ec2.BlobAttributeValue{
			Value: []byte(valueStr),
		}
		break
	default:
		return ErrInvalidInstanceAttr
	}
	_, err := a.client.ModifyInstanceAttributeWithContext(ctx, input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return ErrNotAllowed
		}
		return err
	}
	return nil
}

func (a *awsService) AwaitInstanceOK(ctx context.Context, instanceID string) error {
	if strings.TrimSpace(instanceID) == "" {
		return ErrInstanceNonExisting
	}
	input := &ec2.DescribeInstanceStatusInput{
		InstanceIds: aws.StringSlice([]string{instanceID}),
	}
	for {
		out, err := a.client.DescribeInstanceStatusWithContext(ctx, input)
		if err != nil {
			aerr, ok := err.(awserr.Error)
			if ok && aerr.Code() == accessDeniedErrorCode {
				// No point in retrying if no permission
				return ErrNotAllowed
			}
			logging.Debugf("Got error while waiting for instance:\n%s", err)
			logging.Warning("Error while waiting for instance, trying again...")
			time.Sleep(5 * time.Second)
			continue
		}
		allOK := true
		for _, status := range out.InstanceStatuses {
			var s string
			if status.InstanceStatus != nil && status.InstanceStatus.Status != nil {
				s = *status.InstanceStatus.Status
			}
			if s == "impaired" {
				logging.Error("Instance is in an impaired state")
				return ErrInstanceImpaired
			}
			allOK = allOK && (s == "ok")
		}
		if allOK {
			break
		} else {
			logging.Debug("Still waiting for instance to be OK...")
			time.Sleep(20 * time.Second)
		}
	}
	return nil
}

func (a *awsService) AwaitInstanceRunning(ctx context.Context, instanceID string) error {
	if strings.TrimSpace(instanceID) == "" {
		return ErrInvalidName
	}
	input := &ec2.DescribeInstancesInput{
		InstanceIds: aws.StringSlice([]string{instanceID}),
	}
	err := a.client.WaitUntilInstanceRunningWithContext(ctx, input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return ErrNotAllowed
		}
		return err
	}
	return nil
}

func (a *awsService) AwaitInstanceStopped(ctx context.Context, instanceID string) error {
	if strings.TrimSpace(instanceID) == "" {
		return ErrInvalidName
	}
	input := &ec2.DescribeInstancesInput{
		InstanceIds: aws.StringSlice([]string{instanceID}),
	}
	err := a.client.WaitUntilInstanceStoppedWithContext(ctx, input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return ErrNotAllowed
		}
		return err
	}
	return nil
}

func blockToMap(blockMapping []*ec2.InstanceBlockDeviceMapping) map[string]string {
	res := make(map[string]string)
	for _, m := range blockMapping {
		if m.Ebs != nil {
			res[*m.DeviceName] = *m.Ebs.VolumeId
		}
	}
	return res
}
