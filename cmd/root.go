package cmd

import (
	"io"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/version"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	houstonClient  houston.ClientInterface
	workspaceID    string
	teamID         string
	workspaceRole  string
	deploymentRole string
	role           string
	skipVerCheck   bool
	verboseLevel   string
	// init debug logs should be used only for logs produced during the CLI-initialization, before the SetUpLogs Method has been called
	initDebugLogs = []string{}
)

// NewRootCmd adds all of the primary commands for the cli
func NewRootCmd(client houston.ClientInterface, out io.Writer) *cobra.Command {
	houstonClient = client

	rootCmd := &cobra.Command{
		Use:   "astro",
		Short: "Astronomer - CLI",
		Long:  "astro is a command line interface for working with the Astronomer Platform.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := SetUpLogs(out, verboseLevel); err != nil {
				return err
			}
			printDebugLogs()
			return version.ValidateCompatibility(houstonClient, out, version.CurrVersion, skipVerCheck)
		},
	}

	rootCmd.PersistentFlags().BoolVarP(&skipVerCheck, "skip-version-check", "", false, "skip version compatibility check")
	rootCmd.PersistentFlags().StringVarP(&verboseLevel, "verbosity", "", logrus.WarnLevel.String(), "Log level (debug, info, warn, error, fatal, panic")
	rootCmd.AddCommand(
		newAuthRootCmd(out),
		newWorkspaceCmd(out),
		newVersionCmd(out),
		newUpgradeCheckCmd(out),
		newUserCmd(out),
		newClusterRootCmd(out),
		newDevRootCmd(out),
		newCompletionCmd(out),
		newConfigRootCmd(out),
		newDeploymentRootCmd(out),
		newDeployCmd(),
		newSaRootCmd(out),
		// TODO: remove newAirflowRootCmd, after 1.0 we have only devRootCmd
		newAirflowRootCmd(out),
		newLogsDeprecatedCmd(out),
	)
	return rootCmd
}

// setUpLogs set the log output and the log level
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

func printDebugLogs() {
	for _, log := range initDebugLogs {
		logrus.Debug(log)
	}
	// Free-up memory used by init logs
	initDebugLogs = nil
}
