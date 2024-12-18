package software

import (
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/logger"

	"github.com/astronomer/astro-cli/config"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	// init debug logs should be used only for logs produced during the CLI-initialization, before the SetUpLogs Method has been called
	InitDebugLogs = []string{}

	houstonClient  houston.ClientInterface
	appConfig      *houston.AppConfig
	houstonVersion string

	workspaceID string
	teamID      string
)

// AddCmds adds all the command initialized in this package for the cmd package to import
func AddCmds(client houston.ClientInterface, out io.Writer) []*cobra.Command {
	houstonClient = client

	var err error
	appConfig, err = houston.Call(client.GetAppConfig)(nil)
	if err != nil {
		InitDebugLogs = append(InitDebugLogs, fmt.Sprintf("Error checking feature flag: %s", err.Error()))
	}
	houstonVersion, err = client.GetPlatformVersion(nil)
	if err != nil {
		InitDebugLogs = append(InitDebugLogs, fmt.Sprintf("Unable to get Houston version: %s", err.Error()))
	}

	return []*cobra.Command{
		newDeploymentRootCmd(out),
		newWorkspaceCmd(out),
		NewDeployCmd(),
		newUserCmd(out),
		newTeamCmd(out),
	}
}

// SetUpLogs set the log output and the log level
func SetUpLogs(out io.Writer, level string) error {
	// if level is default means nothing was passed override with config setting
	if level == "warning" {
		level = config.CFG.Verbosity.GetString()
	}
	logger.SetOutput(out)
	lvl, err := logrus.ParseLevel(level)
	if err != nil {
		return err
	}
	logger.SetLevel(lvl)
	return nil
}

func PrintDebugLogs() {
	for _, log := range InitDebugLogs {
		logger.Debug(log)
	}
	// Free-up memory used by init logs
	InitDebugLogs = nil
}
