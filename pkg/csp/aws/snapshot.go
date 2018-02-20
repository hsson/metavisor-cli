package aws

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/brkt/metavisor-cli/pkg/logging"
)

type snapshot struct {
	resource
	sizeGB int64
}

func (s *snapshot) SizeGB() int64 { return s.sizeGB }

func (a *awsService) GetSnapshot(snapshotID string) (Snapshot, error) {
	if strings.TrimSpace(snapshotID) == "" {
		return nil, ErrSnapshotNonExisting
	}
	input := &ec2.DescribeSnapshotsInput{
		SnapshotIds: aws.StringSlice([]string{snapshotID}),
	}
	out, err := a.client.DescribeSnapshots(input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return nil, ErrNotAllowed
		}
		if ok && aerr.Code() == snapshotNotFound {
			return nil, ErrSnapshotNonExisting
		}
		return nil, err
	}
	for _, snap := range out.Snapshots {
		res := &snapshot{
			resource: resource{
				id: *snap.SnapshotId,
			},
			sizeGB: *snap.VolumeSize,
		}
		return res, nil
	}
	// If we got this far the snapshot doesn't exist
	return nil, ErrSnapshotNonExisting
}

func (a *awsService) CreateSnapshot(name, sourceVolumeID string) (Snapshot, error) {
	if strings.TrimSpace(name) == "" {
		return nil, ErrInvalidName
	}
	desc := fmt.Sprintf("Created by metavisor-cli, based on volume %s", sourceVolumeID)
	input := &ec2.CreateSnapshotInput{
		Description: aws.String(desc),
		VolumeId:    aws.String(sourceVolumeID),
	}
	snap, err := a.client.CreateSnapshot(input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return nil, ErrNotAllowed
		}
		return nil, err
	}

	res := &snapshot{
		resource{
			*snap.SnapshotId,
		},
		*snap.VolumeSize,
	}
	logging.Info("Waiting for snapshot to become ready...")
	err = waitForSnapshot(a.client, res.ID())
	if err != nil {
		logging.Error("Snapshot never became ready")
		return nil, err
	}
	nameTags := map[string]string{"Name": name}
	err = a.TagResources(nameTags, res.ID())
	if err == ErrNotAllowed {
		logging.Warning("Insufficient IAM permissions to tag resource, skipping Name")
		return res, nil
	}
	return res, err
}

func (a *awsService) DeleteSnapshot(snapshotID string) error {
	if strings.TrimSpace(snapshotID) == "" {
		return ErrInvalidName
	}
	input := &ec2.DeleteSnapshotInput{
		SnapshotId: aws.String(snapshotID),
	}
	_, err := a.client.DeleteSnapshot(input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return ErrNotAllowed
		} else if ok && aerr.Code() == snapshotNotFound {
			logging.Debug("Tried to delete non-existing snapshot")
			return nil
		}
		return err
	}
	return nil
}

func waitForSnapshot(client *ec2.EC2, snapshotID string) error {
	// Wait for the snapshot to be compelted
	input := &ec2.DescribeSnapshotsInput{
		SnapshotIds: aws.StringSlice([]string{snapshotID}),
		Filters: []*ec2.Filter{&ec2.Filter{
			Name:   aws.String("status"),
			Values: aws.StringSlice([]string{"completed"}),
		}, &ec2.Filter{
			Name:   aws.String("progress"),
			Values: aws.StringSlice([]string{"100%"}),
		}},
	}
	err := client.WaitUntilSnapshotCompleted(input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return ErrNotAllowed
		}
		return err
	}
	return nil
}
