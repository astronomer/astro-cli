package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/iancoleman/strcase"
	"github.com/spf13/cobra"

	"github.com/astronomerio/astro-cli/airflow"
	"github.com/astronomerio/astro-cli/config"
	"github.com/astronomerio/astro-cli/pkg/fileutil"
	"github.com/astronomerio/astro-cli/pkg/git"
)

var (
	projectRoot string
	projectName string
	forceDeploy bool

	airflowRootCmd = &cobra.Command{
		Use:   "airflow",
		Short: "Manage airflow projects and deployments",
		Long:  "Manage airflow projects and deployments",
	}

	airflowInitCmd = &cobra.Command{
		Use:   "init",
		Short: "Scaffold a new airflow project",
		Long:  "Scaffold a new airflow project",
		RunE:  airflowInit,
	}

	airflowCreateCmd = &cobra.Command{
		Use:   "create",
		Short: "Create a new airflow deployment",
		Long:  "Create a new airflow deployment",
		Args:  cobra.ExactArgs(1),
		RunE:  airflowCreate,
	}

	airflowListCmd = &cobra.Command{
		Use:   "list",
		Short: "List airflow clusters",
		Long:  "List all created airflow clusters",
		RunE:  airflowList,
	}

	airflowDeployCmd = &cobra.Command{
		Use:    "deploy",
		Short:  "Deploy an airflow project",
		Long:   "Deploy an airflow project to a given deployment",
		Args:   cobra.MaximumNArgs(1),
		PreRun: ensureProjectDir,
		RunE:   airflowDeploy,
	}

	airflowStartCmd = &cobra.Command{
		Use:    "start",
		Short:  "Start a development airflow cluster",
		Long:   "Start a development airflow cluster",
		PreRun: ensureProjectDir,
		RunE:   airflowStart,
	}

	airflowKillCmd = &cobra.Command{
		Use:    "kill",
		Short:  "Kill a development airflow cluster",
		Long:   "Kill a development airflow cluster",
		PreRun: ensureProjectDir,
		RunE:   airflowKill,
	}

	airflowStopCmd = &cobra.Command{
		Use:    "stop",
		Short:  "Stop a development airflow cluster",
		Long:   "Stop a development airflow cluster",
		PreRun: ensureProjectDir,
		RunE:   airflowStop,
	}

	airflowPSCmd = &cobra.Command{
		Use:    "ps",
		Short:  "List airflow containers",
		Long:   "List airflow containers",
		PreRun: ensureProjectDir,
		RunE:   airflowPS,
	}
)

func init() {
	// Set up project root
	projectRoot, _ = config.ProjectRoot()

	// Airflow root
	RootCmd.AddCommand(airflowRootCmd)

	// Airflow init
	airflowInitCmd.Flags().StringVarP(&projectName, "name", "n", "", "Name of airflow project")
	airflowRootCmd.AddCommand(airflowInitCmd)

	// Airflow create
	airflowRootCmd.AddCommand(airflowCreateCmd)

	// Airflow list
	airflowRootCmd.AddCommand(airflowListCmd)

	// Airflow deploy
	airflowRootCmd.AddCommand(airflowDeployCmd)
	airflowDeployCmd.Flags().BoolVarP(&forceDeploy, "force", "f", false, "Force deploy if uncommited changes")

	// Airflow start
	airflowRootCmd.AddCommand(airflowStartCmd)

	// Airflow kill
	airflowRootCmd.AddCommand(airflowKillCmd)

	// Airflow stop
	airflowRootCmd.AddCommand(airflowStopCmd)

	// Airflow PS
	airflowRootCmd.AddCommand(airflowPSCmd)
}

func ensureProjectDir(cmd *cobra.Command, args []string) {
	if !(len(projectRoot) > 0) {
		fmt.Println("Error: Not in an astronomer project directory")
		os.Exit(1)
	}
}

// Use project name for image name
func airflowInit(cmd *cobra.Command, args []string) error {
	// Grab working directory
	path := fileutil.GetWorkingDir()

	// Validate project name
	if len(projectName) != 0 {
		projectNameValid := regexp.
			MustCompile(`^[A-Za-z0-9]([A-Za-z0-9_-]*[A-Za-z0-9])?$`).
			MatchString

		if !projectNameValid(projectName) {
			return errors.New("Project name is invalid")
		}
	} else {
		projectDirectory := filepath.Base(path)
		projectName = strings.Replace(strcase.ToSnake(projectDirectory), "_", "-", -1)
	}

	exists := config.ProjectConfigExists()
	if !exists {
		config.CreateProjectConfig(path)
	}
	config.CFG.ProjectName.SetProjectString(projectName)
	airflow.Init(path)

	if exists {
		fmt.Printf("Reinitialized existing astronomer project in %s\n", path)
	} else {
		fmt.Printf("Initialized empty astronomer project in %s\n", path)
	}

	return nil
}

func airflowCreate(cmd *cobra.Command, args []string) error {
	return airflow.Create(args[0])
}

func airflowList(cmd *cobra.Command, args []string) error {
	return airflow.List()
}

func airflowDeploy(cmd *cobra.Command, args []string) error {
	releaseName := ""
	if len(args) > 0 {
		releaseName = args[0]
	}
	if git.HasUncommitedChanges() && !forceDeploy {
		fmt.Println("Project directory has uncommmited changes, use `astro airflow deploy [releaseName] -f` to force deploy.")
		return nil
	}
	return airflow.Deploy(projectRoot, releaseName)
}

// Start an airflow cluster
func airflowStart(cmd *cobra.Command, args []string) error {
	return airflow.Start(projectRoot)
}

// Kill an airflow cluster
func airflowKill(cmd *cobra.Command, args []string) error {
	return airflow.Kill(projectRoot)
}

// Stop an airflow cluster
func airflowStop(cmd *cobra.Command, args []string) error {
	return airflow.Stop(projectRoot)
}

// List containers of an airflow cluster
func airflowPS(cmd *cobra.Command, args []string) error {
	return airflow.PS(projectRoot)
}
