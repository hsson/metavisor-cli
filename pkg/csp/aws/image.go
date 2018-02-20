package aws

import (
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/brkt/metavisor-cli/pkg/logging"
)

type image struct {
	resource
	rootDeviceName string
	deviceMapping  map[string]string
	enaSupport     bool
	state          string
}

func (i *image) RootDeviceName() string           { return i.rootDeviceName }
func (i *image) DeviceMapping() map[string]string { return i.deviceMapping }
func (i *image) ENASupport() bool                 { return i.enaSupport }
func (i *image) State() string                    { return i.state }

func (a *awsService) CreateImage(instanceID, name string) (string, error) {
	if strings.TrimSpace(instanceID) == "" {
		return "", ErrInstanceNonExisting
	}
	input := &ec2.CreateImageInput{
		InstanceId: aws.String(instanceID),
		Name:       aws.String(name),
	}

	out, err := a.client.CreateImage(input)
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
	logging.Info("Waiting for image to become available...")
	err = a.client.WaitUntilImageAvailable(&ec2.DescribeImagesInput{
		ImageIds: aws.StringSlice([]string{*out.ImageId}),
	})
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return "", ErrNotAllowed
		}
		logging.Error("AMI never became available")
		return "", err
	}
	return *out.ImageId, nil
}

func (a *awsService) GetImage(imageID string) (Image, error) {
	if strings.TrimSpace(imageID) == "" {
		return nil, ErrImageNonExisting
	}
	input := &ec2.DescribeImagesInput{
		ImageIds: aws.StringSlice([]string{imageID}),
	}
	out, err := a.client.DescribeImages(input)
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
		res := &image{
			resource: resource{
				id: *img.ImageId,
			},
			rootDeviceName: *img.RootDeviceName,
			deviceMapping:  imageBlockToMap(img.BlockDeviceMappings),
			enaSupport:     enaSupport,
			state:          state,
		}
		return res, nil
	}
	// If we got this far, the AMI doesn't exist
	return nil, ErrImageNonExisting
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
