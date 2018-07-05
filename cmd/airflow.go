package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/astronomerio/astro-cli/messages"

	"github.com/iancoleman/strcase"
	"github.com/spf13/cobra"

	"github.com/astronomerio/astro-cli/airflow"
	"github.com/astronomerio/astro-cli/config"
	"github.com/astronomerio/astro-cli/pkg/fileutil"
	"github.com/astronomerio/astro-cli/pkg/git"
)

var (
	projectRoot string
	projectName string
	forceDeploy bool

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
		Use:        "deploy",
		Short:      "Deploy an airflow project",
		Long:       "Deploy an airflow project to a given deployment",
		Args:       cobra.MaximumNArgs(1),
		PreRun:     ensureProjectDir,
		RunE:       airflowDeploy,
		Deprecated: fmt.Sprintf(messages.CLI_CMD_DEPRECATE, "astro deployment list"),
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
	// Set up project root
	projectRoot, _ = config.ProjectRoot()

	// Airflow root
	RootCmd.AddCommand(airflowRootCmd)

	// Airflow init
	airflowInitCmd.Flags().StringVarP(&projectName, "name", "n", "", "Name of airflow project")
	airflowRootCmd.AddCommand(airflowInitCmd)

	// Airflow deploy
	airflowRootCmd.AddCommand(airflowDeployCmd)
	airflowDeployCmd.Flags().BoolVarP(&forceDeploy, "force", "f", false, "Force deploy if uncommited changes")
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
	if !(len(projectRoot) > 0) {
		fmt.Println(messages.CONFIG_PROJECT_DIR_ERROR)
		os.Exit(1)
	}
}

// Use project name for image name
func airflowInit(cmd *cobra.Command, args []string) error {
	// Grab working directory
	path := fileutil.GetWorkingDir()

	// Validate project name
	if len(projectName) != 0 {
		projectNameValid := regexp.
			MustCompile(`^[A-Za-z0-9]([A-Za-z0-9_-]*[A-Za-z0-9])?$`).
			MatchString

		if !projectNameValid(projectName) {
			return errors.New(messages.CONFIG_PROJECT_NAME_ERROR)
		}
	} else {
		projectDirectory := filepath.Base(path)
		projectName = strings.Replace(strcase.ToSnake(projectDirectory), "_", "-", -1)
	}

	exists := config.ProjectConfigExists()
	if !exists {
		config.CreateProjectConfig(path)
	}
	config.CFG.ProjectName.SetProjectString(projectName)
	airflow.Init(path)

	if exists {
		fmt.Printf(messages.CONFIG_REINIT_PROJECT_CONFIG+"\n", path)
	} else {
		fmt.Printf(messages.CONFIG_INIT_PROJECT_CONFIG+"\n", path)
	}

	return nil
}

func airflowDeploy(cmd *cobra.Command, args []string) error {
	ws := workspaceValidator()

	releaseName := ""
	if len(args) > 0 {
		releaseName = args[0]
	}
	if git.HasUncommitedChanges() && !forceDeploy {
		fmt.Println(messages.REGISTRY_UNCOMMITTED_CHANGES)
		return nil
	}
	return airflow.Deploy(projectRoot, releaseName, ws)
}

// Start an airflow cluster
func airflowStart(cmd *cobra.Command, args []string) error {
	return airflow.Start(projectRoot)
}

// Kill an airflow cluster
func airflowKill(cmd *cobra.Command, args []string) error {
	return airflow.Kill(projectRoot)
}

// Stop an airflow cluster
func airflowStop(cmd *cobra.Command, args []string) error {
	return airflow.Stop(projectRoot)
}

// List containers of an airflow cluster
func airflowPS(cmd *cobra.Command, args []string) error {
	return airflow.PS(projectRoot)
}
