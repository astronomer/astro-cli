package cmd

import (
	"fmt"
	"os"

	astro "github.com/astronomer/astro-cli/astro-client"
	cloudCmd "github.com/astronomer/astro-cli/cmd/cloud"
	softwareCmd "github.com/astronomer/astro-cli/cmd/software"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	verboseLevel   string
	houstonClient  houston.ClientInterface
	houstonVersion string
)

const (
	softwarePlatform = "Astronomer Software"
	cloudPlatform    = "Astro"
)

// NewRootCmd adds all of the primary commands for the cli
func NewRootCmd() *cobra.Command {
	httpClient := houston.NewHTTPClient()
	houstonClient = houston.NewClient(httpClient)
	houstonVersion, err := houstonClient.GetPlatformVersion(nil)
	if err != nil {
		logrus.Debugf("Unable to get Houston version: %s", err.Error())
	}

	astroClient := astro.NewAstroClient(httputil.NewHTTPClient())

	ctx := cloudPlatform
	currCtx := context.IsCloudContext()
	if !currCtx {
		ctx = softwarePlatform
	}

	rootCmd := &cobra.Command{
		Use:   "astro",
		Short: "Run Apache Airflow locally and interact with Astronomer",
		Long:  "Welcome to the Astro CLI. Astro is the modern command line interface for data orchestration. You can use it for Astro, Astronomer Software, or local development.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if currCtx {
				return cloudCmd.Setup(cmd, args, astroClient)
			}
			// Software PersistentPreRunE component
			// setting up log verbosity and dumping debug logs collected during CLI-initialization
			if err := softwareCmd.SetUpLogs(os.Stdout, verboseLevel); err != nil {
				return err
			}
			softwareCmd.PrintDebugLogs()
			return nil
		},
	}

	rootCmd.AddCommand(
		newLoginCommand(astroClient, os.Stdout),
		newLogoutCommand(os.Stdout),
		newVersionCommand(),
		newDevRootCmd(),
		newContextCmd(os.Stdout),
		newConfigRootCmd(os.Stdout),
		newAuthCommand(),
	)

	if context.IsCloudContext() { // Include all the commands to be exposed for cloud users
		rootCmd.AddCommand(
			cloudCmd.AddCmds(astroClient, os.Stdout)...,
		)
	} else { // Include all the commands to be exposed for software users
		rootCmd.AddCommand(
			softwareCmd.AddCmds(houstonClient, os.Stdout)...,
		)
		softwareCmd.VersionMatchCmds(rootCmd, []string{"astro"})
		rootCmd.PersistentFlags().StringVarP(&verboseLevel, "verbosity", "", logrus.WarnLevel.String(), "Log level (debug, info, warn, error, fatal, panic")
	}

	rootCmd.SetHelpTemplate(getResourcesHelpTemplate(houstonVersion, ctx))

	return rootCmd
}

func getResourcesHelpTemplate(version, ctx string) string {
	return fmt.Sprintf(`{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}

Current Context: %s{{if and (eq "%s" "Astronomer Software") (ne "%s" "")}}
Platform Version: %s{{end}}

{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}
`, ansi.Bold(ctx), ctx, version, ansi.Bold(version))
}
