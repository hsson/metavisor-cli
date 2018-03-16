package aws

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
)

type image struct {
	resource
	rootDeviceName string
	deviceMapping  map[string]string
	enaSupport     bool
	state          string
	name           string
	description    string
}

func (i *image) RootDeviceName() string           { return i.rootDeviceName }
func (i *image) DeviceMapping() map[string]string { return i.deviceMapping }
func (i *image) ENASupport() bool                 { return i.enaSupport }
func (i *image) State() string                    { return i.state }
func (i *image) Name() string                     { return i.name }
func (i *image) Description() string              { return i.description }

func (a *awsService) CreateImage(ctx context.Context, instanceID, name, desc string) (string, error) {
	if strings.TrimSpace(instanceID) == "" {
		return "", ErrInstanceNonExisting
	}
	input := &ec2.CreateImageInput{
		InstanceId:  aws.String(instanceID),
		Name:        aws.String(name),
		Description: aws.String(desc),
	}
	out, err := a.client.CreateImageWithContext(ctx, input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return "", ErrNotAllowed
		}
		if ok && strings.Contains(aerr.Code(), instanceIDErrorCode) {
			return "", ErrInstanceNonExisting
		}
		return "", err
	}
	return *out.ImageId, nil
}

func (a *awsService) GetImage(ctx context.Context, imageID string) (Image, error) {
	if strings.TrimSpace(imageID) == "" {
		return nil, ErrImageNonExisting
	}
	input := &ec2.DescribeImagesInput{
		ImageIds: aws.StringSlice([]string{imageID}),
	}
	out, err := a.client.DescribeImagesWithContext(ctx, input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return nil, ErrNotAllowed
		}
		if ok && strings.Contains(aerr.Code(), amiIDErrorCode) {
			return nil, ErrImageNonExisting
		}
		return nil, err
	}
	for _, img := range out.Images {
		var enaSupport bool
		if img.EnaSupport != nil {
			enaSupport = *img.EnaSupport
		}
		var state string
		if img.State != nil {
			state = *img.State
		}
		var name string
		if img.Name != nil {
			name = *img.Name
		}
		var desc string
		if img.Description != nil {
			desc = *img.Description
		}
		res := &image{
			resource: resource{
				id: *img.ImageId,
			},
			rootDeviceName: *img.RootDeviceName,
			deviceMapping:  imageBlockToMap(img.BlockDeviceMappings),
			enaSupport:     enaSupport,
			state:          state,
			name:           name,
			description:    desc,
		}
		return res, nil
	}
	// If we got this far, the AMI doesn't exist
	return nil, ErrImageNonExisting
}

func (a *awsService) AwaitImageAvailable(ctx context.Context, imageID string) error {
	if strings.TrimSpace(imageID) == "" {
		return ErrImageNonExisting
	}
	input := &ec2.DescribeImagesInput{
		ImageIds: aws.StringSlice([]string{imageID}),
	}
	err := a.client.WaitUntilImageAvailableWithContext(ctx, input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return ErrNotAllowed
		}
		return err
	}
	return nil
}

func imageBlockToMap(blockMapping []*ec2.BlockDeviceMapping) map[string]string {
	res := make(map[string]string)
	for _, m := range blockMapping {
		if m.Ebs != nil {
			res[*m.DeviceName] = *m.Ebs.SnapshotId
		}
	}
	return res
}
