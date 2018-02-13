package cmd

import (
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
		Run:   airflowInit,
	}

	airflowCreateCmd = &cobra.Command{
		Use:   "create",
		Short: "Create a new airflow deployment",
		Long:  "Create a new airflow deployment",
		Run:   airflowCreate,
	}

	airflowDeployCmd = &cobra.Command{
		Use:   "deploy",
		Short: "Deploy an airflow project",
		Long:  "Deploy an airflow project to a given deployment",
		Args:  cobra.ExactArgs(2),
		Run:   airflowDeploy,
	}

	airflowStatusCmd = &cobra.Command{
		Use:   "status",
		Short: "Print the status of an airflow deployment",
		Long:  "Print the status of an airflow deployment",
		Run:   airflowStatus,
	}

	airflowStartCmd = &cobra.Command{
		Use:   "start",
		Short: "Start a development airflow cluster",
		Long:  "Start a development airflow cluster",
		Run:   airflowStart,
	}

	airflowStopCmd = &cobra.Command{
		Use:   "stop",
		Short: "Stop a development airflow cluster",
		Long:  "Stop a development airflow cluster",
		Run:   airflowStop,
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
}

// TODO: allow specify directory and/or project name (store in .astro/config)
// Use project name for image name
func airflowInit(cmd *cobra.Command, args []string) {
	// Grab working directory
	path := utils.GetWorkingDir()

	// Validate project name
	if len(projectName) != 0 {
		projectNameValid := regexp.
			MustCompile(`^[A-Za-z0-9]([A-Za-z0-9_-]*[A-Za-z0-9])?$`).
			MatchString

		if !projectNameValid(projectName) {
			fmt.Println("Project name is invalid")
			return
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
}

func airflowCreate(cmd *cobra.Command, args []string) {
	fmt.Println(config.GetString(config.CFGProjectName))
	airflow.Create()
}

func airflowDeploy(cmd *cobra.Command, args []string) {
	deploymentName := args[0]
	deploymentTag := args[1]

	airflow.Build(deploymentName, deploymentTag)
	airflow.Deploy(deploymentName, deploymentTag)
}

func airflowStatus(cmd *cobra.Command, args []string) {
}

func airflowStart(cmd *cobra.Command, args []string) {
	// Grab working directory
	path := utils.GetWorkingDir()

	err := airflow.Start(path)
	if err != nil {
		fmt.Println(err)
	}
}

func airflowStop(cmd *cobra.Command, args []string) {
	// Grab working directory
	path := utils.GetWorkingDir()

	err := airflow.Stop(path)
	if err != nil {
		fmt.Println(err)
	}
}
