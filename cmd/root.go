package cmd

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	airflowclient "github.com/astronomer/astro-cli/airflow-client"
	"github.com/astronomer/astro-cli/astro-client-v1"
	astrov1alpha1 "github.com/astronomer/astro-cli/astro-client-v1alpha1"
	"github.com/astronomer/astro-cli/cmd/api"
	cloudCmd "github.com/astronomer/astro-cli/cmd/cloud"
	softwareCmd "github.com/astronomer/astro-cli/cmd/software"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/internal/telemetry"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/credentials"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/keychain"
	"github.com/astronomer/astro-cli/pkg/util"
)

var (
	verboseLevel   string
	houstonClient  houston.ClientInterface
	houstonVersion string
	newSecureStore = keychain.New
)

const (
	softwarePlatform = "Astro Private Cloud"
	cloudPlatform    = "Astro"

	// noInsecureFallbackEnv refuses the plaintext credential fallback for a
	// single invocation. The secure store is constructed before cobra parses
	// flags, so this cannot be a flag; an env var also suits the CI and
	// container settings where the fallback actually fires, and matches how
	// ASTRO_API_TOKEN and ASTRO_DOMAIN are already read.
	noInsecureFallbackEnv = "ASTRO_NO_INSECURE_FALLBACK"
)

// allowInsecureCredentialFallback reports whether credentials may be written to
// a plaintext file when no OS-native secure store is available. It defaults to
// true, preserving the previous config.yaml posture; users who would rather fail
// than store secrets in the clear opt out via
// `astro config set -g no_insecure_fallback true` or ASTRO_NO_INSECURE_FALLBACK.
//
// Either source can switch the protection on and neither can switch it off,
// matching how SkipParse and AutoSelect combine their config and env sources.
// That direction matters here: CheckEnvBool reports false for anything outside
// true/1/yes/y/on, so letting the env win outright would mean a typo'd
// ASTRO_NO_INSECURE_FALLBACK quietly overrode a persisted opt-out and allowed
// the plaintext write it was set to prevent.
func allowInsecureCredentialFallback() bool {
	return !config.CFG.NoInsecureFallback.GetBool() && !util.CheckEnvBool(os.Getenv(noInsecureFallbackEnv))
}

// NewRootCmd adds all of the primary commands for the cli
func NewRootCmd() *cobra.Command {
	var err error
	creds := &credentials.CurrentCredentials{}
	// Keep the plaintext fallback's file next to config.yaml. pkg/keychain
	// can't resolve this itself: the path honors ASTRO_HOME and is owned by
	// package config, which imports pkg/keychain.
	keychain.SetCredentialsDir(config.HomeConfigPath)
	store, storeErr := newSecureStore(allowInsecureCredentialFallback())

	httpClient := houston.NewHTTPClient()
	houstonClient = houston.NewClient(httpClient, creds)

	airflowClient := airflowclient.NewAirflowClient(httputil.NewHTTPClient(), creds)
	astroV1Client := astrov1.NewV1Client(httputil.NewHTTPClient(), creds)
	v1Alpha1Client := astrov1alpha1.NewV1Alpha1Client(httputil.NewHTTPClient(), creds)

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
				CreateRootPersistentPreRunE(storeErr, store, creds, astroV1Client),
				telemetry.CreateTrackingHook(),
			)(cmd, args)
		},
	}

	rootCmd.AddCommand(
		newLoginCommand(store, creds, astroV1Client, os.Stdout),
		newLogoutCommand(store, os.Stdout),
		newAuthRootCmd(store, creds, astroV1Client, os.Stdout),
		newVersionCommand(),
		newDevRootCmd(astroV1Client, store, creds),
		newContextCmd(os.Stdout),
		newConfigRootCmd(os.Stdout),
		newRunCommand(),
		api.NewAPICmd(creds),
		newTelemetryCmd(os.Stdout),
		newTelemetrySendCmd(),
		newOttoCmd(creds),
	)

	if context.IsCloudContext() { // Include all the commands to be exposed for cloud users
		rootCmd.AddCommand(
			cloudCmd.AddCmds(astroV1Client, airflowClient, v1Alpha1Client, creds, os.Stdout)...,
		)
	} else { // Include all the commands to be exposed for software users
		rootCmd.AddCommand(
			softwareCmd.AddCmds(houstonClient, store, os.Stdout)...,
		)
		softwareCmd.VersionMatchCmds(rootCmd, []string{"astro"})
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
