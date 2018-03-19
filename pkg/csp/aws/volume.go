package aws

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/brkt/metavisor-cli/pkg/logging"
)

type volume struct {
	resource
}

func (a *awsService) CreateVolume(ctx context.Context, sourceSnapshotID, volumeType, zone string, size int64) (Volume, error) {
	if strings.TrimSpace(sourceSnapshotID) == "" {
		return nil, ErrInvalidSnapshotID
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
	vol, err := a.client.CreateVolumeWithContext(ctx, input)
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
	return res, nil
}

func (a *awsService) DeleteVolume(ctx context.Context, volumeID string) error {
	if strings.TrimSpace(volumeID) == "" {
		return ErrInvalidVolumeID
	}
	input := &ec2.DeleteVolumeInput{
		VolumeId: aws.String(volumeID),
	}
	_, err := a.client.DeleteVolumeWithContext(ctx, input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return ErrNotAllowed
		} else if ok && strings.Contains(aerr.Code(), volumeNotFound) {
			logging.Debug("Tried to delete non-existing volume")
			return nil
		}
		return err
	}
	return nil
}

func (a *awsService) DetachVolume(ctx context.Context, volumeID, instanceID, deviceName string) error {
	if strings.TrimSpace(volumeID) == "" {
		return ErrInvalidVolumeID
	}

	if strings.TrimSpace(instanceID) == "" {
		return ErrInvalidInstanceID
	}

	input := &ec2.DetachVolumeInput{
		Device:     aws.String(deviceName),
		InstanceId: aws.String(instanceID),
		VolumeId:   aws.String(volumeID),
	}
	_, err := a.client.DetachVolumeWithContext(ctx, input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return ErrNotAllowed
		}
		return err
	}
	logging.Debug("Waiting for volume to be available after detach")
	return a.AwaitVolumeAvailable(ctx, volumeID)
}

func (a *awsService) AttachVolume(ctx context.Context, volumeID, instanceID, deviceName string) error {
	if strings.TrimSpace(volumeID) == "" {
		return ErrInvalidVolumeID
	}

	if strings.TrimSpace(instanceID) == "" {
		return ErrInvalidInstanceID
	}

	input := &ec2.AttachVolumeInput{
		Device:     aws.String(deviceName),
		InstanceId: aws.String(instanceID),
		VolumeId:   aws.String(volumeID),
	}
	_, err := a.client.AttachVolumeWithContext(ctx, input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return ErrNotAllowed
		}
		return err
	}
	logging.Debug("Waiting for volume to be in-use after attach")
	return a.AwaitVolumeInUse(ctx, volumeID)
}

func (a *awsService) AwaitVolumeAvailable(ctx context.Context, volumeID string) error {
	if strings.TrimSpace(volumeID) == "" {
		return ErrInvalidVolumeID
	}
	input := &ec2.DescribeVolumesInput{
		VolumeIds: aws.StringSlice([]string{volumeID}),
	}
	err := a.client.WaitUntilVolumeAvailableWithContext(ctx, input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return ErrNotAllowed
		}
		return err
	}
	return nil
}

func (a *awsService) AwaitVolumeInUse(ctx context.Context, volumeID string) error {
	if strings.TrimSpace(volumeID) == "" {
		return ErrInvalidVolumeID
	}
	input := &ec2.DescribeVolumesInput{
		VolumeIds: aws.StringSlice([]string{volumeID}),
	}
	err := a.client.WaitUntilVolumeInUseWithContext(ctx, input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return ErrNotAllowed
		}
		return err
	}
	return nil
}
