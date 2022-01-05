package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/astronomer/astro-cli/airflow"
	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/messages"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/settings"

	"github.com/iancoleman/strcase"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
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

	runExample = `
# Create default admin user.
astro dev run users create -r Admin -u admin -e admin@example.com -f admin -l user -p admin
`

	// this is used to monkey patch the function in order to write unit test cases
	containerHandlerInit = airflow.ContainerHandlerInit
)

func newAirflowRootCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:        "airflow",
		Aliases:    []string{"a"},
		Short:      "Manage Airflow projects",
		Long:       "Airflow projects contain Airflow code and Deployment configuration",
		Deprecated: "please use `astro dev [subcommands] [flags]` instead",
	}
	cmd.AddCommand(
		newAirflowInitCmd(client, out),
		newAirflowDeployCmd(),
		newAirflowStartCmd(out),
		newAirflowKillCmd(out),
		newAirflowLogsCmd(out),
		newAirflowStopCmd(out),
		newAirflowPSCmd(out),
		newAirflowRunCmd(out),
	)
	return cmd
}

func newDevRootCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "dev",
		Aliases: []string{"d"},
		Short:   "Manage Airflow projects",
		Long:    "Airflow projects contain Airflow code and Deployment configuration",
	}
	cmd.AddCommand(
		newAirflowInitCmd(client, out),
		newAirflowDeployCmd(),
		newAirflowStartCmd(out),
		newAirflowKillCmd(out),
		newAirflowLogsCmd(out),
		newAirflowStopCmd(out),
		newAirflowPSCmd(out),
		newAirflowRunCmd(out),
		newAirflowUpgradeCheckCmd(out),
	)
	return cmd
}

func newAirflowInitCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a new Airflow project",
		Long:  "Scaffold a new Airflow project directory. Will create the necessary files to begin development locally as well as be deployed to Astronomer.",
		Args:  cobra.MaximumNArgs(1),
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			err := SetUpLogs(out, verboseLevel)
			return err
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return airflowInit(cmd, args, client, out)
		},
	}
	cmd.Flags().StringVarP(&projectName, "name", "n", "", "Name of airflow project")
	cmd.Flags().StringVarP(&airflowVersion, "airflow-version", "v", "", "Version of airflow you want to deploy")
	return cmd
}

func newAirflowDeployCmd() *cobra.Command {
	deployCmd := newDeployCmd()
	cmd := &cobra.Command{
		Use:     "deploy DEPLOYMENT",
		Short:   "Deploy an Airflow project",
		Long:    "Deploy an Airflow project to a given Deployment",
		Args:    cobra.MaximumNArgs(1),
		PreRunE: ensureProjectDir,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deployCmd.RunE(cmd, args)
		},
		Deprecated: "Please use new command instead `astro deploy DEPLOYMENT [flags]`",
	}
	cmd.Flags().BoolVarP(&forceDeploy, "force", "f", false, "Force deploy if uncommitted changes")
	cmd.Flags().BoolVarP(&forcePrompt, "prompt", "p", false, "Force prompt to choose target deployment")
	cmd.Flags().BoolVarP(&saveDeployConfig, "save", "s", false, "Save deployment in config for future deploys")
	cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "workspace assigned to deployment")
	return cmd
}

func newAirflowStartCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start an Airflow cluster locally using docker-compose",
		Long:  "Start an Airflow cluster locally using docker-compose",
		Args:  cobra.MaximumNArgs(1),
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			err := SetUpLogs(out, verboseLevel)
			return err
		},
		PreRunE: ensureProjectDir,
		RunE:    airflowStart,
	}
	cmd.Flags().StringVarP(&envFile, "env", "e", ".env", "Location of file containing environment variables")
	return cmd
}

func newAirflowKillCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kill",
		Short: "Kill a locally running Airflow cluster",
		Long:  "Kill a locally running Airflow cluster",
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			err := SetUpLogs(out, verboseLevel)
			return err
		},
		PreRunE: ensureProjectDir,
		RunE:    airflowKill,
	}
	return cmd
}

func newAirflowLogsCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Output logs for a locally running Airflow cluster",
		Long:  "Output logs for a locally running Airflow cluster",
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			err := SetUpLogs(out, verboseLevel)
			return err
		},
		PreRunE: ensureProjectDir,
		RunE:    airflowLogs,
	}
	cmd.Flags().BoolVarP(&followLogs, "follow", "f", false, "Follow log output")
	cmd.Flags().BoolVarP(&schedulerLogs, "scheduler", "s", false, "Output scheduler logs")
	cmd.Flags().BoolVarP(&webserverLogs, "webserver", "w", false, "Output webserver logs")
	return cmd
}

func newAirflowStopCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop a locally running Airflow cluster",
		Long:  "Stop a locally running Airflow cluster",
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			err := SetUpLogs(out, verboseLevel)
			return err
		},
		PreRunE: ensureProjectDir,
		RunE:    airflowStop,
	}
	return cmd
}

func newAirflowPSCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ps",
		Short: "List locally running Airflow containers",
		Long:  "List locally running Airflow containers",
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			err := SetUpLogs(out, verboseLevel)
			return err
		},
		PreRunE: ensureProjectDir,
		RunE:    airflowPS,
	}
	return cmd
}

func newAirflowRunCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a command inside locally running Airflow webserver",
		Long:  "Run a command inside locally running Airflow webserver",
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			err := SetUpLogs(out, verboseLevel)
			return err
		},
		PreRunE:            ensureProjectDir,
		RunE:               airflowRun,
		Example:            runExample,
		DisableFlagParsing: true,
	}
	return cmd
}

func newAirflowUpgradeCheckCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade-check",
		Short: "List DAG and config-level changes required to upgrade to Airflow 2.0",
		Long:  "List DAG and config-level changes required to upgrade to Airflow 2.0",
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			err := SetUpLogs(out, verboseLevel)
			return err
		},
		PreRunE:            ensureProjectDir,
		RunE:               airflowUpgradeCheck,
		Example:            runExample,
		DisableFlagParsing: true,
	}
	return cmd
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
func airflowInit(cmd *cobra.Command, _ []string, client *houston.Client, out io.Writer) error {
	// Validate project name
	if projectName != "" {
		projectNameValid := regexp.
			MustCompile(`^[A-Za-z0-9]([A-Za-z0-9_-]*[A-Za-z0-9])?$`).
			MatchString

		if !projectNameValid(projectName) {
			return errors.New(messages.ErrInvalidConfigProjectName)
		}
	} else {
		projectDirectory := filepath.Base(config.WorkingPath)
		projectName = strings.Replace(strcase.ToSnake(projectDirectory), "_", "-", -1)
	}
	httpClient := airflowversions.NewClient(httputil.NewHTTPClient())
	defaultImageTag, err := prepareDefaultAirflowImageTag(airflowVersion, httpClient, client, out)
	if err != nil {
		return err
	}

	emtpyDir := fileutil.IsEmptyDir(config.WorkingPath)
	if !emtpyDir {
		i, _ := input.Confirm(
			fmt.Sprintf("%s \nYou are not in an empty directory. Are you sure you want to initialize a project?", config.WorkingPath))

		if !i {
			fmt.Println("Canceling project initialization...")
			os.Exit(1)
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
	err = airflow.Init(config.WorkingPath, defaultImageTag)
	if err != nil {
		return err
	}

	if exists {
		fmt.Printf(messages.ConfigReinitProjectConfig+"\n", config.WorkingPath)
	} else {
		fmt.Printf(messages.ConfigInitProjectConfig+"\n", config.WorkingPath)
	}

	return nil
}

// Start an airflow cluster
func airflowStart(cmd *cobra.Command, args []string) error {
	dockerfile := "Dockerfile"
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// Get release name from args, if passed
	if len(args) > 0 {
		envFile = args[0]
	}

	containerHandler, err := containerHandlerInit(config.WorkingPath, envFile)
	if err != nil {
		return err
	}

	airflowDockerVersion, err := airflow.ParseVersionFromDockerFile(config.WorkingPath, dockerfile)
	if err != nil {
		return errors.Wrap(err, "error parsing airflow version from dockerfile")
	}

	err = containerHandler.Start(dockerfile)
	if err != nil {
		return err
	}

	fileState, err := fileutil.Exists("airflow_settings.yaml")
	if err != nil {
		return errors.Wrap(err, messages.SettingsPath)
	}

	if fileState {
		containerID, err := containerHandler.GetContainerID(config.CFG.WebserverContainerName.GetString())
		if err != nil {
			return errors.Wrap(err, messages.ErrContainerStatusCheck)
		}
		settings.ConfigSettings(containerHandler, containerID, airflowDockerVersion)
	}
	return nil
}

// Kill an airflow cluster
func airflowKill(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	containerHandler, err := containerHandlerInit(config.WorkingPath, envFile)
	if err != nil {
		return err
	}

	return containerHandler.Kill()
}

// Outputs logs for a development airflow cluster
func airflowLogs(cmd *cobra.Command, args []string) error {
	// default is to display all logs
	containersNames := make([]string, 0)
	// default is to display all logs
	if !schedulerLogs && !webserverLogs {
		containersNames = append(containersNames, []string{config.CFG.SchedulerContainerName.GetString(), config.CFG.WebserverContainerName.GetString()}...)
	}
	if schedulerLogs {
		containersNames = append(containersNames, config.CFG.SchedulerContainerName.GetString())
	}
	if webserverLogs {
		containersNames = append(containersNames, config.CFG.WebserverContainerName.GetString())
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	containerHandler, err := containerHandlerInit(config.WorkingPath, envFile)
	if err != nil {
		return err
	}

	return containerHandler.Logs(followLogs, containersNames...)
}

// Stop an airflow cluster
func airflowStop(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	containerHandler, err := containerHandlerInit(config.WorkingPath, envFile)
	if err != nil {
		return err
	}

	return containerHandler.Stop()
}

// List containers of an airflow cluster
func airflowPS(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	containerHandler, err := containerHandlerInit(config.WorkingPath, envFile)
	if err != nil {
		return err
	}

	return containerHandler.PS()
}

// airflowRun
func airflowRun(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// Add airflow command, to simplify astro cli usage
	args = append([]string{"airflow"}, args...)
	// ignore last user parameter

	containerHandler, err := containerHandlerInit(config.WorkingPath, envFile)
	if err != nil {
		return err
	}

	return containerHandler.Run(args, "")
}

// airflowUpgradeCheck
func airflowUpgradeCheck(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// Add airflow command, to simplify astro cli usage
	args = append([]string{"bash", "-c", "pip install --no-deps 'apache-airflow-upgrade-check'; python -c 'from packaging.version import Version\nfrom airflow import __version__\nif Version(__version__) < Version(\"1.10.14\"):\n  print(\"Please upgrade your image to Airflow 1.10.14 first, then try again.\");exit(1)\nelse:\n  from airflow.upgrade.checker import __main__;__main__()'"}, args...)

	containerHandler, err := containerHandlerInit(config.WorkingPath, envFile)
	if err != nil {
		return err
	}

	return containerHandler.Run(args, "root")
}

func acceptableVersion(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}
