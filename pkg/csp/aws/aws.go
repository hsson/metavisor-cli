package aws

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/brkt/metavisor-cli/pkg/logging"
)

const (
	accessDeniedErrorCode = "AccessDenied"
	keyNotFoundErrorCode  = "InvalidKeyPair.NotFound"
	instanceIDErrorCode   = "InvalidInstanceID.NotFound"
	snapshotNotFound      = "InvalidSnapshot.NotFound"

	genericVolumeType   = "gp2"
	genericInstanceType = "t2.small"
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
	// ErrKeyNonExisting is returned if a key definetly doesn't exist
	ErrKeyNonExisting = errors.New("key pair doesn't exist")
	// ErrNoAMIInRegion is returned if trying to launch instance in region where
	// there is no Amazon Linux AMI
	ErrNoAMIInRegion = errors.New("no Amazon Linux AMI found in specified region")
	// ErrFailedLaunchingInstance is returned if failing to launch instance
	ErrFailedLaunchingInstance = errors.New("failed to launch instance")
)

// Service is a helper for doing common operations in AWS
type Service interface {
	// EC2Client will return a reference to the raw EC2 client used by the service
	EC2Client() (*ec2.EC2, error)
	// KeyPairExist will determine if a specified key pair exist in AWS or not
	KeyPairExist(key string) (bool, error)
	// CreateKeyPair will create a new key pair with the given name. If no errors
	// occur, the new key material will be returned as a string
	CreateKeyPair(name string) (string, error)
	// RemoveKeyPair will remove the key pair with the specified name
	RemoveKeyPair(name string) error
	// CreateSnapshot will create a snapshot with the given name, based on the specified
	// volume. It can also wait for the snapshot to be ready before returning
	CreateSnapshot(name, sourceVolumeID string) (Snapshot, error)
	// DeleteSnapshot will delete a snapshot with the given ID
	DeleteSnapshot(snapshotID string) error
	// TagResources will attach the given tags to the given resources
	TagResources(tags map[string]string, resourceID ...string) error
	// GetInstance returns an instance representation of the instance with the given ID
	GetInstance(instanceID string) (Instance, error)
	// GetSnapshot returns the snapshot with the given ID
	GetSnapshot(snapshotID string) (Snapshot, error)
	// LaunchGenericInstance will launch a new instance with some sensible defaults.
	// Useful for performing temporary actions.
	LaunchGenericInstance(userdata, keyName string, extraDevices ...NewDevice) (Instance, error)
	// TerminateInstance will terminate the instance with the given ID
	TerminateInstance(instanceID string) error
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

// Instance represents an instance in AWS
type Instance interface {
	Resource
	// RootDeviceName is the name of the root device
	RootDeviceName() string
	// DeviceMapping is a mapping from device name to AWS volume ID
	DeviceMapping() map[string]string
	// PublicIP is the public IP, if it exists, otherwise empty
	PublicIP() string
	// PrivateIP is the private IP, if it exists, otherwise empty
	PrivateIP() string
}

type resource struct {
	id string
}

func (r *resource) ID() string { return r.id }

type snapshot struct {
	resource
	sizeGB int64
}

func (s *snapshot) SizeGB() int64 { return s.sizeGB }

type instance struct {
	resource
	rootDeviceName string
	deviceMapping  map[string]string
	pubIP, privIP  string
}

func (i *instance) RootDeviceName() string           { return i.rootDeviceName }
func (i *instance) DeviceMapping() map[string]string { return i.deviceMapping }
func (i *instance) PublicIP() string                 { return i.pubIP }
func (i *instance) PrivateIP() string                { return i.privIP }

// New will initialize and return a new AWS Service that can be used to perfomr
// common operations in AWS.
func New(region string) (Service, error) {
	if valid := IsValidRegion(region); !valid {
		return nil, ErrNonExistingRegion
	}
	return &awsService{
		region,
		nil,
	}, nil
}

type awsService struct {
	region string
	client *ec2.EC2
}

func (a *awsService) EC2Client() (*ec2.EC2, error) {
	if a.client == nil {
		sess, err := session.NewSession(&aws.Config{
			Region: aws.String(a.region),
		})
		if err != nil {
			return nil, err
		}
		a.client = ec2.New(sess)
	}
	return a.client, nil
}

func (a *awsService) KeyPairExist(keyName string) (bool, error) {
	if strings.TrimSpace(keyName) == "" {
		return false, nil
	}
	client, err := a.EC2Client()
	if err != nil {
		return false, err
	}

	inputFilter := &ec2.DescribeKeyPairsInput{
		KeyNames: aws.StringSlice([]string{keyName}),
	}

	result, err := client.DescribeKeyPairs(inputFilter)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return false, ErrNotAllowed
		} else if ok && aerr.Code() == keyNotFoundErrorCode {
			return false, nil
		}
		return false, err
	}
	if result != nil && result.KeyPairs != nil {
		return len(result.KeyPairs) > 0, nil
	}
	return false, nil
}

func (a *awsService) CreateKeyPair(name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", ErrInvalidName
	}
	if exist, _ := a.KeyPairExist(name); exist {
		return "", ErrInvalidName
	}
	client, err := a.EC2Client()
	if err != nil {
		return "", err
	}
	input := &ec2.CreateKeyPairInput{
		KeyName: aws.String(name),
	}
	result, err := client.CreateKeyPair(input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return "", ErrNotAllowed
		}
		return "", err
	}
	return *result.KeyMaterial, nil
}

func (a *awsService) RemoveKeyPair(name string) error {
	if strings.TrimSpace(name) == "" {
		return ErrInvalidName
	}
	client, err := a.EC2Client()
	if err != nil {
		return err
	}
	input := &ec2.DeleteKeyPairInput{
		KeyName: aws.String(name),
	}
	_, err = client.DeleteKeyPair(input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return ErrNotAllowed
		}
		return err
	}
	return nil
}

func (a *awsService) CreateSnapshot(name, sourceVolumeID string) (Snapshot, error) {
	if strings.TrimSpace(name) == "" {
		return nil, ErrInvalidName
	}
	// TODO: Maybe validate volume ID?
	client, err := a.EC2Client()
	if err != nil {
		return nil, err
	}
	desc := fmt.Sprintf("Created by metavisor-cli, based on volume %s", sourceVolumeID)
	input := &ec2.CreateSnapshotInput{
		Description: aws.String(desc),
		VolumeId:    aws.String(sourceVolumeID),
	}
	snap, err := client.CreateSnapshot(input)
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
	err = waitForSnapshot(client, res.ID())
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
	client, err := a.EC2Client()
	if err != nil {
		return err
	}
	input := &ec2.DeleteSnapshotInput{
		SnapshotId: aws.String(snapshotID),
	}
	_, err = client.DeleteSnapshot(input)
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

func (a *awsService) TagResources(tags map[string]string, resourceID ...string) error {
	if len(tags) == 0 {
		return nil
	}
	client, err := a.EC2Client()
	if err != nil {
		return err
	}
	input := &ec2.CreateTagsInput{
		Resources: aws.StringSlice(resourceID),
		Tags:      mapToEC2Tags(tags),
	}
	_, err = client.CreateTags(input)
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

func (a *awsService) GetInstance(instanceID string) (Instance, error) {
	if strings.TrimSpace(instanceID) == "" {
		return nil, ErrInstanceNonExisting
	}
	client, err := a.EC2Client()
	if err != nil {
		return nil, err
	}
	input := &ec2.DescribeInstancesInput{
		InstanceIds: aws.StringSlice([]string{instanceID}),
	}
	out, err := client.DescribeInstances(input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return nil, ErrNotAllowed
		}
		if ok && aerr.Code() == instanceIDErrorCode {
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
			res := &instance{
				resource: resource{
					id: *inst.InstanceId,
				},
				rootDeviceName: *inst.RootDeviceName,
				deviceMapping:  blockToMap(inst.BlockDeviceMappings),
				pubIP:          puIP,
				privIP:         prIP,
			}
			return res, nil
		}
	}
	// If we got this far, the instance doesn't exist
	return nil, ErrInstanceNonExisting
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

func (a *awsService) GetSnapshot(snapshotID string) (Snapshot, error) {
	if strings.TrimSpace(snapshotID) == "" {
		return nil, ErrInstanceNonExisting
	}
	client, err := a.EC2Client()
	if err != nil {
		return nil, err
	}
	input := &ec2.DescribeSnapshotsInput{
		SnapshotIds: aws.StringSlice([]string{snapshotID}),
	}
	out, err := client.DescribeSnapshots(input)
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

func (a *awsService) LaunchGenericInstance(userdata, keyName string, extraDevices ...NewDevice) (Instance, error) {
	client, err := a.EC2Client()
	if err != nil {
		return nil, err
	}
	if keyExist, err := a.KeyPairExist(keyName); (err != nil && err != ErrNotAllowed) || (!keyExist && err == nil) {
		// If we don't have permission to list keys, we assume it exist and continue
		if err == nil {
			err = ErrKeyNonExisting
		}
		return nil, err
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

	ami := GenericAMI(a.region)
	if ami == "" {
		return nil, ErrNoAMIInRegion
	}
	base64Userdata := base64.StdEncoding.EncodeToString([]byte(userdata))
	input := &ec2.RunInstancesInput{
		EbsOptimized: aws.Bool(false),
		ImageId:      aws.String(ami),
		InstanceType: aws.String(genericInstanceType),
		MinCount:     aws.Int64(1),
		MaxCount:     aws.Int64(1),
		KeyName:      aws.String(keyName),
		UserData:     aws.String(base64Userdata),
	}
	if len(blockDeviceMapping) > 0 {
		input.BlockDeviceMappings = blockDeviceMapping
	}

	out, err := client.RunInstances(input)
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
		res := &instance{
			resource: resource{
				id: *inst.InstanceId,
			},
			rootDeviceName: *inst.RootDeviceName,
			deviceMapping:  blockToMap(inst.BlockDeviceMappings),
			pubIP:          puIP,
			privIP:         prIP,
		}
		logging.Info("Waiting for instance to become ready...")
		err := client.WaitUntilInstanceRunning(&ec2.DescribeInstancesInput{
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
		err = a.TagResources(map[string]string{"Name": "Temporary share-logs instance"}, res.ID())
		if err == ErrNotAllowed {
			logging.Warning("Insufficient IAM permissions to tag resource, skipping Name")
			return res, nil
		}
		return res, nil
	}
	// If this is reached, we never launched any instance
	return nil, ErrFailedLaunchingInstance
}

func (a *awsService) TerminateInstance(instanceID string) error {
	if strings.TrimSpace(instanceID) == "" {
		return ErrInvalidName
	}
	client, err := a.EC2Client()
	if err != nil {
		return err
	}
	input := &ec2.TerminateInstancesInput{
		InstanceIds: aws.StringSlice([]string{instanceID}),
	}
	_, err = client.TerminateInstances(input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return ErrNotAllowed
		} else if ok && aerr.Code() == instanceIDErrorCode {
			logging.Debug("Attempted to terminate non-existing instance")
			return nil
		}
		return err
	}
	return nil
}
