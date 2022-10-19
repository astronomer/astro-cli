package sql

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/sql"
	"github.com/spf13/cobra"
)

var (
	environment       string
	connection        string
	airflowHome       string
	airflowDagsFolder string
	projectDir        string
	verbose           string
)

const flowCmd = "flow"

var (
	flowVersionCmd    = []string{flowCmd, "version"}
	flowInitCmd       = []string{flowCmd, "init"}
	flowValidateCmd   = []string{flowCmd, "validate"}
	flowGenerateCmd   = []string{flowCmd, "generate"}
	flowRunCmd        = []string{flowCmd, "run"}
	versionCmdExample = `
	# Get astro sql cli version
	astro flow version
	`
	initCmdExample = `
	# Initialize a project structure to write workflows using SQL files.
	astro flow init

	# Initialize a project in the current directory
	astro flow init .

	# Initialize a project with creating a directory with the given project name
	astro flow init project_name
	`
	validateCmdExample = `
	# Validate Airflow connection(s) provided in the configuration file from a directory.
	astro flow validate

	# Validate connections for a project
	astro flow validate project_name

	# Validate a specific connection
	astro flow validate --connection database_conn_id

	# Validate connection for a specific environment
	astro flow validate --environment production
	`
	generateCmdExample = `
	# Generate the Airflow DAG from a directory of SQL files
	astro flow generate

	# Generate for a project
	astro flow generate project_name
	`
	runCmdExample = `
	# Run a workflow locally from a project directory. This task assumes that there is a local airflow DB (can be a SQLite file),
	# that has been initialized with Airflow tables
	astro flow run

	# Run for a project
	astro flow run project_name

	# Run for a specific environment
	astro flow run --environment production
	`
)

func createProjectDir(projectDir string) (string, string, error) {
	if !filepath.IsAbs(projectDir) || projectDir == "" || projectDir == "." {
		currentDir, err := os.Getwd()
		if err != nil {
			err = fmt.Errorf("error getting current directory")
			return "", "", err
		}
		projectDir = filepath.Join(currentDir, projectDir)
	}

	err := os.MkdirAll(projectDir, os.ModePerm)
	if err != nil {
		err = fmt.Errorf("error creating project directory %s", projectDir)
		return "", "", err
	}

	absoluteProjectDirSplit := strings.Split(projectDir, "/")
	relativeProjectDir := absoluteProjectDirSplit[len(absoluteProjectDirSplit)-1]
	mountDir := strings.Join([]string{"", relativeProjectDir}, "/")

	return projectDir, mountDir, nil
}

func buildCommonVars() map[string]string {
	vars := make(map[string]string)
	if environment != "" {
		vars["env"] = environment
	}
	if airflowHome != "" {
		vars["airflow_home"] = airflowHome
	}
	if airflowDagsFolder != "" {
		vars["airflow_dags_folder"] = airflowDagsFolder
	}
	if connection != "" {
		vars["connection"] = connection
	}
	return vars
}

func versionCmd(args []string) error {
	absoluteProjectDir, mountDirectory, err := createProjectDir(projectDir)
	if err != nil {
		return err
	}
	vars := buildCommonVars()

	err = sql.CommonDockerUtil(flowVersionCmd, args, vars, absoluteProjectDir, mountDirectory)
	if err != nil {
		return fmt.Errorf("error running %v: %w", flowVersionCmd, err)
	}

	return nil
}

func initCmd(args []string) error {
	projectDir = "."
	if len(args) > 0 {
		projectDir = args[0]
	}

	absoluteProjectDir, mountDirectory, err := createProjectDir(projectDir)

	projectDirSplit := strings.Split(projectDir, "/")
	args[0] = strings.Join([]string{"", projectDirSplit[len(projectDirSplit)-1]}, "/")
	if err != nil {
		return err
	}
	vars := buildCommonVars()

	err = sql.CommonDockerUtil(flowInitCmd, args, vars, absoluteProjectDir, mountDirectory)
	if err != nil {
		return fmt.Errorf("error running %v: %w", flowInitCmd, err)
	}

	return nil
}

func validateCmd(args []string) error {
	absoluteProjectDir, mountDirectory, err := createProjectDir(projectDir)
	if err != nil {
		return err
	}

	vars := buildCommonVars()

	if len(args) == 0 {
		args = append(args, mountDirectory)
	}

	err = sql.CommonDockerUtil(flowValidateCmd, args, vars, absoluteProjectDir, mountDirectory)
	if err != nil {
		return fmt.Errorf("error running %v: %w", flowValidateCmd, err)
	}

	return nil
}

func generateCmd(args []string) error {
	absoluteProjectDir, mountDirectory, err := createProjectDir(projectDir)
	if err != nil {
		return err
	}
	vars := buildCommonVars()
	vars["project-dir"] = mountDirectory
	if len(args) < 1 {
		return fmt.Errorf("workflow argument not given")
	}
	err = sql.CommonDockerUtil(flowGenerateCmd, args, vars, absoluteProjectDir, mountDirectory)
	if err != nil {
		return fmt.Errorf("error running %v: %w", flowGenerateCmd, err)
	}

	return nil
}

func runCmd(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("argument workflow_name not given")
	}

	absoluteProjectDir, mountDirectory, err := createProjectDir(projectDir)
	if err != nil {
		return err
	}

	vars := buildCommonVars()
	vars["project-dir"] = mountDirectory
	if verbose != "" {
		vars["verbose"] = verbose
	}

	err = sql.CommonDockerUtil(flowRunCmd, args, vars, absoluteProjectDir, mountDirectory)
	if err != nil {
		return fmt.Errorf("error running %v: %w", flowRunCmd, err)
	}
	return nil
}

func NewFlowVersionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "version",
		Short:   "Get the version of flow being used",
		Long:    "Get the version of flow being used",
		Example: versionCmdExample,
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return versionCmd(args)
		},
	}

	return cmd
}

func NewFlowInitCommand() *cobra.Command { // nolint:dupl
	cmd := &cobra.Command{
		Use:     "init",
		Short:   "Initialize flow directory",
		Long:    "Initialize flow directory",
		Example: initCmdExample,
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return initCmd(args)
		},
	}
	cmd.Flags().StringVarP(&airflowHome, "airflow_home", "a", "", "Set the Airflow Home")
	cmd.Flags().StringVarP(&airflowDagsFolder, "airflow_dags_folder", "d", "", "Set the DAGs Folder")
	return cmd
}

func NewFlowValidateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "validate",
		Short:   "Validate connections",
		Long:    "Validate connections",
		Example: validateCmdExample,
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return validateCmd(args)
		},
	}
	cmd.Flags().StringVarP(&projectDir, "project_dir", "p", ".", "Directory of the project. Default: current directory")
	cmd.Flags().StringVarP(&connection, "connection", "c", "", "Identifier of the connection to be validated. By default checks all the env connections.")
	return cmd
}

func NewFlowGenerateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "generate",
		Short:   "Generate DAGs",
		Long:    "Generate DAGs",
		Example: generateCmdExample,
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return generateCmd(args)
		},
	}
	cmd.Flags().StringVarP(&projectDir, "project_dir", "p", ".", "Directory of the project. Default: current directory")

	return cmd
}

func NewFlowRunCommand() *cobra.Command { // nolint:dupl
	cmd := &cobra.Command{
		Use:     "run",
		Short:   "Run workflow",
		Long:    "Run workflow",
		Example: runCmdExample,
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCmd(args)
		},
	}
	cmd.Flags().StringVarP(&projectDir, "project_dir", "p", ".", "Directory of the project. Default: current directory")
	cmd.Flags().StringVarP(&verbose, "verbose", "v", "", "Boolean value indicating whether to show airflow logs")
	return cmd
}

func NewFlowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "flow",
		Short: "Run flow commands",
		Long:  "Forward flow subcommands to the flow python package",
	}
	cmd.PersistentFlags().StringVarP(&environment, "env", "e", "", "environment for the flow project")
	cmd.AddCommand(NewFlowVersionCommand())
	cmd.AddCommand(NewFlowInitCommand())
	cmd.AddCommand(NewFlowValidateCommand())
	cmd.AddCommand(NewFlowGenerateCommand())
	cmd.AddCommand(NewFlowRunCommand())

	return cmd
}
