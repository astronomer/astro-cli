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

const (
	versionCmd  = "flow version"
	validateCmd = "flow validate"
)

func version(cmd string) error {
	fmt.Println("hello version")
	sql.CommonDockerUtil([]string{"flow", "version"}, map[string]string{})

	return nil
}

func validate(cmd string) error {
	fmt.Println("hello validate")
	sql.CommonDockerUtil([]string{"flow", "validate"}, map[string]string{"environment": environment, "connection": connection})

	return nil
}

func NewFlowVersionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Get the version of flow being used",
		Long:  "Get the version of flow being used",
		RunE: func(cmd *cobra.Command, args []string) error {
			return version(versionCmd)
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
			return validate(validateCmd)
		},
	}
	cmd.Flags().StringVarP(&connection, "connection", "c", "", "Connection to use for the validate command")
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
	cmd.AddCommand(NewFlowValidateCommand())

	return cmd
}
