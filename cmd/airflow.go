package cmd

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/airflow"
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
	triggererLogs          bool
	noCache                bool
	schedulerExec          bool
	postgresExec           bool
	webserverExec          bool
	triggererExec          bool
	connections            bool
	variables              bool
	pools                  bool
	envExport              bool
	noBrowser              bool
	compose                bool
	conflictTest           bool
	versionTest            bool
	dagTest                bool
	waitTime               time.Duration
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

# Initialize a new Astro project with the latest version of Astronomer Certified. Use this only if you run on Astronomer Software
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
`
	dockerfile = "Dockerfile"

	configReinitProjectConfigMsg = "Reinitialized existing Astro project in %s\n"
	configInitProjectConfigMsg   = "Initialized empty Astro project in %s"

	// this is used to monkey patch the function in order to write unit test cases
	containerHandlerInit = airflow.ContainerHandlerInit
	getDefaultImageTag   = airflowversions.GetDefaultImageTag
	projectNameUnique    = airflow.ProjectNameUnique

	pytestDir = "/tests"

	airflowUpgradeCheckCmd = []string{"bash", "-c", "pip install --no-deps 'apache-airflow-upgrade-check'; python -c 'from packaging.version import Version\nfrom airflow import __version__\nif Version(__version__) < Version(\"1.10.14\"):\n  print(\"Please upgrade your image to Airflow 1.10.14 first, then try again.\");exit(1)\nelse:\n  from airflow.upgrade.checker import __main__;__main__()'"}
	errPytestArgs          = errors.New("you can only pass one pytest file or directory")
	buildSecrets           = []string{}
	errNoCompose           = errors.New("cannot use '--compose-file' without '--compose' flag")
	TemplateList           = airflow.FetchTemplateList
	defaultWaitTime        = 1 * time.Minute
)

func newDevRootCmd(platformCoreClient astroplatformcore.CoreClient, astroCoreClient astrocore.CoreClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "dev",
		Aliases: []string{"d"},
		Short:   "Run your Astro project in a local Airflow environment",
		Long:    "Run an Apache Airflow environment on your local machine to test your project, including DAGs, Python Packages, and plugins.",
	}
	cmd.AddCommand(
		newAirflowInitCmd(),
		newAirflowStartCmd(astroCoreClient),
		newAirflowRunCmd(),
		newAirflowPSCmd(),
		newAirflowLogsCmd(),
		newAirflowStopCmd(),
		newAirflowKillCmd(),
		newAirflowPytestCmd(),
		newAirflowParseCmd(),
		newAirflowRestartCmd(astroCoreClient),
		newAirflowUpgradeCheckCmd(),
		newAirflowBashCmd(),
		newAirflowObjectRootCmd(),
		newAirflowUpgradeTestCmd(platformCoreClient),
	)
	return cmd
}

func newAirflowInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "init",
		Short:   "Create a new Astro project in your working directory",
		Long:    "Create a new Astro project in your working directory. This generates the files you need to start an Airflow environment on your local machine and deploy your project to a Deployment on Astro or Astronomer Software.",
		Example: initCloudExample,
		Args:    cobra.MaximumNArgs(1),
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		RunE: airflowInit,
	}
	cmd.Flags().StringVarP(&projectName, "name", "n", "", "Name of Astro project")
	cmd.Flags().StringVarP(&airflowVersion, "airflow-version", "a", "", "Version of Airflow you want to create an Astro project with. If not specified, latest is assumed. You can change this version in your Dockerfile at any time.")
	cmd.Flags().StringVarP(&fromTemplate, "from-template", "t", "", "Provides a list of templates to select from and create the local astro project based on the selected template. Please note template based astro projects use the latest runtime version, so runtime-version and airflow-version flags will be ignored when creating a project with template flag")
	cmd.Flag("from-template").NoOptDefVal = "select-template"
	var err error
	var avoidACFlag bool

	// In case user is connected to Astronomer Platform and is connected to older version of platform
	if context.IsCloudContext() || houstonVersion == "" || (!context.IsCloudContext() && houston.VerifyVersionMatch(houstonVersion, houston.VersionRestrictions{GTE: "0.29.0"})) {
		cmd.Flags().StringVarP(&runtimeVersion, "runtime-version", "v", "", "Specify a version of Astro Runtime that you want to create an Astro project with. If not specified, the latest is assumed. You can change this version in your Dockerfile at any time.")
	} else { // default to using AC flag, since runtime is not available for these cases
		useAstronomerCertified = true
		avoidACFlag = true
	}

	if !context.IsCloudContext() && !avoidACFlag {
		cmd.Example = initSoftwareExample
		cmd.Flags().BoolVarP(&useAstronomerCertified, "use-astronomer-certified", "", false, "If specified, initializes a project using Astronomer Certified Airflow image instead of Astro Runtime.")
	}

	_, err = context.GetCurrentContext()
	if err != nil && !avoidACFlag { // Case when user is not logged in to any platform
		cmd.Flags().BoolVarP(&useAstronomerCertified, "use-astronomer-certified", "", false, "If specified, initializes a project using Astronomer Certified Airflow image instead of Astro Runtime.")
		_ = cmd.Flags().MarkHidden("use-astronomer-certified")
	}
	return cmd
}

func newAirflowUpgradeTestCmd(platformCoreClient astroplatformcore.CoreClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade-test",
		Short: "Run tests to see if your environment and DAGs are compatible with a new version of Airflow or Astro Runtime. This test will produce a series of reports where you can see the test results.",
		Long:  "Run tests to see if your environment and DAGs are compatible with a new version of Airflow or Astro Runtime. This test will produce a series of reports where you can see the test results.",
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return airflowUpgradeTest(cmd, platformCoreClient)
		},
	}
	cmd.Flags().StringVarP(&airflowVersion, "airflow-version", "a", "", "The version of Airflow you want to upgrade to. The default is the latest available version. Tests are run against the equivalent Astro Runtime version. ")
	cmd.Flags().BoolVarP(&versionTest, "version-test", "", false, "Only run version tests. These tests show you how the versions of your dependencies will change after you upgrade.")
	cmd.Flags().BoolVarP(&dagTest, "dag-test", "d", false, "Only run DAG tests. These tests check whether your DAGs will generate import errors after you upgrade.")
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
		Use:   "start",
		Short: "Start a local Airflow environment",
		Long:  "Start a local Airflow environment. This command will spin up 4 Docker containers on your machine, each for a different Airflow component: Webserver, scheduler, triggerer and metadata database.",
		Args:  cobra.MaximumNArgs(1),
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		PreRunE: airflow.EnsureRuntimePreRunHook,
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
	if !config.CFG.DisableEnvObjects.GetBool() {
		cmd.Flags().StringVarP(&workspaceID, "workspace-id", "w", "", "ID of the Workspace to retrieve environment connections from. If not specified uses the current Workspace.")
		cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "ID of the Deployment to retrieve environment connections from")
	}

	return cmd
}

func newAirflowPSCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ps",
		Short: "List locally running Airflow containers",
		Long:  "List locally running Airflow containers",
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		PreRunE: airflow.SetRuntimeIfExistsPreRunHook,
		RunE:    airflowPS,
	}
	return cmd
}

func newAirflowRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "run",
		Short:              "Run Airflow CLI commands within your local Airflow environment",
		Long:               "Run Airflow CLI commands within your local Airflow environment. These commands run in the webserver container but can interact with your local scheduler, workers, and metadata database.",
		PreRunE:            airflow.EnsureRuntimePreRunHook,
		RunE:               airflowRun,
		Example:            RunExample,
		DisableFlagParsing: true,
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	return cmd
}

func newAirflowLogsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "logs",
		Short:   "Display component logs for your local Airflow environment",
		Long:    "Display scheduler, worker, and webserver logs for your local Airflow environment",
		PreRunE: airflow.SetRuntimeIfExistsPreRunHook,
		RunE:    airflowLogs,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	cmd.Flags().BoolVarP(&followLogs, "follow", "f", false, "Follow log output")
	cmd.Flags().BoolVarP(&schedulerLogs, "scheduler", "s", false, "Output scheduler logs")
	cmd.Flags().BoolVarP(&webserverLogs, "webserver", "w", false, "Output webserver logs")
	cmd.Flags().BoolVarP(&triggererLogs, "triggerer", "t", false, "Output triggerer logs")
	return cmd
}

func newAirflowStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop all locally running Airflow containers",
		Long:  "Stop all Airflow containers running on your local machine. This command preserves container data.",
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		PreRunE: airflow.SetRuntimeIfExistsPreRunHook,
		RunE:    airflowStop,
	}
	return cmd
}

func newAirflowKillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kill",
		Short: "Kill all locally running Airflow containers",
		Long:  "Kill all Airflow containers running on your local machine. This command permanently deletes all container data.",
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		PreRunE:  airflow.KillPreRunHook,
		RunE:     airflowKill,
		PostRunE: airflow.KillPostRunHook,
	}
	return cmd
}

func newAirflowRestartCmd(astroCoreClient astrocore.CoreClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart all locally running Airflow containers",
		Long:  "Restart all Airflow containers running on your local machine. This command stops and then starts locally running containers to apply changes to your local environment.",
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		PreRunE: airflow.SetRuntimeIfExistsPreRunHook,
		RunE: func(cmd *cobra.Command, args []string) error {
			return airflowRestart(cmd, args, astroCoreClient)
		},
	}
	cmd.Flags().DurationVar(&waitTime, "wait", defaultWaitTime, "Duration to wait for webserver to get healthy. The default is 5 minutes. Use --wait 2m to wait for 2 minutes.")
	cmd.Flags().StringVarP(&envFile, "env", "e", ".env", "Location of file containing environment variables")
	cmd.Flags().BoolVarP(&noCache, "no-cache", "", false, "Do not use cache when building container image")
	cmd.Flags().StringVarP(&customImageName, "image-name", "i", "", "Name of a custom built image to restart airflow with")
	cmd.Flags().StringVarP(&settingsFile, "settings-file", "s", "airflow_settings.yaml", "Settings or env file to import airflow objects from")
	cmd.Flags().StringSliceVar(&buildSecrets, "build-secrets", []string{}, "Mimics docker build --secret flag. See https://docs.docker.com/build/building/secrets/ for more information. Example input id=mysecret,src=secrets.txt")
	if !config.CFG.DisableEnvObjects.GetBool() {
		cmd.Flags().StringVarP(&workspaceID, "workspace-id", "w", "", "ID of the Workspace to retrieve environment connections from. If not specified uses the current Workspace.")
		cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "ID of the Deployment to retrieve environment connections from")
	}

	return cmd
}

func newAirflowPytestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pytest [pytest file/directory]",
		Short: "Run pytests in a local Airflow environment",
		Long:  "This command spins up a local Python environment to run pytests against your DAGs. If a specific pytest file is not specified, all pytests in the tests directory will be run. To run pytests with a different environment file, specify that with the '--env' flag. ",
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		PreRunE: utils.EnsureProjectDir,
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
		Use:   "parse",
		Short: "parse all DAGs in your Astro project for errors",
		Long:  "This command spins up a local Python environment and checks your DAGs for syntax and import errors.",
		Args:  cobra.MaximumNArgs(1),
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		PreRunE: utils.EnsureProjectDir,
		RunE:    airflowParse,
	}
	cmd.Flags().StringVarP(&envFile, "env", "e", ".env", "Location of file containing environment variables")
	cmd.Flags().StringVarP(&customImageName, "image-name", "i", "", "Name of a custom built image to run parse with")
	cmd.Flags().StringSliceVar(&buildSecrets, "build-secrets", []string{}, "Mimics docker build --secret flag. See https://docs.docker.com/build/building/secrets/ for more information. Example input id=mysecret,src=secrets.txt")

	return cmd
}

func newAirflowUpgradeCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade-check",
		Short: "List DAG and config-level changes required to upgrade to Airflow 2.0",
		Long:  "List DAG and config-level changes required to upgrade to Airflow 2.0",
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		PreRunE:            airflow.EnsureRuntimePreRunHook,
		RunE:               airflowUpgradeCheck,
		DisableFlagParsing: true,
	}
	return cmd
}

func newAirflowBashCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bash",
		Short: "Exec into a running an Airflow container",
		Long:  "Use this command to Exec into either the Webserver, Sechduler, Postgres, or Triggerer ListContainer to run bash commands",
		Args:  cobra.MaximumNArgs(1),
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		PreRunE: airflow.EnsureRuntimePreRunHook,
		RunE:    airflowBash,
	}
	cmd.Flags().BoolVarP(&schedulerExec, "scheduler", "s", false, "Exec into the scheduler container")
	cmd.Flags().BoolVarP(&webserverExec, "webserver", "w", false, "Exec into the webserver container")
	cmd.Flags().BoolVarP(&postgresExec, "postgres", "p", false, "Exec into the postgres container")
	cmd.Flags().BoolVarP(&triggererExec, "triggerer", "t", false, "Exec into the triggerer container")
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
		Use:   "import",
		Short: "Create and update local Airflow connections, variables, and pools from a local YAML file",
		Long:  "This command creates all connections, variables, and pools from a YAML configuration file in your local Airflow environment. Airflow must be running locally for this command to work",
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		PreRunE: airflow.EnsureRuntimePreRunHook,
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
		Use:   "export",
		Short: "Export local Airflow connections, variables, pools, and startup configurations as YAML or environment variables.",
		Long:  "Export local Airflow connections, variables, or pools as YAML or environment variables. Airflow must be running locally to export Airflow objects. Use the '--compose' flag to export the Compose file used to start up Airflow.",
		Args:  cobra.MaximumNArgs(1),
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		PreRunE: airflow.EnsureRuntimePreRunHook,
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
func airflowInit(cmd *cobra.Command, args []string) error {
	// Validate project name
	if projectName != "" {
		// error if project name has spaces
		if len(args) > 0 {
			return errProjectNameSpaces
		}
		projectNameValid := regexp.
			MustCompile(`^(?i)[a-z0-9]([a-z0-9_-]*[a-z0-9])$`).
			MatchString

		if !projectNameValid(projectName) {
			return errConfigProjectName
		}
	} else {
		projectDirectory := filepath.Base(config.WorkingPath)
		projectName = strings.Replace(strcase.ToSnake(projectDirectory), "_", "-", -1)
	}

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

	// Validate runtimeVersion and airflowVersion
	if airflowVersion != "" && runtimeVersion != "" {
		return errInvalidBothAirflowAndRuntimeVersions
	}
	if useAstronomerCertified && runtimeVersion != "" {
		fmt.Println("You provided a runtime version with the --use-astronomer-certified flag. Thus, this command will ignore the --runtime-version value you provided.")
		runtimeVersion = ""
	}

	// If user provides a runtime version, use it, otherwise retrieve the latest one (matching Airflow Version if provided)
	var err error
	defaultImageTag := runtimeVersion
	if defaultImageTag == "" {
		httpClient := airflowversions.NewClient(httputil.NewHTTPClient(), useAstronomerCertified)
		defaultImageTag = prepareDefaultAirflowImageTag(airflowVersion, httpClient)
	}

	defaultImageName := airflow.AstroRuntimeImageName
	if useAstronomerCertified {
		defaultImageName = airflow.AstronomerCertifiedImageName
		fmt.Printf("Initializing Astro project\nPulling Airflow development files from Astronomer Certified Airflow Version %s\n", defaultImageTag)
	} else {
		fmt.Printf("Initializing Astro project\nPulling Airflow development files from Astro Runtime %s\n", defaultImageTag)
	}

	emptyDir := fileutil.IsEmptyDir(config.WorkingPath)

	if !emptyDir {
		i, _ := input.Confirm(
			fmt.Sprintf("%s \nYou are not in an empty directory. Are you sure you want to initialize a project?", config.WorkingPath))

		if !i {
			fmt.Println("Canceling project initialization...")
			return nil
		}
	}

	exists := config.ProjectConfigExists()
	if !exists {
		config.CreateProjectConfig(config.WorkingPath)
	}

	err = config.CFG.ProjectName.SetProjectString(projectName)
	if err != nil {
		return err
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// Execute method
	err = airflow.Init(config.WorkingPath, defaultImageName, defaultImageTag, fromTemplate)
	if err != nil {
		return err
	}

	if exists {
		fmt.Printf(configReinitProjectConfigMsg+"\n", config.WorkingPath)
	} else {
		fmt.Printf(configInitProjectConfigMsg+"\n", config.WorkingPath)
	}

	return nil
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

	defaultImageName := airflow.AstroRuntimeImageName
	defaultImageTag := runtimeVersion
	if customImageName != "" {
		fmt.Printf("Testing an upgrade to custom Airflow image: %s\n", customImageName)
	} else {
		// If user provides a runtime version, use it, otherwise retrieve the latest one (matching Airflow Version if provided
		if defaultImageTag == "" {
			httpClient := airflowversions.NewClient(httputil.NewHTTPClient(), useAstronomerCertified)
			defaultImageTag = prepareDefaultAirflowImageTag(airflowVersion, httpClient)
		}
		if useAstronomerCertified {
			defaultImageName = airflow.AstronomerCertifiedImageName
			fmt.Printf("Testing an upgrade to Astronomer Certified Airflow version %s\n\n", defaultImageTag)
		} else {
			fmt.Printf("Testing an upgrade to Astro Runtime %s\n", defaultImageTag)
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

	err = containerHandler.UpgradeTest(defaultImageTag, deploymentID, defaultImageName, customImageName, buildSecretString, conflictTest, versionTest, dagTest, platformCoreClient)
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
	if !config.CFG.DisableEnvObjects.GetBool() && (workspaceID != "" || deploymentID != "") {
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

	if !schedulerLogs && !webserverLogs && !triggererLogs {
		containersNames = append(containersNames, []string{airflow.WebserverDockerContainerName, airflow.SchedulerDockerContainerName, airflow.TriggererDockerContainerName}...)
	}
	if webserverLogs {
		containersNames = append(containersNames, []string{airflow.WebserverDockerContainerName}...)
	}
	if schedulerLogs {
		containersNames = append(containersNames, []string{airflow.SchedulerDockerContainerName}...)
	}
	if triggererLogs {
		containersNames = append(containersNames, []string{airflow.TriggererDockerContainerName}...)
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

	err = containerHandler.Stop(true)
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
	if !config.CFG.DisableEnvObjects.GetBool() && (workspaceID != "" || deploymentID != "") {
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

	fmt.Println("Running Pytest\nThis may take a minute if you have not run this command before…")

	containerHandler, err := containerHandlerInit(config.WorkingPath, envFile, dockerfile, imageName)
	if err != nil {
		return err
	}

	buildSecretString = util.GetbuildSecretString(buildSecrets)

	exitCode, err := containerHandler.Pytest(pytestFile, customImageName, "", pytestArgs, buildSecretString)
	if err != nil {
		if strings.Contains(exitCode, "1") { // exit code is 1 meaning tests failed
			return errors.New("pytests failed")
		}
		return err
	}

	fmt.Println("\n" + ansi.Green("✔") + " All Pytests passed!")
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

// airflowUpgradeCheck
func airflowUpgradeCheck(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// Add airflow command, to simplify astro cli usage
	args = append(airflowUpgradeCheckCmd, args...)

	containerHandler, err := containerHandlerInit(config.WorkingPath, "", dockerfile, "")
	if err != nil {
		return err
	}

	return containerHandler.Run(args, "root")
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

func prepareDefaultAirflowImageTag(airflowVersion string, httpClient *airflowversions.Client) string {
	defaultImageTag, _ := getDefaultImageTag(httpClient, airflowVersion)

	if defaultImageTag == "" {
		if useAstronomerCertified {
			fmt.Println("WARNING! There was a network issue getting the latest Astronomer Certified image. Your Dockerfile may not contain the latest version")
			defaultImageTag = airflowversions.DefaultAirflowVersion
		} else {
			fmt.Println("WARNING! There was a network issue getting the latest Astro Runtime image. Your Dockerfile may not contain the latest version")
			defaultImageTag = airflowversions.DefaultRuntimeVersion
		}
	}
	return defaultImageTag
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
