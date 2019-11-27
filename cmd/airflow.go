package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/iancoleman/strcase"
	"github.com/pkg/errors"

	"github.com/astronomer/astro-cli/messages"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/fileutil"
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
astro dev run create_user -r Admin -u admin -e admin@example.com -f admin -l user -p admin
`
)

func newAirflowRootCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:        "airflow",
		Aliases:    []string{"a"},
		Short:      "Manage airflow projects",
		Long:       "Airflow projects are a single top-level directory which represents a single production Airflow deployment",
		Deprecated: "could please use new command instead `astro dev [subcommands] [flags]`",
	}
	cmd.AddCommand(
		newAirflowInitCmd(client, out),
		newAirflowDeployCmd(client, out),
		newAirflowStartCmd(client, out),
		newAirflowKillCmd(client, out),
		newAirflowLogsCmd(client, out),
		newAirflowStopCmd(client, out),
		newAirflowPSCmd(client, out),
		newAirflowRunCmd(client, out),
	)
	return cmd
}

func newDevRootCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "dev",
		Aliases: []string{"d"},
		Short:   "Manage airflow projects",
		Long:    "Airflow projects are a single top-level directory which represents a single production Airflow deployment",
	}
	cmd.AddCommand(
		newAirflowInitCmd(client, out),
		newAirflowDeployCmd(client, out),
		newAirflowStartCmd(client, out),
		newAirflowKillCmd(client, out),
		newAirflowLogsCmd(client, out),
		newAirflowStopCmd(client, out),
		newAirflowPSCmd(client, out),
		newAirflowRunCmd(client, out),
	)
	return cmd
}

func newAirflowInitCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a new airflow project",
		Long:  "Scaffold a new airflow project directory. Will create the necessary files to begin development locally as well as be deployed to the Astronomer Platform.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return airflowInit(cmd, args, client, out)
		},
	}
	cmd.Flags().StringVarP(&projectName, "name", "n", "", "Name of airflow project")
	cmd.Flags().StringVarP(&airflowVersion, "airflow-version", "v", "", "Version of airflow you want to deploy")
	return cmd
}

func newAirflowDeployCmd(client *houston.Client, out io.Writer) *cobra.Command {
	deployCmd := newDeployCmd(client, out)
	cmd := &cobra.Command{
		Use:     "deploy DEPLOYMENT",
		Short:   "Deploy an airflow project",
		Long:    "Deploy an airflow project to a given deployment",
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
	cmd.Flags().StringVar(&workspaceId, "workspace-id", "", "workspace assigned to deployment")
	return cmd
}

func newAirflowStartCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "start",
		Short:   "Start a development airflow cluster",
		Long:    "Start a development airflow cluster",
		Args:    cobra.MaximumNArgs(1),
		PreRunE: ensureProjectDir,
		RunE:    airflowStart,
	}
	cmd.Flags().StringVarP(&envFile, "env", "e", ".env", "Location of file containing environment variables")
	return cmd
}

func newAirflowKillCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "kill",
		Short:   "Kill a development airflow cluster",
		Long:    "Kill a development airflow cluster",
		PreRunE: ensureProjectDir,
		RunE:    airflowKill,
	}
	return cmd
}

func newAirflowLogsCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "logs",
		Short:   "Output logs for a development airflow cluster",
		Long:    "Output logs for a development airflow cluster",
		PreRunE: ensureProjectDir,
		RunE:    airflowLogs,
	}
	cmd.Flags().BoolVarP(&followLogs, "follow", "f", false, "Follow log output")
	cmd.Flags().BoolVarP(&schedulerLogs, "scheduler", "s", false, "Output scheduler logs")
	cmd.Flags().BoolVarP(&webserverLogs, "webserver", "w", false, "Output webserver logs")
	return cmd
}

func newAirflowStopCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "stop",
		Short:   "Stop a development airflow cluster",
		Long:    "Stop a development airflow cluster",
		PreRunE: ensureProjectDir,
		RunE:    airflowStop,
	}
	return cmd
}

func newAirflowPSCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ps",
		Short:   "List airflow containers",
		Long:    "List airflow containers",
		PreRunE: ensureProjectDir,
		RunE:    airflowPS,
	}
	return cmd
}

func newAirflowRunCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:                "run",
		Short:              "Run any command inside airflow webserver",
		Long:               "Run any command inside airflow webserver",
		PreRunE:            ensureProjectDir,
		RunE:               airflowRun,
		Example:            RunExample,
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
func airflowInit(cmd *cobra.Command, args []string, client *houston.Client, out io.Writer) error {
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

	r := houston.Request{
		Query: houston.DeploymentInfoRequest,
	}

	defaultImageTag := ""
	wsResp, err := r.DoWithClient(client)
	if err == nil {
		defaultImageTag = wsResp.Data.DeploymentConfig.DefaultAirflowImageTag
	}

	// TODO: @andriisoldatenko rethink or remove this logic
	// acceptableAirflowVersions := wsResp.Data.DeploymentConfig.AirflowVersions
	// if airflowVersion != "" && !acceptableVersion(airflowVersion, acceptableAirflowVersions) {
	//  	return errors.Errorf(messages.ERROR_INVALID_AIRFLOW_VERSION, strings.Join(acceptableAirflowVersions, ", "))
	// }

	if len(defaultImageTag) == 0 {
		defaultImageTag = "latest-onbuild"
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
	err = airflow.Init(config.WorkingPath, defaultImageTag)
	if err != nil {
		return err
	}

	if exists {
		fmt.Printf(messages.CONFIG_REINIT_PROJECT_CONFIG+"\n", config.WorkingPath)
	} else {
		fmt.Printf(messages.CONFIG_INIT_PROJECT_CONFIG+"\n", config.WorkingPath)
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
