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
	softwareCmd "github.com/astronomer/astro-cli/cmd/software"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/houston"
	nexuscmds "github.com/astronomer/astro-cli/internal/nexus"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/telemetry"

	nexusapp "github.com/astronomer/nexus/app"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Skip heavy pre-run logic for commands that opt out via annotation
			if cmd.Annotations[telemetry.SkipPreRunAnnotation] == "true" {
				return nil
			}
			return utils.ChainRunEs(
				SetupLogging,
				CreateRootPersistentPreRunE(astroCoreClient, platformCoreClient),
				telemetry.CreateTrackingHook(),
			)(cmd, args)
		},
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
		newTelemetryCmd(os.Stdout),
		newTelemetrySendCmd(),
	)

	if context.IsCloudContext() {
		// Register nexus API commands first (noun-verb format),
		// then merge cloud commands so they fill gaps nexus doesn't cover.
		nexusCtx, nexusErr := nexusapp.NewContextWithConfigProvider(&nexuscmds.ConfigAdapter{})
		if nexusErr == nil {
			nexuscmds.RegisterNounVerb(rootCmd, nexusCtx)
		}
		mergeCloudCommands(rootCmd,
			cloudCmd.AddCmds(platformCoreClient, astroCoreClient, airflowClient, astroCoreIamClient, os.Stdout),
		)
	} else {
		rootCmd.AddCommand(
			softwareCmd.AddCmds(houstonClient, os.Stdout)...,
		)
		softwareCmd.VersionMatchCmds(rootCmd, []string{"astro"})
	}

	rootCmd.SetHelpTemplate(getResourcesHelpTemplate(houstonVersion, ctx))
	rootCmd.PersistentFlags().StringVarP(&verboseLevel, "verbosity", "", logrus.WarnLevel.String(), "Log level (debug, info, warn, error, fatal, panic")

	return rootCmd
}

// mergeCloudCommands adds cloud commands to the root, filling gaps that nexus
// didn't cover. For nouns that nexus already registered (e.g. "deployment"),
// cloud's subcommands and metadata are merged onto the existing parent.
// If the cloud command is directly runnable (has RunE) and the nexus noun is
// only a group, the RunE and associated properties are preserved so the
// command remains executable (e.g. "astro deploy <id>" still works alongside
// "astro deploy create/list/...").
func mergeCloudCommands(rootCmd *cobra.Command, cloudCmds []*cobra.Command) {
	for _, cc := range cloudCmds {
		existing := nexuscmds.FindSubcommand(rootCmd, cc.Name())
		if existing == nil {
			rootCmd.AddCommand(cc)
			continue
		}
		existing.PersistentFlags().AddFlagSet(cc.PersistentFlags())
		if len(existing.Aliases) == 0 {
			existing.Aliases = cc.Aliases
		}
		if existing.Long == "" {
			existing.Long = cc.Long
		}
		if existing.Short == "" {
			existing.Short = cc.Short
		}
		if cc.RunE != nil && existing.RunE == nil && existing.Run == nil {
			existing.RunE = cc.RunE
			existing.Args = cc.Args
			existing.PreRunE = cc.PreRunE
			existing.Example = cc.Example
			existing.Use = cc.Use
			cc.Flags().VisitAll(func(f *pflag.Flag) {
				if existing.Flags().Lookup(f.Name) == nil {
					existing.Flags().AddFlag(f)
				}
			})
		}
		for _, sub := range cc.Commands() {
			if nexuscmds.FindSubcommand(existing, sub.Name()) == nil {
				existing.AddCommand(sub)
			}
		}
	}
}

func getResourcesHelpTemplate(houstonVersion, ctx string) string {
	return fmt.Sprintf(`{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}

Current Context: %s{{if and (eq "%s" "Astro Private Cloud") (ne "%s" "")}}
Platform Version: %s{{end}}

{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}
`, ansi.Bold(ctx), ctx, houstonVersion, ansi.Bold(houstonVersion))
}
