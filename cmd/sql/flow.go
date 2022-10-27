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
	verbose           bool
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

func executeAbout(cmd *cobra.Command, args []string) error {
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

func executeVersion(cmd *cobra.Command, args []string) error {
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

func executeInit(cmd *cobra.Command, args []string) error {
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

func executeValidate(cmd *cobra.Command, args []string) error {
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

func executeGenerate(cmd *cobra.Command, args []string) error {
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

func executeRun(cmd *cobra.Command, args []string) error {
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

	if verbose {
		args = append(args, "--verbose")
	}

	cmdString := []string{cmd.Parent().Name(), cmd.Name()}
	err = sql.CommonDockerUtil(cmdString, args, vars, mountDirs)
	if err != nil {
		return fmt.Errorf("error running %v: %w", cmdString, err)
	}
	return nil
}

func executeHelp(cmd *cobra.Command, cmdString []string) {
	err := sql.CommonDockerUtil(cmdString, nil, nil, nil)
	if err != nil {
		panic(fmt.Errorf("error running %v: %w", cmdString, err))
	}
}

func aboutCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "about",
		Args:         cobra.MaximumNArgs(1),
		RunE:         executeAbout,
		SilenceUsage: true,
	}
	cmd.SetHelpFunc(executeHelp)
	return cmd
}

func versionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "version",
		Args:         cobra.MaximumNArgs(1),
		RunE:         executeVersion,
		SilenceUsage: true,
	}
	cmd.SetHelpFunc(executeHelp)
	return cmd
}

func initCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "init",
		Args:         cobra.MaximumNArgs(1),
		RunE:         executeInit,
		SilenceUsage: true,
	}
	cmd.SetHelpFunc(executeHelp)
	cmd.Flags().StringVarP(&airflowHome, "airflow_home", "a", "", "")
	cmd.Flags().StringVarP(&airflowDagsFolder, "airflow_dags_folder", "d", "", "")
	return cmd
}

func validateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "validate",
		Args:         cobra.MaximumNArgs(1),
		RunE:         executeValidate,
		SilenceUsage: true,
	}
	cmd.SetHelpFunc(executeHelp)
	cmd.Flags().StringVarP(&projectDir, "project_dir", "p", ".", "")
	cmd.Flags().StringVarP(&connection, "connection", "c", "", "")
	return cmd
}

func generateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "generate",
		Args:         cobra.MaximumNArgs(1),
		RunE:         executeGenerate,
		SilenceUsage: true,
	}
	cmd.SetHelpFunc(executeHelp)
	cmd.Flags().StringVarP(&projectDir, "project_dir", "p", ".", "")
	return cmd
}

func runCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "run",
		Args:         cobra.MaximumNArgs(1),
		RunE:         executeRun,
		SilenceUsage: true,
	}
	cmd.SetHelpFunc(executeHelp)
	cmd.Flags().StringVarP(&projectDir, "project_dir", "p", ".", "")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "")
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
		Run: func(cmd *cobra.Command, args []string) {
			executeHelp(cmd, []string{cmd.Name(), "--help"})
		},
		SilenceUsage: true,
	}
	cmd.SetHelpFunc(executeHelp)
	cmd.PersistentFlags().StringVarP(&environment, "env", "e", "", "")
	cmd.AddCommand(versionCommand())
	cmd.AddCommand(aboutCommand())
	cmd.AddCommand(initCommand())
	cmd.AddCommand(validateCommand())
	cmd.AddCommand(generateCommand())
	cmd.AddCommand(runCommand())
	return cmd
}
