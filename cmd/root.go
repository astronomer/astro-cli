package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/cmd/registry"
	"github.com/sirupsen/logrus"

	airflowclient "github.com/astronomer/astro-cli/airflow-client"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroiamcore "github.com/astronomer/astro-cli/astro-client-iam-core"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	cloudCmd "github.com/astronomer/astro-cli/cmd/cloud"
	softwareCmd "github.com/astronomer/astro-cli/cmd/software"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/logger"
	"github.com/astronomer/astro-cli/version"

	"github.com/google/go-github/v48/github"
	"github.com/spf13/cobra"
)

var (
	verboseLevel   string
	houstonClient  houston.ClientInterface
	houstonVersion string

	// The commands not to run cloud auth setup for
	excludedCloudAuthSetupCommands = []string{
		"astro config",
		"astro dev",
		"astro run",
	}
)

var RootPersistentPreRunE func(*cobra.Command, []string) error

const (
	softwarePlatform = "Astronomer Software"
	cloudPlatform    = "Astro"
)

// NewRootCmd adds all of the primary commands for the cli
func NewRootCmd() *cobra.Command {
	httpClient := houston.NewHTTPClient()
	houstonClient = houston.NewClient(httpClient)
	airflowClient := airflowclient.NewAirflowClient(httputil.NewHTTPClient())
	astroCoreClient := astrocore.NewCoreClient(httputil.NewHTTPClient())
	astroCoreIamClient := astroiamcore.NewIamCoreClient(httputil.NewHTTPClient())
	platformCoreClient := astroplatformcore.NewPlatformCoreClient(httputil.NewHTTPClient())

	ctx, isCloudCtx := initializeContext()

	createRootPersistentPreRunE(isCloudCtx, platformCoreClient, astroCoreClient)

	rootCmd := createRootCommand()

	addCommands(rootCmd, isCloudCtx, astroCoreClient, platformCoreClient, airflowClient, astroCoreIamClient, os.Stdout)

	rootCmd.SetHelpTemplate(getResourcesHelpTemplate(houstonVersion, ctx))
	rootCmd.PersistentFlags().StringVarP(&verboseLevel, "verbosity", "", logrus.WarnLevel.String(), "Log level (debug, info, warn, error, fatal, panic")

	return rootCmd
}

func initializeContext() (string, bool) {
	isCloudCtx := context.IsCloudContext()
	var ctx string
	if isCloudCtx {
		ctx = cloudPlatform
	} else {
		ctx = softwarePlatform
		var err error
		houstonVersion, err = houstonClient.GetPlatformVersion(nil)
		if err != nil {
			softwareCmd.InitDebugLogs = append(softwareCmd.InitDebugLogs, fmt.Sprintf("Unable to get Houston version: %s", err.Error()))
		}
	}
	return ctx, isCloudCtx
}

func createRootPersistentPreRunE(isCloudCtx bool, platformCoreClient *astroplatformcore.ClientWithResponses, astroCoreClient *astrocore.ClientWithResponses) {
	RootPersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if err := softwareCmd.SetUpLogs(os.Stdout, verboseLevel); err != nil {
			return err
		}
		if config.CFG.UpgradeMessage.GetBool() {
			githubClient := github.NewClient(&http.Client{Timeout: 3 * time.Second})
			err := version.CompareVersions(githubClient, "astronomer", "astro-cli")
			if err != nil {
				softwareCmd.InitDebugLogs = append(softwareCmd.InitDebugLogs, "Error comparing CLI versions: "+err.Error())
			}
		}
		if isCloudCtx {
			if shouldDoCloudAuthSetup(cmd) {
				err := cloudCmd.AuthSetup(cmd, platformCoreClient, astroCoreClient)
				if err != nil {
					if strings.Contains(err.Error(), "token is invalid or malformed") {
						return errors.New("API Token is invalid or malformed")
					}
					if strings.Contains(err.Error(), "the API token given has expired") {
						return errors.New("API Token is expired")
					}
					softwareCmd.InitDebugLogs = append(softwareCmd.InitDebugLogs, "Error during cmd setup: "+err.Error())
				}
			} else {
				logger.Logger.Debug("Skipping cloud auth validation")
			}
		}
		softwareCmd.PrintDebugLogs()
		return nil
	}
}

func createRootCommand() *cobra.Command {
	return &cobra.Command{
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
		PersistentPreRunE: RootPersistentPreRunE,
	}
}

func addCommands(rootCmd *cobra.Command, isCloudCtx bool, astroCoreClient *astrocore.ClientWithResponses, platformCoreClient *astroplatformcore.ClientWithResponses, airflowClient *airflowclient.HTTPClient, astroCoreIamClient *astroiamcore.ClientWithResponses, out *os.File) {
	rootCmd.AddCommand(
		newLoginCommand(astroCoreClient, platformCoreClient, out),
		newLogoutCommand(out),
		newVersionCommand(),
		newDevRootCmd(platformCoreClient, astroCoreClient),
		newContextCmd(out),
		newConfigRootCmd(out),
		newRunCommand(),
	)

	if isCloudCtx {
		rootCmd.AddCommand(
			cloudCmd.AddCmds(platformCoreClient, astroCoreClient, airflowClient, astroCoreIamClient, out)...,
		)
	} else {
		rootCmd.AddCommand(
			softwareCmd.AddCmds(houstonClient, out)...,
		)
		softwareCmd.VersionMatchCmds(rootCmd, []string{"astro"})
	}

	rootCmd.AddCommand(
		registry.AddCmds(out)...,
	)
}

func getResourcesHelpTemplate(houstonVersion, ctx string) string {
	return fmt.Sprintf(`{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}

Current Context: %s{{if and (eq "%s" "Astronomer Software") (ne "%s" "")}}
Platform Version: %s{{end}}

{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}
`, ansi.Bold(ctx), ctx, houstonVersion, ansi.Bold(houstonVersion))
}

func shouldDoCloudAuthSetup(cmd *cobra.Command) bool {
	commandPath := cmd.CommandPath()
	for _, prefix := range excludedCloudAuthSetupCommands {
		if strings.HasPrefix(commandPath, prefix) {
			return false
		}
	}
	return true
}
