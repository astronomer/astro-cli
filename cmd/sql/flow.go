package sql

import (
	"fmt"
	"os"
	"path/filepath"

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

func getAbsolutePath(path string) (string, error) {
	if !filepath.IsAbs(path) || path == "" || path == "." {
		currentDir, err := os.Getwd()
		if err != nil {
			err = fmt.Errorf("error getting current directory %w", err)
			return "", err
		}
		path = filepath.Join(currentDir, path)
	}
	return path, nil
}

func createProjectDir(projectDir string) (mountDir string, err error) {
	projectDir, err = getAbsolutePath(projectDir)
	if err != nil {
		return "", err
	}

	err = os.MkdirAll(projectDir, os.ModePerm)
	if err != nil {
		err = fmt.Errorf("error creating project directory %s: %w", projectDir, err)
		return "", err
	}

	return projectDir, nil
}

func buildCommonVars() map[string]string {
	vars := make(map[string]string)
	if environment != "" {
		vars["env"] = environment
	}
	if connection != "" {
		vars["connection"] = connection
	}
	return vars
}

func getBaseMountDirs(projectDir string) ([]string, error) {
	mountDir, err := createProjectDir(projectDir)
	if err != nil {
		return nil, err
	}
	mountDirs := []string{mountDir}
	return mountDirs, nil
}

func aboutCmd(cmd *cobra.Command, args []string) error {
	vars := buildCommonVars()
	mountDirs, err := getBaseMountDirs(projectDir)
	if err != nil {
		return err
	}

	cmdString := []string{cmd.Parent().Name(), cmd.Name()}
	err = sql.CommonDockerUtil(cmdString, args, vars, mountDirs)
	if err != nil {
		return fmt.Errorf("error running %v: %w", cmdString, err)
	}

	return nil
}

func versionCmd(cmd *cobra.Command, args []string) error {
	vars := buildCommonVars()
	mountDirs, err := getBaseMountDirs(projectDir)
	if err != nil {
		return err
	}

	cmdString := []string{cmd.Parent().Name(), cmd.Name()}
	err = sql.CommonDockerUtil(cmdString, args, vars, mountDirs)
	if err != nil {
		return fmt.Errorf("error running %v: %w", cmdString, err)
	}

	return nil
}

func initCmd(cmd *cobra.Command, args []string) error {
	projectDir = "."
	if len(args) > 0 {
		projectDir = args[0]
	}

	vars := buildCommonVars()

	mountDirs, err := getBaseMountDirs(projectDir)
	args = mountDirs

	if err != nil {
		return err
	}

	if airflowHome != "" {
		mountDirs = append(mountDirs, airflowHome)
		vars["airflow-home"] = airflowHome
	}

	if airflowDagsFolder != "" {
		mountDirs = append(mountDirs, airflowDagsFolder)
		vars["airflow-dags-folder"] = airflowDagsFolder
	}

	cmdString := []string{cmd.Parent().Name(), cmd.Name()}
	err = sql.CommonDockerUtil(cmdString, args, vars, mountDirs)
	if err != nil {
		return fmt.Errorf("error running %v: %w", cmdString, err)
	}

	return nil
}

func validateCmd(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		projectDir = args[0]
	}

	vars := buildCommonVars()

	mountDirs, err := getBaseMountDirs(projectDir)
	if err != nil {
		return err
	}
	args = mountDirs

	cmdString := []string{cmd.Parent().Name(), cmd.Name()}
	err = sql.CommonDockerUtil(cmdString, args, vars, mountDirs)
	if err != nil {
		return fmt.Errorf("error running %v: %w", cmdString, err)
	}

	return nil
}

func generateCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return sql.ArgNotSetError("workflow_name")
	}

	vars := buildCommonVars()
	mountDirs, err := getBaseMountDirs(projectDir)
	if err != nil {
		return err
	}

	projectDir, err = getAbsolutePath(projectDir)
	if err != nil {
		return err
	}
	vars["project-dir"] = projectDir

	cmdString := []string{cmd.Parent().Name(), cmd.Name()}
	err = sql.CommonDockerUtil(cmdString, args, vars, mountDirs)
	if err != nil {
		return fmt.Errorf("error running %v: %w", cmdString, err)
	}

	return nil
}

func runCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return sql.ArgNotSetError("workflow_name")
	}

	vars := buildCommonVars()
	mountDirs, err := getBaseMountDirs(projectDir)
	if err != nil {
		return err
	}

	projectDir, err = getAbsolutePath(projectDir)
	if err != nil {
		return err
	}
	vars["project-dir"] = projectDir

	if verbose != "" {
		vars["verbose"] = verbose
	}

	cmdString := []string{cmd.Parent().Name(), cmd.Name()}
	err = sql.CommonDockerUtil(cmdString, args, vars, mountDirs)
	if err != nil {
		return fmt.Errorf("error running %v: %w", cmdString, err)
	}
	return nil
}

func help(cmd *cobra.Command, cmdString []string) {
	err := sql.CommonDockerUtil(cmdString, nil, nil, nil)
	if err != nil {
		panic(fmt.Errorf("error running %v: %w", cmdString, err))
	}
}

func NewFlowAboutCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "about",
		Args: cobra.MaximumNArgs(1),
		RunE: aboutCmd,
	}
	cmd.SetHelpFunc(help)
	return cmd
}

func NewFlowVersionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "version",
		Args: cobra.MaximumNArgs(1),
		RunE: versionCmd,
	}
	cmd.SetHelpFunc(help)
	return cmd
}

func NewFlowInitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "init",
		Args: cobra.MaximumNArgs(1),
		RunE: initCmd,
	}
	cmd.SetHelpFunc(help)
	cmd.Flags().StringVarP(&airflowHome, "airflow_home", "a", "", "")
	cmd.Flags().StringVarP(&airflowDagsFolder, "airflow_dags_folder", "d", "", "")
	return cmd
}

func NewFlowValidateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "validate",
		Args: cobra.MaximumNArgs(1),
		RunE: validateCmd,
	}
	cmd.SetHelpFunc(help)
	cmd.Flags().StringVarP(&projectDir, "project_dir", "p", ".", "")
	cmd.Flags().StringVarP(&connection, "connection", "c", "", "")
	return cmd
}

func NewFlowGenerateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "generate",
		Args: cobra.MaximumNArgs(1),
		RunE: generateCmd,
	}
	cmd.SetHelpFunc(help)
	cmd.Flags().StringVarP(&projectDir, "project_dir", "p", ".", "")
	return cmd
}

func NewFlowRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "run",
		Args: cobra.MaximumNArgs(1),
		RunE: runCmd,
	}
	cmd.SetHelpFunc(help)
	cmd.Flags().StringVarP(&projectDir, "project_dir", "p", ".", "")
	cmd.Flags().StringVarP(&verbose, "verbose", "v", "", "")
	return cmd
}

func login(cmd *cobra.Command, args []string) error {
	// flow currently does not require login
	return nil
}

func NewFlowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "flow",
		PersistentPreRunE: login,
	}
	cmd.SetHelpFunc(help)
	cmd.PersistentFlags().StringVarP(&environment, "env", "e", "", "")
	cmd.AddCommand(NewFlowVersionCommand())
	cmd.AddCommand(NewFlowAboutCommand())
	cmd.AddCommand(NewFlowInitCommand())
	cmd.AddCommand(NewFlowValidateCommand())
	cmd.AddCommand(NewFlowGenerateCommand())
	cmd.AddCommand(NewFlowRunCommand())
	return cmd
}
