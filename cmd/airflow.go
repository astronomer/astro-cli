package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/iancoleman/strcase"
	"github.com/pkg/errors"

	"github.com/astronomer/astro-cli/messages"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/git"
)

var (
	projectName      string
	airflowVersion   string
	envFile          string
	followLogs       bool
	forceDeploy      bool
	forcePrompt      bool
	saveDeployConfig bool
	schedulerLogs    bool
	webserverLogs    bool

	RunExample = `
  # Create default admin user.
  astro airflow run create_user -r Admin -u admin -e admin@example.com -f admin -l user -p admin
`

	airflowRootCmd = &cobra.Command{
		Use:   "airflow",
		Short: "Manage airflow projects and deployments",
		Long:  "Airflow projects are a single top-level directory which represents a single production Airflow deployment",
	}

	airflowInitCmd = &cobra.Command{
		Use:   "init",
		Short: "Scaffold a new airflow project",
		Long:  "Scaffold a new airflow project directory. Will create the necessary files to begin development locally as well as be deployed to the Astronomer Platform.",
		Args:  cobra.MaximumNArgs(1),
		RunE:  airflowInit,
	}

	airflowDeployCmd = &cobra.Command{
		Use:     "deploy DEPLOYMENT",
		Short:   "Deploy an airflow project",
		Long:    "Deploy an airflow project to a given deployment",
		Args:    cobra.MaximumNArgs(1),
		PreRunE: ensureProjectDir,
		RunE:    airflowDeploy,
	}

	airflowStartCmd = &cobra.Command{
		Use:     "start",
		Short:   "Start a development airflow cluster",
		Long:    "Start a development airflow cluster",
		Args:    cobra.MaximumNArgs(1),
		PreRunE: ensureProjectDir,
		RunE:    airflowStart,
	}

	airflowKillCmd = &cobra.Command{
		Use:     "kill",
		Short:   "Kill a development airflow cluster",
		Long:    "Kill a development airflow cluster",
		PreRunE: ensureProjectDir,
		RunE:    airflowKill,
	}

	airflowLogsCmd = &cobra.Command{
		Use:     "logs",
		Short:   "Output logs for a development airflow cluster",
		Long:    "Output logs for a development airflow cluster",
		PreRunE: ensureProjectDir,
		RunE:    airflowLogs,
	}

	airflowStopCmd = &cobra.Command{
		Use:     "stop",
		Short:   "Stop a development airflow cluster",
		Long:    "Stop a development airflow cluster",
		PreRunE: ensureProjectDir,
		RunE:    airflowStop,
	}

	airflowPSCmd = &cobra.Command{
		Use:     "ps",
		Short:   "List airflow containers",
		Long:    "List airflow containers",
		PreRunE: ensureProjectDir,
		RunE:    airflowPS,
	}

	airflowRunCmd = &cobra.Command{
		Use:                "run",
		Short:              "Run any command inside airflow webserver",
		Long:               "Run any command inside airflow webserver",
		PreRunE:            ensureProjectDir,
		RunE:               airflowRun,
		Example:            RunExample,
		DisableFlagParsing: true,
	}
)

func init() {
	// Airflow root
	RootCmd.AddCommand(airflowRootCmd)

	// Airflow init
	airflowInitCmd.Flags().StringVarP(&projectName, "name", "n", "", "Name of airflow project")
	airflowInitCmd.Flags().StringVarP(&airflowVersion, "airflow-version", "v", "", "Version of airflow you want to deploy")
	airflowRootCmd.AddCommand(airflowInitCmd)

	// Airflow deploy
	airflowRootCmd.AddCommand(airflowDeployCmd)
	airflowDeployCmd.Flags().BoolVarP(&forceDeploy, "force", "f", false, "Force deploy if uncommited changes")
	airflowDeployCmd.Flags().BoolVarP(&forcePrompt, "prompt", "p", false, "Force prompt to choose target deployment")
	airflowDeployCmd.Flags().BoolVarP(&saveDeployConfig, "save", "s", false, "Save deployment in config for future deploys")
	airflowDeployCmd.Flags().StringVar(&workspaceId, "workspace-id", "", "workspace assigned to deployment")

	// Airflow start
	airflowRootCmd.AddCommand(airflowStartCmd)
	airflowStartCmd.Flags().StringVarP(&envFile, "env", "e", ".env", "Location of file containing environment variables")

	// Airflow kill
	airflowRootCmd.AddCommand(airflowKillCmd)

	// Airflow logs
	airflowRootCmd.AddCommand(airflowLogsCmd)
	airflowLogsCmd.Flags().BoolVarP(&followLogs, "follow", "f", false, "Follow log output")
	airflowLogsCmd.Flags().BoolVarP(&schedulerLogs, "scheduler", "s", false, "Output scheduler logs")
	airflowLogsCmd.Flags().BoolVarP(&webserverLogs, "webserver", "w", false, "Output webserver logs")

	// Airflow stop
	airflowRootCmd.AddCommand(airflowStopCmd)

	// Airflow PS
	airflowRootCmd.AddCommand(airflowPSCmd)

	// Airflow Run
	airflowRootCmd.AddCommand(airflowRunCmd)
}

func ensureProjectDir(cmd *cobra.Command, args []string) error {
	isProjectDir, err := config.IsProjectDir(config.WorkingPath)
	if err != nil {
		return errors.Wrap(err, "cannot ensure is a project directory")
	}

	if !isProjectDir {
		return errors.New("not in a project directory")
	}

	projectConfigFile := filepath.Join(config.WorkingPath, config.ConfigDir, config.ConfigFileNameWithExt)

	configExists, err := fileutil.Exists(projectConfigFile)
	if err != nil {
		return errors.Wrapf(err, "failed to check existence of '%s'", projectConfigFile)
	}

	if !configExists {
		return errors.New("project config file does not exists")
	}

	return nil
}

// Use project name for image name
func airflowInit(cmd *cobra.Command, args []string) error {
	// Validate project name
	if len(projectName) != 0 {
		projectNameValid := regexp.
			MustCompile(`^[A-Za-z0-9]([A-Za-z0-9_-]*[A-Za-z0-9])?$`).
			MatchString

		if !projectNameValid(projectName) {
			return errors.New(messages.CONFIG_PROJECT_NAME_ERROR)
		}
	} else {
		projectDirectory := filepath.Base(config.WorkingPath)
		projectName = strings.Replace(strcase.ToSnake(projectDirectory), "_", "-", -1)
	}

	acceptableAirflowVersions := []string{"1.9.0", "1.10.3"}

	if airflowVersion != "" && !acceptableVersion(airflowVersion, acceptableAirflowVersions) {
		return errors.Errorf(messages.ERROR_INVALID_AIRFLOW_VERSION, strings.Join(acceptableAirflowVersions, ", "))
	}

	emtpyDir := fileutil.IsEmptyDir(config.WorkingPath)
	if !emtpyDir {
		i, _ := input.InputConfirm(
			fmt.Sprintf("%s \nYou are not in an empty directory. Are you sure you want to initialize a project?", config.WorkingPath))

		if !i {
			fmt.Println("Cancelling project initialization...")
			os.Exit(1)
		}
	}
	exists := config.ProjectConfigExists()
	if !exists {
		config.CreateProjectConfig(config.WorkingPath)
	}
	config.CFG.ProjectName.SetProjectString(projectName)

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// Execute method
	airflow.Init(config.WorkingPath, airflowVersion)

	if exists {
		fmt.Printf(messages.CONFIG_REINIT_PROJECT_CONFIG+"\n", config.WorkingPath)
	} else {
		fmt.Printf(messages.CONFIG_INIT_PROJECT_CONFIG+"\n", config.WorkingPath)
	}

	return nil
}

func airflowDeploy(cmd *cobra.Command, args []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
		// fmt.Println("Default workspace id not set, set default workspace id or pass a workspace in via the --workspace-id flag")
	}

	releaseName := ""

	// Get release name from args, if passed
	if len(args) > 0 {
		releaseName = args[0]
	}

	// Save releasename in config if specified
	if len(releaseName) > 0 && saveDeployConfig {
		config.CFG.ProjectDeployment.SetProjectString(releaseName)
	}

	if git.HasUncommitedChanges() && !forceDeploy {
		fmt.Println(messages.REGISTRY_UNCOMMITTED_CHANGES)
		return nil
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return airflow.Deploy(config.WorkingPath, releaseName, ws, forcePrompt)
}

// Start an airflow cluster
func airflowStart(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// Get release name from args, if passed
	if len(args) > 0 {
		envFile = args[0]
	}

	return airflow.Start(config.WorkingPath, envFile)
}

// Kill an airflow cluster
func airflowKill(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return airflow.Kill(config.WorkingPath)
}

// Outputs logs for a development airflow cluster
func airflowLogs(cmd *cobra.Command, args []string) error {
	// default is to display all logs
	if !schedulerLogs && !webserverLogs {
		schedulerLogs = true
		webserverLogs = true
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return airflow.Logs(config.WorkingPath, webserverLogs, schedulerLogs, followLogs)
}

// Stop an airflow cluster
func airflowStop(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return airflow.Stop(config.WorkingPath)
}

// List containers of an airflow cluster
func airflowPS(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return airflow.PS(config.WorkingPath)
}

// airflowRun
func airflowRun(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// Add airflow command, to simplify astro cli usage
	args = append([]string{"airflow"}, args...)
	return airflow.Run(config.WorkingPath, args)
}

func acceptableVersion(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}
