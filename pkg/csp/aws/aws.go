package aws

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/brkt/metavisor-cli/pkg/logging"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

const (
	accessDeniedErrorCode   = "AccessDenied"
	keyNotFoundErrorCode    = "InvalidKeyPair.NotFound"
	instanceIDErrorCode     = "InvalidInstanceID"
	amiIDErrorCode          = "InvalidAMIID"
	volumeNotFound          = "InvalidVolumeID"
	snapshotNotFound        = "InvalidSnapshot.NotFound"
	vpcNotFoundErrorCode    = "VPCResourceNotSpecified"
	subnetNotFoundErrorCode = "InvalidSubnetID.NotFound"

	genericVolumeType = "gp2"

	// SmallInstanceType is a generic smaller instance type in AWS
	SmallInstanceType = "t2.small"
	// LargerInstanceType is a generic larger instance type in AWS
	LargerInstanceType = "m4.large"

	// SriovNetIsSupported is the value of SriovNetSupport when it's actually
	// supported on the instance.
	SriovNetIsSupported = "simple"
)

var (
	// ErrNotAllowed is returned if the current user does not have enough
	// IAM permissions to perform a certain action
	ErrNotAllowed = errors.New("insufficient IAM permissions")
	// ErrNonExistingRegion is returned if a specified region does not exist
	ErrNonExistingRegion = errors.New("the specified region does not exist")
	// ErrInvalidName is returned if trying to specify an invalid name
	ErrInvalidName = errors.New("name is invalid")
	// ErrInstanceNonExisting is returned if trying to specify invalid instance ID
	ErrInstanceNonExisting = errors.New("instance doesn't exist")
	// ErrSnapshotNonExisting is returned if specified snapshot doesn't exist
	ErrSnapshotNonExisting = errors.New("snapshot doesn't exist")
	// ErrImageNonExisting is returned if specified AMI doesn't exist
	ErrImageNonExisting = errors.New("image doesn't exist")
	// ErrKeyNonExisting is returned if a key definetly doesn't exist
	ErrKeyNonExisting = errors.New("key pair doesn't exist")
	// ErrNoAMIInRegion is returned if trying to launch instance in region where
	// there is no Amazon Linux AMI
	ErrNoAMIInRegion = errors.New("no Amazon Linux AMI found in specified region")
	// ErrFailedLaunchingInstance is returned if failing to launch instance
	ErrFailedLaunchingInstance = errors.New("failed to launch instance")
	// ErrInvalidVolumeType is returned if trying to use non-existing volume type
	ErrInvalidVolumeType = errors.New("specified volume type is not valid")
	// ErrInvalidInstanceAttr is returned if trying to modify a non-existing attribute
	ErrInvalidInstanceAttr = errors.New("specified instance attribue doesn't exist")
	// ErrInvalidInstanceAttrValue is returned if a specified attribute value doesn't match the attribute type
	ErrInvalidInstanceAttrValue = errors.New("specified attribute value is of incorrect type")
	// ErrInstanceImpaired is returned if instance status is "impaired"
	ErrInstanceImpaired = errors.New("instance is in impaired state")
	// ErrInvalidARN is returned if a specified ARN can't be assumed
	ErrInvalidARN = errors.New("failed to assume role with given ARN")
	// ErrRequiresSubnet is returned when a subnet must be explicitly specified in order to launch an instance
	ErrRequiresSubnet = errors.New("a subnet ID must be specified to launch instance")
	// ErrInvalidSubnetID is returned if a specified subnet ID does not work
	ErrInvalidSubnetID = errors.New("the specified subnet is not valid")
)

var validVolumeTypes = []string{"gp2", "io1", "st1", "sc1", "standard"}

// Service is a helper for doing common operations in AWS
type Service interface {
	// KeyPairExist will determine if a specified key pair exist in AWS or not
	KeyPairExist(ctx context.Context, key string) (bool, error)
	// CreateKeyPair will create a new key pair with the given name. If no errors
	// occur, the new key material will be returned as a string
	CreateKeyPair(ctx context.Context, name string) (string, error)
	// RemoveKeyPair will remove the key pair with the specified name
	RemoveKeyPair(ctx context.Context, name string) error
	// CreateSnapshot will create a snapshot with the given name, based on the specified
	// volume. It can also wait for the snapshot to be ready before returning
	CreateSnapshot(ctx context.Context, name, sourceVolumeID string) (Snapshot, error)
	// DeleteSnapshot will delete a snapshot with the given ID
	DeleteSnapshot(ctx context.Context, snapshotID string) error
	// TagResources will attach the given tags to the given resources
	TagResources(ctx context.Context, tags map[string]string, resourceID ...string) error
	// GetSnapshot returns the snapshot with the given ID
	GetSnapshot(ctx context.Context, snapshotID string) (Snapshot, error)
	// GetInstance returns an instance representation of the instance with the given ID
	GetInstance(ctx context.Context, instanceID string) (Instance, error)
	// LaunchInstance will use a new instance with the specified attributes
	LaunchInstance(ctx context.Context, image, instanceType, userData, keyName, subnetID string, extraDevices ...NewDevice) (Instance, error)
	// TerminateInstance will terminate the instance with the given ID
	TerminateInstance(ctx context.Context, instanceID string) error
	// StopInstance will stop the instance with the given ID
	StopInstance(ctx context.Context, instanceID string) error
	// StartInstance will start the instance with the given ID
	StartInstance(ctx context.Context, instanceID string) error
	// ModifyInstanceAttribute changes the specified attribute of an instance
	ModifyInstanceAttribute(ctx context.Context, instanceID string, attr instanceAttribute, value interface{}) error
	// AwaitInstanceOK will block until the instance status is OK
	AwaitInstanceOK(ctx context.Context, instanceID string) error
	// AwaitInstanceRunning will block until instance is running
	AwaitInstanceRunning(ctx context.Context, instanceID string) error
	// AwaitInstanceStopped will block until instance is stopped
	AwaitInstanceStopped(ctx context.Context, instanceID string) error
	// CreateImage will create a new AMI based on an instance
	CreateImage(ctx context.Context, instanceID, name, desc string) (string, error)
	// GetImage returns the AMI with the given ID
	GetImage(ctx context.Context, imageID string) (Image, error)
	// AwaitImageAvailable will block until image is available
	AwaitImageAvailable(ctx context.Context, imageID string) error
	// CreateVolume will create a new volume in AWS
	CreateVolume(ctx context.Context, sourceSnapshotID, volumeType, zone string, size int64) (Volume, error)
	// DeleteVolume will delete the specified volume
	DeleteVolume(ctx context.Context, volumeID string) error
	// DetachVolume will detach a specified volume in AWS
	DetachVolume(ctx context.Context, volumeID, instanceID, deviceName string) error
	// AttachVolume will attach a specified volume to a specified instance in AWS
	AttachVolume(ctx context.Context, volumeID, instanceID, deviceName string) error
	// AwaitVolumeAvailable will block until volume is available
	AwaitVolumeAvailable(ctx context.Context, volumeID string) error
	// AwaitVolumeInUse will block until volume is in-use
	AwaitVolumeInUse(ctx context.Context, volumeID string) error
	// DeleteInstanceDevicesOnTermination makes sure devices are deleted when instance is terminated
	DeleteInstanceDevicesOnTermination(ctx context.Context, instanceID string) error
}

// NewDevice is can be passed when launching new instance to add extra
// volumes to instances.
type NewDevice struct {
	DeviceName string
	SnapshotID string
}

// Resource is a generic AWS resource
type Resource interface {
	ID() string
}

// Snapshot is a snapshot in AWS
type Snapshot interface {
	Resource
	// SizeGB is the size of the snapshot in GiB
	SizeGB() int64
}

// Image is an AMI in AWS
type Image interface {
	Resource
	// RootDeviceName is the name of the root device
	RootDeviceName() string
	// DeviceMapping is a mapping from device name to AWS snapshot ID
	DeviceMapping() map[string]string
	// ENASupport is if the image supports ENA or not
	ENASupport() bool
	// State is the current state of the AMI
	State() string
	// Name is the name of the AMI
	Name() string
	// Description is the description of the AMI
	Description() string
}

// Volume is a volume in AWS
type Volume interface {
	Resource
}

// Instance represents an instance in AWS
type Instance interface {
	Resource
	// InstanceType is the type of the instance, e.g. m4.large
	InstanceType() string
	// RootDeviceName is the name of the root device
	RootDeviceName() string
	// DeviceMapping is a mapping from device name to AWS volume ID
	DeviceMapping() map[string]string
	// PublicIP is the public IP, if it exists, otherwise empty
	PublicIP() string
	// PrivateIP is the private IP, if it exists, otherwise empty
	PrivateIP() string
	// AvailabilityZone is the availability zone of the instance
	AvailabilityZone() string
	// SriovNetSupport specifies if enhanced networking is supported, "simple" == supported
	SriovNetSupport() string
	// ENASupport is if the instance supports ENA or not
	ENASupport() bool
}

type instanceAttribute int

const (
	// AttrSriovNetSupport is for changing instances' SriovNetSupport attribute
	AttrSriovNetSupport instanceAttribute = iota
	// AttrENASupport is for changing instances' ENA support attribute
	AttrENASupport
	// AttrUserData is for changing the user data of an instance
	AttrUserData
)

type resource struct {
	id string
}

func (r *resource) ID() string { return r.id }

// IAMConfig can be specified when creating a new AWS Service to assume a
// role before doing any operations.
type IAMConfig struct {
	RoleARN      string
	MFADeviceARN string
	MFACode      string
}

// New will initialize and return a new AWS Service that can be used to perform
// common operations in AWS.
func New(region string, iamConf *IAMConfig) (Service, error) {
	if valid := IsValidRegion(region); !valid {
		return nil, ErrNonExistingRegion
	}
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(region),
	})
	if err != nil {
		return nil, err
	}
	var creds *credentials.Credentials
	if iamConf != nil && strings.TrimSpace(iamConf.RoleARN) != "" {
		creds, err = assumeIAMRole(sess, *iamConf)
		if err != nil {
			logging.Error("Failed to assume IAM role")
			return nil, err
		}
	}
	service := new(awsService)
	service.region = region
	if creds != nil {
		service.client = ec2.New(sess, &aws.Config{
			Credentials: creds,
		})
	} else {
		service.client = ec2.New(sess)
	}
	return service, nil
}

func assumeIAMRole(sess *session.Session, conf IAMConfig) (*credentials.Credentials, error) {
	if conf.MFACode != "" && conf.MFADeviceARN == "" {
		logging.Warning("Specified MFA code without MFA device, skipping MFA")
		conf.MFACode = ""
	}
	promptProvider := func(p *stscreds.AssumeRoleProvider) {
		p.SerialNumber = aws.String(conf.MFADeviceARN)
		p.TokenProvider = func() (string, error) {
			var v string
			fmt.Fprint(os.Stderr, "Assume Role MFA token code: ")
			_, err := fmt.Scanln(&v)
			logging.Debugf("Got MFA code from user")
			return v, err
		}
	}

	codeProvider := func(p *stscreds.AssumeRoleProvider) {
		p.SerialNumber = aws.String(conf.MFADeviceARN)
		p.TokenCode = aws.String(conf.MFACode)
	}
	var creds *credentials.Credentials

	if conf.MFADeviceARN != "" && conf.MFACode != "" {
		logging.Debug("Assuming IAM role with specified code")
		creds = stscreds.NewCredentials(sess, conf.RoleARN, codeProvider)
	} else if conf.MFADeviceARN != "" {
		logging.Debug("Assuming IAM role with prompting for code")
		creds = stscreds.NewCredentials(sess, conf.RoleARN, promptProvider)
	} else {
		logging.Debug("Assuming IAM role without MFA device")
		creds = stscreds.NewCredentials(sess, conf.RoleARN)
	}
	if _, err := creds.Get(); err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			logging.Error(aerr.Message())
			return nil, ErrNotAllowed
		} else if ok {
			logging.Error(aerr.Message())
			return nil, ErrInvalidARN
		}
		logging.Debugf("Could not assume role: %s", err)
		return nil, ErrInvalidARN
	}
	return creds, nil
}

type awsService struct {
	region string
	client *ec2.EC2
}

func (a *awsService) TagResources(ctx context.Context, tags map[string]string, resourceID ...string) error {
	if len(tags) == 0 {
		return nil
	}
	input := &ec2.CreateTagsInput{
		Resources: aws.StringSlice(resourceID),
		Tags:      mapToEC2Tags(tags),
	}
	_, err := a.client.CreateTagsWithContext(ctx, input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return ErrNotAllowed
		}
		return err
	}
	return nil
}

func (a *awsService) DeleteInstanceDevicesOnTermination(ctx context.Context, instanceID string) error {
	if strings.TrimSpace(instanceID) == "" {
		return ErrInvalidID
	}
	inst, err := a.GetInstance(ctx, instanceID)
	if err != nil {
		return err
	}
	input := &ec2.ModifyInstanceAttributeInput{
		InstanceId: aws.String(instanceID),
	}
	blockSpec := []*ec2.InstanceBlockDeviceMappingSpecification{}
	for dev, volID := range inst.DeviceMapping() {
		spec := &ec2.InstanceBlockDeviceMappingSpecification{
			DeviceName: aws.String(dev),
		}
		ebs := &ec2.EbsInstanceBlockDeviceSpecification{
			DeleteOnTermination: aws.Bool(true),
			VolumeId:            aws.String(volID),
		}
		spec.Ebs = ebs
		blockSpec = append(blockSpec, spec)
	}
	input.BlockDeviceMappings = blockSpec
	_, err = a.client.ModifyInstanceAttributeWithContext(ctx, input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return ErrNotAllowed
		}
		return err
	}
	return nil
}

func mapToEC2Tags(tags map[string]string) []*ec2.Tag {
	res := []*ec2.Tag{}
	for key, value := range tags {
		tag := &ec2.Tag{
			Key:   aws.String(key),
			Value: aws.String(value),
		}
		res = append(res, tag)
	}
	return res
}
