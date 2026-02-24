package cmd

import (
	"fmt"
	"os"

	airflowclient "github.com/astronomer/astro-cli/airflow-client"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroiamcore "github.com/astronomer/astro-cli/astro-client-iam-core"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	"github.com/astronomer/astro-cli/cmd/api"
	cloudCmd "github.com/astronomer/astro-cli/cmd/cloud"
	"github.com/astronomer/astro-cli/cmd/registry"
	softwareCmd "github.com/astronomer/astro-cli/cmd/software"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/houston"
	nexuscmds "github.com/astronomer/astro-cli/internal/nexus"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/httputil"

	nexusapp "github.com/astronomer/nexus/app"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	verboseLevel   string
	houstonClient  houston.ClientInterface
	houstonVersion string
)

const (
	softwarePlatform = "Astro Private Cloud"
	cloudPlatform    = "Astro"
)

// NewRootCmd adds all of the primary commands for the cli
func NewRootCmd() *cobra.Command {
	var err error
	httpClient := houston.NewHTTPClient()
	houstonClient = houston.NewClient(httpClient)

	airflowClient := airflowclient.NewAirflowClient(httputil.NewHTTPClient())
	astroCoreClient := astrocore.NewCoreClient(httputil.NewHTTPClient())
	astroCoreIamClient := astroiamcore.NewIamCoreClient(httputil.NewHTTPClient())
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

Welcome to the Astro CLI, the modern command line interface for data orchestration. You can use it for Astro, Astro Private Cloud, or Local Development.`,
		PersistentPreRunE: utils.ChainRunEs(
			SetupLogging,
			CreateRootPersistentPreRunE(astroCoreClient, platformCoreClient),
		),
	}

	rootCmd.AddCommand(
		newLoginCommand(astroCoreClient, platformCoreClient, os.Stdout),
		newLogoutCommand(os.Stdout),
		newVersionCommand(),
		newDevRootCmd(platformCoreClient, astroCoreClient),
		newContextCmd(os.Stdout),
		newConfigRootCmd(os.Stdout),
		newRunCommand(),
		api.NewAPICmd(),
	)

	if context.IsCloudContext() { // Include all the commands to be exposed for cloud users
		rootCmd.AddCommand(
			cloudCmd.AddCmds(platformCoreClient, astroCoreClient, airflowClient, astroCoreIamClient, os.Stdout)...,
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

	nexusCtx, nexusErr := nexusapp.NewContext()
	if nexusErr == nil {
		nexuscmds.RegisterCommands(rootCmd, nexusCtx)
	}

	rootCmd.SetHelpTemplate(getResourcesHelpTemplate(houstonVersion, ctx))
	rootCmd.PersistentFlags().StringVarP(&verboseLevel, "verbosity", "", logrus.WarnLevel.String(), "Log level (debug, info, warn, error, fatal, panic")

	return rootCmd
}

func getResourcesHelpTemplate(houstonVersion, ctx string) string {
	return fmt.Sprintf(`{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}

Current Context: %s{{if and (eq "%s" "Astro Private Cloud") (ne "%s" "")}}
Platform Version: %s{{end}}

{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}
`, ansi.Bold(ctx), ctx, houstonVersion, ansi.Bold(houstonVersion))
}
