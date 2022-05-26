package software

import (
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/houston"

	"github.com/astronomer/astro-cli/config"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	// init debug logs should be used only for logs produced during the CLI-initialization, before the SetUpLogs Method has been called
	initDebugLogs = []string{}
	houstonClient houston.ClientInterface
	workspaceID   string
	appConfig     *houston.AppConfig
)

// AddCmds adds all the command initialized in this package for the cmd package to import
func AddCmds(client houston.ClientInterface, out io.Writer) []*cobra.Command {
	houstonClient = client

	var err error
	appConfig, err = houstonClient.GetAppConfig()
	if err != nil {
		initDebugLogs = append(initDebugLogs, fmt.Sprintf("Error checking feature flag: %s", err.Error()))
	}

	return []*cobra.Command{
		newDeploymentRootCmd(out),
		newWorkspaceCmd(out),
		newDeployCmd(),
		newUserCmd(out),
	}
}

// SetUpLogs set the log output and the log level
func SetUpLogs(out io.Writer, level string) error {
	// if level is default means nothing was passed override with config setting
	if level == "warning" {
		level = config.CFG.Verbosity.GetString()
	}
	logrus.SetOutput(out)
	lvl, err := logrus.ParseLevel(level)
	if err != nil {
		return err
	}
	logrus.SetLevel(lvl)
	return nil
}

func PrintDebugLogs() {
	for _, log := range initDebugLogs {
		logrus.Debug(log)
	}
	// Free-up memory used by init logs
	initDebugLogs = nil
}
