package wrap

import (
	"context"
	"time"

	"github.com/brkt/metavisor-cli/pkg/mv"

	"github.com/brkt/metavisor-cli/pkg/csp/aws"
	"github.com/brkt/metavisor-cli/pkg/logging"
)

func awsWrapInstance(ctx context.Context, awsSvc aws.Service, region, id string, conf Config) (string, error) {
	if !aws.IsInstanceID(id) {
		return "", aws.ErrInvalidID
	}
	err := awsVerifyConfig(conf)
	if err != nil {
		return "", err
	}
	inst, err := awsSvc.GetInstance(ctx, id)
	if err != nil {
		return "", err
	}
	err = awsVerifyInstance(inst)
	if err != nil {
		return "", err
	}

	if conf.MetavisorAMI == "" {
		// Get the metavisor AMI if it was not specified as an option
		mvAMI, err := getMetavisorAMI(ctx, conf.MetavisorVersion, region)
		if err != nil {
			return "", err
		}
		conf.MetavisorAMI = mvAMI
	}
	// Get the Metavisor snapshot attached to the AMI
	mvSnapshot, mvENASupport, err := awsMetavisorSnapshot(ctx, awsSvc, conf.MetavisorAMI)
	if err != nil {
		return "", err
	}
	mvVolumeSIze := mvSnapshot.SizeGB()
	logging.Debugf("MV snapshot is %d GiB", mvVolumeSIze)

	// Stop the instance so that devices can be modified
	logging.Info("Stopping the instance")
	err = awsSvc.StopInstance(ctx, id)
	if err != nil {
		// Could not stop the instance
		return "", err
	}
	logging.Info("Instance stopped")

	// Set userdata on instance based on parameters
	logging.Info("Generating new instance userdata")
	if conf.ServiceDomain == "" {
		conf.ServiceDomain = ProdDomain
	}
	err = awsSetInstanceUserdata(ctx, awsSvc, inst, conf.ServiceDomain, conf.Token)
	if err != nil {
		return "", err
	}
	logging.Info("Successfully set userdata on instance")

	logging.Info("Creating new Metavisor root volume")
	// Create a new volume from the MV snapshot
	mvVol, err := awsSvc.CreateVolume(ctx, mvSnapshot.ID(), rootVolumeType, inst.AvailabilityZone(), mvSnapshot.SizeGB())
	if err != nil {
		// Could not create MV root volume
		return "", err
	}
	mv.QueueCleanup(func() {
		// Clean this volume up if wrapping fails
		logging.Info("Deleting Metavisor volume")
		err := awsSvc.DeleteVolume(context.Background(), mvVol.ID())
		if err != nil {
			logging.Errorf("Failed to clean up MV volume: %s", mvVol.ID())
			logging.Debugf("Could not delete volume: %s", err)
		}
	}, true)
	logging.Debugf("Created MV root volume %s", mvVol.ID())

	// Move guest volume and attach MV volume as root device
	inst, err = awsShuffleInstanceVolumes(ctx, awsSvc, inst, mvVol.ID())
	if err != nil {
		return "", err
	}

	awsEnableSriovNetSupport(ctx, awsSvc, inst)
	err = awsEnableENASupport(ctx, awsSvc, inst, mvENASupport)
	if err != nil {
		return "", err
	}

	err = awsFinalizeInstance(ctx, awsSvc, inst)
	return inst.ID(), err
}

func awsVerifyInstance(instance aws.Instance) error {
	if _, hasRootDevice := instance.DeviceMapping()[instance.RootDeviceName()]; !hasRootDevice {
		return ErrNoRootDevice
	}
	for _, t := range disallowedInstanceTypes {
		if t == instance.InstanceType() {
			logging.Errorf("Instance has unsupported instance type %s", instance.InstanceType())
			return ErrInvalidType
		}
	}
	if _, sdfAvailable := instance.DeviceMapping()[GuestDeviceName]; sdfAvailable {
		logging.Errorf("The device %s must be available to wrap with Metavisor", GuestDeviceName)
		return ErrDeviceOccupied
	}
	return nil
}

func awsVerifyConfig(conf Config) error {
	if conf.MetavisorVersion != "" && conf.MetavisorAMI != "" {
		logging.Debug("Both MV version and MV AMI specified, using AMI")
	}
	if conf.MetavisorAMI != "" && !aws.IsAMIID(conf.MetavisorAMI) {
		// User specified an invalid MV AMI
		logging.Error("The specified Metavisor AMI is not a valid AMI ID")
		return ErrInvalidAMI
	}
	if conf.Token != "" {
		isValid := isValidToken(conf.Token)
		if !isValid {
			// The specified token is not a valid launch token
			logging.Error("The specified token is not a launch token")
			return ErrInvalidLaunchToken
		}
	}
	return nil
}

func awsShuffleInstanceVolumes(ctx context.Context, service aws.Service, instance aws.Instance, mvVolID string) (aws.Instance, error) {
	logging.Infof("Moving guest volume to %s", GuestDeviceName)
	instanceRootVolID, exist := instance.DeviceMapping()[instance.RootDeviceName()]
	if !exist {
		// Instance has no root device, we already checked this, so it should be fine
		return nil, ErrNoRootDevice
	}
	mv.QueueCleanup(func() {
		// We should clean up the instance volume on failures, because when detaching the volume
		// from the instance it will no longer get automatically deleted on instance termination
		logging.Info("Deleting instance volume")
		err := service.DeleteVolume(context.Background(), instanceRootVolID)
		if err != nil {
			logging.Errorf("Failed to clean up instance volume: %s", instanceRootVolID)
			logging.Debugf("Could not delete volume: %s", err)
		}
	}, true)

	instanceRootDeviceName := instance.RootDeviceName()
	err := service.DetachVolume(ctx, instanceRootVolID, instance.ID(), instanceRootDeviceName)
	if err != nil {
		// Could not detach instance root device
		return nil, err
	}
	logging.Debug("Detached instance root device")
	err = service.AttachVolume(ctx, instanceRootVolID, instance.ID(), GuestDeviceName)
	if err != nil {
		// Could not attach volume
		return nil, err
	}
	logging.Debugf("Attached instance root device to %s", GuestDeviceName)
	logging.Debug("Guest volume successfully moved")
	logging.Infof("Attaching Metavisor root to %s", instanceRootDeviceName)
	err = service.AttachVolume(ctx, mvVolID, instance.ID(), instanceRootDeviceName)
	if err != nil {
		// Could not attach MV root device
		return nil, err
	}

	logging.Info("Waiting for Metavisor and instance volumes to be attached")
	// Wait for devices to get attached and shows up in instance block device mapping
	return awaitInstanceDevices(ctx, service, instance, mvVolID, instanceRootVolID)
}

// Here we also want to return if the MV has ENA support or not, as this is needed later
func awsMetavisorSnapshot(ctx context.Context, service aws.Service, mvImageID string) (mvSnapshot aws.Snapshot, enaSupport bool, err error) {
	logging.Debugf("Fetching AMI %s from AWS", mvImageID)
	mvImage, err := service.GetImage(ctx, mvImageID)
	if err != nil {
		return mvSnapshot, enaSupport, err
	}
	logging.Debug("Determining snapshot from Metavisor image")
	mvSnapshotID, exist := mvImage.DeviceMapping()[mvImage.RootDeviceName()]
	if !exist {
		// Something is wrong with this MV AMI, it doesn't have any root device
		return mvSnapshot, enaSupport, ErrInvalidAMI
	}
	logging.Debugf("Fetching snapshot %s from AWS", mvSnapshotID)
	mvSnapshot, err = service.GetSnapshot(ctx, mvSnapshotID)
	if err != nil {
		return mvSnapshot, enaSupport, err
	}
	return mvSnapshot, mvImage.ENASupport(), nil
}

func awsSetInstanceUserdata(ctx context.Context, service aws.Service, instance aws.Instance, domain, token string) error {
	userdata, err := generateUserdataString(token, domain, compressUserdata)
	if err != nil {
		return ErrBadUserdata
	}
	err = service.ModifyInstanceAttribute(ctx, instance.ID(), aws.AttrUserData, userdata)
	if err != nil {
		switch err {
		case aws.ErrNotAllowed:
			logging.Error("Not enough IAM permissions to set userdata on instance")
			return err
		default:
			logging.Error("Failed to set userdata on instance")
			return ErrBadUserdata
		}
	}
	return nil
}

func awsEnableSriovNetSupport(ctx context.Context, service aws.Service, instance aws.Instance) {
	// Enable sriovNetSupport on the instance if it's not already enabled
	if instance.SriovNetSupport() != aws.SriovNetIsSupported {
		logging.Debug("Enabling sriovNetSupport on instance")
		err := service.ModifyInstanceAttribute(ctx, instance.ID(), aws.AttrSriovNetSupport, aws.SriovNetIsSupported)
		if err != nil {
			logging.Debugf("Failed to enable sriovNetSupport:\n%s", err)
			logging.Warningf("Failed to enable sriovNetSupport for instance %s", instance.ID())
		}
	}
}

func awsEnableENASupport(ctx context.Context, service aws.Service, instance aws.Instance, mvENASupport bool) error {
	// Enable ENA support if the MV supports it and it's not already enabled on the instance
	logging.Debugf("ENA support: metavisor=%t, guest=%t", mvENASupport, instance.ENASupport())
	if mvENASupport && !instance.ENASupport() {
		logging.Info("Enabling ENA support on instance")
		err := service.ModifyInstanceAttribute(ctx, instance.ID(), aws.AttrENASupport, true)
		if err != nil {
			logging.Error("Failed to enable ENA support on the instance")
			return err
		}
	}
	return nil
}

func awsFinalizeInstance(ctx context.Context, service aws.Service, instance aws.Instance) error {
	// Wrapping is complete, start the instance again
	logging.Info("Starting instance again")
	err := service.StartInstance(ctx, instance.ID())
	if err != nil {
		logging.Error("Failed to start instance after wrapping it with Metavisor")
		return err
	}

	// The DeleteOnTerminate attribute gets reset when detaching stuff, make sure
	// it's enabled again.
	logging.Debug("Setting instance devices to delete on termination")
	err = service.DeleteInstanceDevicesOnTermination(ctx, instance.ID())
	if err != nil {
		if err == aws.ErrNotAllowed {
			logging.Warning("Not enough IAM permissions to set devices to delete on termination, skipping...")
		} else {
			return err
		}
	}
	return nil
}

func awaitInstanceDevices(ctx context.Context, service aws.Service, instance aws.Instance, mvVolID, guestVolID string) (aws.Instance, error) {
	maxTries := 60
	sleepTime := 10 * time.Second
	for try := 1; try <= maxTries; try++ {
		inst, err := service.GetInstance(ctx, instance.ID())
		if err != nil {
			if err == aws.ErrNotAllowed {
				// No point in retrying if we don't have permissions
				logging.Error("Not enough IAM permissions to get instance details")
				return nil, err
			}
			logging.Warning("Failed to get instance details, retrying...")
			continue
		}
		gVID, guestAttached := inst.DeviceMapping()[GuestDeviceName]
		mVID, mvAttached := inst.DeviceMapping()[instance.RootDeviceName()]
		if guestAttached && mvAttached && guestVolID == gVID && mvVolID == mVID {
			logging.Info("Volumes successfully attached")
			return inst, nil
		}
		logging.Debug("Got instance device mapping:")
		for d, v := range inst.DeviceMapping() {
			logging.Debugf("\t%s: %s", d, v)
		}
		if try == maxTries {
			logging.Error("Volumes never got attached to instance")
			return nil, ErrTimedOut
		}

		logging.Infof("Attempt %d: Still waiting for volumes to attach", try)
		time.Sleep(sleepTime)
	}
	return nil, ErrTimedOut
}
