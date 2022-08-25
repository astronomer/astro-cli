package cmd

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/astronomer/astro-cli/airflow"
	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/util"
	"github.com/iancoleman/strcase"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	useAstronomerCertified bool
	projectName            string
	runtimeVersion         string
	airflowVersion         string
	envFile                string
	pytestFile             string
	customImageName        string
	followLogs             bool
	schedulerLogs          bool
	webserverLogs          bool
	triggererLogs          bool
	noCache                bool
	schedulerExec          bool
	postgresExec           bool
	webserverExec          bool
	triggererExec          bool
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

	houstonVersion string
)

func newDevRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "dev",
		Aliases: []string{"d"},
		Short:   "Run your Astro project in a local Airflow environment",
		Long:    "Run an Apache Airflow environment on your local machine to test your project, including DAGs, Python Packages, and plugins.",
	}
	cmd.AddCommand(
		newAirflowInitCmd(),
		newAirflowStartCmd(),
		newAirflowRunCmd(),
		newAirflowPSCmd(),
		newAirflowLogsCmd(),
		newAirflowStopCmd(),
		newAirflowKillCmd(),
		newAirflowPytestCmd(),
		newAirflowParseCmd(),
		newAirflowRestartCmd(),
		newAirflowUpgradeCheckCmd(),
		newAirflowBashCmd(),
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
	var err error
	var avoidACFlag bool
	houstonVersion, err = houstonClient.GetPlatformVersion()
	if err != nil {
		logrus.Debugf("Error retreiving Astronomer Software platform version: %s", err.Error())
	}

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

func newAirflowStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start a local Airflow environment",
		Long:  "Start a local Airflow environment. This command will spin up 3 Docker containers on your machine, each for a different Airflow component: Webserver, Scheduler, and Postgres.",
		Args:  cobra.MaximumNArgs(1),
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		PreRunE: utils.EnsureProjectDir,
		RunE:    airflowStart,
	}
	cmd.Flags().StringVarP(&envFile, "env", "e", ".env", "Location of file containing environment variables")
	cmd.Flags().BoolVarP(&noCache, "no-cache", "", false, "Do not use cache when building container image")
	cmd.Flags().StringVarP(&customImageName, "image-name", "i", "", "Name of a custom built image to start airflow with")
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
		PreRunE: utils.EnsureProjectDir,
		RunE:    airflowPS,
	}
	return cmd
}

func newAirflowRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "run",
		Short:              "Run Airflow CLI commands within your local Airflow environment",
		Long:               "Run Airflow CLI commands within your local Airflow environment. These commands are run in the Webserver container but can interact with your local Scheduler, Workers, and Postgres Database.",
		PreRunE:            utils.EnsureProjectDir,
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
		Long:    "Display Scheduler, Worker, and Webserver logs for your local Airflow environment",
		PreRunE: utils.EnsureProjectDir,
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
		PreRunE: utils.EnsureProjectDir,
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
		PreRunE: utils.EnsureProjectDir,
		RunE:    airflowKill,
	}
	return cmd
}

func newAirflowRestartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart all locally running Airflow containers",
		Long:  "Restart all Airflow containers running on your local machine. This command stops and then starts locally running containers to apply changes to your local environment.",
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		PreRunE: utils.EnsureProjectDir,
		RunE:    airflowRestart,
	}
	cmd.Flags().StringVarP(&envFile, "env", "e", ".env", "Location of file containing environment variables")
	cmd.Flags().BoolVarP(&noCache, "no-cache", "", false, "Do not use cache when building container image")
	cmd.Flags().StringVarP(&customImageName, "image-name", "i", "", "Name of a custom built image to restart airflow with")
	return cmd
}

func newAirflowPytestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pytest [pytest file/directory]",
		Short: "Run pytests in a local Airflow environment",
		Long:  "This command spins up a local Python environment to run pytests against your DAGs. If a specific pytest file is not specified, all pytests in the tests directory will be run. To run pytests with a different environment file, specify that with the '--env' flag. ",
		Args:  cobra.MaximumNArgs(1),
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		PreRunE: utils.EnsureProjectDir,
		RunE:    airflowPytest,
	}
	cmd.Flags().StringVarP(&envFile, "env", "e", ".env", "Location of file containing environment variables")
	cmd.Flags().StringVarP(&customImageName, "image-name", "i", "", "Name of a custom built image to run pytest with")
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
		PreRunE:            utils.EnsureProjectDir,
		RunE:               airflowUpgradeCheck,
		DisableFlagParsing: true,
	}
	return cmd
}

func newAirflowBashCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bash",
		Short: "Exec into a running an Airflow container",
		Long:  "Use this command to Exec into either the Webserver, Sechduler, Postgres, or Triggerer Container to run bash commands",
		Args:  cobra.MaximumNArgs(1),
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		PreRunE: utils.EnsureProjectDir,
		RunE:    airflowBash,
	}
	cmd.Flags().BoolVarP(&schedulerExec, "scheduler", "s", false, "Exec into the scheduler container")
	cmd.Flags().BoolVarP(&webserverExec, "webserver", "w", false, "Exec into the webserver container")
	cmd.Flags().BoolVarP(&postgresExec, "postgres", "p", false, "Exec into the postgres container")
	cmd.Flags().BoolVarP(&triggererExec, "triggerer", "t", false, "Exec into the triggerer container")
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
	err = airflow.Init(config.WorkingPath, defaultImageName, defaultImageTag)
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

// Start an airflow cluster
func airflowStart(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// Get release name from args, if passed
	if len(args) > 0 {
		envFile = args[0]
	}

	containerHandler, err := containerHandlerInit(config.WorkingPath, envFile, dockerfile, "", false)
	if err != nil {
		return err
	}

	return containerHandler.Start(customImageName, noCache)
}

// airflowRun
func airflowRun(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// Add airflow command, to simplify astro cli usage
	args = append([]string{"airflow"}, args...)
	// ignore last user parameter

	containerHandler, err := containerHandlerInit(config.WorkingPath, "", dockerfile, "", false)
	if err != nil {
		return err
	}

	return containerHandler.Run(args, "")
}

// List containers of an airflow cluster
func airflowPS(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	containerHandler, err := containerHandlerInit(config.WorkingPath, "", dockerfile, "", false)
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

	containerHandler, err := containerHandlerInit(config.WorkingPath, "", dockerfile, "", false)
	if err != nil {
		return err
	}

	return containerHandler.Logs(followLogs, containersNames...)
}

// Kill an airflow cluster
func airflowKill(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	containerHandler, err := containerHandlerInit(config.WorkingPath, "", dockerfile, "", false)
	if err != nil {
		return err
	}

	return containerHandler.Kill()
}

// Stop an airflow cluster
func airflowStop(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	containerHandler, err := containerHandlerInit(config.WorkingPath, "", dockerfile, "", false)
	if err != nil {
		return err
	}

	return containerHandler.Stop()
}

// Stop an airflow cluster
func airflowRestart(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	containerHandler, err := containerHandlerInit(config.WorkingPath, envFile, dockerfile, "", false)
	if err != nil {
		return err
	}

	err = containerHandler.Stop()
	if err != nil {
		return err
	}

	// Get release name from args, if passed
	if len(args) > 0 {
		envFile = args[0]
	}

	return containerHandler.Start(customImageName, noCache)
}

// run pytest on an airflow project
func airflowPytest(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// Get release name from args, if passed
	if len(args) > 0 {
		pytestFile = args[0]
	}

	// Check if tests directory exists
	fileExist, err := util.Exists(config.WorkingPath + pytestDir)
	if err != nil {
		return err
	}

	if !fileExist {
		return errors.New("the 'tests' directory does not exist, please run `astro dev init` to create it")
	}

	imageName, err := projectNameUnique(false)
	if err != nil {
		return err
	}

	fmt.Println("Running Pytest\nThis may take a minute if you have not run this command before…")

	containerHandler, err := containerHandlerInit(config.WorkingPath, envFile, dockerfile, imageName, true)
	if err != nil {
		return err
	}

	exitCode, err := containerHandler.Pytest(customImageName, pytestFile, "")
	if err != nil {
		if strings.Contains(exitCode, "1") { // exit code is 1 meaning tests failed
			return errors.New("pytests failed")
		}
		return err
	}

	fmt.Println("\nAll Pytests passed!")
	return err
}

func airflowParse(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	imageName, err := projectNameUnique(false)
	if err != nil {
		return err
	}

	containerHandler, err := containerHandlerInit(config.WorkingPath, envFile, dockerfile, imageName, true)
	if err != nil {
		return err
	}

	return containerHandler.Parse(customImageName, "")
}

// airflowUpgradeCheck
func airflowUpgradeCheck(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// Add airflow command, to simplify astro cli usage
	args = append(airflowUpgradeCheckCmd, args...)

	containerHandler, err := containerHandlerInit(config.WorkingPath, "", dockerfile, "", false)
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

	containerHandler, err := containerHandlerInit(config.WorkingPath, "", dockerfile, "", false)
	if err != nil {
		return err
	}

	fmt.Printf("Execing into the %s container\n\n", container)
	return containerHandler.Bash(container)
}

func prepareDefaultAirflowImageTag(airflowVersion string, httpClient *airflowversions.Client) string {
	defaultImageTag, _ := getDefaultImageTag(httpClient, airflowVersion)

	if defaultImageTag == "" {
		if useAstronomerCertified || (!context.IsCloudContext() && houston.VerifyVersionMatch(houstonVersion, houston.VersionRestrictions{GTE: "0.29.0"})) {
			fmt.Println("WARNING! There was a network issue getting the latest Astronomer Certified image. Your Dockerfile may not contain the latest version")
			defaultImageTag = airflowversions.DefaultAirflowVersion
		} else {
			fmt.Println("WARNING! There was a network issue getting the latest Astro Runtime image. Your Dockerfile may not contain the latest version")
			defaultImageTag = airflowversions.DefaultRuntimeVersion
		}
	}
	return defaultImageTag
}
