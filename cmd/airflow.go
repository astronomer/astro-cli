package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/astronomerio/astro-cli/airflow"
	"github.com/astronomerio/astro-cli/config"
	"github.com/astronomerio/astro-cli/utils"
	"github.com/iancoleman/strcase"
	"github.com/spf13/cobra"
)

var (
	projectName string

	airflowRootCmd = &cobra.Command{
		Use:   "airflow",
		Short: "Manage airflow projects and deployments",
		Long:  "Manage airflow projects and deployments",
	}

	airflowInitCmd = &cobra.Command{
		Use:   "init",
		Short: "Scaffold a new airflow project",
		Long:  "Scaffold a new airflow project",
		RunE:  airflowInit,
	}

	airflowCreateCmd = &cobra.Command{
		Use:   "create",
		Short: "Create a new airflow deployment",
		Long:  "Create a new airflow deployment",
		RunE:  airflowCreate,
	}

	airflowDeployCmd = &cobra.Command{
		Use:   "deploy",
		Short: "Deploy an airflow project",
		Long:  "Deploy an airflow project to a given deployment",
		Args:  cobra.ExactArgs(2),
		RunE:  airflowDeploy,
	}

	airflowStatusCmd = &cobra.Command{
		Use:   "status",
		Short: "Print the status of an airflow deployment",
		Long:  "Print the status of an airflow deployment",
		RunE:  airflowStatus,
	}

	airflowStartCmd = &cobra.Command{
		Use:   "start",
		Short: "Start a development airflow cluster",
		Long:  "Start a development airflow cluster",
		RunE:  airflowStart,
	}

	airflowStopCmd = &cobra.Command{
		Use:   "stop",
		Short: "Stop a development airflow cluster",
		Long:  "Stop a development airflow cluster",
		RunE:  airflowStop,
	}

	airflowPSCmd = &cobra.Command{
		Use:   "ps",
		Short: "List airflow containers",
		Long:  "List airflow containers",
		RunE:  airflowPS,
	}
)

func init() {
	// Airflow root
	RootCmd.AddCommand(airflowRootCmd)

	// Airflow init
	airflowInitCmd.Flags().StringVarP(&projectName, "name", "n", "", "Name of airflow project")
	airflowRootCmd.AddCommand(airflowInitCmd)

	// Airflow create
	airflowRootCmd.AddCommand(airflowCreateCmd)

	// Airflow deploy
	airflowRootCmd.AddCommand(airflowDeployCmd)

	// Airflow status
	airflowRootCmd.AddCommand(airflowStatusCmd)

	// Airflow start
	airflowRootCmd.AddCommand(airflowStartCmd)

	// Airflow stop
	airflowRootCmd.AddCommand(airflowStopCmd)

	// Airflow PS
	airflowRootCmd.AddCommand(airflowPSCmd)
}

// projectRoot returns the project root
func projectRoot() string {
	path, err := config.ProjectRoot()
	if err != nil {
		fmt.Printf("Error searching for project dir: %v\n", err)
		os.Exit(1)
	}
	return path
}

// TODO: allow specify directory and/or project name (store in .astro/config)
// Use project name for image name
func airflowInit(cmd *cobra.Command, args []string) error {
	// Grab working directory
	path := utils.GetWorkingDir()

	// Validate project name
	if len(projectName) != 0 {
		projectNameValid := regexp.
			MustCompile(`^[A-Za-z0-9]([A-Za-z0-9_-]*[A-Za-z0-9])?$`).
			MatchString

		if !projectNameValid(projectName) {
			return errors.New("Project name is invalid")
		}
	} else {
		projectDirectory := filepath.Base(path)
		projectName = strings.Replace(strcase.ToSnake(projectDirectory), "_", "-", -1)
	}

	exists := config.ProjectConfigExists()
	if !exists {
		config.CreateProjectConfig(path)
	}
	config.SetProjectString(config.CFGProjectName, projectName)
	airflow.Init(path)

	if exists {
		fmt.Printf("Reinitialized existing astronomer project in %s\n", path)
	} else {
		fmt.Printf("Initialized empty astronomer project in %s\n", path)
	}

	return nil
}

func airflowCreate(cmd *cobra.Command, args []string) error {
	return airflow.Create()
}

func airflowDeploy(cmd *cobra.Command, args []string) error {
	return airflow.Deploy(projectRoot(), args[0], args[1])
}

// Get airflow status
func airflowStatus(cmd *cobra.Command, args []string) error {
	return nil
}

// Start airflow
func airflowStart(cmd *cobra.Command, args []string) error {
	return airflow.Start(projectRoot())
}

// Stop airflow
func airflowStop(cmd *cobra.Command, args []string) error {
	return airflow.Stop(projectRoot())
}

// Airflow PS
func airflowPS(cmd *cobra.Command, args []string) error {
	return airflow.PS(projectRoot())
}
