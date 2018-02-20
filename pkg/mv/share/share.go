package share

import (
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/brkt/metavisor-cli/pkg/logging"

	"github.com/brkt/metavisor-cli/pkg/csp/aws"
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

// LogsAWS will get the MV logs of an instance or snapshot in AWS and return
// the path to the resuling log archive.
func LogsAWS(region, id, path, keyName, keyPath, bastHost, bastUser, bastPath string) (string, error) {
	logging.Info("Getting metavisor logs...")
	if !aws.IsInstanceID(id) && !aws.IsSnapshotID(id) {
		return "", aws.ErrInvalidID
	}
	path, err := parseOutPath(path)
	if err != nil {
		return path, err
	}

	awsSvc, err := aws.New(region)
	if err != nil {
		return "", err
	}

	keyExist, err := awsSvc.KeyPairExist(keyName)
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
		keyName = randomName
		keyContent, err := awsSvc.CreateKeyPair(randomName)
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
		defer awsSvc.RemoveKeyPair(randomName)
		p, err := writeToTempFile(keyContent)
		keyPath = p
		defer os.Remove(p)
	}

	snap, err := awsSnapFromID(id, awsSvc)
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
	instance, err := awsSvc.LaunchInstance(ami, aws.SmallInstanceType, userdata, keyName, instanceName, device)
	if err != nil {
		switch err {
		case aws.ErrNotAllowed:
			logging.Error("Not enough IAM permissions to launch an instance")
			break
		case aws.ErrKeyNonExisting:
			logging.Errorf("The key pair '%s' does not exist in AWS", keyName)
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
	defer awsSvc.TerminateInstance(instance.ID())
	if instance.PublicIP() == "" {
		logging.Info("Waiting for public IP to become available...")
		instance, err = awsAwaitPublicIP(instance.ID(), awsSvc)
		if err != nil {
			// Instance has no public IP, can't continue...
			return "", err
		}
	}

	scpConfig := scp.Config{
		Username: "ec2-user",
		Host:     instance.PublicIP(),
		Key:      keyPath,
	}
	// TODO: Bastion support not implemented yet
	if bastHost != "" || bastPath != "" || bastUser != "" {
		scpProxy := &scp.Proxy{
			Username: bastUser,
			Host:     bastHost,
			Key:      bastPath,
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
