package sql

import (
	"github.com/astronomer/astro-cli/sql"
	"github.com/spf13/cobra"
)

var (
	environment string
	connection  string
)

func versionCmd() error {
	err := sql.CommonDockerUtil([]string{"flow", "version"}, map[string]string{})
	if err != nil {
		panic(err)
	}
	return nil
}

func initCmd() error {
	vars := make(map[string]string)
	if environment != "" {
		vars["environment"] = environment
	}
	err := sql.CommonDockerUtil([]string{"flow", "init"}, vars)
	if err != nil {
		panic(err)
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
	err := sql.CommonDockerUtil([]string{"flow", "validate"}, vars)
	if err != nil {
		panic(err)
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
	err := sql.CommonDockerUtil([]string{"flow", "generate"}, vars)
	if err != nil {
		panic(err)
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

	err := sql.CommonDockerUtil([]string{"flow", "run"}, vars)
	if err != nil {
		panic(err)
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
