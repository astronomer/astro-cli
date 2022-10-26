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
	flowVersionCmd  = []string{flowCmd, "version"}
	flowAboutCmd    = []string{flowCmd, "about"}
	flowInitCmd     = []string{flowCmd, "init"}
	flowValidateCmd = []string{flowCmd, "validate"}
	flowGenerateCmd = []string{flowCmd, "generate"}
	flowRunCmd      = []string{flowCmd, "run"}
)

func getLeafDirectory(directoryPath string) string {
	directoryPathSplit := strings.Split(directoryPath, "/")
	relativeProjectDir := directoryPathSplit[len(directoryPathSplit)-1]
	leafDirectory := "/" + relativeProjectDir
	return leafDirectory
}

func createProjectDir(projectDir string) (sourceMountDir, targetMountDir string, err error) {
	if !filepath.IsAbs(projectDir) || projectDir == "" || projectDir == "." {
		currentDir, err := os.Getwd()
		if err != nil {
			err = fmt.Errorf("error getting current directory %w", err)
			return "", "", err
		}
		projectDir = filepath.Join(currentDir, projectDir)
	}

	err = os.MkdirAll(projectDir, os.ModePerm)
	if err != nil {
		err = fmt.Errorf("error creating project directory %s: %w", projectDir, err)
		return "", "", err
	}

	mountDir := getLeafDirectory(projectDir)

	return projectDir, mountDir, nil
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

func getBaseMountVolumes(projectDir string) ([]sql.MountVolume, error) {
	projectMountSourceDir, projectMountTargetDir, err := createProjectDir(projectDir)
	if err != nil {
		return nil, err
	}
	volumeMounts := make([]sql.MountVolume, 0)
	volumeMounts = append(volumeMounts, sql.MountVolume{SourceDirectory: projectMountSourceDir, TargetDirectory: projectMountTargetDir})
	return volumeMounts, nil
}

func aboutCmd(args []string) error {
	vars := buildCommonVars()
	volumeMounts, err := getBaseMountVolumes(projectDir)
	if err != nil {
		return err
	}

	err = sql.CommonDockerUtil(flowAboutCmd, args, vars, volumeMounts)
	if err != nil {
		return fmt.Errorf("error running %v: %w", flowAboutCmd, err)
	}

	return nil
}

func versionCmd(args []string) error {
	vars := buildCommonVars()
	volumeMounts, err := getBaseMountVolumes(projectDir)
	if err != nil {
		return err
	}

	err = sql.CommonDockerUtil(flowVersionCmd, args, vars, volumeMounts)
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

	vars := buildCommonVars()

	volumeMounts, err := getBaseMountVolumes(projectDir)
	if err != nil {
		return err
	}

	if airflowHome != "" {
		airflowMountTargetDir := getLeafDirectory(airflowHome)
		volumeMounts = append(volumeMounts, sql.MountVolume{SourceDirectory: airflowHome, TargetDirectory: airflowMountTargetDir})
		vars["airflow-home"] = airflowMountTargetDir
	}

	if airflowDagsFolder != "" {
		dagsMountTargetDir := getLeafDirectory(airflowDagsFolder)
		volumeMounts = append(volumeMounts, sql.MountVolume{SourceDirectory: airflowDagsFolder, TargetDirectory: dagsMountTargetDir})
		vars["airflow-dags-folder"] = dagsMountTargetDir
	}

	args = []string{volumeMounts[0].TargetDirectory}

	if err != nil {
		return err
	}

	err = sql.CommonDockerUtil(flowInitCmd, args, vars, volumeMounts)
	if err != nil {
		return fmt.Errorf("error running %v: %w", flowInitCmd, err)
	}

	return nil
}

func validateCmd(args []string) error {
	vars := buildCommonVars()
	volumeMounts, err := getBaseMountVolumes(projectDir)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		args = append(args, volumeMounts[0].TargetDirectory)
	}

	err = sql.CommonDockerUtil(flowValidateCmd, args, vars, volumeMounts)
	if err != nil {
		return fmt.Errorf("error running %v: %w", flowValidateCmd, err)
	}

	return nil
}

func generateCmd(args []string) error {
	if len(args) < 1 {
		return sql.ArgNotSetError("workflow_name")
	}

	vars := buildCommonVars()
	volumeMounts, err := getBaseMountVolumes(projectDir)
	if err != nil {
		return err
	}
	vars["project-dir"] = volumeMounts[0].TargetDirectory

	err = sql.CommonDockerUtil(flowGenerateCmd, args, vars, volumeMounts)
	if err != nil {
		return fmt.Errorf("error running %v: %w", flowGenerateCmd, err)
	}

	return nil
}

func runCmd(args []string) error {
	if len(args) < 1 {
		return sql.ArgNotSetError("workflow_name")
	}

	vars := buildCommonVars()
	volumeMounts, err := getBaseMountVolumes(projectDir)
	if err != nil {
		return err
	}
	vars["project-dir"] = volumeMounts[0].TargetDirectory
	if verbose != "" {
		vars["verbose"] = verbose
	}

	err = sql.CommonDockerUtil(flowRunCmd, args, vars, volumeMounts)
	if err != nil {
		return fmt.Errorf("error running %v: %w", flowRunCmd, err)
	}
	return nil
}

func help(c *cobra.Command, s []string) {
	cmdName := c.Name()
	cmd := []string{flowCmd, cmdName}
	if cmdName == flowCmd {
		cmd = []string{flowCmd}
	}
	sql.CommonDockerUtil(cmd, []string{"--help"}, nil, nil) // nolint:errcheck
}

func NewFlowAboutCommand() *cobra.Command { // nolint:dupl
	cmd := &cobra.Command{
		Use:  "about",
		Args: cobra.MaximumNArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return aboutCmd(args)
		},
	}
	cmd.SetHelpFunc(help)
	return cmd
}

func NewFlowVersionCommand() *cobra.Command { // nolint:dupl
	cmd := &cobra.Command{
		Use:  "version",
		Args: cobra.MaximumNArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return versionCmd(args)
		},
	}
	cmd.SetHelpFunc(help)
	return cmd
}

func NewFlowInitCommand() *cobra.Command { // nolint:dupl
	cmd := &cobra.Command{
		Use:  "init",
		Args: cobra.MaximumNArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return initCmd(args)
		},
	}
	cmd.SetHelpFunc(help)
	cmd.Flags().StringVarP(&airflowHome, "airflow_home", "a", "", "Set the Airflow Home")
	cmd.Flags().StringVarP(&airflowDagsFolder, "airflow_dags_folder", "d", "", "Set the DAGs Folder")
	return cmd
}

func NewFlowValidateCommand() *cobra.Command { // nolint:dupl
	cmd := &cobra.Command{
		Use:  "validate",
		Args: cobra.MaximumNArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return validateCmd(args)
		},
	}
	cmd.SetHelpFunc(help)
	cmd.Flags().StringVarP(&projectDir, "project_dir", "p", ".", "Directory of the project. Default: current directory")
	cmd.Flags().StringVarP(&connection, "connection", "c", "", "Identifier of the connection to be validated. By default checks all the env connections.")
	return cmd
}

func NewFlowGenerateCommand() *cobra.Command { // nolint:dupl
	cmd := &cobra.Command{
		Use:  "generate",
		Args: cobra.MaximumNArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return generateCmd(args)
		},
	}
	cmd.SetHelpFunc(help)
	cmd.Flags().StringVarP(&projectDir, "project_dir", "p", ".", "Directory of the project. Default: current directory")
	return cmd
}

func NewFlowRunCommand() *cobra.Command { // nolint:dupl
	cmd := &cobra.Command{
		Use:  "run",
		Args: cobra.MaximumNArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCmd(args)
		},
	}
	cmd.SetHelpFunc(help)
	cmd.Flags().StringVarP(&projectDir, "project_dir", "p", ".", "Directory of the project. Default: current directory")
	cmd.Flags().StringVarP(&verbose, "verbose", "v", "", "Boolean value indicating whether to show airflow logs")
	return cmd
}

func NewFlowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "flow",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	cmd.SetHelpFunc(help)
	cmd.PersistentFlags().StringVarP(&environment, "env", "e", "", "environment for the flow project")
	cmd.AddCommand(NewFlowVersionCommand())
	cmd.AddCommand(NewFlowAboutCommand())
	cmd.AddCommand(NewFlowInitCommand())
	cmd.AddCommand(NewFlowValidateCommand())
	cmd.AddCommand(NewFlowGenerateCommand())
	cmd.AddCommand(NewFlowRunCommand())
	return cmd
}
