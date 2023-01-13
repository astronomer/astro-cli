package sql

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/astronomer/astro-cli/cmd/cloud"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/sql"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slices"
)

// Reference to reusable cobra commands
var configCmd *cobra.Command
var generateCmd *cobra.Command

// All cmd names
const (
	flowCmdName        = "flow"
	aboutCmdName       = "about"
	configCmdName      = "config"
	generateCmdName    = "generate"
	initCmdName        = "init"
	runCmdName         = "run"
	validateCmdName    = "validate"
	versionCmdName     = "version"
	deployCmdName      = "deploy"
	runtimeImagePrefix = "quay.io/astronomer/astro-runtime:"
)

// All cmd flags
var (
	airflowDagsFolder string
	airflowHome       string
	connection        string
	dataDir           string
	debug             bool
	env               string
	generateTasks     bool
	noDebug           bool
	noGenerateTasks   bool
	noVerbose         bool
	outputDir         string
	projectDir        string
	verbose           bool
	workflowName      string
)

const (
	astroDockerfilePath       = "Dockerfile"
	astroRequirementsfilePath = "requirements.txt"
)

var (
	ErrNoBaseAstroRuntimeImage = errors.New("base image is not an Astro runtime image in the provided Dockerfile")
	ErrPythonSDKVersionNotMet  = errors.New("required version for Python SDK dependency not met")
)

var astroRuntimeVersionRegex = regexp.MustCompile(runtimeImagePrefix + "([^-]*)")

func getAstroDockerfileRuntimeVersion() (string, error) {
	file, err := os.Open(astroDockerfilePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Scan()
	text := scanner.Text()
	if !strings.Contains(text, runtimeImagePrefix) {
		return "", ErrNoBaseAstroRuntimeImage
	}

	runtimeVersion := astroRuntimeVersionRegex.FindStringSubmatch(text)[1]

	return runtimeVersion, nil
}

// Build the cmd string to execute
func buildCmd(cmd *cobra.Command, args []string) ([]string, error) {
	globalCmdArgs := initGlobalCmdArgs()
	localCmdArgs, err := initLocalCmdArgs(cmd, args)
	if err != nil {
		return nil, err
	}
	localCmdArgs = extendLocalCmdArgsWithFlags(cmd, localCmdArgs)
	return append(append(globalCmdArgs, cmd.Name()), localCmdArgs...), nil
}

// Initialize persistent/global flags inserted before the cmd
func initGlobalCmdArgs() []string {
	var args []string
	if debug {
		args = append(args, "--debug")
	}
	if noDebug {
		args = append(args, "--no-debug")
	}
	return args
}

// Initialize specific cmd args by setting the cmd flags, resolving filepaths and overwriting args
func initLocalCmdArgs(cmd *cobra.Command, args []string) ([]string, error) {
	var err error
	switch cmd.Name() {
	case initCmdName:
		projectDir, err = resolvePath(getProjectDirFromArgs(args))
		if err != nil {
			return nil, err
		}
		if airflowHome != "" {
			airflowHome, err = resolvePath(airflowHome)
			if err != nil {
				return nil, err
			}
		}
		if airflowDagsFolder != "" {
			airflowDagsFolder, err = resolvePath(airflowDagsFolder)
			if err != nil {
				return nil, err
			}
		}
		if dataDir != "" {
			dataDir, err = resolvePath(dataDir)
			if err != nil {
				return nil, err
			}
		}
		return []string{projectDir}, nil
	case configCmdName:
		projectDir, err = resolvePath(projectDir)
		if err != nil {
			return nil, err
		}
	case validateCmdName:
		projectDir, err = resolvePath(getProjectDirFromArgs(args))
		if err != nil {
			return nil, err
		}
		return []string{projectDir}, nil
	case generateCmdName:
		projectDir, err = resolvePath(projectDir)
		if err != nil {
			return nil, err
		}
		if outputDir != "" {
			outputDir, err = resolvePath(outputDir)
			if err != nil {
				return nil, err
			}
		}
	case runCmdName:
		projectDir, err = resolvePath(projectDir)
		if err != nil {
			return nil, err
		}
	}
	return args, nil
}

// Get the projectDir flag from args if given
func getProjectDirFromArgs(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return "."
}

// Read config cmd output for retrieving config settings such as airflow_home
func readConfigCmdOutput(key string) (string, error) {
	args := []string{"get", key}
	output, err := readCmdOutput(configCmd, args)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil // remove spaces such as \r\n
}

// Resolve filepath to absolute
func resolvePath(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("error resolving path %v: %w", path, err)
	}
	return path, nil
}

// Extends args with flags e.g. "--project-dir ." or "--verbose"
func extendLocalCmdArgsWithFlags(cmd *cobra.Command, args []string) []string {
	switch cmd.Name() {
	case initCmdName:
		if airflowHome != "" {
			args = append(args, "--airflow-home", airflowHome)
		}
		if airflowDagsFolder != "" {
			args = append(args, "--airflow-dags-folder", airflowDagsFolder)
		}
		if dataDir != "" {
			args = append(args, "--data-dir", dataDir)
		}
	case configCmdName:
		args = append(args, "--project-dir", projectDir, "--env", env)
	case validateCmdName:
		args = append(args, "--env", env, "--connection", connection)
		if verbose {
			args = append(args, "--verbose")
		}
		if noVerbose {
			args = append(args, "--no-verbose")
		}
	case generateCmdName:
		args = append(args, "--project-dir", projectDir, "--env", env)
		if generateTasks {
			args = append(args, "--generate-tasks")
		}
		if noGenerateTasks {
			args = append(args, "--no-generate-tasks")
		}
		if verbose {
			args = append(args, "--verbose")
		}
		if noVerbose {
			args = append(args, "--no-verbose")
		}
		if outputDir != "" {
			args = append(args, "--output-dir", outputDir)
		}
	case runCmdName:
		args = append(args, "--project-dir", projectDir, "--env", env)
		if generateTasks {
			args = append(args, "--generate-tasks")
		}
		if noGenerateTasks {
			args = append(args, "--no-generate-tasks")
		}
		if verbose {
			args = append(args, "--verbose")
		}
		if noVerbose {
			args = append(args, "--no-verbose")
		}
	}
	return args
}

// Create mounts for a given cmd
func createMounts(cmd *cobra.Command) ([]string, error) {
	dirs, err := getDirs(cmd)
	if err != nil {
		return nil, err
	}
	return resolvePathsAndMakeDirs(dirs)
}

// Get all directories for a given cmd
func getDirs(cmd *cobra.Command) ([]string, error) {
	var dirs []string
	var err error
	switch cmd.Name() {
	case initCmdName:
		dirs = append(dirs, projectDir)
		if airflowHome != "" {
			dirs = append(dirs, airflowHome)
		}
		if airflowDagsFolder != "" {
			dirs = append(dirs, airflowDagsFolder)
		}
		if dataDir != "" {
			dirs = append(dirs, dataDir)
		}
	case configCmdName:
		dirs = append(dirs, projectDir)
	case validateCmdName:
		dirs = append(dirs, projectDir)
		airflowHome, err = readConfigCmdOutput("airflow_home")
		if err != nil {
			return nil, err
		}
		dirs = append(dirs, airflowHome)
		dataDir, err = readConfigCmdOutput("data_dir")
		if err != nil {
			return nil, err
		}
		dirs = append(dirs, dataDir)
	case generateCmdName:
		dirs = append(dirs, projectDir)
		if outputDir != "" {
			dirs = append(dirs, outputDir)
		}
		airflowHome, err = readConfigCmdOutput("airflow_home")
		if err != nil {
			return nil, err
		}
		dirs = append(dirs, airflowHome)
		airflowDagsFolder, err = readConfigCmdOutput("airflow_dags_folder")
		if err != nil {
			return nil, err
		}
		dirs = append(dirs, airflowDagsFolder)
		dataDir, err = readConfigCmdOutput("data_dir")
		if err != nil {
			return nil, err
		}
		dirs = append(dirs, dataDir)
	case runCmdName:
		dirs = append(dirs, projectDir)
		airflowHome, err = readConfigCmdOutput("airflow_home")
		if err != nil {
			return nil, err
		}
		dirs = append(dirs, airflowHome)
		airflowDagsFolder, err = readConfigCmdOutput("airflow_dags_folder")
		if err != nil {
			return nil, err
		}
		dirs = append(dirs, airflowDagsFolder)
		dataDir, err = readConfigCmdOutput("data_dir")
		if err != nil {
			return nil, err
		}
		dirs = append(dirs, dataDir)
	}
	return dirs, nil
}

// Resolve dirs to absolute and create them
func resolvePathsAndMakeDirs(dirs []string) ([]string, error) {
	resolvedDirs := make([]string, len(dirs))
	index := 0
	for _, dir := range dirs {
		absPath, err := resolvePath(dir)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(resolvedDirs, absPath) {
			if err := os.MkdirAll(absPath, os.ModePerm); err != nil {
				return resolvedDirs, fmt.Errorf("error creating directories for %v: %w", absPath, err)
			}
			resolvedDirs[index] = absPath
			index++
		}
	}
	return resolvedDirs, nil
}

// Execute cobra cmd with args and write to stdout
func executeCmd(cmd *cobra.Command, args []string) error {
	cmdString, err := buildCmd(cmd, args)
	if err != nil {
		return err
	}
	mountDirs, err := createMounts(cmd)
	if err != nil {
		return err
	}
	exitCode, _, err := sql.ExecuteCmdInDocker(cmdString, mountDirs, false)
	if err != nil {
		return fmt.Errorf("error running %v: %w", cmdString, err)
	}
	if exitCode != 0 {
		return sql.DockerNonZeroExitCodeError(exitCode)
	}
	return nil
}

func executeDeployCmd(cmd *cobra.Command, args []string) error {
	astroRuntimeVersion, err := getAstroDockerfileRuntimeVersion()
	if err != nil {
		return err
	}
	SQLCLIVersion, err := sql.GetPypiVersion(sql.AstroSQLCLIProjectURL)
	if err != nil {
		return err
	}
	requiredRuntimeVersion, requiredPythonSDKVersion, err := sql.GetPythonSDKComptability(sql.AstroSQLCLIConfigURL, SQLCLIVersion)
	if err != nil {
		return err
	}
	runtimeVersionMet, err := utils.IsRequiredVersionMet(astroRuntimeVersion, requiredRuntimeVersion)
	if err != nil {
		return err
	}
	if !runtimeVersionMet {
		pythonSDKPromptContent := input.PromptContent{
			ErrorMsg: "Please say y/n.",
			Label:    fmt.Sprintf("Would you like to add the required version %s of Python SDK dependency to requirements.txt? Otherwise, the deployment will not proceed.", requiredPythonSDKVersion),
		}
		result, err := input.PromptGetConfirmation(pythonSDKPromptContent)
		if err != nil {
			return err
		}
		if !result {
			return ErrPythonSDKVersionNotMet
		}
		requiredPythonSDKDependency := "\nastro-sdk-python==" + requiredPythonSDKVersion
		b, err := os.ReadFile(astroRequirementsfilePath)
		if err != nil {
			return err
		}
		existingRequirements := string(b)
		if !strings.Contains(existingRequirements, requiredPythonSDKDependency) {
			f, err := os.OpenFile(astroRequirementsfilePath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, sql.FileWriteMode)
			if err != nil {
				return err
			}

			defer f.Close()
			if _, err = f.WriteString(requiredPythonSDKDependency); err != nil {
				return err
			}
		}
	}
	projectDir, err = resolvePath(projectDir)
	if err != nil {
		return err
	}
	dagsPath := filepath.Join(projectDir, ".deploy/"+env+"/dags")
	if err := os.MkdirAll(dagsPath, os.ModePerm); err != nil {
		return fmt.Errorf("error creating directories for %v: %w", dagsPath, err)
	}
	generateCmdArgs := []string{"--output-dir", dagsPath}
	if workflowName != "" {
		generateCmdArgs = append(generateCmdArgs, workflowName)
		err = executeCmd(generateCmd, generateCmdArgs)
		if err != nil {
			return err
		}
	} else {
		items, _ := os.ReadDir("workflows")
		for _, item := range items {
			if item.IsDir() {
				err = executeCmd(generateCmd, append(generateCmdArgs, item.Name()))
				if err != nil {
					return err
				}
			}
		}
	}

	astroDeployCmd := cloud.NewDeployCmd()
	astroDeployArgs := []string{"--dags-path", dagsPath}
	astroDeployCmd.SetArgs(astroDeployArgs)

	_, err = astroDeployCmd.ExecuteC()
	if err != nil {
		return err
	}

	return nil
}

// Execute cobra cmd with args and return output
func readCmdOutput(cmd *cobra.Command, args []string) (string, error) {
	cmdString, err := buildCmd(cmd, args)
	if err != nil {
		return "", err
	}
	mountDirs, err := createMounts(cmd)
	if err != nil {
		return "", err
	}
	exitCode, output, err := sql.ExecuteCmdInDocker(cmdString, mountDirs, true)
	if err != nil {
		return "", fmt.Errorf("error running %v: %w", cmdString, err)
	}
	if exitCode != 0 {
		return "", sql.DockerNonZeroExitCodeError(exitCode)
	}
	outputString, err := sql.ConvertReadCloserToString(output)
	if err != nil {
		return "", err
	}
	return outputString, nil
}

// Execute help cmd
func executeHelp(cmd *cobra.Command, args []string) {
	var cmdString []string
	if cmd.Name() != flowCmdName {
		cmdString = []string{cmd.Name(), "--help"}
	}
	exitCode, _, err := sql.ExecuteCmdInDocker(cmdString, nil, false)
	if err != nil {
		panic(fmt.Errorf("error running %v: %w", cmdString, err))
	}
	if exitCode != 0 {
		panic(sql.DockerNonZeroExitCodeError(exitCode))
	}
}

func aboutCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          aboutCmdName,
		Args:         cobra.MaximumNArgs(1),
		RunE:         executeCmd,
		SilenceUsage: true,
	}
	cmd.SetHelpFunc(executeHelp)
	return cmd
}

func versionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          versionCmdName,
		Args:         cobra.MaximumNArgs(1),
		RunE:         executeCmd,
		SilenceUsage: true,
	}
	cmd.SetHelpFunc(executeHelp)
	return cmd
}

func initCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          initCmdName,
		RunE:         executeCmd,
		SilenceUsage: true,
	}
	cmd.SetHelpFunc(executeHelp)
	cmd.Flags().StringVar(&airflowHome, "airflow-home", "", "")
	cmd.Flags().StringVar(&airflowDagsFolder, "airflow-dags-folder", "", "")
	cmd.Flags().StringVar(&dataDir, "data-dir", "", "")
	return cmd
}

func configCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          configCmdName,
		RunE:         executeCmd,
		SilenceUsage: true,
	}
	cmd.SetHelpFunc(executeHelp)
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "")
	cmd.Flags().StringVar(&env, "env", "default", "")
	return cmd
}

func validateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          validateCmdName,
		RunE:         executeCmd,
		SilenceUsage: true,
	}
	cmd.SetHelpFunc(executeHelp)
	cmd.Flags().StringVar(&env, "env", "default", "")
	cmd.Flags().StringVar(&connection, "connection", "", "")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "")
	cmd.Flags().BoolVar(&noVerbose, "no-verbose", false, "")
	return cmd
}

//nolint:dupl
func generateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          generateCmdName,
		RunE:         executeCmd,
		SilenceUsage: true,
	}
	cmd.SetHelpFunc(executeHelp)
	cmd.Flags().BoolVar(&generateTasks, "generate-tasks", false, "")
	cmd.Flags().BoolVar(&noGenerateTasks, "no-generate-tasks", false, "")
	cmd.Flags().StringVar(&env, "env", "default", "")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "")
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "")
	cmd.Flags().BoolVar(&noVerbose, "no-verbose", false, "")
	return cmd
}

//nolint:dupl
func runCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          runCmdName,
		RunE:         executeCmd,
		SilenceUsage: true,
	}
	cmd.SetHelpFunc(executeHelp)
	cmd.Flags().BoolVar(&generateTasks, "generate-tasks", false, "")
	cmd.Flags().BoolVar(&noGenerateTasks, "no-generate-tasks", false, "")
	cmd.Flags().StringVar(&env, "env", "default", "")
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "")
	cmd.Flags().BoolVar(&noVerbose, "no-verbose", false, "")
	return cmd
}

//nolint:dupl
func deployCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          deployCmdName,
		RunE:         executeDeployCmd,
		SilenceUsage: true,
	}
	cmd.Flags().StringVar(&env, "env", "default", "")
	cmd.Flags().StringVar(&workflowName, "workflow-name", "", "")
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "")
	return cmd
}

func login(cmd *cobra.Command, args []string) error {
	// flow currently does not require login
	return nil
}

func NewFlowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               flowCmdName,
		Short:             "Run flow commands",
		PersistentPreRunE: login,
		Run:               executeHelp,
		SilenceUsage:      true,
	}
	cmd.SetHelpFunc(executeHelp)
	cmd.PersistentFlags().BoolVar(&debug, "debug", false, "")
	cmd.PersistentFlags().BoolVar(&noDebug, "no-debug", false, "")
	cmd.AddCommand(versionCommand())
	cmd.AddCommand(aboutCommand())
	cmd.AddCommand(initCommand())
	configCmd = configCommand()
	cmd.AddCommand(validateCommand())
	generateCmd = generateCommand()
	cmd.AddCommand(generateCmd)
	cmd.AddCommand(runCommand())
	// Currently, we only support Astronomer cloud deployments. Software deploy support is planned to be added in a later release.
	if context.IsCloudContext() {
		cmd.AddCommand(deployCommand())
	}
	return cmd
}
