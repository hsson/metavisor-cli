package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"

	"github.com/brkt/metavisor-cli/pkg/logging"
	"github.com/brkt/metavisor-cli/pkg/mv"
	"github.com/brkt/metavisor-cli/pkg/mv/share"
	"github.com/brkt/metavisor-cli/pkg/mv/wrap"

	"gopkg.in/alecthomas/kingpin.v2"
)

const (
	helpText = "This is a command-line interface for working with the Metavisor"

	// Env variable to always output things in JSON where applicable
	envOutputJSON = "MV_OUTPUT_JSON"
	// Env variable to set default region to use for AWS commands
	envAWSRegion = "MV_AWS_REGION"
	// Env variable to set default subnet to use for launching instances
	envAWSSubnet = "MV_AWS_SUBNET"
	// Env variable to set launch token to use for wrapping
	envLaunchToken = "MV_LAUNCH_TOKEN"
	// Env variable to set a custom service domain
	envServiceDomain = "MV_SERVICE_DOMAIN"

	// DefaultShareLogsDir is where MV logs will be stored as default
	DefaultShareLogsDir = "./"
)

var (
	app = kingpin.New("metavisor", helpText)

	// AWS commands
	awsCommand        = app.Command("aws", "Perform operations related to AWS")
	awsCommandIAM     = awsCommand.Flag("iam", "Role ARN to assume when performing operations").PlaceHolder("ARN").String()
	awsCommandIAMMFA  = awsCommand.Flag("iam-mfa", "MFA device ARN to use for MFA").PlaceHolder("ARN").String()
	awsCommandIAMCode = awsCommand.Flag("iam-code", "MFA code to use with MFA device, prompted if not specified").PlaceHolder("CODE").String()

	// AWS Wrap an instance
	awsWrapInstance        = awsCommand.Command("wrap-instance", "Wrap a running instance with Metavisor")
	awsWrapInstanceRegion  = awsWrapInstance.Flag("region", fmt.Sprintf("The AWS region to look for the instance in (overrides $%s)", envAWSRegion)).Required().Envar(envAWSRegion).String()
	awsWrapInstanceToken   = awsWrapInstance.Flag("token", fmt.Sprintf("Launch token used to identify the Metavisor (overrides $%s)", envLaunchToken)).Required().Envar(envLaunchToken).String() // TODO: Make non-required
	awsWrapInstanceVersion = awsWrapInstance.Flag("metavisor-version", "Which version of the MV to use").PlaceHolder("VERSION").String()
	awsWrapInstanceAMI     = awsWrapInstance.Flag("metavisor-image", "AMI ID of MV to use, must be in correct region").Hidden().PlaceHolder("AMI-ID").String()
	awsWrapInstanceDomain  = awsWrapInstance.Flag("service-domain", "Specify which Yeti to talk to").Hidden().PlaceHolder("DOMAIN").Envar(envServiceDomain).String()
	awsWrapInstanceID      = awsWrapInstance.Arg("ID", "ID of the instance to wrap").Required().String()

	// AWS Wrap an image
	awsWrapAMI        = awsCommand.Command("wrap-ami", "Wrap a regular AMI with Metavisor")
	awsWrapAMIRegion  = awsWrapAMI.Flag("region", fmt.Sprintf("The AWS region to look for the AMI in (overrides $%s)", envAWSRegion)).Required().Envar(envAWSRegion).String()
	awsWrapAMIToken   = awsWrapAMI.Flag("token", fmt.Sprintf("Launch token used to identify the Metavisor (overrides $%s)", envLaunchToken)).Required().Envar(envLaunchToken).String() // TODO: Make non-required
	awsWrapAMIVersion = awsWrapAMI.Flag("metavisor-version", "Which version of the MV to use").PlaceHolder("VERSION").String()
	awsWrapAMIAMI     = awsWrapAMI.Flag("metavisor-image", "AMI ID of MV to use, must be in correct region").Hidden().PlaceHolder("AMI-ID").String()
	awsWrapAMIDomain  = awsWrapAMI.Flag("service-domain", "Specify which Yeti to talk to").Hidden().PlaceHolder("DOMAIN").Envar(envServiceDomain).String()
	awsWrapAMISubnet  = awsWrapAMI.Flag("subnet-id", fmt.Sprintf("Use specified subnet when launching instances (overrides $%s)", envAWSSubnet)).PlaceHolder("ID").Envar(envAWSSubnet).String()
	awsWrapAMIID      = awsWrapAMI.Arg("ID", "ID of the instance to wrap").Required().String()

	// AWS Share logs
	awsShareLogs            = awsCommand.Command("share-logs", "Get the Metavisor logs from an instance or snapshot")
	awsShareLogsRegion      = awsShareLogs.Flag("region", fmt.Sprintf("The AWS region to look for the resource in (overrides $%s)", envAWSRegion)).Required().Envar(envAWSRegion).String()
	awsShareLogsOutPath     = awsShareLogs.Flag("output-path", "Path to store downloaded logs").Default(DefaultShareLogsDir).PlaceHolder("PATH").String()
	awsShareLogsKeyName     = awsShareLogs.Flag("key-name", "Name of SSH key in AWS to use (attempts to create one if not specified)").PlaceHolder("NAME").String()
	awsShareLogsKeyPath     = awsShareLogs.Flag("key-path", "Path to SSH private key to use (uses SSH agent if not specified)").PlaceHolder("PATH").String()
	awsShareLogsBastionHost = awsShareLogs.Flag("bastion-host", "Host of bastion to tunnel through").PlaceHolder("HOST").Hidden().String() // TODO: Support bastion
	awsShareLogsBastionUser = awsShareLogs.Flag("bastion-user", "Bastion username to tunnel through").PlaceHolder("NAME").Hidden().String()
	awsShareLogsBastionKey  = awsShareLogs.Flag("bastion-key-path", "Key in bastion to use when tunneling").PlaceHolder("PATH").Hidden().String()
	awsShareLogsSubnet      = awsShareLogs.Flag("subnet-id", fmt.Sprintf("Use specified subnet when launching instances (overrides $%s)", envAWSSubnet)).PlaceHolder("ID").Envar(envAWSSubnet).String()
	awsShareLogsID          = awsShareLogs.Arg("ID", "ID of instance or snapshot to get logs from").Required().String()

	// Generic commands
	versionCommand  = app.Command("version", "Get version information about the CLI and the Metavisor")
	versionWithJSON = versionCommand.Flag("json", fmt.Sprintf("Output information as JSON (overrides $%s)", envOutputJSON)).Envar(envOutputJSON).Short('J').Bool()

	listCommand  = app.Command("list", "List all available versions of the Metavisor")
	listWithJSON = listCommand.Flag("json", fmt.Sprintf("Output information as JSON (overrides $%s)", envOutputJSON)).Envar(envOutputJSON).Short('J').Bool()

	logVerbose = app.Flag("verbose", "Set logging level to Debug").Short('v').Bool()

	// ErrGeneric is returned when we can't figure out what error happened, but we don't want to show the actual error
	// to the user
	ErrGeneric = errors.New("an unexpected error occured")
)

func main() {
	app.HelpFlag.Hidden()
	logging.LogLevel = logging.LevelInfo
	logging.LogFileNamePrefix = "metavisor-cli-log"
	logging.LogToFile = true
	command, err := app.Parse(os.Args[1:])
	if err != nil {
		app.Usage(os.Args[1:])
		fmt.Printf("error: %s\n", err)
		os.Exit(1)
		return
	}
	if *logVerbose {
		logging.LogLevel = logging.LevelDebug
	}

	ctx := context.Background()
	switch command {
	case versionCommand.FullCommand():
		runWithInterrupt(ctx, showVersion)
		break
	case listCommand.FullCommand():
		runWithInterrupt(ctx, listMetavisors)
		break
	case awsWrapInstance.FullCommand():
		runWithInterrupt(ctx, wrapInstance)
		break
	case awsWrapAMI.FullCommand():
		runWithInterrupt(ctx, wrapAMI)
		break
	case awsShareLogs.FullCommand():
		runWithInterrupt(ctx, shareLogs)
		break
	}
}

func showVersion(ctx context.Context) {
	exit := 0
	versionInfo, err := mv.GetInfo(ctx)
	if err != nil {
		// Could not fetch MV version. Log to debug and still show CLI version
		logging.Debug("Could not determine latest MV version, only showing CLI version")
		exit = 1
	}
	output, err := mv.FormatInfo(versionInfo, *versionWithJSON)
	if err != nil {
		// Could not marshal information to JSON
		logging.Fatal(ErrGeneric)
		return
	}
	fmt.Println(output)
	os.Exit(exit)
}

func listMetavisors(ctx context.Context) {
	mvs, err := mv.GetMetavisorVersions(ctx)
	if err != nil {
		// Could not fetch available MV versions
		logging.Fatal("Could not fetch available MV versions")
		return
	}
	output, err := mv.FormatMetavisors(mvs, *listWithJSON)
	if err != nil {
		// Could not marshal versions to JSON
		logging.Fatal(ErrGeneric)
		return
	}
	fmt.Println(output)
}

func runWithInterrupt(ctx context.Context, f func(ctx context.Context)) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	interrupt := make(chan os.Signal)
	signal.Notify(interrupt, os.Interrupt)
	done := make(chan struct{})

	var wg sync.WaitGroup
	go func() {
		wg.Add(1)
		f(ctx)
		done <- struct{}{}
	}()

	select {
	case <-done:
		wg.Done()
		break
	case <-interrupt:
		// User interrupted with ^C
		logging.Warning("Execution interrupted")
		cancel()
		break
	}

	wg.Wait()
	return
}

func wrapInstance(ctx context.Context) {
	conf := wrap.Config{
		Token:            *awsWrapInstanceToken,
		MetavisorVersion: *awsWrapInstanceVersion,
		MetavisorAMI:     *awsWrapInstanceAMI,
		ServiceDomain:    *awsWrapInstanceDomain,
		IAMRoleARN:       *awsCommandIAM,
		IAMDeviceARN:     *awsCommandIAMMFA,
		IAMCode:          *awsCommandIAMCode,
	}
	inst, err := wrap.Instance(ctx, *awsWrapInstanceRegion, *awsWrapInstanceID, conf)
	if err != nil {
		// Could not wrap instance, show error
		logging.Fatal(err)
		return
	}
	logging.Info("Successfully wrapped instance:")
	logging.Output(inst)
}

func wrapAMI(ctx context.Context) {
	conf := wrap.Config{
		Token:            *awsWrapAMIToken,
		MetavisorVersion: *awsWrapAMIVersion,
		MetavisorAMI:     *awsWrapAMIAMI,
		ServiceDomain:    *awsWrapAMIDomain,
		SubnetID:         *awsWrapAMISubnet,
		IAMRoleARN:       *awsCommandIAM,
		IAMDeviceARN:     *awsCommandIAMMFA,
		IAMCode:          *awsCommandIAMCode,
	}
	ami, err := wrap.Image(ctx, *awsWrapAMIRegion, *awsWrapAMIID, conf)
	if err != nil {
		// Could not wrap image, show error
		logging.Fatal(err)
		return
	}
	logging.Info("Successfully wrapped image:")
	logging.Output(ami)
}

func shareLogs(ctx context.Context) {
	conf := share.Config{
		LogsPath:              *awsShareLogsOutPath,
		AWSKeyName:            *awsShareLogsKeyName,
		PrivateKeyPath:        *awsShareLogsKeyPath,
		BastionHost:           *awsShareLogsBastionHost,
		BastionUsername:       *awsShareLogsBastionUser,
		BastionPrivateKeyPath: *awsShareLogsBastionKey,
		SubnetID:              *awsShareLogsSubnet,
		IAMRoleARN:            *awsCommandIAM,
		IAMDeviceARN:          *awsCommandIAMMFA,
		IAMCode:               *awsCommandIAMCode,
	}
	logs, err := share.LogsAWS(ctx, *awsShareLogsRegion, *awsShareLogsID, conf)
	if err != nil {
		// Could not get logs, show error
		logging.Fatal(err)
		return
	}
	logging.Info("Logs saved to:")
	logging.Output(logs)
}
