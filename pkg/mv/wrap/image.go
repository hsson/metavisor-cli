package wrap

import (
	"context"
	"fmt"
	"time"

	"github.com/brkt/metavisor-cli/pkg/csp/aws"
	"github.com/brkt/metavisor-cli/pkg/logging"
	"github.com/brkt/metavisor-cli/pkg/mv"
)

const newNameTemplate = "Metavisor wrapped image based on %s (%s)"

func awsWrapImage(ctx context.Context, awsSvc aws.Service, region, id string, conf Config) (string, error) {
	if !aws.IsAMIID(id) {
		return "", aws.ErrInvalidID
	}
	if conf.Token != "" {
		isValid := isValidToken(conf.Token)
		if !isValid {
			// The specified token is not a valid launch token
			logging.Error("The specified token is not a launch token")
			return "", ErrInvalidLaunchToken
		}
	}

	// Launch a new instance
	logging.Info("Launching temporary wrapper instance")
	instanceName := "Temporary-Metavisor-wrapper-instance"
	inst, err := awsSvc.LaunchInstance(ctx, id, aws.LargerInstanceType, "", "", instanceName)
	if err != nil {
		switch err {
		case aws.ErrNotAllowed:
			logging.Error("Not enough IAM permissions to launch instance")
		default:
			logging.Error("Could not launch instance based on specified AMI")
		}
		return "", err
	}
	mv.QueueCleanup(func() {
		// Finally clean up temporary instance
		logging.Info("Cleaning up temporary instance")
		err = awsSvc.TerminateInstance(context.Background(), inst.ID())
		if err != nil {
			logging.Warningf("Failed to cleanup temporary instance %s", inst.ID())
			logging.Debugf("Error when cleaning up instance: %s", err)
		}
		logging.Infof("Instance %s terminated", inst.ID())
	}, false)
	logging.Info("Instance is ready")

	// Then wrap the instance
	logging.Info("Wrapping the temporary instance with Metavisor")
	instID, err := awsWrapInstance(ctx, awsSvc, region, inst.ID(), conf)
	if err != nil {
		logging.Error("Failed to wrap the temporary instance")
		return "", err
	}
	logging.Infof("Successfully wrapped temporary instance %s", instID)
	logging.Info("Waiting for instance to become ready before creating AMI...")

	err = awsSvc.AwaitInstanceOK(ctx, instID)
	if err != nil {
		switch err {
		case aws.ErrNotAllowed:
			logging.Error("Not enough IAM permissions to get instance health status")
		case aws.ErrInstanceImpaired:
			logging.Error("The instance is not passing health checks")
		default:
			logging.Error("An error occurred while waiting for instance to get healthy")
		}
		return "", err
	}
	logging.Info("Instance is ready")

	// Now create an AMI from the instance
	logging.Info("Creating new AMI based on wrapped instance")
	name := fmt.Sprintf(newNameTemplate, id, time.Now().Format("2006-01-02 15.04.05"))
	ami, err := awsSvc.CreateImage(ctx, instID, name)
	if err != nil {
		logging.Error("Failed to create new AMI")
		return "", err
	}

	return ami, nil
}
