package share

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/brkt/metavisor-cli/pkg/csp/aws"
	"github.com/brkt/metavisor-cli/pkg/logging"
	"github.com/brkt/metavisor-cli/pkg/mv"
	"github.com/brkt/metavisor-cli/pkg/scp"
)

const (
	// DefaultLogArchiveName will be used if the specified output path is just a directory
	DefaultLogArchiveName = "mv-logs.tar.gz"
)

var (
	// ErrFileExist is returned if specifying an output path leading to an existing file
	ErrFileExist = errors.New("the specified output file already exist")

	// ErrNoRootVolume is returned if the specified instance has no root volume
	ErrNoRootVolume = errors.New("the specified resource has no root volume")

	// ErrNoPublicIP is returned if the temporary instance launched doesn't have any IP
	ErrNoPublicIP = errors.New("instance has no public IP")

	// ErrLogTimeout is returned if logs are never successfully downlaoded after several retries
	ErrLogTimeout = errors.New("timed out while waiting for logs to download")
)

// Config can be used to specify extra parameters when sharing logs
type Config struct {
	LogsPath              string
	AWSKeyName            string
	PrivateKeyPath        string
	BastionHost           string
	BastionUsername       string
	BastionPrivateKeyPath string
	IAMRoleARN            string
	IAMDeviceARN          string
	IAMCode               string
}

// LogsAWS will get the MV logs of an instance or snapshot in AWS and return
// the path to the resuling log archive.
func LogsAWS(ctx context.Context, region, id string, conf Config) (string, error) {
	logging.Info("Getting metavisor logs...")
	res := make(chan mv.MaybeString, 1)

	go func() {
		out, err := awsShareLogs(ctx, region, id, conf)
		res <- mv.MaybeString{
			Result: out,
			Error:  err,
		}
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

// TODO: Refactor this huge function...
func awsShareLogs(ctx context.Context, region, id string, conf Config) (string, error) {
	if !aws.IsInstanceID(id) && !aws.IsSnapshotID(id) {
		return "", aws.ErrInvalidID
	}
	path, err := parseOutPath(conf.LogsPath)
	if err != nil {
		return path, err
	}
	awsSvc, err := aws.New(region, &aws.IAMConfig{
		RoleARN:      conf.IAMRoleARN,
		MFADeviceARN: conf.IAMDeviceARN,
		MFACode:      conf.IAMCode,
	})
	if err != nil {
		return "", err
	}

	keyExist, err := awsSvc.KeyPairExist(ctx, conf.AWSKeyName)
	if err != nil {
		if err == aws.ErrNotAllowed {
			// Not allowed to check if key exist, assume it's correct and continue
			logging.Warning("Not allowed to check if key exists in AWS, assuming it does...")
			keyExist = true
		} else {
			return "", err
		}
	}
	if !keyExist {
		// Create a temporary key to be used
		rand.Seed(time.Now().Unix())
		randomName := fmt.Sprintf("MetavisorTemporaryKey-%d", rand.Int())
		logging.Debugf("Creating temporray key pair with name: %s", randomName)
		conf.AWSKeyName = randomName
		keyContent, err := awsSvc.CreateKeyPair(ctx, randomName)
		if err != nil {
			if err == aws.ErrNotAllowed {
				// The use does not have IAM permission to create key pair, tell
				// the user to specify a key with --key
				logging.Error("Not enough IAM permissions to create a new key pair")
				logging.Error("Please specify an existing key with the --key flag instead")
				return "", err
			}
			return "", err
		}
		mv.QueueCleanup(func() {
			logging.Info("Deleting temporary AWS key pair")
			err := awsSvc.RemoveKeyPair(context.Background(), randomName)
			if err != nil {
				logging.Errorf("Failed to clean up key pair in AWS: %s", randomName)
				logging.Debugf("Error when deleting key pair in AWS: %s", err)
			}
		}, false)
		p, err := writeToTempFile(keyContent)
		conf.PrivateKeyPath = p
		mv.QueueCleanup(func() {
			logging.Infof("Deleting temporary private key")
			err := os.Remove(p)
			if err != nil {
				logging.Errorf("Failed to clean up private key: %s", p)
				logging.Debugf("Could not delete file: %s", err)
			}
		}, false)
	}

	snap, err := awsSnapFromID(ctx, id, awsSvc)
	if err != nil {
		return "", err
	}
	logging.Debugf("Getting logs from snapshot: %s", snap.ID())

	// Launch a temporary instance
	_, logsFile := filepath.Split(path)
	userdata := awsCreateUserData(logsFile)
	device := aws.NewDevice{
		DeviceName: "/dev/sdg",
		SnapshotID: snap.ID(),
	}
	logging.Info("Launching a temporary instance to get logs...")
	ami := aws.GenericAMI(region)
	if ami == "" {
		return "", aws.ErrNoAMIInRegion
	}
	instanceName := "Temporary-share-logs-instance"
	instance, err := awsSvc.LaunchInstance(ctx, ami, aws.SmallInstanceType, userdata, conf.AWSKeyName, instanceName, device)
	if err != nil {
		switch err {
		case aws.ErrNotAllowed:
			logging.Error("Not enough IAM permissions to launch an instance")
			break
		case aws.ErrKeyNonExisting:
			logging.Errorf("The key pair '%s' does not exist in AWS", conf.AWSKeyName)
			break
		case aws.ErrNoAMIInRegion:
			logging.Error("There is no AMI available in the specified region")
			break
		case aws.ErrFailedLaunchingInstance:
			logging.Error("Failed launching temporary instance")
			break
		}
		return "", err
	}
	mv.QueueCleanup(func() {
		logging.Infof("Terminating temporary instance %s", instance.ID())
		err := awsSvc.TerminateInstance(context.Background(), instance.ID())
		if err != nil {
			logging.Errorf("Failed to cleanup instance: %s", instance.ID())
			logging.Debugf("Got error when terminating instance: %s", err)
		}
	}, false)
	if instance.PublicIP() == "" {
		logging.Info("Waiting for public IP to become available...")
		instance, err = awsAwaitPublicIP(ctx, instance.ID(), awsSvc)
		if err != nil {
			// Instance has no public IP, can't continue...
			return "", err
		}
	}

	scpConfig := scp.Config{
		Username: "ec2-user",
		Host:     instance.PublicIP(),
		Key:      conf.PrivateKeyPath,
	}
	// TODO: Bastion support not implemented yet
	if conf.BastionHost != "" || conf.BastionPrivateKeyPath != "" || conf.BastionUsername != "" {
		scpProxy := &scp.Proxy{
			Username: conf.BastionUsername,
			Host:     conf.BastionHost,
			Key:      conf.BastionPrivateKeyPath,
		}
		scpConfig.Proxy = scpProxy
	}
	scpClient, err := scp.New(scpConfig)
	if err != nil {
		// Bad config, should not happen...
		logging.Error("Failed to create SCP client with the specified config")
		return "", err
	}

	logging.Info("Downloading logs from temporary instance...")
	for try := 1; try <= 60; try++ {
		err = scpClient.DownloadFile(fmt.Sprintf("/tmp/%s", logsFile), path)
		if err != nil {
			logging.Warningf("Attempt %d: Instance refused connection, trying again...", try)
			time.Sleep(15 * time.Second)
			continue
		}
		logging.Info("Successfully downloaded logs")

		return path, nil
	}
	return "", errors.New("Timed out while waiting for logs to download")
}

// This function will construct a valid output path based on what the
// user entered. It will create subdirectories if needed
func parseOutPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		logging.Debug("No out path specified, using default")
		return filepath.Join(filepath.Dir(""), DefaultLogArchiveName), nil
	}
	path = filepath.FromSlash(path)

	if info, err := os.Stat(path); err == nil {
		if !info.IsDir() {
			// The specified path is a file that already exist
			logging.Error("The specified out file already exist")
			return "", ErrFileExist
		}
		// Path is a dir, join with default file name
		logging.Debug("Using default file name in the specified directory")
		path = filepath.Join(path, DefaultLogArchiveName)
		return path, nil
	}
	// Path is a file that does not exist, create required
	// directories for path
	logging.Debug("Creating required directories for specified out file")
	err := os.MkdirAll(filepath.Dir(path), os.ModePerm)
	return path, err
}

func writeToTempFile(content string) (string, error) {
	file, err := ioutil.TempFile("", "mv-temp-key")
	if err != nil {
		return "", err
	}
	defer file.Close()
	logging.Debugf("Created temporary key file at: %s", file.Name())
	_, err = file.WriteString(content)
	if err != nil {
		return "", err
	}
	logging.Debug("Setting permission 0400 on key file")
	err = file.Chmod(0400)
	if err != nil {
		logging.Error("Failed setting permissions on key file")
		return "", err
	}
	return file.Name(), nil
}
