package sql

import (
	"fmt"

	"github.com/astronomer/astro-cli/sql"
	"github.com/spf13/cobra"
)

var (
	environment string
	connection  string
)

const flowCmd = "flow"

var (
	flowVersionCmd  = []string{flowCmd, "version"}
	flowInitCmd     = []string{flowCmd, "init"}
	flowValidateCmd = []string{flowCmd, "validate"}
	flowGenerateCmd = []string{flowCmd, "generate"}
	flowRunCmd      = []string{flowCmd, "run"}
)

func versionCmd() error {
	err := sql.CommonDockerUtil(flowVersionCmd, map[string]string{})
	if err != nil {
		return fmt.Errorf("error running %v: %w", flowVersionCmd, err)
	}
	return nil
}

func initCmd() error {
	vars := make(map[string]string)
	if environment != "" {
		vars["environment"] = environment
	}
	err := sql.CommonDockerUtil(flowInitCmd, vars)
	if err != nil {
		return fmt.Errorf("error running %v: %w", flowInitCmd, err)
	}

	return nil
}

func validateCmd() error {
	vars := make(map[string]string)
	if environment != "" {
		vars["environment"] = environment
	}
	if connection != "" {
		vars["connection"] = connection
	}
	err := sql.CommonDockerUtil(flowValidateCmd, vars)
	if err != nil {
		return fmt.Errorf("error running %v: %w", flowValidateCmd, err)
	}
	return nil
}

func generateCmd() error {
	vars := make(map[string]string)
	if environment != "" {
		vars["environment"] = environment
	}
	if connection != "" {
		vars["connection"] = connection
	}
	err := sql.CommonDockerUtil(flowGenerateCmd, vars)
	if err != nil {
		return fmt.Errorf("error running %v: %w", flowGenerateCmd, err)
	}
	return nil
}

func runCmd() error {
	vars := make(map[string]string)
	if environment != "" {
		vars["environment"] = environment
	}
	if connection != "" {
		vars["connection"] = connection
	}

	err := sql.CommonDockerUtil(flowRunCmd, vars)
	if err != nil {
		return fmt.Errorf("error running %v: %w", flowRunCmd, err)
	}
	return nil
}

func NewFlowVersionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Get the version of flow being used",
		Long:  "Get the version of flow being used",
		RunE: func(cmd *cobra.Command, args []string) error {
			return versionCmd()
		},
	}

	return cmd
}

func NewFlowInitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize flow directory",
		Long:  "Initialize flow directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			return initCmd()
		},
	}
	return cmd
}

func NewFlowValidateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate connections",
		Long:  "Validate connections",
		RunE: func(cmd *cobra.Command, args []string) error {
			return validateCmd()
		},
	}
	cmd.Flags().StringVarP(&connection, "connection", "c", "", "Connection to use for the validate command")
	return cmd
}

func NewFlowGenerateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate DAGs",
		Long:  "Generate DAGs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return generateCmd()
		},
	}
	return cmd
}

func NewFlowRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run workflow",
		Long:  "Run workflow",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCmd()
		},
	}
	cmd.Flags().StringVarP(&connection, "connection", "c", "", "Connection to use for the run command")
	return cmd
}

func NewFlowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "flow",
		Short: "Run flow commands",
		Long:  "Forward flow subcommands to the flow python package",
	}
	cmd.PersistentFlags().StringVarP(&environment, "env", "e", "default", "Environment for the flow project")
	cmd.AddCommand(NewFlowVersionCommand())
	cmd.AddCommand(NewFlowInitCommand())
	cmd.AddCommand(NewFlowValidateCommand())
	cmd.AddCommand(NewFlowGenerateCommand())
	cmd.AddCommand(NewFlowRunCommand())

	return cmd
}
