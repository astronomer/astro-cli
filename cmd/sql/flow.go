package sql

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/houston"

	"github.com/astronomer/astro-cli/context"

	"github.com/astronomer/astro-cli/cmd/software"
	"github.com/astronomer/astro-cli/config"

	"github.com/astronomer/astro-cli/astro-client"
	cloudDeployment "github.com/astronomer/astro-cli/cloud/deployment"
	cloudWorkspace "github.com/astronomer/astro-cli/cloud/workspace"
	"github.com/astronomer/astro-cli/cmd/cloud"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/input"
	softwareDeployment "github.com/astronomer/astro-cli/software/deployment"
	softwareWorkspace "github.com/astronomer/astro-cli/software/workspace"
	"github.com/astronomer/astro-cli/sql"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slices"
)

// Reference to reusable cobra commands
var (
	configCmd   *cobra.Command
	generateCmd *cobra.Command
	versionCmd  *cobra.Command
)

// All cmd names
const (
	flowCmdName     = "flow"
	aboutCmdName    = "about"
	configCmdName   = "config"
	generateCmdName = "generate"
	initCmdName     = "init"
	runCmdName      = "run"
	validateCmdName = "validate"
	versionCmdName  = "version"
	deployCmdName   = "deploy"
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
	includeUpstream   bool
	noDebug           bool
	noGenerateTasks   bool
	noVerbose         bool
	outputDir         string
	projectDir        string
	deleteDags        bool
	taskID            string
	verbose           bool
)

var (
	ErrInvalidInstalledFlowVersion = errors.New("invalid flow version installed")
	ErrNotCloudContext             = errors.New("currently, we only support Astronomer cloud deployments. Software deploy support is planned to be added in a later release. ")
	ErrTooManyArgs                 = errors.New("too many arguments supplied to the command. Refer --help for the usage of the command")
	Os                             = sql.NewOsBind
	RemoveDirectory                = os.RemoveAll
)

type VersionCmdResponse struct {
	Version string `json:"version"`
}

// Return the astro cloud or software config (astroDeploymentID and astroWorkspaceID) for the current env
func getAstroDeploymentConfig() (astroDeploymentID, astroWorkspaceID string, err error) {
	configJSON, err := readConfigCmdOutput("get", "--json")
	if err != nil {
		return
	}

	var deploymentConfig map[string]interface{}
	if err = json.Unmarshal([]byte(configJSON), &deploymentConfig); err != nil {
		return
	}

	envConfig := deploymentConfig[env].(map[string]interface{})
	deployment, _ := envConfig["deployment"].(map[string]interface{})
	astroDeploymentID, _ = deployment["astro_deployment_id"].(string)
	astroWorkspaceID, _ = deployment["astro_workspace_id"].(string)
	return
}

// Prompt the user for astro cloud config (astroDeploymentID and/or astroWorkspaceID) and return them
//
//nolint:gocritic
func promptAstroEnvironmentConfig(astroDeploymentID, astroWorkspaceID string, isCloud bool) (selectedAstroDeploymentID, selectedAstroWorkspaceID string, err error) {
	if astroDeploymentID == "" && astroWorkspaceID == "" {
		if isCloud {
			selectedAstroDeploymentID, selectedAstroWorkspaceID, err = getCloudDeploymentAndWorkspaceID()
		} else {
			selectedAstroDeploymentID, selectedAstroWorkspaceID, err = getSoftwareDeploymentAndWorkspaceID()
		}
		if err != nil {
			return
		}
	} else if astroDeploymentID == "" {
		selectedAstroWorkspaceID = astroWorkspaceID
		if isCloud {
			selectedAstroDeploymentID, err = getCloudDeploymentIDWithWorkspaceID(selectedAstroWorkspaceID)
		} else {
			selectedAstroDeploymentID, err = getSoftwareDeploymentIDWithWorkspaceID(selectedAstroWorkspaceID)
		}
		if err != nil {
			return
		}
	} else if astroWorkspaceID == "" {
		selectedAstroDeploymentID = astroDeploymentID
		if isCloud {
			selectedAstroWorkspaceID, err = getCloudWorkspaceID()
		} else {
			selectedAstroWorkspaceID, err = getSoftwareWorkspaceID()
		}
		if err != nil {
			return
		}
	} else {
		selectedAstroDeploymentID = astroDeploymentID
		selectedAstroWorkspaceID = astroWorkspaceID
	}
	return selectedAstroDeploymentID, selectedAstroWorkspaceID, nil
}

func getCloudWorkspaceID() (string, error) {
	fmt.Println("\nWhich Astro Cloud workspace should be associated with " + env + "?")
	astroClient := astro.NewAstroClient(httputil.NewHTTPClient())
	workspace, err := cloudWorkspace.GetWorkspaceSelection(astroClient, os.Stdout)
	if err != nil {
		return "", err
	}
	return workspace, nil
}

func getSoftwareWorkspaceID() (string, error) {
	fmt.Println("\nWhich Astro Software workspace should be associated with " + env + "?")
	houstonClient := GetHoustonClient()
	workspace, err := softwareWorkspace.GetWorkspaceSelectionID(houstonClient, os.Stdout)
	if err != nil {
		return "", err
	}
	return workspace, nil
}

func getCloudDeploymentIDWithWorkspaceID(astroWorkspaceID string) (string, error) {
	astroClient := astro.NewAstroClient(httputil.NewHTTPClient())
	deployments, err := cloudDeployment.GetDeployments(astroWorkspaceID, astroClient)
	if err != nil {
		return "", err
	}
	deployment, err := cloudDeployment.SelectDeployment(deployments, "Which Astro Cloud deployment should be associated with "+env+"?")
	if err != nil {
		return "", err
	}
	return deployment.ID, nil
}

func getSoftwareDeploymentIDWithWorkspaceID(astroWorkspaceID string) (string, error) {
	houstonClient := GetHoustonClient()
	deployments, err := softwareDeployment.GetDeployments(astroWorkspaceID, houstonClient)
	if err != nil {
		return "", err
	}
	deployment, err := softwareDeployment.SelectDeployment(deployments, "Which Astro Software deployment should be associated with "+env+"?")
	if err != nil {
		return "", err
	}
	return deployment.ID, nil
}

func getCloudDeploymentAndWorkspaceID() (deploymentID, workspaceID string, err error) {
	astroClient := astro.NewAstroClient(httputil.NewHTTPClient())
	fmt.Println("\nWhich Astro Cloud workspace should be associated with " + env + "?")
	workspace, err := cloudWorkspace.GetWorkspaceSelection(astroClient, os.Stdout)
	if err != nil {
		return
	}
	deployments, err := cloudDeployment.GetDeployments(workspace, astroClient)
	if err != nil {
		return
	}
	deployment, err := cloudDeployment.SelectDeployment(deployments, "Which Astro deployment should be associated with "+env+"?")
	if err != nil {
		return
	}
	deploymentID = deployment.ID
	workspaceID = deployment.Workspace.ID
	return
}

func getSoftwareDeploymentAndWorkspaceID() (deploymentID, workspaceID string, err error) {
	houstonClient := GetHoustonClient()
	fmt.Println("\nWhich Astro Software workspace should be associated with " + env + "?")
	workspace, err := softwareWorkspace.GetWorkspaceSelectionID(houstonClient, os.Stdout)
	if err != nil {
		return
	}
	deployments, err := softwareDeployment.GetDeployments(workspace, houstonClient)
	if err != nil {
		return
	}
	deployment, err := softwareDeployment.SelectDeployment(deployments, "Which Software deployment should be associated with "+env+"?")
	if err != nil {
		return
	}
	deploymentID = deployment.ID
	workspaceID = deployment.Workspace.ID
	return
}

var GetHoustonClient = func() houston.ClientInterface {
	httpClient := houston.NewHTTPClient()
	houstonClient := houston.NewClient(httpClient)
	return houstonClient
}

// Set the astro cloud or software config (astroDeploymentID and astroWorkspaceID) for the current env.
func setAstroConfig(astroDeploymentID, astroWorkspaceID string) error {
	if err := executeCmd(configCmd, []string{"set", "deploy", "--astro-workspace-id", astroWorkspaceID, "--astro-deployment-id", astroDeploymentID}); err != nil {
		return err
	}
	return nil
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
		airflowHome, err = resolvePath(airflowHome)
		if err != nil {
			return nil, err
		}
		airflowDagsFolder, err = resolvePath(airflowDagsFolder)
		if err != nil {
			return nil, err
		}
		dataDir, err = resolvePath(dataDir)
		if err != nil {
			return nil, err
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
		outputDir, err = resolvePath(outputDir)
		if err != nil {
			return nil, err
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
func readConfigCmdOutput(args ...string) (string, error) {
	output, err := readCmdOutput(configCmd, args)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil // remove spaces such as \r\n
}

// Read version cmd output for retrieving installed version of SQL CLI
var getInstalledFlowVersion = func() (string, error) {
	output, err := readCmdOutput(versionCmd, []string{"--json"})
	if err != nil {
		return "", err
	}
	var resp VersionCmdResponse
	err = json.Unmarshal([]byte(output), &resp)
	if err != nil {
		return "", err
	}
	return resp.Version, nil
}

// Resolve filepath to absolute
func resolvePath(path string) (string, error) {
	if path == "" { // base negative case in which no path is passed
		return "", nil
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("error resolving path %v: %w", path, err)
	}
	return path, nil
}

// Extends args with flags e.g. "--project-dir ." or "--verbose"
//
//nolint:gocognit
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
		if verbose {
			args = append(args, "--verbose")
		}
		if noVerbose {
			args = append(args, "--no-verbose")
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
		if includeUpstream {
			args = append(args, "--include-upstream")
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
		if taskID != "" {
			args = append(args, "--task-id", taskID)
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
		airflowHome, err = readConfigCmdOutput("get", "airflow_home")
		if err != nil {
			return nil, err
		}
		dirs = append(dirs, airflowHome)
		dataDir, err = readConfigCmdOutput("get", "data_dir")
		if err != nil {
			return nil, err
		}
		dirs = append(dirs, dataDir)
	case generateCmdName:
		dirs = append(dirs, projectDir)
		if outputDir != "" {
			dirs = append(dirs, outputDir)
		}
		airflowHome, err = readConfigCmdOutput("get", "airflow_home")
		if err != nil {
			return nil, err
		}
		dirs = append(dirs, airflowHome)
		airflowDagsFolder, err = readConfigCmdOutput("get", "airflow_dags_folder")
		if err != nil {
			return nil, err
		}
		dirs = append(dirs, airflowDagsFolder)
		dataDir, err = readConfigCmdOutput("get", "data_dir")
		if err != nil {
			return nil, err
		}
		dirs = append(dirs, dataDir)
	case runCmdName:
		dirs = append(dirs, projectDir)
		airflowHome, err = readConfigCmdOutput("get", "airflow_home")
		if err != nil {
			return nil, err
		}
		dirs = append(dirs, airflowHome)
		airflowDagsFolder, err = readConfigCmdOutput("get", "airflow_dags_folder")
		if err != nil {
			return nil, err
		}
		dirs = append(dirs, airflowDagsFolder)
		dataDir, err = readConfigCmdOutput("get", "data_dir")
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

func generateWorkflows(dagsPath, workflowName string) error {
	generateCmdArgs := []string{"--output-dir", dagsPath}
	if workflowName != "" {
		generateCmdArgs = append(generateCmdArgs, workflowName)
		err := executeCmd(generateCmd, generateCmdArgs)
		if err != nil {
			return err
		}
	} else {
		workflowsDir := filepath.Join(projectDir, "workflows")
		items, _ := Os().ReadDir(workflowsDir)
		if len(items) == 0 {
			fmt.Printf("No workflows found in directory %v. No DAGs to deploy and existing DAGs will be deleted from the deployment.", workflowsDir)
			return nil
			// TODO: Prompt for confirmation.
		}
		for _, item := range items {
			if item.IsDir() {
				err := executeCmd(generateCmd, append(generateCmdArgs, item.Name()))
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func executeDeployCmd(cmd *cobra.Command, args []string) error {
	if len(args) > 1 {
		return ErrTooManyArgs
	}

	pythonSDKPromptContent := input.PromptContent{
		Label: "Would you like to add the required version of Python SDK dependency to requirements.txt? Otherwise, the deployment will not proceed.",
	}
	promptRunner := input.GetYesNoSelector(pythonSDKPromptContent)

	installedSQLCLIVersion, err := getInstalledFlowVersion()
	if err != nil {
		return err
	}
	err = sql.EnsurePythonSdkVersionIsMet(promptRunner, installedSQLCLIVersion)
	if err != nil {
		return err
	}

	projectDir, err = resolvePath(projectDir)
	if err != nil {
		return err
	}
	dagsPath := filepath.Join(config.WorkingPath, "dags/.astro-sql-deploy/"+env)

	if err := updateDags(dagsPath, args); err != nil {
		return fmt.Errorf("error updating DAGs based on workflows: %w", err)
	}

	if context.IsCloudContext() {
		err = uploadDagsToCloud(dagsPath)
		if err != nil {
			return err
		}
	} else {
		err = uploadDagsToSoftware()
		if err != nil {
			return fmt.Errorf("error uploading DAGs to software: %w", err)
		}
	}
	if deleteDags {
		err = RemoveDirectory(dagsPath)
		if err != nil {
			return fmt.Errorf("error deleting DAGs directory: %w", err)
		}
	}
	return nil
}

func uploadDagsToSoftware() error {
	astroDeploymentID, astroWorkspaceID, err := getAstroDeploymentConfig()
	if err != nil {
		return err
	}
	astroDeploymentID, astroWorkspaceID, err = promptAstroEnvironmentConfig(astroDeploymentID, astroWorkspaceID, false)
	if err != nil {
		return err
	}

	fmt.Printf("Deployment ID: %s, workspace ID: %s\n", astroDeploymentID, astroWorkspaceID)

	if err := setAstroConfig(astroDeploymentID, astroWorkspaceID); err != nil {
		return err
	}

	astroDeployCmd := software.NewDeployCmd()
	astroDeployCmd.SetArgs([]string{astroDeploymentID, "--workspace-id", astroWorkspaceID})
	if _, err := astroDeployCmd.ExecuteC(); err != nil {
		return err
	}
	return nil
}

func uploadDagsToCloud(dagsPath string) error {
	astroDeploymentID, astroWorkspaceID, err := getAstroDeploymentConfig()
	if err != nil {
		return err
	}
	astroDeploymentID, astroWorkspaceID, err = promptAstroEnvironmentConfig(astroDeploymentID, astroWorkspaceID, true)
	if err != nil {
		return err
	}
	if err := setAstroConfig(astroDeploymentID, astroWorkspaceID); err != nil {
		return err
	}

	astroDeployCmd := cloud.NewDeployCmd()
	astroDeployCmd.SetArgs([]string{astroDeploymentID, "--workspace-id", astroWorkspaceID, "--dags-path", dagsPath})
	if _, err := astroDeployCmd.ExecuteC(); err != nil {
		return err
	}
	return nil
}

func updateDags(dagsPath string, args []string) error {
	if err := os.RemoveAll(dagsPath); err != nil {
		return fmt.Errorf("error removing dags path: %w", err)
	}
	if err := os.MkdirAll(dagsPath, os.ModePerm); err != nil {
		return fmt.Errorf("error creating directories for %v: %w", dagsPath, err)
	}
	var workflowName string
	if len(args) > 0 {
		workflowName = args[0]
	}
	if err := generateWorkflows(dagsPath, workflowName); err != nil {
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
	cmd.Flags().BoolVar(&verbose, "verbose", false, "")
	cmd.Flags().BoolVar(&noVerbose, "no-verbose", false, "")
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
	cmd.Flags().StringVar(&taskID, "task-id", "", "")
	cmd.Flags().BoolVar(&includeUpstream, "include-upstream", false, "")
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
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "")
	cmd.Flags().BoolVar(&deleteDags, "delete-dags", false, "If you would like to delete flow generated DAGs after deploying to astronomer. This would mean that the DAGs disappear if you run `astro deploy` in the future.")
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
	versionCmd = versionCommand()
	cmd.AddCommand(versionCmd)
	cmd.AddCommand(aboutCommand())
	cmd.AddCommand(initCommand())
	configCmd = configCommand()
	cmd.AddCommand(validateCommand())
	generateCmd = generateCommand()
	cmd.AddCommand(generateCmd)
	cmd.AddCommand(runCommand())
	cmd.AddCommand(deployCommand())
	return cmd
}
