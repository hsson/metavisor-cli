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

package wrap

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/immutable/metavisor-cli/pkg/csp/aws"
	"github.com/immutable/metavisor-cli/pkg/logging"
	"github.com/immutable/metavisor-cli/pkg/mv"
)

// Config can be passed to specify optional parameters when wrapping
type Config struct {
	Token            string
	MetavisorVersion string
	MetavisorAMI     string
	ServiceDomain    string
	IAMRoleARN       string
	IAMDeviceARN     string
	IAMCode          string
	SubnetID         string
}

const (
	// GuestDeviceName is where the MV expects the guest OS to be mounted
	// after being wrapped
	GuestDeviceName = "/dev/sdf"
	// ProdDomain is the domain of the production service
	ProdDomain = "mgmt.brkt.com"

	rootVolumeType = "gp2"
)

var disallowedInstanceTypes = []string{
	"t2.nano",
	"t1.micro",
}

var (
	// ErrInvalidType is returned if trying to wrap an instance that has an
	// unsupported instance type
	ErrInvalidType = errors.New("invalid instance type")

	// ErrDeviceOccupied is returned if trying to wrap an instance which already
	// has the device where we put the guest volume occupied
	ErrDeviceOccupied = fmt.Errorf("instance already has %s mounted", GuestDeviceName)

	// ErrInvalidAMI is returned if trying to specify an invalid AMI
	ErrInvalidAMI = errors.New("specified AMI is not valid")

	// ErrNoMVVersion is returned if the latest MV version could not be determined
	ErrNoMVVersion = errors.New("could not find metavisor version")

	// ErrNoMVInRegion is returned if there is no MV AMI available in the specified region
	ErrNoMVInRegion = errors.New("no MV available in specified region")

	// ErrNoRootDevice is returned if the instance doesn't have a root device
	ErrNoRootDevice = errors.New("instance doesn't have any root device")

	// ErrTimedOut is returned if something times out while waiting
	ErrTimedOut = errors.New("timed out while waiting")

	// ErrBadUserdata is returned if userdata could not be generated
	ErrBadUserdata = errors.New("could not generate userdata for instance")

	// ErrInvalidMetavisorVersion is returned if trying to specify a metavisor version with
	// --metavisor-version which doesn't exist
	ErrInvalidMetavisorVersion = errors.New("specified metavisor version doesn't exist, use the\"list\" command to find available versions")
)

// Instance will wrap a given instance with the Metavisor. The specified
// ID must exist in the specified region. A config can be optionally
// specified to give extra parameters when wrapping. If a specified
// parameter is invalid, an error will be returned, otherwise the
// ID of the wrapped instance will be returned (typically the same
// as the ID given as a parameter).
func Instance(ctx context.Context, region, id string, conf Config) (string, error) {
	logging.Infof("Wrapping instance %s with Metavisor...", id)
	res := make(chan mv.MaybeString, 1)

	go func() {
		iamConf := &aws.IAMConfig{
			RoleARN:      conf.IAMRoleARN,
			MFADeviceARN: conf.IAMDeviceARN,
			MFACode:      conf.IAMCode,
		}
		if strings.TrimSpace(region) == "" {
			// If no region is specified for the wrap instance command, the CLI
			// will try to figure it out. This is possible since the instance ID
			// should be locally unique across regions within the current account,
			// especially for a limited time frame
			logging.Info("No region was specified, attempting to find it automatically")
			reg, err := aws.FindInstanceRegion(id, iamConf)
			if err != nil {
				if err == aws.ErrAmbigiousInstanceRegion {
					logging.Warning("Please specify instance region with: --region")
				}
				res <- mv.MaybeString{Result: "", Error: err}
				return
			}
			logging.Infof("Found instance in region %s", reg)
			region = reg
		}
		service, err := aws.New(region, iamConf)
		if err != nil {
			if err == aws.ErrInvalidARN {
				logging.Error("Failed to assume IAM role")
			}
			res <- mv.MaybeString{Result: "", Error: err}
			return
		}
		inst, err := awsWrapInstance(ctx, service, region, id, conf)
		res <- mv.MaybeString{Result: inst, Error: err}
	}()
	select {
	case <-ctx.Done():
		// Context was cancelled, cleanup
		mv.Cleanup(false)
		return "", mv.ErrInterrupted
	case r := <-res:
		mv.Cleanup(r.Error == nil)
		return r.Result, r.Error
	}
}

// Image will wrap a given image with the Metavisor, and then output
// a new image that can be used to launch instances. The specified
// image ID must exist in the specified region. A config can be optionally
// specified ot give extra parameters when wrapping.
func Image(ctx context.Context, region, id string, conf Config) (string, error) {
	logging.Infof("Creating wrapped image based on %s...", id)
	res := make(chan mv.MaybeString, 1)

	go func() {
		service, err := aws.New(region, &aws.IAMConfig{
			RoleARN:      conf.IAMRoleARN,
			MFADeviceARN: conf.IAMDeviceARN,
			MFACode:      conf.IAMCode,
		})
		if err != nil {
			if err == aws.ErrInvalidARN {
				logging.Error("Failed to assume IAM role")
			}
			res <- mv.MaybeString{Result: "", Error: err}
			return
		}
		img, err := awsWrapImage(ctx, service, region, id, conf)
		res <- mv.MaybeString{Result: img, Error: err}
	}()

	select {
	case <-ctx.Done():
		// Context was cancelled, cleanup
		mv.Cleanup(false)
		return "", mv.ErrInterrupted
	case r := <-res:
		mv.Cleanup(r.Error == nil)
		return r.Result, r.Error
	}
}

func getMetavisorAMI(ctx context.Context, version, region string) (string, error) {
	// If no version was specified, get the latest version
	if version == "" {
		logging.Info("Getting the latest Metavisor version...")
		v, err := getLatestMVVersion(ctx)
		if err != nil {
			return "", err
		}
		version = v
	}
	logging.Infof("Using Metavisor version %s", version)
	return getAMIForVersion(ctx, version, region)
}

func getAMIForVersion(ctx context.Context, version, region string) (string, error) {
	mapping, err := mv.GetImagesForVersionAWS(ctx, version)
	if err != nil {
		logging.Error("Could not find AMIs for the specified MV version")
		logging.Debugf("Got error when trying to get MV AMI: %s", err)
		return "", ErrInvalidMetavisorVersion
	}
	ami, exist := mapping[region]
	if !exist {
		logging.Error("The Metavisor is not available in the specified region")
		return "", ErrNoMVInRegion
	}
	return ami, nil
}

func getLatestMVVersion(ctx context.Context) (string, error) {
	versions, err := mv.GetMetavisorVersions(ctx)
	if err != nil {
		return "", err
	}
	if versions.Latest == "" {
		logging.Error("Could not determine the latest MV version")
		return "", ErrNoMVVersion
	}
	return versions.Latest, nil
}
