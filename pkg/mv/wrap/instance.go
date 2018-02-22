package wrap

import (
	"time"

	"github.com/brkt/metavisor-cli/pkg/csp/aws"
	"github.com/brkt/metavisor-cli/pkg/logging"
)

func awsWrapInstance(awsSvc aws.Service, region, id string, conf Config) (string, error) {
	if !aws.IsInstanceID(id) {
		return "", aws.ErrInvalidID
	}
	err := awsVerifyConfig(conf)
	if err != nil {
		return "", err
	}
	inst, err := awsSvc.GetInstance(id)
	if err != nil {
		return "", err
	}
	err = awsVerifyInstance(inst)
	if err != nil {
		return "", err
	}

	if conf.MetavisorAMI == "" {
		// Get the metavisor AMI if it was not specified as an option
		mvAMI, err := getMetavisorAMI(conf.MetavisorVersion, region)
		if err != nil {
			return "", err
		}
		conf.MetavisorAMI = mvAMI
	}
	// Get the Metavisor snapshot attached to the AMI
	mvSnapshot, mvENASupport, err := awsMetavisorSnapshot(awsSvc, conf.MetavisorAMI)
	if err != nil {
		return "", err
	}
	mvVolumeSIze := mvSnapshot.SizeGB()
	logging.Debugf("MV snapshot is %d GiB", mvVolumeSIze)

	// Stop the instance so that devices can be modified
	logging.Info("Stopping the instance")
	awsSvc.StopInstance(id)
	logging.Info("Instance stopped")

	// Set userdata on instance based on parameters
	logging.Info("Generating new instance userdata")
	if conf.ServiceDomain == "" {
		conf.ServiceDomain = ProdDomain
	}
	err = awsSetInstanceUserdata(awsSvc, inst, conf.ServiceDomain, conf.Token)
	if err != nil {
		return "", err
	}
	logging.Info("Successfully set userdata on instance")

	logging.Info("Creating new Metavisor root volume")
	// Create a new volume from the MV snapshot
	mvVol, err := awsSvc.CreateVolume(mvSnapshot.ID(), rootVolumeType, inst.AvailabilityZone(), mvSnapshot.SizeGB())
	if err != nil {
		// Could not create MV root volume
		return "", err
	}
	logging.Debugf("Created MV root volume %s", mvVol.ID())

	// Move guest volume and attach MV volume as root device
	inst, err = awsShuffleInstanceVolumes(awsSvc, inst, mvVol.ID())
	if err != nil {
		return "", err
	}

	awsEnableSriovNetSupport(awsSvc, inst)
	err = awsEnableENASupport(awsSvc, inst, mvENASupport)
	if err != nil {
		return "", err
	}

	err = awsFinalizeInstance(awsSvc, inst)
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
	return nil
}

func awsShuffleInstanceVolumes(service aws.Service, instance aws.Instance, mvVolID string) (aws.Instance, error) {
	logging.Infof("Moving guest volume to %s", GuestDeviceName)
	instanceRootVolID, exist := instance.DeviceMapping()[instance.RootDeviceName()]
	if !exist {
		// Instance has no root device, we already checked this, so it should be fine
		return nil, ErrNoRootDevice
	}

	instanceRootDeviceName := instance.RootDeviceName()
	err := service.DetachVolume(instanceRootVolID, instance.ID(), instanceRootDeviceName)
	if err != nil {
		// Could not detach instance root device
		return nil, err
	}
	logging.Debug("Detached instance root device")
	err = service.AttachVolume(instanceRootVolID, instance.ID(), GuestDeviceName)
	if err != nil {
		// Could not attach volume
		return nil, err
	}
	logging.Debugf("Attached instance root device to %s", GuestDeviceName)
	logging.Debug("Guest volume successfully moved")
	logging.Infof("Attaching Metavisor root to %s", instanceRootDeviceName)
	err = service.AttachVolume(mvVolID, instance.ID(), instanceRootDeviceName)
	if err != nil {
		// Could not attach MV root device
		return nil, err
	}

	logging.Info("Waiting for Metavisor and instance volumes to be attached")
	// Wait for devices to get attached and shows up in instance block device mapping
	return awaitInstanceDevices(service, instance, mvVolID, instanceRootVolID)
}

// Here we also want to return if the MV has ENA support or not, as this is needed later
func awsMetavisorSnapshot(service aws.Service, mvImageID string) (mvSnapshot aws.Snapshot, enaSupport bool, err error) {
	logging.Debugf("Fetching AMI %s from AWS", mvImageID)
	mvImage, err := service.GetImage(mvImageID)
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
	mvSnapshot, err = service.GetSnapshot(mvSnapshotID)
	if err != nil {
		return mvSnapshot, enaSupport, err
	}
	return mvSnapshot, mvImage.ENASupport(), nil
}

func awsSetInstanceUserdata(service aws.Service, instance aws.Instance, domain, token string) error {
	userdata, err := generateUserdataString(token, domain, compressUserdata)
	if err != nil {
		return ErrBadUserdata
	}
	err = service.ModifyInstanceAttribute(instance.ID(), aws.AttrUserData, userdata)
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

func awsEnableSriovNetSupport(service aws.Service, instance aws.Instance) {
	// Enable sriovNetSupport on the instance if it's not already enabled
	if instance.SriovNetSupport() != aws.SriovNetIsSupported {
		logging.Debug("Enabling sriovNetSupport on instance")
		err := service.ModifyInstanceAttribute(instance.ID(), aws.AttrSriovNetSupport, aws.SriovNetIsSupported)
		if err != nil {
			logging.Debugf("Failed to enable sriovNetSupport:\n%s", err)
			logging.Warningf("Failed to enable sriovNetSupport for instance %s", instance.ID())
		}
	}
}

func awsEnableENASupport(service aws.Service, instance aws.Instance, mvENASupport bool) error {
	// Enable ENA support if the MV supports it and it's not already enabled on the instance
	logging.Debugf("ENA support: metavisor=%t, guest=%t", mvENASupport, instance.ENASupport())
	if mvENASupport && !instance.ENASupport() {
		logging.Info("Enabling ENA support on instance")
		err := service.ModifyInstanceAttribute(instance.ID(), aws.AttrENASupport, true)
		if err != nil {
			logging.Error("Failed to enable ENA support on the instance")
			return err
		}
	}
	return nil
}

func awsFinalizeInstance(service aws.Service, instance aws.Instance) error {
	// Wrapping is complete, start the instance again
	logging.Info("Starting instance again")
	err := service.StartInstance(instance.ID())
	if err != nil {
		logging.Error("Failed to start instance after wrapping it with Metavisor")
		return err
	}

	// The DeleteOnTerminate attribute gets reset when detaching stuff, make sure
	// it's enabled again.
	logging.Debug("Setting instance devices to delete on termination")
	err = service.DeleteInstanceDevicesOnTermination(instance.ID())
	if err != nil {
		if err == aws.ErrNotAllowed {
			logging.Warning("Not enough IAM permissions to set devices to delete on termination, skipping...")
		} else {
			return err
		}
	}
	return nil
}

func awaitInstanceDevices(service aws.Service, instance aws.Instance, mvVolID, guestVolID string) (aws.Instance, error) {
	maxTries := 60
	sleepTime := 10 * time.Second
	for try := 1; try <= maxTries; try++ {
		inst, err := service.GetInstance(instance.ID())
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
