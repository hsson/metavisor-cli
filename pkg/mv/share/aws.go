package share

import (
	"context"
	"fmt"
	"time"

	"github.com/brkt/metavisor-cli/pkg/mv"

	"github.com/brkt/metavisor-cli/pkg/logging"

	"github.com/brkt/metavisor-cli/pkg/csp/aws"
)

const awsUserDataTemplate = `#!/bin/bash
sudo mount -t ufs -o ro,ufstype=ufs2 /dev/xvdg4 /mnt
sudo tar czvf /tmp/temp_logs -C /mnt ./log ./crash
mv /tmp/temp_logs /tmp/%s`

func awsInstanceToSnap(ctx context.Context, awsService aws.Service, instanceID string) (aws.Snapshot, error) {
	inst, err := awsService.GetInstance(ctx, instanceID)
	if err != nil {
		logging.Errorf("Could not get an instance with the ID '%s'", instanceID)
		return nil, err
	}
	rootName := inst.RootDeviceName()
	rootID, ok := inst.DeviceMapping()[rootName]
	if !ok {
		return nil, ErrNoRootVolume
	}
	snapName := "Temporary share-logs snapshot"
	logging.Infof("Creating a temporary snapshot with name: %s", snapName)
	return awsService.CreateSnapshot(ctx, snapName, rootID)
}

func awsCreateUserData(logFileName string) string {
	userdata := fmt.Sprintf(awsUserDataTemplate, logFileName)
	logging.Debugf("Generated the following userdata:\n%s", userdata)
	return userdata
}

// Turn an instance ID or snapshot ID into a Snapshot reference
func awsSnapFromID(ctx context.Context, id string, awsSvc aws.Service) (aws.Snapshot, error) {
	if aws.IsInstanceID(id) {
		logging.Debugf("The ID '%s' is an instance", id)
		// We must create a snapshot from the instance
		s, err := awsInstanceToSnap(ctx, awsSvc, id)
		if err != nil {
			// Could not create snapshot from instance
			logging.Errorf("Failed to create snapshot from instance %s", id)
			return nil, err
		}
		mv.QueueCleanup(func() {
			logging.Info("Removing temporary snapshot")
			err := awsSvc.DeleteSnapshot(context.Background(), s.ID())
			if err != nil {
				logging.Errorf("Failed to delete snapshot %s", s.ID())
				logging.Debugf("Got error when deleting snapshot: %s", err)
			}
		}, false)
		return s, nil
	} else if aws.IsSnapshotID(id) {
		logging.Debugf("The ID '%s' is a snapshot", id)
		s, err := awsSvc.GetSnapshot(ctx, id)
		if err != nil {
			// THe specified instance doesn't exist or insufficient
			// permissions
			return nil, err
		}
		return s, nil
	} else {
		logging.Debugf("'%s' is neither an instance or a snapshot", id)
		return nil, aws.ErrInvalidID
	}
}

func awsAwaitPublicIP(ctx context.Context, instanceID string, awsSvc aws.Service) (aws.Instance, error) {
	maxTries := 10
	for try := 1; try <= maxTries; try++ {
		inst, err := awsSvc.GetInstance(ctx, instanceID)
		if err == nil && inst.PublicIP() != "" {
			return inst, nil
		}
		logging.Debugf("Still waiting for public IP from %s...", instanceID)
		time.Sleep(5 * time.Second)
	}
	logging.Debugf("%s never got a public IP", instanceID)
	return nil, ErrNoPublicIP
}
