package cmd

import (
	"github.com/astronomerio/astro-cli/airflow"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	deploymentName string
	deploymentTag  string

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
)

func init() {
	// Airflow root
	RootCmd.AddCommand(airflowRootCmd)

	// Airflow create
	airflowRootCmd.AddCommand(airflowCreateCmd)

	// Airflow deploy
	airflowDeployCmd.Flags().StringVarP(&deploymentName, "name", "n", "", "Name of airflow deployment")
	airflowDeployCmd.Flags().StringVarP(&deploymentTag, "version", "v", "", "Version of airflow deployment")
	viper.BindPFlag("name", airflowDeployCmd.Flags().Lookup("name"))
	viper.BindPFlag("version", airflowDeployCmd.Flags().Lookup("version"))
	airflowRootCmd.AddCommand(airflowDeployCmd)

	// Airflow status
	airflowRootCmd.AddCommand(airflowStatusCmd)
}

func airflowInit(cmd *cobra.Command, args []string) {
	airflow.Create()
}

func airflowCreate(cmd *cobra.Command, args []string) {
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
