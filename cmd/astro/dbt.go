package astro

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrodeploy "github.com/astronomer/astro-cli/internal/platform/astro/deploy"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/pkg/cosmosboost"
)

var (
	mountPath      string
	dbtProjectPath string

	DeployBundle = astrodeploy.DeployBundle
	DeleteBundle = astrodeploy.DeleteBundle
)

const (
	dbtDefaultMountPathPrefix = "/usr/local/airflow/dbt/"
	dbtProjectYmlFilename     = "dbt_project.yml"
	dbtBundleType             = "dbt"
	dbtWaitTime               = 300 * time.Second
)

func newDbtCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dbt",
		Short: "Manage dbt projects deployed to your Deployments",
	}
	// The results leave by cmd.OutOrStdout(), which must be the stdout the
	// tree was built with: under json strayStdoutToStderr points os.Stdout
	// at stderr, and an unbound command would follow it there.
	cmd.SetOut(out)
	cmd.AddCommand(
		newDbtDeployCmd(),
		newDbtDeleteCmd(),
		newDbtCleanupCmd(),
	)
	applyPreferredFlagsIn(cmd)
	return cmd
}

func newDbtCleanupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cleanup [PATH]...",
		Args:  cobra.ArbitraryArgs,
		Short: "Remove the Cosmos Boost artifacts under each path",
		Long:  "Remove the artifacts the Cosmos Boost pre-deploy step wrote under each path (default: the current directory). Run it after disabling cosmos_boost.pre_deploy, because a disabled deploy leaves earlier deploys' artifacts in place.",
		RunE:  cleanupDbt,
		Example: `  # Remove the artifacts under the current directory
  astro dbt cleanup

  # Remove them under two dbt projects
  astro dbt cleanup <PATH> <PATH>`,
	}
	cliout.AddOutputFlag(cmd, &dbtOutput)
	return cmd
}

func cleanupDbt(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true
	report, err := cosmosboost.Cleanup(args...)
	if err != nil {
		return err
	}
	return cliout.Renderer{Format: dbtOutput, Out: cmd.OutOrStdout()}.Emit(newDbtCleanupJSON(&report), cliout.Text(renderDbtCleanup))
}

//nolint:dupl // the duplication is acceptable here
func newDbtDeployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy [DEPLOYMENT_ID]",
		Short: "Deploy your dbt project to a Deployment on Astro",
		Long:  "Deploy your dbt project to a Deployment on Astro. This command bundles your dbt project files and uploads it to your Deployment.",
		Args:  cobra.MaximumNArgs(1),
		RunE:  deployDbt,
		Example: `  # Deploy the dbt project in the current directory, picking the Deployment from a list
  astro dbt deploy

  # Deploy a dbt project elsewhere to a given Deployment
  astro dbt deploy <DEPLOYMENT_ID> --project-path <PATH>`,
	}

	cmd.Flags().StringVarP(&mountPath, "mount-path", "m", "", fmt.Sprintf("Path to mount dbt project in Airflow, for reference by Dags. Default %s{dbt project name}", dbtDefaultMountPathPrefix))
	cmd.Flags().StringVarP(&dbtProjectPath, "project-path", "p", "", "Path to the dbt project to deploy. Default current directory")
	cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "Workspace for your Deployment")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the Deployment to deploy to")
	addWorkspaceFlag(cmd.Flags(), "", "Workspace for your Deployment")
	addDeploymentFlag(cmd.Flags(), "Deployment to deploy to: a Deployment id, or a Deployment name")
	cmd.Flags().StringVarP(&deployDescription, "description", "", "", "Description to store on the deploy")
	cmd.Flags().BoolVarP(&waitForDeploy, "wait", "w", false, "Wait for the Deployment to become healthy before ending the command")
	cmd.Flags().DurationVarP(&waitTime, "wait-time", "t", dbtWaitTime, "Time to wait for the Deployment to become healthy before ending the command. Can only be used with --wait=true")
	cliout.AddOutputFlag(cmd, &dbtOutput)

	return cmd
}

func deployDbt(cmd *cobra.Command, args []string) error {
	format := dbtOutput
	cmd.SilenceUsage = true
	// The notes below, and the ones the platform code prints on the way, are
	// the output text mode has always shown; under json they go to stderr.
	defer strayStdoutToStderr(format)()

	// if the dbt project path is not provided, use the current directory
	if dbtProjectPath == "" {
		dbtProjectPath = config.WorkingPath
	}

	// check that the dbt project path is not within an Astro project
	withinAstroProject, err := config.IsWithinProjectDir(dbtProjectPath)
	if err != nil {
		return fmt.Errorf("failed to verify dbt project path is not within an Astro project: %w", err)
	}
	if !withinAstroProject {
		withinAstroProject = isWithinManifestProject(dbtProjectPath)
	}
	if withinAstroProject {
		return fmt.Errorf("dbt project is within an Astro project. Use 'astro deploy' to deploy your Astro project")
	}

	// check that there is a valid dbt project at the dbt project path
	err = validateDbtProjectExists(dbtProjectPath)
	if err != nil {
		return err
	}

	// extract the dbt project's name
	dbtProjectName, err := extractDbtProjectName(dbtProjectPath)
	if err != nil {
		return fmt.Errorf("dbt project name not found in %s: %w", dbtProjectPath, err)
	}

	// if the workspace ID is not provided, try to find a valid workspace
	if workspaceID == "" {
		var err error
		workspaceID, err = coalesceWorkspace()
		if err != nil {
			return fmt.Errorf("failed to find a valid workspace: %w", err)
		}
	}

	if cmd.Flags().Changed("wait-time") && !waitForDeploy {
		return errors.New("cannot use --wait-time with --wait=false")
	}

	// get the deployment id to deploy the dbt project to
	deploymentID, read, err := resolveBundleDeployment(args, workspaceID, deploymentName, noCreateUnderJSON(format, "to deploy to"))
	if err != nil {
		return err
	}
	fmt.Println("Initiating dbt deploy for deployment ID: " + deploymentID)

	// if the mount path is not provided, derive it from the dbt project name
	if mountPath == "" {
		mountPath = dbtDefaultMountPathPrefix + dbtProjectName
		fmt.Printf("Generated mount path from dbt project name: %s\n", mountPath)
	}

	// deploy the dbt project as a bundle
	deployBundleInput := &astrodeploy.DeployBundleInput{
		BundlePath:    dbtProjectPath,
		MountPath:     mountPath,
		DeploymentID:  deploymentID,
		Deployment:    read,
		BundleType:    dbtBundleType,
		Description:   deployDescription,
		AstroV1Client: astroV1Client,
	}
	res, err := DeployBundle(deployBundleInput)
	if err != nil {
		return err
	}
	projectPath, err := filepath.Abs(dbtProjectPath)
	if err != nil {
		projectPath = dbtProjectPath
	}
	r := cliout.Renderer{Format: format, Out: cmd.OutOrStdout()}
	return publishThenWait(cmd, format, waitForDeploy, res.DeploymentID, waitTime, func(waitErr error) error {
		return r.Emit(newDbtDeployJSON(&res, dbtProjectName, projectPath, waitForDeploy, waitErr), renderBundleUploaded(res.BundleVersion))
	})
}

//nolint:dupl // the duplication is acceptable here
func newDbtDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [DEPLOYMENT_ID]",
		Short: "Delete a dbt project from a Deployment on Astro",
		Long:  "Delete a dbt project bundle from a Deployment. This removes the uploaded dbt project files but does not affect Dags that were generated from the project.",
		Args:  cobra.MaximumNArgs(1),
		RunE:  deleteDbt,
		Example: `  # Delete the dbt project in the current directory, picking the Deployment from a list
  astro dbt delete

  # Delete it from a given Deployment, by its mount path
  astro dbt delete <DEPLOYMENT_ID> --mount-path <MOUNT_PATH>`,
	}

	cmd.Flags().StringVarP(&mountPath, "mount-path", "m", "", fmt.Sprintf("Mount path of the dbt project to be deleted from the Deployment. Default %s{dbt project name}", dbtDefaultMountPathPrefix))
	cmd.Flags().StringVarP(&dbtProjectPath, "project-path", "p", "", "Path to the dbt project to delete from the Deployment. Default current directory")
	cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "Workspace for your Deployment")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the Deployment to delete the dbt project from")
	addWorkspaceFlag(cmd.Flags(), "", "Workspace for your Deployment")
	addDeploymentFlag(cmd.Flags(), "Deployment to delete the dbt project from: a Deployment id, or a Deployment name")
	cmd.Flags().StringVarP(&deployDescription, "description", "", "", "Description to store on the deploy")
	cmd.Flags().BoolVarP(&waitForDeploy, "wait", "w", false, "Wait for the Deployment to become healthy before ending the command")
	cmd.Flags().DurationVarP(&waitTime, "wait-time", "t", dbtWaitTime, "Time to wait for the Deployment to become healthy before ending the command. Can only be used with --wait=true")
	cliout.AddOutputFlag(cmd, &dbtOutput)
	return cmd
}

func deleteDbt(cmd *cobra.Command, args []string) error {
	format := dbtOutput
	cmd.SilenceUsage = true
	defer strayStdoutToStderr(format)()

	// if the workspace ID is not provided, try to find a valid workspace
	if workspaceID == "" {
		var err error
		workspaceID, err = coalesceWorkspace()
		if err != nil {
			return fmt.Errorf("failed to find a valid workspace: %w", err)
		}
	}

	if cmd.Flags().Changed("wait-time") && !waitForDeploy {
		return errors.New("cannot use --wait-time with --wait=false")
	}

	// get the deployment id to delete the dbt project
	deploymentID, read, err := resolveBundleDeployment(args, workspaceID, deploymentName, noCreateUnderJSON(format, "to delete from"))
	if err != nil {
		return err
	}
	fmt.Println("Initiating dbt delete deploy for deployment ID: " + deploymentID)

	// if the mount path is not provided, derive it from the dbt project name
	if mountPath == "" {
		// if the dbt project path is not provided, use the current directory
		if dbtProjectPath == "" {
			dbtProjectPath = config.WorkingPath
		}

		// check that there is a valid dbt project at the dbt project path
		err := validateDbtProjectExists(dbtProjectPath)
		if err != nil {
			return err
		}

		// extract the dbt project's name
		dbtProjectName, err := extractDbtProjectName(dbtProjectPath)
		if err != nil {
			return fmt.Errorf("dbt project name not found in %s: %w", dbtProjectPath, err)
		}

		mountPath = dbtDefaultMountPathPrefix + dbtProjectName
	}

	deleteBundleInput := &astrodeploy.DeleteBundleInput{
		MountPath:     mountPath,
		DeploymentID:  deploymentID,
		Deployment:    read,
		BundleType:    dbtBundleType,
		Description:   deployDescription,
		AstroV1Client: astroV1Client,
	}
	res, err := DeleteBundle(deleteBundleInput)
	if err != nil {
		return err
	}
	r := cliout.Renderer{Format: format, Out: cmd.OutOrStdout()}
	return publishThenWait(cmd, format, waitForDeploy, res.DeploymentID, waitTime, func(waitErr error) error {
		return r.Emit(newDbtDeleteJSON(&res, waitForDeploy, waitErr), renderDbtDeleted(res.MountPath))
	})
}

func validateDbtProjectExists(dbtProjectPath string) error {
	dbtProjectYamlPath := filepath.Join(dbtProjectPath, dbtProjectYmlFilename)

	_, err := os.Stat(dbtProjectYamlPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("dbt project file not found at %s. Please run this command in the root of your dbt project, or use --project-path to specify the dbt project path", dbtProjectYamlPath)
	}
	return err
}

func extractDbtProjectName(dbtProjectPath string) (string, error) {
	dbtProjectYamlPath := filepath.Join(dbtProjectPath, dbtProjectYmlFilename)

	var dbtProject map[string]interface{}
	file, err := os.Open(dbtProjectYamlPath)
	if err != nil {
		return "", fmt.Errorf("could not open %s: %w", dbtProjectYamlPath, err)
	}
	defer file.Close()
	decoder := yaml.NewDecoder(file)
	err = decoder.Decode(&dbtProject)
	if err != nil {
		return "", fmt.Errorf("could not decode %s: %w", dbtProjectYamlPath, err)
	}

	dbtProjectName, ok := dbtProject["name"].(string)
	if !ok || dbtProjectName == "" {
		return "", errors.New("invalid dbt project name")
	}

	return dbtProjectName, nil
}

// resolveBundleDeployment is the Deployment a bundle command acts on: the
// argument, --deployment, the one --deployment-name names, or the one picked
// from the Workspace's. It returns the id, and the Deployment itself when it
// read it (picked, or found by name), so the command need not read it again;
// read is nil for one named by id.
//
// noCreateFor decides what a Workspace with no Deployment does. Empty, the
// run walks through creating one, as text mode always has. Set (under
// --output json, where nothing may be asked), it is what the command would
// have done with the Deployment ("to deploy to"), and the run fails saying
// so, rather than asking a question answered by a --name this command does
// not have.
func resolveBundleDeployment(args []string, workspaceID, deploymentName, noCreateFor string) (id string, read *astrov1.Deployment, err error) {
	// if provided, use the deployment ID from the command argument
	if len(args) > 0 {
		return args[0], nil, nil
	}
	// or the one given as --deployment
	if deploymentArg != "" {
		return deploymentArg, nil, nil
	}

	// otherwise, prompt the user to select a deployment
	selectedDeployment, err := deployment.GetDeployment(workspaceID, "", deploymentName, noCreateFor != "", nil, astroV1Client)
	if err != nil {
		return "", nil, err
	}
	if selectedDeployment.Id == "" {
		return "", nil, deployment.ErrNothingTo(workspaceID, "", deploymentName, noCreateFor)
	}
	return selectedDeployment.Id, &selectedDeployment, nil
}

// noCreateUnderJSON is resolveBundleDeployment's noCreateFor for a command
// in format: what under json, and nothing (the create walk-through) in text.
func noCreateUnderJSON(format cliout.Format, what string) string {
	if format == cliout.FormatJSON {
		return what
	}
	return ""
}
