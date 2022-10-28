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

func buildCommonFlags() map[string]string {
	flags := make(map[string]string)
	if environment != "" {
		flags["env"] = environment
	}
	if connection != "" {
		flags["connection"] = connection
	}
	return flags
}

func getBaseMountDirs(projectDir string) ([]string, error) {
	mountDir, err := createProjectDir(projectDir)
	if err != nil {
		return nil, err
	}
	mountDirs := []string{mountDir}
	return mountDirs, nil
}

func buildflagsAndMountDirs(projectDir string, setProjectDir, setAirflowHome, setAirflowDagsFolder bool) (flags map[string]string, mountDirs []string, err error) {
	flags = buildCommonFlags()
	mountDirs, err = getBaseMountDirs(projectDir)
	if err != nil {
		return nil, nil, err
	}

	if setProjectDir {
		projectDir, err = getAbsolutePath(projectDir)
		if err != nil {
			return nil, nil, err
		}
		flags["project-dir"] = projectDir
	}

	if setAirflowHome && airflowHome != "" {
		airflowHomeAbs, err := getAbsolutePath(airflowHome)
		if err != nil {
			return nil, nil, err
		}
		flags["airflow-home"] = airflowHomeAbs
		mountDirs = append(mountDirs, airflowHomeAbs)
	}

	if setAirflowDagsFolder && airflowDagsFolder != "" {
		airflowDagsFolderAbs, err := getAbsolutePath(airflowDagsFolder)
		if err != nil {
			return nil, nil, err
		}
		flags["airflow-dags-folder"] = airflowDagsFolderAbs
		mountDirs = append(mountDirs, airflowDagsFolderAbs)

	}

	return flags, mountDirs, nil
}

func executeCmd(cmd *cobra.Command, args []string, flags map[string]string, mountDirs []string) error {
	cmdString := []string{cmd.Parent().Name(), cmd.Name()}
	err := sql.CommonDockerUtil(cmdString, args, flags, mountDirs)
	if err != nil {
		return fmt.Errorf("error running %v: %w", cmdString, err)
	}

	return nil
}

func executeBase(cmd *cobra.Command, args []string) error {
	flags, mountDirs, err := buildflagsAndMountDirs(projectDir, false, false, false)
	if err != nil {
		return err
	}
	return executeCmd(cmd, args, flags, mountDirs)
}

func executeInit(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		projectDir = args[0]
	}

	flags, mountDirs, err := buildflagsAndMountDirs(projectDir, false, true, true)
	if err != nil {
		return err
	}

	args = []string{mountDirs[0]}

	return executeCmd(cmd, args, flags, mountDirs)
}

func executeValidate(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		projectDir = args[0]
	}

	flags, mountDirs, err := buildflagsAndMountDirs(projectDir, false, false, false)
	if err != nil {
		return err
	}

	args = []string{mountDirs[0]}

	return executeCmd(cmd, args, flags, mountDirs)
}

func executeGenerate(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return sql.ArgNotSetError("workflow_name")
	}

	flags, mountDirs, err := buildflagsAndMountDirs(projectDir, true, false, false)
	if err != nil {
		return err
	}

	return executeCmd(cmd, args, flags, mountDirs)
}

func executeRun(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return sql.ArgNotSetError("workflow_name")
	}

	flags, mountDirs, err := buildflagsAndMountDirs(projectDir, true, false, false)
	if err != nil {
		return err
	}

	if verbose {
		args = append(args, "--verbose")
	}

	return executeCmd(cmd, args, flags, mountDirs)
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
		RunE:         executeBase,
		SilenceUsage: true,
	}
	cmd.SetHelpFunc(executeHelp)
	return cmd
}

func versionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "version",
		Args:         cobra.MaximumNArgs(1),
		RunE:         executeBase,
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
	cmd.Flags().StringVarP(&airflowHome, "airflow-home", "a", "", "")
	cmd.Flags().StringVarP(&airflowDagsFolder, "airflow-dags-folder", "d", "", "")
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
	cmd.Flags().StringVarP(&projectDir, "project-dir", "p", ".", "")
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
	cmd.Flags().StringVarP(&projectDir, "project-dir", "p", ".", "")
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
		Short:             "Run flow commands",
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
