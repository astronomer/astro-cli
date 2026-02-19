package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/airflow/runtimes"
	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	"github.com/astronomer/astro-cli/cloud/environment"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/util"
	"github.com/iancoleman/strcase"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	useAstronomerCertified bool
	remoteExecutionEnabled bool
	remoteImageRepository  string
	projectName            string
	runtimeVersion         string
	airflowVersion         string
	fromTemplate           string
	envFile                string
	customImageName        string
	settingsFile           string
	composeFile            string
	exportComposeFile      string
	pytestArgs             string
	pytestFile             string
	workspaceID            string
	deploymentID           string
	buildSecretString      string
	followLogs             bool
	schedulerLogs          bool
	webserverLogs          bool
	apiServerLogs          bool
	triggererLogs          bool
	dagProcessorLogs       bool
	noCache                bool
	schedulerExec          bool
	postgresExec           bool
	webserverExec          bool
	triggererExec          bool
	apiServerExec          bool
	dagProcessorExec       bool
	connections            bool
	variables              bool
	pools                  bool
	envExport              bool
	noBrowser              bool
	compose                bool
	versionTest            bool
	dagTest                bool
	lintTest               bool
	lintDeprecations       bool
	lintFix                bool
	lintConfigFile         string
	waitTime               time.Duration
	forceKill              bool
	containerRuntime       runtimes.ContainerRuntime
	RunExample             = `
# Create default admin user.
astro dev run users create -r Admin -u admin -e admin@example.com -f admin -l user -p admin
`
	initSoftwareExample = `
# Initialize a new Astro project with the latest version of Astro Runtime
astro dev init

# Initialize a new Astro project with Astro Runtime 4.1.0
astro dev init --runtime-version 4.1.0

# Initialize a new Astro project with the latest Astro Runtime version based on Airflow 2.2.3
astro dev init --airflow-version 2.2.3

# Initialize a new Astro project with the latest version of Astronomer Certified. Use this only if you run on Astro Private Cloud
astro dev init --use-astronomer-certified

# Initialize a new Astro project with the latest version of Astronomer Certified based on Airflow 2.2.3
astro dev init --use-astronomer-certified --airflow-version 2.2.3
`

	initCloudExample = `
# Initialize a new Astro project with the latest version of Astro Runtime
astro dev init

# Initialize a new Astro project with Astro Runtime 4.1.0
astro dev init --runtime-version 4.1.0

# Initialize a new Astro project with the latest Astro Runtime version based on Airflow 2.2.3
astro dev init --airflow-version 2.2.3

# Initialize a new template based Astro project with the latest Astro Runtime version
astro dev init --from-template

# Initialize a new Astro project with remote execution support
astro dev init --remote-execution-enabled

# Initialize a new Astro project with remote execution support and specify the remote image repository
astro dev init --remote-execution-enabled --remote-image-repository quay.io/acme/my-deployment-image
`
	dockerfile = "Dockerfile"

	configReinitProjectConfigMsg = "Reinitialized existing Astro project in %s\n"
	configInitProjectConfigMsg   = "Initialized empty Astro project in %s\n"

	// this is used to monkey patch the function in order to write unit test cases
	containerHandlerInit = airflow.ContainerHandlerInit
	localHandlerInit     = airflow.StandaloneHandlerInit
	getDefaultImageTag   = airflowversions.GetDefaultImageTag
	projectNameUnique    = airflow.ProjectNameUnique

	pytestDir = "/tests"

	errPytestArgs               = errors.New("you can only pass one pytest file or directory")
	buildSecrets                = []string{}
	errNoCompose                = errors.New("cannot use '--compose-file' without '--compose' flag")
	TemplateList                = airflow.FetchTemplateList
	defaultWaitTime             = 1 * time.Minute
	directoryPermissions uint32 = 0o755
	localForeground      bool
)

func newDevRootCmd(platformCoreClient astroplatformcore.CoreClient, astroCoreClient astrocore.CoreClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "dev",
		Aliases: []string{"d"},
		Short:   "Run your Astro project in a local Airflow environment",
		Long:    "Run an Apache Airflow environment on your local machine to test your project, including DAGs, Python Packages, and plugins.",
		// Most astro dev sub-commands require the container runtime,
		// so we set that configuration in this persistent pre-run hook.
		// A few sub-commands don't require this, so they explicitly
		// clobber it with a no-op function.
		PersistentPreRunE: utils.ChainRunEs(
			SetupLogging,
			ConfigureContainerRuntime,
		),
	}
	cmd.AddCommand(
		newAirflowInitCmd(),
		newAirflowStartCmd(astroCoreClient),
		newAirflowBuildCmd(),
		newAirflowRunCmd(),
		newAirflowPSCmd(),
		newAirflowLogsCmd(),
		newAirflowStopCmd(),
		newAirflowKillCmd(),
		newAirflowPytestCmd(),
		newAirflowParseCmd(),
		newAirflowRestartCmd(astroCoreClient),
		newAirflowBashCmd(),
		newAirflowObjectRootCmd(),
		newAirflowUpgradeTestCmd(platformCoreClient),
		newAirflowLocalCmd(astroCoreClient),
	)
	return cmd
}

func newAirflowInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "init",
		Short:   "Create a new Astro project in your working directory",
		Long:    "Create a new Astro project in your working directory. This generates the files you need to start an Airflow environment on your local machine and deploy your project to a Deployment on Astro or Astro Private Cloud.",
		Example: initCloudExample,
		Args:    cobra.MaximumNArgs(1),
		RunE:    airflowInit,
		// Override the root PersistentPreRunE to prevent looking up for container runtime.
		PersistentPreRunE: SetupLogging,
	}
	cmd.Flags().StringVarP(&projectName, "name", "n", "", "Name of Astro project")
	cmd.Flags().StringVarP(&airflowVersion, "airflow-version", "a", "", "Version of Airflow you want to create an Astro project with. If not specified, latest is assumed. You can change this version in your Dockerfile at any time.")
	cmd.Flags().StringVarP(&fromTemplate, "from-template", "t", "", "Provides a list of templates to select from and create the local astro project based on the selected template. Please note template based astro projects use the latest runtime version, so runtime-version and airflow-version flags will be ignored when creating a project with template flag")
	cmd.Flag("from-template").NoOptDefVal = "select-template"

	var avoidACFlag bool

	// In case user is connected to Astronomer Platform and is connected to older version of platform
	if context.IsCloudContext() || houstonVersion == "" || houston.VerifyVersionMatch(houstonVersion, houston.VersionRestrictions{GTE: "0.29.0"}) {
		cmd.Flags().StringVarP(&runtimeVersion, "runtime-version", "v", "", "Specify a version of Astro Runtime that you want to create an Astro project with. If not specified, the latest is assumed. You can change this version in your Dockerfile at any time.")
	} else { // default to using AC flag, since runtime is not available for these cases
		useAstronomerCertified = true
		avoidACFlag = true
	}

	if context.IsCloudContext() {
		cmd.Flags().BoolVarP(&remoteExecutionEnabled, "remote-execution-enabled", "", false, "Enable remote execution support for the Astro project. This will generate additional client files that would be useful to maintain agents in remote deployment.")
		cmd.Flags().StringVarP(&remoteImageRepository, "remote-image-repository", "", "", "Remote Docker repository for the client image (e.g. quay.io/acme/my-deployment-image). This flag is only used when --remote-execution-enabled is set.")
	}

	if !context.IsCloudContext() && !avoidACFlag {
		cmd.Example = initSoftwareExample
		cmd.Flags().BoolVarP(&useAstronomerCertified, "use-astronomer-certified", "", false, "If specified, initializes a project using Astronomer Certified Airflow image instead of Astro Runtime.")
	}

	if _, err := context.GetCurrentContext(); err != nil && !avoidACFlag { // Case when user is not logged in to any platform
		cmd.Flags().BoolVarP(&useAstronomerCertified, "use-astronomer-certified", "", false, "If specified, initializes a project using Astronomer Certified Airflow image instead of Astro Runtime.")
		_ = cmd.Flags().MarkHidden("use-astronomer-certified")
	}
	return cmd
}

func newAirflowUpgradeTestCmd(platformCoreClient astroplatformcore.CoreClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "upgrade-test",
		Short:   "Run tests to see if your environment and DAGs are compatible with a new version of Airflow or Astro Runtime. This test will produce a series of reports where you can see the test results.",
		Long:    "Run tests to see if your environment and DAGs are compatible with a new version of Airflow or Astro Runtime. This test will produce a series of reports where you can see the test results.",
		PreRunE: EnsureRuntime,
		RunE: func(cmd *cobra.Command, args []string) error {
			return airflowUpgradeTest(cmd, platformCoreClient)
		},
	}
	cmd.Flags().StringVarP(&airflowVersion, "airflow-version", "a", "", "The version of Airflow you want to upgrade to. The default is the latest available version. Tests are run against the equivalent Astro Runtime version.")
	cmd.Flags().BoolVarP(&versionTest, "version-test", "", false, "Only run version tests. These tests show you how the versions of your dependencies will change after you upgrade.")
	cmd.Flags().BoolVarP(&dagTest, "dag-test", "d", false, "Only run DAG tests. These tests check whether your DAGs will generate import errors after you upgrade.")
	cmd.Flags().BoolVarP(&lintTest, "lint-test", "l", false, "Only run ruff lint tests. These tests check whether your DAGs are compatible with Airflow.")
	cmd.Flags().BoolVarP(&lintDeprecations, "lint-deprecations", "", false, "Include Airflow deprecations in lint tests.")
	cmd.Flags().BoolVarP(&lintFix, "fix", "", false, "Automatically apply lint fixes where possible.")
	cmd.Flags().StringVarP(&lintConfigFile, "lint-config-file", "", "", "Relative path within project to a custom ruff config file. If not specified, a default config will be used.")
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "i", "", "ID of the Deployment you want run dependency tests against.")
	cmd.Flags().StringVarP(&customImageName, "image-name", "n", "", "Name of the upgraded image. Updates the FROM line in your Dockerfile to pull this image for the upgrade.")
	cmd.Flags().StringSliceVar(&buildSecrets, "build-secrets", []string{}, "Expose a secret to containers. Equivalent to 'docker build --secret'. Example input id=mysecret,src=secrets.txt")
	var err error
	var avoidACFlag bool

	// In case user is connected to Astronomer Platform and is connected to older version of platform
	if context.IsCloudContext() || houstonVersion == "" || (!context.IsCloudContext() && houston.VerifyVersionMatch(houstonVersion, houston.VersionRestrictions{GTE: "0.29.0"})) {
		cmd.Flags().StringVarP(&runtimeVersion, "runtime-version", "v", "", "The version of Astro Runtime you want to upgrade to. The default is the latest available version.")
	} else { // default to using AC flag, since runtime is not available for these cases
		useAstronomerCertified = true
		avoidACFlag = true
	}

	if !context.IsCloudContext() && !avoidACFlag {
		cmd.Flags().BoolVarP(&useAstronomerCertified, "use-astronomer-certified", "", false, "Use an Astronomer Certified image instead of Astro Runtime. Use the airflow-version flag to specify your AC version.")
	}

	_, err = context.GetCurrentContext()
	if err != nil && !avoidACFlag { // Case when user is not logged in to any platform
		cmd.Flags().BoolVarP(&useAstronomerCertified, "use-astronomer-certified", "", false, "Use an Astronomer Certified image instead of Astro Runtime. Use the airflow-version flag to specify your AC version.")
		_ = cmd.Flags().MarkHidden("use-astronomer-certified")
	}

	return cmd
}

func newAirflowStartCmd(astroCoreClient astrocore.CoreClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "start",
		Short:   "Start a local Airflow environment",
		Long:    "Start a local Airflow environment. This command will spin up 4 Docker containers on your machine, each for a different Airflow component: Webserver, scheduler, triggerer and metadata database.",
		Args:    cobra.MaximumNArgs(1),
		PreRunE: EnsureRuntime,
		RunE: func(cmd *cobra.Command, args []string) error {
			return airflowStart(cmd, args, astroCoreClient)
		},
	}
	cmd.Flags().StringVarP(&envFile, "env", "e", ".env", "Location of file containing environment variables")
	cmd.Flags().BoolVarP(&noCache, "no-cache", "", false, "Do not use cache when building container image")
	cmd.Flags().StringVarP(&customImageName, "image-name", "i", "", "Name of a custom built image to start airflow with")
	cmd.Flags().StringVarP(&settingsFile, "settings-file", "s", "airflow_settings.yaml", "Settings file from which to import airflow objects")
	cmd.Flags().BoolVarP(&noBrowser, "no-browser", "n", false, "Don't bring up the browser once the Webserver is healthy")
	cmd.Flags().DurationVar(&waitTime, "wait", defaultWaitTime, "Duration to wait for webserver to get healthy. The default is 5 minutes. Use --wait 2m to wait for 2 minutes.")
	cmd.Flags().StringVarP(&composeFile, "compose-file", "", "", "Location of a custom compose file to use for starting Airflow")
	cmd.Flags().StringSliceVar(&buildSecrets, "build-secrets", []string{}, "Mimics docker build --secret flag. See https://docs.docker.com/build/building/secrets/ for more information. Example input id=mysecret,src=secrets.txt")
	cmd.Flags().StringVarP(&workspaceID, "workspace-id", "w", "", "ID of the Workspace to retrieve environment connections from. If not specified uses the current Workspace.")
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "ID of the Deployment to retrieve environment connections from")

	return cmd
}

func newAirflowPSCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ps",
		Short:   "List locally running Airflow containers",
		Long:    "List locally running Airflow containers",
		PreRunE: SetRuntimeIfExists,
		RunE:    airflowPS,
	}
	return cmd
}

func newAirflowRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "run",
		Short:              "Run Airflow CLI commands within your local Airflow environment",
		Long:               "Run Airflow CLI commands within your local Airflow environment. These commands run in the webserver container but can interact with your local scheduler, workers, and metadata database.",
		PreRunE:            EnsureRuntime,
		RunE:               airflowRun,
		Example:            RunExample,
		DisableFlagParsing: true,
	}
	return cmd
}

func newAirflowLogsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "logs",
		Short:   "Display component logs for your local Airflow environment",
		Long:    "Display scheduler, worker, api-server, and webserver logs for your local Airflow environment",
		PreRunE: SetRuntimeIfExists,
		RunE:    airflowLogs,
	}
	cmd.Flags().BoolVarP(&followLogs, "follow", "f", false, "Follow log output")
	cmd.Flags().BoolVarP(&schedulerLogs, "scheduler", "s", false, "Output scheduler logs")
	cmd.Flags().BoolVarP(&webserverLogs, "webserver", "w", false, "Output webserver logs")
	cmd.Flags().BoolVarP(&apiServerLogs, "api-server", "a", false, "Output api-server logs")
	cmd.Flags().BoolVarP(&triggererLogs, "triggerer", "t", false, "Output triggerer logs")
	cmd.Flags().BoolVarP(&dagProcessorLogs, "dag-processor", "d", false, "Output dag-processor logs")
	return cmd
}

func newAirflowStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "stop",
		Short:   "Stop all locally running Airflow containers",
		Long:    "Stop all Airflow containers running on your local machine. This command preserves container data.",
		PreRunE: SetRuntimeIfExists,
		RunE:    airflowStop,
	}
	return cmd
}

func newAirflowKillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:      "kill",
		Short:    "Kill all locally running Airflow containers",
		Long:     "Kill all Airflow containers running on your local machine. This command permanently deletes all container data.",
		PreRunE:  KillPreRunHook,
		RunE:     airflowKill,
		PostRunE: KillPostRunHook,
	}
	return cmd
}

func newAirflowLocalCmd(astroCoreClient astrocore.CoreClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "local",
		Short: "Run Airflow locally without Docker",
		Long:  "Run Airflow locally without Docker using 'airflow standalone'. Requires 'uv' to be installed. By default the process is backgrounded; use --foreground to stream output in the terminal.",
		// Override PersistentPreRunE so we don't require a container runtime.
		PersistentPreRunE: SetupLogging,
		PreRunE:           EnsureLocalRuntime,
		RunE: func(cmd *cobra.Command, args []string) error {
			return airflowLocal(cmd, astroCoreClient)
		},
	}
	cmd.Flags().StringVarP(&envFile, "env", "e", ".env", "Location of file containing environment variables")
	cmd.Flags().StringVarP(&settingsFile, "settings-file", "s", "airflow_settings.yaml", "Settings file from which to import airflow objects")
	cmd.Flags().DurationVar(&waitTime, "wait", defaultWaitTime, "Duration to wait for the API server to become healthy")
	cmd.Flags().StringVarP(&workspaceID, "workspace-id", "w", "", "ID of the Workspace to retrieve environment connections from")
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "ID of the Deployment to retrieve environment connections from")
	cmd.Flags().BoolVarP(&localForeground, "foreground", "f", false, "Run in the foreground instead of backgrounding the process")

	cmd.AddCommand(
		newAirflowLocalResetCmd(),
		newAirflowLocalStopCmd(),
		newAirflowLocalLogsCmd(),
	)

	return cmd
}

func newAirflowLocalResetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "reset",
		Short:   "Reset the local environment",
		Long:    "Reset the local environment by removing all generated files (.venv, cached constraints, airflow.db, logs). The next run of 'astro dev local' will start fresh.",
		PreRunE: EnsureLocalRuntime,
		RunE:    airflowLocalReset,
	}
	return cmd
}

func newAirflowLocalStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "stop",
		Short:   "Stop the local Airflow process",
		PreRunE: EnsureLocalRuntime,
		RunE:    airflowLocalStop,
	}
	return cmd
}

func newAirflowLocalLogsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "logs",
		Short:   "View local Airflow logs",
		PreRunE: EnsureLocalRuntime,
		RunE:    airflowLocalLogs,
	}
	cmd.Flags().BoolVarP(&followLogs, "follow", "f", false, "Follow log output")
	return cmd
}

func newAirflowRestartCmd(astroCoreClient astrocore.CoreClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "restart",
		Short:   "Restart all locally running Airflow containers",
		Long:    "Restart all Airflow containers running on your local machine. This command stops and then starts locally running containers to apply changes to your local environment.",
		PreRunE: SetRuntimeIfExists,
		RunE: func(cmd *cobra.Command, args []string) error {
			return airflowRestart(cmd, args, astroCoreClient)
		},
	}
	cmd.Flags().DurationVar(&waitTime, "wait", defaultWaitTime, "Duration to wait for webserver to get healthy. The default is 5 minutes. Use --wait 2m to wait for 2 minutes.")
	cmd.Flags().StringVarP(&envFile, "env", "e", ".env", "Location of file containing environment variables")
	cmd.Flags().BoolVarP(&noCache, "no-cache", "", false, "Do not use cache when building container image")
	cmd.Flags().BoolVarP(&forceKill, "kill", "k", false, "Kill all running containers and remove all data before restarting. This permanently deletes all container data.")
	cmd.Flags().StringVarP(&customImageName, "image-name", "i", "", "Name of a custom built image to restart airflow with")
	cmd.Flags().StringVarP(&settingsFile, "settings-file", "s", "airflow_settings.yaml", "Settings or env file to import airflow objects from")
	cmd.Flags().StringSliceVar(&buildSecrets, "build-secrets", []string{}, "Mimics docker build --secret flag. See https://docs.docker.com/build/building/secrets/ for more information. Example input id=mysecret,src=secrets.txt")
	cmd.Flags().StringVarP(&workspaceID, "workspace-id", "w", "", "ID of the Workspace to retrieve environment connections from. If not specified uses the current Workspace.")
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "ID of the Deployment to retrieve environment connections from")

	return cmd
}

func newAirflowPytestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "pytest [pytest file/directory]",
		Short:   "Run pytests in a local Airflow environment",
		Long:    "This command spins up a local Python environment to run pytests against your DAGs. If a specific pytest file is not specified, all pytests in the tests directory will be run. To run pytests with a different environment file, specify that with the '--env' flag. ",
		PreRunE: EnsureRuntime,
		RunE:    airflowPytest,
	}
	cmd.Flags().StringVarP(&pytestArgs, "args", "a", "", "pytest arguments you'd like passed to the pytest command. Surround the args in quotes. For example 'astro dev pytest --args \"--cov-config path\"'")
	cmd.Flags().StringVarP(&envFile, "env", "e", ".env", "Location of file containing environment variables")
	cmd.Flags().StringVarP(&customImageName, "image-name", "i", "", "Name of a custom built image to run pytest with")
	cmd.Flags().StringSliceVar(&buildSecrets, "build-secrets", []string{}, "Mimics docker build --secret flag. See https://docs.docker.com/build/building/secrets/ for more information. Example input id=mysecret,src=secrets.txt")

	return cmd
}

func newAirflowParseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "parse",
		Short:   "Parse all DAGs in your Astro project for errors",
		Long:    "This command spins up a local Python environment and checks your DAGs for syntax and import errors.",
		Args:    cobra.MaximumNArgs(1),
		PreRunE: EnsureRuntime,
		RunE:    airflowParse,
	}
	cmd.Flags().StringVarP(&envFile, "env", "e", ".env", "Location of file containing environment variables")
	cmd.Flags().StringVarP(&customImageName, "image-name", "i", "", "Name of a custom built image to run parse with")
	cmd.Flags().StringSliceVar(&buildSecrets, "build-secrets", []string{}, "Mimics docker build --secret flag. See https://docs.docker.com/build/building/secrets/ for more information. Example input id=mysecret,src=secrets.txt")

	return cmd
}

func newAirflowBuildCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "build",
		Short:   "Build your Astro project into a Docker image",
		Long:    "Build your Astro project into a Docker image without starting the local Airflow environment. This is useful for testing that your project builds successfully or for preparing an image before deployment.",
		PreRunE: EnsureRuntime,
		RunE:    airflowBuild,
	}
	cmd.Flags().BoolVarP(&noCache, "no-cache", "", false, "Do not use cache when building container image")
	cmd.Flags().StringVarP(&customImageName, "image-name", "i", "", "Name of a custom built image to tag as the project image")
	cmd.Flags().StringSliceVar(&buildSecrets, "build-secrets", []string{}, "Mimics docker build --secret flag. See https://docs.docker.com/build/building/secrets/ for more information. Example input id=mysecret,src=secrets.txt")

	return cmd
}

func newAirflowBashCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "bash",
		Short:   "Exec into a running an Airflow container",
		Long:    "Use this command to exec into a container to run bash commands",
		Args:    cobra.MaximumNArgs(1),
		PreRunE: EnsureRuntime,
		RunE:    airflowBash,
	}
	cmd.Flags().BoolVarP(&schedulerExec, "scheduler", "s", false, "Exec into the scheduler container")
	cmd.Flags().BoolVarP(&webserverExec, "webserver", "w", false, "Exec into the webserver container")
	cmd.Flags().BoolVarP(&postgresExec, "postgres", "p", false, "Exec into the postgres container")
	cmd.Flags().BoolVarP(&triggererExec, "triggerer", "t", false, "Exec into the triggerer container")
	cmd.Flags().BoolVarP(&apiServerExec, "api-server", "a", false, "Exec into the api-server container")
	cmd.Flags().BoolVarP(&dagProcessorExec, "dag-processor", "d", false, "Exec into the dag-processor container")
	return cmd
}

func newAirflowObjectRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "object",
		Aliases: []string{"obj", "objects"},
		Short:   "Configure your local Airflow environment.",
		Long:    "Manage local Airflow connections, variables, and pools. Import or export your objects to your Airflow settings file. Configure Airflow's startup behavior using a Compose file.",
	}
	cmd.AddCommand(
		newObjectImportCmd(),
		newObjectExportCmd(),
	)
	return cmd
}

func newObjectImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "import",
		Short:   "Create and update local Airflow connections, variables, and pools from a local YAML file",
		Long:    "This command creates all connections, variables, and pools from a YAML configuration file in your local Airflow environment. Airflow must be running locally for this command to work",
		PreRunE: EnsureRuntime,
		RunE:    airflowSettingsImport,
	}
	cmd.Flags().BoolVarP(&connections, "connections", "c", false, "Import connections from a settings YAML file")
	cmd.Flags().BoolVarP(&variables, "variables", "v", false, "Import variables from a settings YAML file")
	cmd.Flags().BoolVarP(&pools, "pools", "p", false, "Import pools from a settings YAML file")
	cmd.Flags().StringVarP(&settingsFile, "settings-file", "s", "airflow_settings.yaml", "The settings YAML file from which to import Airflow objects. Default is 'airflow_settings.yaml'")
	return cmd
}

func newObjectExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "export",
		Short:   "Export local Airflow connections, variables, pools, and startup configurations as YAML or environment variables.",
		Long:    "Export local Airflow connections, variables, or pools as YAML or environment variables. Airflow must be running locally to export Airflow objects. Use the '--compose' flag to export the Compose file used to start up Airflow.",
		Args:    cobra.MaximumNArgs(1),
		PreRunE: EnsureRuntime,
		RunE:    airflowSettingsExport,
	}
	cmd.Flags().BoolVarP(&connections, "connections", "c", false, "Export connections to a settings YAML or env file")
	cmd.Flags().BoolVarP(&variables, "variables", "v", false, "Export variables to a settings YAML or env file")
	cmd.Flags().BoolVarP(&pools, "pools", "p", false, "Export pools to a settings file. Note pools cannot be exported to a env file ")
	cmd.Flags().StringVarP(&settingsFile, "settings-file", "s", "airflow_settings.yaml", "The location of the file to store exported Airflow objects as YAML. Default is 'airflow_settings.yaml'")
	cmd.Flags().BoolVarP(&envExport, "env-export", "n", false, "Export Airflow objects as Astro environment variables.")
	cmd.Flags().BoolVarP(&compose, "compose", "", false, "Export the Compose file used to start Airflow locally.")
	cmd.Flags().StringVarP(&envFile, "env", "e", ".env", "The location of the file to store exported Airflow objects as Astro environment variables. Default is '.env'.")
	cmd.Flags().StringVarP(&exportComposeFile, "compose-file", "", "compose.yaml", "The location to export the Compose file used to start Airflow locally.")
	return cmd
}

// Use project name for image name
func airflowInit(cmd *cobra.Command, args []string) error { //nolint:gocognit,gocyclo
	name, err := ensureProjectName(args, projectName)
	if err != nil {
		return err
	}
	projectName = name

	if fromTemplate == "select-template" {
		selectedTemplate, err := selectedTemplate()
		if err != nil {
			return fmt.Errorf("unable to select template from list: %w", err)
		}
		fromTemplate = selectedTemplate
	} else if fromTemplate != "" {
		templateList, err := TemplateList()
		if err != nil {
			return fmt.Errorf("unable to fetch template list: %w", err)
		}
		if !isValidTemplate(templateList, fromTemplate) {
			return fmt.Errorf("%s is not a valid template name. Available templates are: %s", fromTemplate, templateList)
		}
	}

	// Handle remote execution setup - use flag value or prompt for registry if needed
	var registryEndpoint string
	if remoteExecutionEnabled {
		registryEndpoint, err = getRegistryEndpoint()
		if err != nil {
			return fmt.Errorf("unable to get registry endpoint: %w", err)
		}
	}

	// Validate runtimeVersion and airflowVersion
	if airflowVersion != "" && runtimeVersion != "" {
		return errInvalidBothAirflowAndRuntimeVersions
	}
	if useAstronomerCertified && runtimeVersion != "" {
		fmt.Println("You provided a runtime version with the --use-astronomer-certified flag. Thus, this command will ignore the --runtime-version value you provided.")
		runtimeVersion = ""
	}

	// If user provides a runtime version, use it, otherwise retrieve the latest one (matching Airflow Version if provided)
	imageTag := runtimeVersion
	if imageTag == "" {
		httpClient := airflowversions.NewClient(httputil.NewHTTPClient(), useAstronomerCertified, false)
		imageTag, err = getDefaultImageTag(httpClient, airflowVersion, "", false)
		if err != nil {
			return fmt.Errorf("error getting default image tag: %w", err)
		}
	}

	var imageName string
	if useAstronomerCertified {
		imageName = airflow.AstronomerCertifiedImageName
	} else {
		switch airflowversions.AirflowMajorVersionForRuntimeVersion(imageTag) {
		case "3":
			imageName = airflow.AstroRuntimeAirflow3ImageName
		case "2":
			imageName = airflow.AstroRuntimeAirflow2ImageName
		default:
			return errors.New("unsupported Airflow major version for runtime version " + imageTag)
		}
	}

	clientImageTag := ""
	if remoteExecutionEnabled {
		httpClient := airflowversions.NewClient(httputil.NewHTTPClient(), false, true)
		clientImageTag, err = getDefaultImageTag(httpClient, "", imageTag, false)
		if err != nil {
			return fmt.Errorf("error getting default client image tag: %w", err)
		}
	}

	// Ensure the project directory is created if a positional argument is provided.
	newProjectPath, err := ensureProjectDirectory(args, config.WorkingPath, projectName)
	if err != nil {
		return err
	}

	// Update the config setting.
	config.WorkingPath = newProjectPath

	emptyDir := fileutil.IsEmptyDir(config.WorkingPath)

	if !emptyDir {
		i, _ := input.Confirm(
			fmt.Sprintf("%s is not an empty directory. Are you sure you want to initialize a project here?", config.WorkingPath))

		if !i {
			fmt.Println("Canceling project initialization...")
			return nil
		}
	}

	exists, err := config.IsProjectDir(config.WorkingPath)
	if err != nil {
		return err
	}
	if !exists {
		config.CreateProjectConfig(config.WorkingPath)
	}

	err = config.CFG.ProjectName.SetProjectString(projectName)
	if err != nil {
		return err
	}

	// Save the registry endpoint if provided during remote execution setup
	if remoteExecutionEnabled && registryEndpoint != "" {
		err := config.CFG.RemoteClientRegistry.SetProjectString(registryEndpoint)
		if err != nil {
			return fmt.Errorf("failed to save remote client registry: %w", err)
		}
		fmt.Println("If you want to modify the remote repository down the line, use the `astro config set <config-name> <config-value>` command while being inside the local project")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// Execute method
	err = airflow.Init(config.WorkingPath, imageName, imageTag, fromTemplate, clientImageTag)
	if err != nil {
		return err
	}

	if exists {
		fmt.Printf(configReinitProjectConfigMsg, config.WorkingPath)
	} else {
		fmt.Printf(configInitProjectConfigMsg, config.WorkingPath)
	}

	return nil
}

// ensureProjectDirectory creates a new project directory if a positional argument is provided.
func ensureProjectDirectory(args []string, workingPath, projectName string) (string, error) {
	// Return early if no positional argument was provided.
	if len(args) == 0 {
		return workingPath, nil
	}

	// Construct the path to our desired project directory.
	newProjectPath := filepath.Join(workingPath, projectName)

	// Determine if the project directory already exists.
	projectDirExists, err := fileutil.Exists(newProjectPath, nil)
	if err != nil {
		return "", err
	}

	// If the project directory does not exist, create it.
	if !projectDirExists {
		err := os.Mkdir(newProjectPath, os.FileMode(directoryPermissions)) //nolint:gosec
		if err != nil {
			return "", err
		}
	}

	// Return the path we just created.
	return newProjectPath, nil
}

func ensureProjectName(args []string, projectName string) (string, error) {
	// If the project name is specified with the --name flag,
	// it cannot be specified as a positional argument as well, so return an error.
	if projectName != "" && len(args) > 0 {
		return "", errConfigProjectNameSpecifiedTwice
	}

	// The first positional argument is the project name.
	// If the project name is provided in this way, we'll
	// attempt to create a directory with that name.
	if projectName == "" && len(args) > 0 {
		projectName = args[0]
	}

	// Validate project name
	if projectName != "" {
		projectNameValid := regexp.
			MustCompile(`^(?i)[a-z0-9]([a-z0-9_-]*[a-z0-9])$`).
			MatchString

		if !projectNameValid(projectName) {
			return "", errConfigProjectName
		}
	} else {
		projectDirectory := filepath.Base(config.WorkingPath)
		projectName = strings.Replace(strcase.ToSnake(projectDirectory), "_", "-", -1)
	}

	return projectName, nil
}

func airflowUpgradeTest(cmd *cobra.Command, platformCoreClient astroplatformcore.CoreClient) error { //nolint:gocognit
	// Validate runtimeVersion and airflowVersion
	if airflowVersion != "" && runtimeVersion != "" {
		return errInvalidBothAirflowAndRuntimeVersionsUpgrade
	}
	if runtimeVersion != "" && customImageName != "" {
		return errInvalidBothCustomImageandVersion
	}
	if airflowVersion != "" && customImageName != "" {
		return errInvalidBothCustomImageandVersion
	}

	if customImageName != "" {
		fmt.Printf("Testing an upgrade to custom Airflow image: %s\n", customImageName)
	} else if runtimeVersion == "" {
		// If user provides a runtime version, use it, otherwise retrieve the latest one (matching Airflow Version if provided
		httpClient := airflowversions.NewClient(httputil.NewHTTPClient(), useAstronomerCertified, false)
		var err error
		runtimeVersion, err = getDefaultImageTag(httpClient, airflowVersion, "", false)
		if err != nil {
			return fmt.Errorf("error getting default image tag: %w", err)
		}
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	imageName := "tmp-upgrade-test"

	containerHandler, err := containerHandlerInit(config.WorkingPath, envFile, dockerfile, imageName)
	if err != nil {
		return err
	}

	// add upgrade-test* to the gitignore
	err = fileutil.AddLineToFile(filepath.Join(config.WorkingPath, ".gitignore"), "upgrade-test*", "")
	if err != nil {
		fmt.Printf("failed to add 'upgrade-test*' to .gitignore: %s", err.Error())
	}

	buildSecretString = util.GetbuildSecretString(buildSecrets)

	err = containerHandler.UpgradeTest(runtimeVersion, deploymentID, customImageName, buildSecretString, versionTest, dagTest, lintTest, lintDeprecations, lintFix, lintConfigFile, platformCoreClient)
	if err != nil {
		return err
	}

	return nil
}

// Start an airflow cluster
func airflowStart(cmd *cobra.Command, args []string, astroCoreClient astrocore.CoreClient) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// Get release name from args, if passed
	if len(args) > 0 {
		envFile = args[0]
	}

	var envConns map[string]astrocore.EnvironmentObjectConnection
	if workspaceID != "" || deploymentID != "" {
		var err error
		envConns, err = environment.ListConnections(workspaceID, deploymentID, astroCoreClient)
		if err != nil {
			return err
		}
	}

	containerHandler, err := containerHandlerInit(config.WorkingPath, envFile, dockerfile, "")
	if err != nil {
		return err
	}

	buildSecretString = util.GetbuildSecretString(buildSecrets)

	return containerHandler.Start(customImageName, settingsFile, composeFile, buildSecretString, noCache, noBrowser, waitTime, envConns)
}

// airflowLocal starts Airflow locally without Docker.
func airflowLocal(cmd *cobra.Command, astroCoreClient astrocore.CoreClient) error {
	cmd.SilenceUsage = true

	var envConns map[string]astrocore.EnvironmentObjectConnection
	if workspaceID != "" || deploymentID != "" {
		var err error
		envConns, err = environment.ListConnections(workspaceID, deploymentID, astroCoreClient)
		if err != nil {
			return err
		}
	}

	containerHandler, err := localHandlerInit(config.WorkingPath, envFile, dockerfile, "")
	if err != nil {
		return err
	}

	// Set foreground mode if the flag was provided
	if sa, ok := containerHandler.(*airflow.Standalone); ok {
		sa.SetForeground(localForeground)
	}

	return containerHandler.Start("", settingsFile, "", "", false, false, waitTime, envConns)
}

// airflowLocalReset removes local environment files.
func airflowLocalReset(cmd *cobra.Command, _ []string) error {
	cmd.SilenceUsage = true

	containerHandler, err := localHandlerInit(config.WorkingPath, envFile, dockerfile, "")
	if err != nil {
		return err
	}

	return containerHandler.Kill()
}

// airflowLocalStop stops the local Airflow process.
func airflowLocalStop(cmd *cobra.Command, _ []string) error {
	cmd.SilenceUsage = true

	containerHandler, err := localHandlerInit(config.WorkingPath, envFile, dockerfile, "")
	if err != nil {
		return err
	}

	return containerHandler.Stop(false)
}

// airflowLocalLogs streams the local Airflow log file.
func airflowLocalLogs(cmd *cobra.Command, _ []string) error {
	cmd.SilenceUsage = true

	containerHandler, err := localHandlerInit(config.WorkingPath, envFile, dockerfile, "")
	if err != nil {
		return err
	}

	return containerHandler.Logs(followLogs)
}

// airflowRun
func airflowRun(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// Add airflow command, to simplify astro cli usage
	args = append([]string{"airflow"}, args...)
	// ignore last user parameter

	containerHandler, err := containerHandlerInit(config.WorkingPath, "", dockerfile, "")
	if err != nil {
		return err
	}

	return containerHandler.Run(args, "")
}

// List containers of an airflow cluster
func airflowPS(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	containerHandler, err := containerHandlerInit(config.WorkingPath, "", dockerfile, "")
	if err != nil {
		return err
	}

	return containerHandler.PS()
}

// Outputs logs for a development airflow cluster
func airflowLogs(cmd *cobra.Command, args []string) error {
	// default is to display all logs
	containersNames := make([]string, 0)

	if !schedulerLogs && !webserverLogs && !triggererLogs && !apiServerLogs && !dagProcessorLogs {
		containersNames = append(containersNames, []string{airflow.WebserverDockerContainerName, airflow.SchedulerDockerContainerName, airflow.TriggererDockerContainerName, airflow.APIServerDockerContainerName, airflow.DAGProcessorDockerContainerName}...)
	}
	if webserverLogs {
		containersNames = append(containersNames, []string{airflow.WebserverDockerContainerName}...)
	}
	if apiServerLogs {
		containersNames = append(containersNames, []string{airflow.APIServerDockerContainerName}...)
	}
	if schedulerLogs {
		containersNames = append(containersNames, []string{airflow.SchedulerDockerContainerName}...)
	}
	if triggererLogs {
		containersNames = append(containersNames, []string{airflow.TriggererDockerContainerName}...)
	}
	if dagProcessorLogs {
		containersNames = append(containersNames, []string{airflow.DAGProcessorDockerContainerName}...)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	containerHandler, err := containerHandlerInit(config.WorkingPath, "", dockerfile, "")
	if err != nil {
		return err
	}

	return containerHandler.Logs(followLogs, containersNames...)
}

// Kill an airflow cluster
func airflowKill(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	containerHandler, err := containerHandlerInit(config.WorkingPath, "", dockerfile, "")
	if err != nil {
		return err
	}

	return containerHandler.Kill()
}

// Stop an airflow cluster
func airflowStop(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	containerHandler, err := containerHandlerInit(config.WorkingPath, "", dockerfile, "")
	if err != nil {
		return err
	}

	return containerHandler.Stop(false)
}

// Stop an airflow cluster
func airflowRestart(cmd *cobra.Command, args []string, astroCoreClient astrocore.CoreClient) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	containerHandler, err := containerHandlerInit(config.WorkingPath, envFile, dockerfile, "")
	if err != nil {
		return err
	}

	if forceKill {
		err = containerHandler.Kill()
	} else {
		err = containerHandler.Stop(true)
	}
	if err != nil {
		return err
	}

	// Get release name from args, if passed
	if len(args) > 0 {
		envFile = args[0]
	}
	// don't startup browser on restart
	noBrowser = true

	var envConns map[string]astrocore.EnvironmentObjectConnection
	if workspaceID != "" || deploymentID != "" {
		var err error
		envConns, err = environment.ListConnections(workspaceID, deploymentID, astroCoreClient)
		if err != nil {
			return err
		}
	}

	buildSecretString = util.GetbuildSecretString(buildSecrets)

	return containerHandler.Start(customImageName, settingsFile, composeFile, buildSecretString, noCache, noBrowser, waitTime, envConns)
}

// run pytest on an airflow project
func airflowPytest(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// Get release name from args, if passed
	if len(args) > 0 {
		pytestFile = args[0]
	}

	if len(args) > 1 {
		return errPytestArgs
	}

	// Check if tests directory exists
	fileExist, err := util.Exists(config.WorkingPath + pytestDir)
	if err != nil {
		return err
	}

	if !fileExist {
		return errors.New("the 'tests' directory does not exist, please run `astro dev init` to create it")
	}

	imageName, err := projectNameUnique()
	if err != nil {
		return err
	}

	fmt.Println("Running your test suite…")

	containerHandler, err := containerHandlerInit(config.WorkingPath, envFile, dockerfile, imageName)
	if err != nil {
		return err
	}

	buildSecretString = util.GetbuildSecretString(buildSecrets)

	exitCode, err := containerHandler.Pytest(pytestFile, customImageName, "", pytestArgs, buildSecretString)
	if err != nil {
		if strings.Contains(exitCode, "1") { // exit code is 1 meaning tests failed
			return errors.New("pytest failed")
		}
		return err
	}

	fmt.Println("\n" + ansi.Green("✔") + " All tests passed!")
	return err
}

func airflowParse(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	imageName, err := projectNameUnique()
	if err != nil {
		return err
	}

	containerHandler, err := containerHandlerInit(config.WorkingPath, envFile, dockerfile, imageName)
	if err != nil {
		return err
	}

	buildSecretString = util.GetbuildSecretString(buildSecrets)

	return containerHandler.Parse(customImageName, "", buildSecretString)
}

// Build the Airflow project image
func airflowBuild(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	imageName, err := projectNameUnique()
	if err != nil {
		return err
	}

	containerHandler, err := containerHandlerInit(config.WorkingPath, envFile, dockerfile, imageName)
	if err != nil {
		return err
	}

	buildSecretString = util.GetbuildSecretString(buildSecrets)

	return containerHandler.Build(customImageName, buildSecretString, noCache)
}

// Exec into an airflow container
func airflowBash(cmd *cobra.Command, args []string) error {
	// figure out what container to exec into
	container := ""

	if triggererExec {
		container = airflow.TriggererDockerContainerName
	}
	if postgresExec {
		container = airflow.PostgresDockerContainerName
	}
	if webserverExec {
		container = airflow.WebserverDockerContainerName
	}
	if schedulerExec {
		container = airflow.SchedulerDockerContainerName
	}
	if apiServerExec {
		container = airflow.APIServerDockerContainerName
	}
	if dagProcessorExec {
		container = airflow.DAGProcessorDockerContainerName
	}
	// exec into secheduler by default
	if container == "" {
		container = airflow.SchedulerDockerContainerName
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	containerHandler, err := containerHandlerInit(config.WorkingPath, "", dockerfile, "")
	if err != nil {
		return err
	}

	fmt.Printf("Execing into the %s container\n\n", container)
	return containerHandler.Bash(container)
}

func airflowSettingsImport(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	containerHandler, err := containerHandlerInit(config.WorkingPath, "", dockerfile, "")
	if err != nil {
		return err
	}
	return containerHandler.ImportSettings(settingsFile, envFile, connections, variables, pools)
}

func airflowSettingsExport(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	containerHandler, err := containerHandlerInit(config.WorkingPath, "", dockerfile, "")
	if err != nil {
		return err
	}

	if compose {
		fmt.Println("Exporting compose file to " + exportComposeFile)
		return containerHandler.ComposeExport(settingsFile, exportComposeFile)
	}
	if !compose && cmd.Flags().Changed("compose-file") {
		return errNoCompose
	}

	return containerHandler.ExportSettings(settingsFile, envFile, connections, variables, pools, envExport)
}

func isValidTemplate(templateList []string, template string) bool {
	return slices.Contains(templateList, template)
}

func selectedTemplate() (string, error) {
	templateList, err := TemplateList()
	if err != nil {
		return "", fmt.Errorf("unable to fetch template list: %w", err)
	}
	selectedTemplate, err := airflow.SelectTemplate(templateList)
	if err != nil {
		return "", fmt.Errorf("unable to select template from list: %w", err)
	}

	return selectedTemplate, nil
}

// validateRegistryEndpoint validates the format of a Docker registry endpoint
func getRegistryEndpoint() (string, error) {
	var registryEndpoint string
	// Use flag value if provided, otherwise check config, otherwise prompt
	if remoteImageRepository != "" {
		registryEndpoint = remoteImageRepository
		// Validate the registry endpoint format
		if err := config.ValidateRegistryEndpoint(registryEndpoint); err != nil {
			return "", fmt.Errorf("invalid registry endpoint format: %w", err)
		}
	} else {
		registryEndpoint = config.CFG.RemoteClientRegistry.GetString()
		if registryEndpoint == "" {
			fmt.Println("Enter the remote Docker repository for the client image (leave blank if not known but you will not be able to use the Astro CLI to deploy the client image until configured)")
			registryEndpoint = input.Text("Remote client image repository endpoint (e.g. quay.io/acme/my-deployment-image): ")

			if registryEndpoint != "" {
				// Validate the registry endpoint format
				if err := config.ValidateRegistryEndpoint(registryEndpoint); err != nil {
					return "", fmt.Errorf("invalid registry endpoint format: %w", err)
				}
			}
		}
	}
	return registryEndpoint, nil
}
