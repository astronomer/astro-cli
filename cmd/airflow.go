package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/astronomerio/astro-cli/pkg/input"
	"github.com/iancoleman/strcase"
	"github.com/pkg/errors"

	"github.com/astronomerio/astro-cli/messages"

	"github.com/spf13/cobra"

	"github.com/astronomerio/astro-cli/airflow"
	"github.com/astronomerio/astro-cli/config"
	"github.com/astronomerio/astro-cli/pkg/fileutil"
	"github.com/astronomerio/astro-cli/pkg/git"
)

var (
	projectName      string
	forceDeploy      bool
	saveDeployConfig bool
	forcePrompt      bool

	airflowRootCmd = &cobra.Command{
		Use:   "airflow",
		Short: "Manage airflow projects and deployments",
		Long:  "Airflow projects are a single top-level directory which represents a single production Airflow deployment",
	}

	airflowInitCmd = &cobra.Command{
		Use:   "init",
		Short: "Scaffold a new airflow project",
		Long:  "Scaffold a new airflow project directory. Will create the necessary files to begin development locally as well as be deployed to the Astronomer Platform.",
		RunE:  airflowInit,
	}

	airflowDeployCmd = &cobra.Command{
		Use:    "deploy DEPLOYMENT",
		Short:  "Deploy an airflow project",
		Long:   "Deploy an airflow project to a given deployment",
		Args:   cobra.MaximumNArgs(1),
		PreRun: ensureProjectDir,
		RunE:   airflowDeploy,
	}

	airflowStartCmd = &cobra.Command{
		Use:    "start",
		Short:  "Start a development airflow cluster",
		Long:   "Start a development airflow cluster",
		PreRun: ensureProjectDir,
		RunE:   airflowStart,
	}

	airflowKillCmd = &cobra.Command{
		Use:    "kill",
		Short:  "Kill a development airflow cluster",
		Long:   "Kill a development airflow cluster",
		PreRun: ensureProjectDir,
		RunE:   airflowKill,
	}

	airflowStopCmd = &cobra.Command{
		Use:    "stop",
		Short:  "Stop a development airflow cluster",
		Long:   "Stop a development airflow cluster",
		PreRun: ensureProjectDir,
		RunE:   airflowStop,
	}

	airflowPSCmd = &cobra.Command{
		Use:    "ps",
		Short:  "List airflow containers",
		Long:   "List airflow containers",
		PreRun: ensureProjectDir,
		RunE:   airflowPS,
	}
)

func init() {
	// Airflow root
	RootCmd.AddCommand(airflowRootCmd)

	// Airflow init
	airflowInitCmd.Flags().StringVarP(&projectName, "name", "n", "", "Name of airflow project")
	airflowRootCmd.AddCommand(airflowInitCmd)

	// Airflow deploy
	airflowRootCmd.AddCommand(airflowDeployCmd)
	airflowDeployCmd.Flags().BoolVarP(&forceDeploy, "force", "f", false, "Force deploy if uncommited changes")
	airflowDeployCmd.Flags().BoolVarP(&forcePrompt, "prompt", "p", false, "Force prompt to choose target deployment")
	airflowDeployCmd.Flags().BoolVarP(&saveDeployConfig, "save", "s", false, "Save deployment in config for future deploys")
	airflowDeployCmd.Flags().StringVar(&workspaceId, "workspace-id", "", "workspace assigned to deployment")

	// Airflow start
	airflowRootCmd.AddCommand(airflowStartCmd)

	// Airflow kill
	airflowRootCmd.AddCommand(airflowKillCmd)

	// Airflow stop
	airflowRootCmd.AddCommand(airflowStopCmd)

	// Airflow PS
	airflowRootCmd.AddCommand(airflowPSCmd)
}

func ensureProjectDir(cmd *cobra.Command, args []string) {
	if !config.IsProjectDir(config.WorkingPath) {
		fmt.Println(messages.CONFIG_PROJECT_DIR_ERROR)
		os.Exit(1)
	}
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

	emtpyDir := fileutil.IsEmptyDir(config.WorkingPath)
	if !emtpyDir {
		i, _ := input.InputConfirm(
			fmt.Sprintf("%s \nYou are not in an empty directory, are you you want to initialize a project?", config.WorkingPath))

		if !i {
			fmt.Println("Cancelling project initialization...\n")
			os.Exit(1)
		}
	}
	exists := config.ProjectConfigExists()
	if !exists {
		config.CreateProjectConfig(config.WorkingPath)
	}
	config.CFG.ProjectName.SetProjectString(projectName)
	airflow.Init(config.WorkingPath)

	if exists {
		fmt.Printf(messages.CONFIG_REINIT_PROJECT_CONFIG+"\n", config.WorkingPath)
	} else {
		fmt.Printf(messages.CONFIG_INIT_PROJECT_CONFIG+"\n", config.WorkingPath)
	}

	return nil
}

func airflowDeploy(cmd *cobra.Command, args []string) error {
	ws := workspaceValidator()

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
	return airflow.Deploy(config.WorkingPath, releaseName, ws, forcePrompt)
}

// Start an airflow cluster
func airflowStart(cmd *cobra.Command, args []string) error {
	return airflow.Start(config.WorkingPath)
}

// Kill an airflow cluster
func airflowKill(cmd *cobra.Command, args []string) error {
	return airflow.Kill(config.WorkingPath)
}

// Stop an airflow cluster
func airflowStop(cmd *cobra.Command, args []string) error {
	return airflow.Stop(config.WorkingPath)
}

// List containers of an airflow cluster
func airflowPS(cmd *cobra.Command, args []string) error {
	return airflow.PS(config.WorkingPath)
}
