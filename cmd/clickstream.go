package cmd

import (
	"github.com/spf13/cobra"
)

var (

	clickstreamRootCmd = &cobra.Command{
		Use:   "clickstream",
		Short: "Manage clickstream projects and deployments",
		Long:  "Manage clickstream projects and deployments",
	}

  clickstreamInitCmd = &cobra.Command{
		Use:   "init",
		Short: "Create a new clickstream project",
		Long:  "Create a new clickstream project",
		Run:   clickstreamInit,
	}

	clickstreamCreateCmd = &cobra.Command{
		Use:   "create",
		Short: "Create a new clickstream deployment",
		Long:  "Create a new clickstream deployment",
		Run:   clickstreamCreate,
	}

	clickstreamDeployCmd = &cobra.Command{
		Use:   "deploy",
		Short: "Deploy a clickstream project",
		Long:  "Deploy a clickstream project to a given deployment",
		Args:  cobra.ExactArgs(2),
		Run:   clickstreamDeploy,
	}

	clickstreamStatusCmd = &cobra.Command{
		Use:   "status",
		Short: "Print the status of a clickstream deployment",
		Long:  "Print the status of a clickstream deployment",
		Run:   clickstreamStatus,
	}
)

func init() {
	// Clickstream root
	RootCmd.AddCommand(clickstreamRootCmd)

	// Clickstream init
	clickstreamRootCmd.AddCommand(clickstreamInitCmd)

	// Clickstream create
	clickstreamRootCmd.AddCommand(clickstreamCreateCmd)

	// Clickstream deploy
	clickstreamRootCmd.AddCommand(clickstreamDeployCmd)

	// Clickstream status
	clickstreamRootCmd.AddCommand(clickstreamStatusCmd)
}

func clickstreamInit(cmd *cobra.Command, args []string) {
}

func clickstreamCreate(cmd *cobra.Command, args []string) {
}

func clickstreamDeploy(cmd *cobra.Command, args []string) {
}

func clickstreamStatus(cmd *cobra.Command, args []string) {
}
