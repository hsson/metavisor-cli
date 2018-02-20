package aws

import (
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/brkt/metavisor-cli/pkg/logging"
)

type volume struct {
	resource
}

func (a *awsService) CreateVolume(sourceSnapshotID, volumeType, zone string, size int64) (Volume, error) {
	if strings.TrimSpace(sourceSnapshotID) == "" {
		return nil, ErrInvalidID
	}
	var validVolume bool
	for i := range validVolumeTypes {
		if validVolumeTypes[i] == volumeType {
			validVolume = true
			break
		}
	}
	if !validVolume {
		// The specified volume type is not valid
		logging.Errorf("Bad volume type, use of of %s", validVolumeTypes)
		return nil, ErrInvalidVolumeType
	}
	input := &ec2.CreateVolumeInput{
		SnapshotId:       aws.String(sourceSnapshotID),
		VolumeType:       aws.String(volumeType),
		Size:             aws.Int64(size),
		AvailabilityZone: aws.String(zone),
	}
	vol, err := a.client.CreateVolume(input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return nil, ErrNotAllowed
		}
		return nil, err
	}
	res := &volume{
		resource: resource{
			id: *vol.VolumeId,
		},
	}
	err = a.client.WaitUntilVolumeAvailable(&ec2.DescribeVolumesInput{
		VolumeIds: aws.StringSlice([]string{res.ID()}),
	})
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return nil, ErrNotAllowed
		}
		return nil, err
	}
	return res, nil
}

func (a *awsService) DetachVolume(volumeID, instanceID, deviceName string) error {
	if strings.TrimSpace(volumeID) == "" || strings.TrimSpace(instanceID) == "" {
		return ErrInvalidID
	}
	input := &ec2.DetachVolumeInput{
		Device:     aws.String(deviceName),
		InstanceId: aws.String(instanceID),
		VolumeId:   aws.String(volumeID),
	}
	_, err := a.client.DetachVolume(input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return ErrNotAllowed
		}
		return err
	}
	logging.Debug("Waiting for volume to be available after detach")
	err = a.client.WaitUntilVolumeAvailable(&ec2.DescribeVolumesInput{
		VolumeIds: aws.StringSlice([]string{volumeID}),
	})
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return ErrNotAllowed
		}
		return err
	}
	return nil
}

func (a *awsService) AttachVolume(volumeID, instanceID, deviceName string) error {
	if strings.TrimSpace(volumeID) == "" || strings.TrimSpace(instanceID) == "" {
		return ErrInvalidID
	}
	input := &ec2.AttachVolumeInput{
		Device:     aws.String(deviceName),
		InstanceId: aws.String(instanceID),
		VolumeId:   aws.String(volumeID),
	}
	_, err := a.client.AttachVolume(input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return ErrNotAllowed
		}
		return err
	}
	logging.Debug("Waiting for volume to be in-use after attach")
	err = a.client.WaitUntilVolumeInUse(&ec2.DescribeVolumesInput{
		VolumeIds: aws.StringSlice([]string{volumeID}),
	})
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return ErrNotAllowed
		}
		return err
	}
	return nil
}
