package cmd

import (
	"errors"
	"fmt"
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
	projectRoot string
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
		Args:  cobra.ExactArgs(1),
		RunE:  checkForProject(airflowDeploy),
	}

	airflowStartCmd = &cobra.Command{
		Use:   "start",
		Short: "Start a development airflow cluster",
		Long:  "Start a development airflow cluster",
		RunE:  checkForProject(airflowStart),
	}

	airflowStopCmd = &cobra.Command{
		Use:   "stop",
		Short: "Stop a development airflow cluster",
		Long:  "Stop a development airflow cluster",
		RunE:  checkForProject(airflowStop),
	}

	airflowPSCmd = &cobra.Command{
		Use:   "ps",
		Short: "List airflow containers",
		Long:  "List airflow containers",
		RunE:  checkForProject(airflowPS),
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

	// Airflow create
	airflowRootCmd.AddCommand(airflowCreateCmd)

	// Airflow deploy
	airflowRootCmd.AddCommand(airflowDeployCmd)

	// Airflow start
	airflowRootCmd.AddCommand(airflowStartCmd)

	// Airflow stop
	airflowRootCmd.AddCommand(airflowStopCmd)

	// Airflow PS
	airflowRootCmd.AddCommand(airflowPSCmd)
}

// Check for project wraps functions that can only be run within a project directory
// and will return an error otherwise.
func checkForProject(f func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if len(projectRoot) > 0 {
			return f(cmd, args)
		}
		return errors.New("Not in an astronomer project directory")
	}
}

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
	return airflow.Deploy(projectRoot, args[0])
}

// Start airflow
func airflowStart(cmd *cobra.Command, args []string) error {
	return airflow.Start(projectRoot)
}

// Stop airflow
func airflowStop(cmd *cobra.Command, args []string) error {
	return airflow.Stop(projectRoot)
}

// Airflow PS
func airflowPS(cmd *cobra.Command, args []string) error {
	return airflow.PS(projectRoot)
}
