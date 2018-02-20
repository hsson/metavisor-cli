package aws

import (
	"encoding/base64"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/brkt/metavisor-cli/pkg/logging"
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

func (a *awsService) GetInstance(instanceID string) (Instance, error) {
	if strings.TrimSpace(instanceID) == "" {
		return nil, ErrInstanceNonExisting
	}
	input := &ec2.DescribeInstancesInput{
		InstanceIds: aws.StringSlice([]string{instanceID}),
	}
	out, err := a.client.DescribeInstances(input)
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

func (a *awsService) LaunchInstance(imageID, instanceType, userData, keyName, instanceName string, extraDevices ...NewDevice) (Instance, error) {
	if !IsAMIID(imageID) {
		return nil, ErrInvalidID
	}
	if strings.TrimSpace(keyName) != "" {
		if keyExist, err := a.KeyPairExist(keyName); (err != nil && err != ErrNotAllowed) || (!keyExist && err == nil) {
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

	out, err := a.client.RunInstances(input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return nil, ErrNotAllowed
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
		logging.Info("Waiting for instance to become ready...")
		err := a.client.WaitUntilInstanceRunning(&ec2.DescribeInstancesInput{
			InstanceIds: aws.StringSlice([]string{res.ID()}),
		})
		if err != nil {
			aerr, ok := err.(awserr.Error)
			if ok && aerr.Code() == accessDeniedErrorCode {
				return nil, ErrNotAllowed
			}
			logging.Info("Instance never became ready...")
			return nil, err
		}
		if strings.TrimSpace(instanceName) != "" {
			err = a.TagResources(map[string]string{"Name": instanceName}, res.ID())
			if err == ErrNotAllowed {
				logging.Warning("Insufficient IAM permissions to tag resource, skipping Name")
				return res, nil
			}
		}
		return res, nil
	}
	// If this is reached, we never launched any instance
	return nil, ErrFailedLaunchingInstance
}

func (a *awsService) StopInstance(instanceID string) error {
	if strings.TrimSpace(instanceID) == "" {
		return ErrInvalidName
	}
	input := &ec2.StopInstancesInput{
		InstanceIds: aws.StringSlice([]string{instanceID}),
	}
	_, err := a.client.StopInstances(input)
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
	// Wait for the instance to stop
	logging.Info("Waiting for instance to stop...")
	err = a.client.WaitUntilInstanceStopped(&ec2.DescribeInstancesInput{
		InstanceIds: aws.StringSlice([]string{instanceID}),
	})
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return ErrNotAllowed
		}
		logging.Error("Instance never stopped")
		return err
	}
	return nil
}

func (a *awsService) StartInstance(instanceID string) error {
	if strings.TrimSpace(instanceID) == "" {
		return ErrInvalidName
	}
	input := &ec2.StartInstancesInput{
		InstanceIds: aws.StringSlice([]string{instanceID}),
	}
	_, err := a.client.StartInstances(input)
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
	// Wait for the instance to start
	logging.Info("Waiting for instance to start...")
	err = a.client.WaitUntilInstanceRunning(&ec2.DescribeInstancesInput{
		InstanceIds: aws.StringSlice([]string{instanceID}),
	})
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return ErrNotAllowed
		}
		logging.Error("Instance never got ready")
		return err
	}
	return nil
}

func (a *awsService) TerminateInstance(instanceID string) error {
	if strings.TrimSpace(instanceID) == "" {
		return ErrInvalidName
	}
	input := &ec2.TerminateInstancesInput{
		InstanceIds: aws.StringSlice([]string{instanceID}),
	}
	_, err := a.client.TerminateInstances(input)
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

func (a *awsService) ModifyInstanceAttribute(instanceID string, attr instanceAttribute, value interface{}) error {
	if strings.TrimSpace(instanceID) == "" {
		return ErrInvalidID
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
	_, err := a.client.ModifyInstanceAttribute(input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return ErrNotAllowed
		}
		return err
	}
	return nil
}

func (a *awsService) AwaitInstanceOK(instanceID string) error {
	if strings.TrimSpace(instanceID) == "" {
		return ErrInstanceNonExisting
	}
	input := &ec2.DescribeInstanceStatusInput{
		InstanceIds: aws.StringSlice([]string{instanceID}),
	}
	for {
		out, err := a.client.DescribeInstanceStatus(input)
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

func blockToMap(blockMapping []*ec2.InstanceBlockDeviceMapping) map[string]string {
	res := make(map[string]string)
	for _, m := range blockMapping {
		if m.Ebs != nil {
			res[*m.DeviceName] = *m.Ebs.VolumeId
		}
	}
	return res
}
