package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/docker"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/messages"
	"github.com/astronomer/astro-cli/pkg/git"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"

	"github.com/spf13/cobra"
)

var (
	errNoWorkspaceID             = errors.New("no workspace id provided")
	errNoDomainSet               = errors.New("no domain set, re-authenticate")
	errInvalidDeploymentName     = errors.New(messages.ErrHoustonDeploymentName)
	errDeploymentNotFound        = errors.New(messages.ErrNoHoustonDeployment)
	errInvalidDeploymentSelected = errors.New(messages.HoustonInvalidDeploymentKey)
)

var (
	// these are used to monkey patch the function in order to write unit test cases
	imageHandlerInit = airflow.ImageHandlerInit

	getDeploymentInfoRequest = &houston.Request{Query: houston.DeploymentInfoRequest}
	getDeploymentInfo        = getDeploymentInfoRequest.Do
)

var tab = printutil.Table{
	Padding:        []int{5, 30, 30, 50},
	DynamicPadding: true,
	Header:         []string{"#", "LABEL", "DEPLOYMENT NAME", "WORKSPACE", "DEPLOYMENT ID"},
}

type ErrWorkspaceNotFound struct {
	workspaceID string
}

func (e ErrWorkspaceNotFound) Error() string {
	return fmt.Sprintf("no workspaces with id (%s) found", e.workspaceID)
}

var deployExample = `
Deployment you would like to deploy to Airflow cluster:

  $ astro deploy <deployment name>

Menu will be presented if you do not specify a deployment name:

  $ astro deploy
`

func newDeployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "deploy DEPLOYMENT",
		Short:   "Deploy an Airflow project",
		Long:    "Deploy an Airflow project to an Astronomer Cluster",
		Args:    cobra.MaximumNArgs(1),
		PreRunE: ensureProjectDir,
		RunE:    deploy,
		Example: deployExample,
		Aliases: []string{"airflow deploy"},
	}
	cmd.Flags().BoolVarP(&forceDeploy, "force", "f", false, "Force deploy if uncommitted changes")
	cmd.Flags().BoolVarP(&forcePrompt, "prompt", "p", false, "Force prompt to choose target deployment")
	cmd.Flags().BoolVarP(&saveDeployConfig, "save", "s", false, "Save deployment in config for future deploys")
	cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "workspace assigned to deployment")
	return cmd
}

func deploy(cmd *cobra.Command, args []string) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return fmt.Errorf("failed to find a valid workspace: %w", err)
	}

	releaseName := ""

	// Get release name from args, if passed
	if len(args) > 0 {
		releaseName = args[0]
	}

	// Save release name in config if specified
	if len(releaseName) > 0 && saveDeployConfig {
		err = config.CFG.ProjectDeployment.SetProjectString(releaseName)
		if err != nil {
			return err
		}
	}

	if git.HasUncommittedChanges() && !forceDeploy {
		fmt.Println(messages.RegistryUncommittedChanges)
		return nil
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return deployAirflow(config.WorkingPath, releaseName, ws, forcePrompt)
}

func deployAirflow(path, name, wsID string, prompt bool) error {
	if wsID == "" {
		return errNoWorkspaceID
	}

	// Validate workspace
	currentWorkspace, err := validateWorkspace(wsID)
	if err != nil {
		return err
	}

	// Get Deployments from workspace ID
	deReq := houston.Request{
		Query:     houston.DeploymentsGetRequest,
		Variables: map[string]interface{}{"workspaceId": currentWorkspace.ID},
	}

	deResp, err := deReq.Do()
	if err != nil {
		return err
	}

	deployments := deResp.Data.GetDeployments

	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}

	cloudDomain := c.Domain
	if cloudDomain == "" {
		return errNoDomainSet
	}

	// Use config deployment if provided
	if name == "" {
		name = config.CFG.ProjectDeployment.GetProjectString()
	}

	if name != "" && !deploymentNameExists(name, deployments) {
		return errInvalidDeploymentName
	}

	// Prompt user for deployment if no deployment passed in
	if name == "" || prompt {
		if len(deployments) == 0 {
			return errDeploymentNotFound
		}

		fmt.Printf(messages.HoustonDeploymentHeader, cloudDomain)
		fmt.Println(messages.HoustonSelectDeploymentPrompt)

		deployMap := map[string]houston.Deployment{}
		for i := range deployments {
			deployment := deployments[i]
			index := i + 1
			tab.AddRow([]string{strconv.Itoa(index), deployment.Label, deployment.ReleaseName, currentWorkspace.Label, deployment.ID}, false)

			deployMap[strconv.Itoa(index)] = deployment
		}

		tab.Print(os.Stdout)
		choice := input.Text("\n> ")
		selected, ok := deployMap[choice]
		if !ok {
			return errInvalidDeploymentSelected
		}
		name = selected.ReleaseName
	}

	nextTag := ""
	for i := range deployments {
		deployment := deployments[i]
		if deployment.ReleaseName == name {
			nextTag = deployment.DeploymentInfo.NextCli
		}
	}

	fmt.Printf(messages.HoustonDeploymentPrompt, name)

	// Build the image to deploy
	err = buildPushDockerImage(c, name, path, nextTag, cloudDomain)
	if err != nil {
		return err
	}
	fmt.Println("Successfully pushed Docker image to Astronomer registry. Navigate to the Astronomer UI for confirmation that your deploy was successful.")

	return nil
}

func validateWorkspace(wsID string) (houston.Workspace, error) {
	var currentWorkspace houston.Workspace

	wsReq := houston.Request{
		Query:     houston.WorkspacesGetRequest,
		Variables: map[string]interface{}{"workspaceId": wsID},
	}

	wsResp, err := wsReq.Do()
	if err != nil {
		return currentWorkspace, err
	}

	if len(wsResp.Data.GetWorkspaces) == 0 {
		return currentWorkspace, ErrWorkspaceNotFound{workspaceID: wsID}
	}

	for i := range wsResp.Data.GetWorkspaces {
		workspace := wsResp.Data.GetWorkspaces[i]
		if workspace.ID == wsID {
			currentWorkspace = workspace
			break
		}
	}
	return currentWorkspace, nil
}

// Find deployment name in deployments slice
func deploymentNameExists(name string, deployments []houston.Deployment) bool {
	for idx := range deployments {
		deployment := deployments[idx]
		if deployment.ReleaseName == name {
			return true
		}
	}
	return false
}

func buildPushDockerImage(c config.Context, name, path, nextTag, cloudDomain string) error {
	// Build our image
	fmt.Println(messages.ImageBuildingPrompt)

	// parse dockerfile
	cmds, err := docker.ParseFile(filepath.Join(path, "Dockerfile"))
	if err != nil {
		return fmt.Errorf("failed to parse dockerfile: %s: %w", filepath.Join(path, "Dockerfile"), err)
	}

	image, tag := docker.GetImageTagFromParsedFile(cmds)
	if config.CFG.ShowWarnings.GetBool() && !validImageRepo(image) {
		i, _ := input.Confirm(fmt.Sprintf(messages.WarningInvalidImageName, image))
		if !i {
			fmt.Println("Canceling deploy...")
			os.Exit(1)
		}
	}
	// Get valid image tags for platform using Deployment Info request
	diResp, err := getDeploymentInfo()
	if err != nil {
		return err
	}

	if config.CFG.ShowWarnings.GetBool() && !diResp.Data.DeploymentConfig.IsValidTag(tag) {
		validTags := strings.Join(diResp.Data.DeploymentConfig.GetValidTags(tag), ", ")
		i, _ := input.Confirm(fmt.Sprintf(messages.WarningInvalidNameTag, tag, validTags))
		if !i {
			fmt.Println("Canceling deploy...")
			os.Exit(1)
		}
	}
	imageHandler, err := imageHandlerInit(name)
	if err != nil {
		return err
	}
	err = imageHandler.Build(config.WorkingPath)
	if err != nil {
		return err
	}
	return imageHandler.Push(cloudDomain, c.Token, nextTag)
}

func validImageRepo(image string) bool {
	validDockerfileBaseImages := map[string]bool{
		"quay.io/astronomer/ap-airflow": true,
		"astronomerinc/ap-airflow":      true,
	}
	result, ok := validDockerfileBaseImages[image]
	if !ok {
		return false
	}
	return result
}
