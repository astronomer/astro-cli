package cmd

import (
	"fmt"
	"os"

	"github.com/astronomerio/astro-cli/airflow"
	"github.com/astronomerio/astro-cli/config"
	"github.com/spf13/cobra"
)

var (
	projectRoot string
	projectName string
	forceDeploy bool

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
		Use:    "deploy",
		Short:  "Deploy an airflow project",
		Long:   "Deploy an airflow project to a given deployment",
		Args:   cobra.ExactArgs(1),
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
	airflowDeployCmd.Flags().BoolVarP(&forceDeploy, "force-deploy", "f", false, "Force deploy if uncommited changes")

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
		fmt.Println("Error: Not in an astronomer project directory")
		os.Exit(1)
	}
}

// Use project name for image name
func airflowInit(cmd *cobra.Command, args []string) error {
	return airflow.Init(projectName)
}

func airflowCreate(cmd *cobra.Command, args []string) error {
	return airflow.Create()
}

func airflowDeploy(cmd *cobra.Command, args []string) error {
	if utils.HasUncommitedChanges() && !forceDeploy {
		fmt.Println("Project directory has uncommmited changes, use `astro airflow deploy [releaseName] -f` to force deploy.")
		return nil
	}

	return airflow.Deploy(projectRoot, args[0])
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
