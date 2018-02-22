package aws

import (
	"errors"
	"strings"

	"github.com/brkt/metavisor-cli/pkg/logging"

	"github.com/aws/aws-sdk-go/aws/endpoints"
)

const (
	idPrefix   = "i-"
	snapPrefix = "snap-"
	amiPrefix  = "ami-"
)

// Amazon Linux AMIs (HVM EBS) collected on Feb 14 2018, from:
// https://aws.amazon.com/amazon-linux-ami/
// Regions can be found translated to IDs here:
// https://docs.aws.amazon.com/general/latest/gr/rande.html#ec2_region
var genericAMIMap = map[string]string{
	"us-east-1":      "ami-97785bed",
	"us-east-2":      "ami-f63b1193",
	"us-west-1":      "ami-824c4ee2",
	"us-west-2":      "ami-f2d3638a",
	"ap-northeast-1": "ami-ceafcba8",
	"ap-northeast-2": "ami-863090e8",
	"ap-south-1":     "ami-531a4c3c",
	"ap-southeast-1": "ami-68097514",
	"ap-southeast-2": "ami-942dd1f6",
	"ca-central-1":   "ami-a954d1cd",
	"cn-north-1":     "ami-cb19c4a6",
	"eu-central-1":   "ami-5652ce39",
	"eu-west-1":      "ami-d834aba1",
	"eu-west-2":      "ami-403e2524",
	"eu-west-3":      "ami-8ee056f3",
	"sa-east-1":      "ami-84175ae8",
	"us-gov-west-1":  "ami-56f87137",
}

var (
	// ErrInvalidID is returned if a specified ID is not of correct format
	ErrInvalidID = errors.New("the specified ID is not of valid format")
)

// IsInstanceID determines if the specified ID belong to an instance or not
func IsInstanceID(id string) bool {
	// TODO: Maybe use regex?
	return strings.HasPrefix(id, idPrefix)
}

// IsSnapshotID determines if the specified ID belong to a snapshot or not
func IsSnapshotID(id string) bool {
	// TODO: Maybe use regex?
	return strings.HasPrefix(id, snapPrefix)
}

// IsAMIID determines if the specified ID belong to an AMI or not
func IsAMIID(id string) bool {
	// TODO: Maybe use regex?
	return strings.HasPrefix(id, amiPrefix)
}

// IsValidRegion will validate a specified region to make sure it exist in AWS
func IsValidRegion(region string) bool {
	if strings.TrimSpace(region) == "" {
		return false
	}
	regions, exists := endpoints.RegionsForService(endpoints.DefaultPartitions(), endpoints.AwsPartitionID, endpoints.Ec2ServiceID)
	if !exists {
		// This should actually never happen, as region always exist in the standard partition
		// for EC2, but return this error anyway rather than panicing.
		logging.Warning("Failed to get available regions from AWS, something might be wrong...")
		return false
	}
	_, ok := regions[region]
	if !ok {
		// The specified region doesn't actually exist
		return false
	}
	return true
}

// GenericAMI will return an AMI in the specified region that can be used
// to launch instances.
func GenericAMI(region string) string {
	ami, exist := genericAMIMap[region]
	if !exist {
		logging.Errorf("There is no available AMI in the region '%s'", region)
		return ""
	}
	return ami
}
