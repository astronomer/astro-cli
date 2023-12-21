package cmd

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/astronomer/astro-cli/cmd/registry"

	airflowclient "github.com/astronomer/astro-cli/airflow-client"
	astro "github.com/astronomer/astro-cli/astro-client"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	cloudCmd "github.com/astronomer/astro-cli/cmd/cloud"
	softwareCmd "github.com/astronomer/astro-cli/cmd/software"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/version"

	"github.com/google/go-github/v48/github"
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
	var err error
	httpClient := houston.NewHTTPClient()
	houstonClient = houston.NewClient(httpClient)

	airflowClient := airflowclient.NewAirflowClient(httputil.NewHTTPClient())
	astroClient := astro.NewAstroClient(httputil.NewHTTPClient())
	astroCoreClient := astrocore.NewCoreClient(httputil.NewHTTPClient())
	platformCoreClient := astroplatformcore.NewPlatformCoreClient(httputil.NewHTTPClient())

	ctx := cloudPlatform
	isCloudCtx := context.IsCloudContext()
	if !isCloudCtx {
		ctx = softwarePlatform
		houstonVersion, err = houstonClient.GetPlatformVersion(nil)
		if err != nil {
			softwareCmd.InitDebugLogs = append(softwareCmd.InitDebugLogs, fmt.Sprintf("Unable to get Houston version: %s", err.Error()))
		}
	}

	rootCmd := &cobra.Command{
		Use:   "astro",
		Short: "Run Apache Airflow locally and interact with Astronomer",
		Long: `
 ________   ______   _________  ______    ______             ______   __        ________
/_______/\ /_____/\ /________/\/_____/\  /_____/\           /_____/\ /_/\      /_______/\
\::: _  \ \\::::_\/_\__.::.__\/\:::_ \ \ \:::_ \ \   _______\:::__\/ \:\ \     \__.::._\/
 \::(_)  \ \\:\/___/\  \::\ \   \:(_) ) )_\:\ \ \ \ /______/\\:\ \  __\:\ \       \::\ \
  \:: __  \ \\_::._\:\  \::\ \   \: __ '\ \\:\ \ \ \\__::::\/ \:\ \/_/\\:\ \____  _\::\ \__
   \:.\ \  \ \ /____\:\  \::\ \   \ \ '\ \ \\:\_\ \ \          \:\_\ \ \\:\/___/\/__\::\__/\
    \__\/\__\/ \_____\/   \__\/    \_\/ \_\/ \_____\/           \_____\/ \_____\/\________\/

Welcome to the Astro CLI, the modern command line interface for data orchestration. You can use it for Astro, Astronomer Software, or Local Development.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Check for latest version
			if config.CFG.UpgradeMessage.GetBool() {
				// create github client with 3 second timeout, setting an aggressive timeout since its not mandatory to get a response in each command execution
				githubClient := github.NewClient(&http.Client{Timeout: 3 * time.Second})
				// compare current version to latest
				err = version.CompareVersions(githubClient, "astronomer", "astro-cli")
				if err != nil {
					softwareCmd.InitDebugLogs = append(softwareCmd.InitDebugLogs, "Error comparing CLI versions: "+err.Error())
				}
			}
			if isCloudCtx {
				err = cloudCmd.Setup(cmd, astroClient, platformCoreClient, astroCoreClient)
				if err != nil {
					softwareCmd.InitDebugLogs = append(softwareCmd.InitDebugLogs, "Error during cmd setup: "+err.Error())
				}
			}
			// common PersistentPreRunE component between software & cloud
			// setting up log verbosity and dumping debug logs collected during CLI-initialization
			if err := softwareCmd.SetUpLogs(os.Stdout, verboseLevel); err != nil {
				return err
			}
			softwareCmd.PrintDebugLogs()
			return nil
		},
	}

	rootCmd.AddCommand(
		newLoginCommand(astroClient, astroCoreClient, platformCoreClient, os.Stdout),
		newLogoutCommand(os.Stdout),
		newVersionCommand(),
		newDevRootCmd(platformCoreClient, astroCoreClient),
		newContextCmd(os.Stdout),
		newConfigRootCmd(os.Stdout),
		newRunCommand(),
	)

	if context.IsCloudContext() { // Include all the commands to be exposed for cloud users
		rootCmd.AddCommand(
			cloudCmd.AddCmds(astroClient, platformCoreClient, astroCoreClient, airflowClient, os.Stdout)...,
		)
	} else { // Include all the commands to be exposed for software users
		rootCmd.AddCommand(
			softwareCmd.AddCmds(houstonClient, os.Stdout)...,
		)
		softwareCmd.VersionMatchCmds(rootCmd, []string{"astro"})
	}

	rootCmd.AddCommand( // include all the commands for interacting with the registry
		registry.AddCmds(os.Stdout)...,
	)

	rootCmd.SetHelpTemplate(getResourcesHelpTemplate(houstonVersion, ctx))
	rootCmd.PersistentFlags().StringVarP(&verboseLevel, "verbosity", "", logrus.WarnLevel.String(), "Log level (debug, info, warn, error, fatal, panic")

	return rootCmd
}

func getResourcesHelpTemplate(houstonVersion, ctx string) string {
	return fmt.Sprintf(`{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}

Current Context: %s{{if and (eq "%s" "Astronomer Software") (ne "%s" "")}}
Platform Version: %s{{end}}

{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}
`, ansi.Bold(ctx), ctx, houstonVersion, ansi.Bold(houstonVersion))
}
