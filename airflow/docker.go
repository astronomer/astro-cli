package airflow

import (
	"bytes"
	"context"

	// #nosec
	"crypto/md5"
	"fmt"
	"html/template"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/Masterminds/semver"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"

	p "github.com/docker/libcompose/project"
	"github.com/docker/libcompose/project/options"

	"github.com/astronomer/astro-cli/airflow/include"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/docker"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/messages"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
	"github.com/astronomer/astro-cli/settings"
	"github.com/docker/libcompose/cli/logger"
	dockercompose "github.com/docker/libcompose/docker"
	"github.com/docker/libcompose/docker/ctx"
	"github.com/pkg/errors"
)

const (
	componentName = "airflow"
	dockerStateUp = "Up"

	projectStopTimeout    = 5
	defaultAirflowVersion = uint64(0x1) //nolint:gomnd
)

type ErrWorkspaceNotFound struct {
	workspaceID string
}

func (e ErrWorkspaceNotFound) Error() string {
	return fmt.Sprintf("no workspaces with id (%s) found", e.workspaceID)
}

var tab = printutil.Table{
	Padding:        []int{5, 30, 30, 50},
	DynamicPadding: true,
	Header:         []string{"#", "LABEL", "DEPLOYMENT NAME", "WORKSPACE", "DEPLOYMENT ID"},
}

// ComposeConfig is input data to docker compose yaml template
type ComposeConfig struct {
	PostgresUser         string
	PostgresPassword     string
	PostgresHost         string
	PostgresPort         string
	AirflowEnvFile       string
	AirflowImage         string
	AirflowHome          string
	AirflowUser          string
	AirflowWebserverPort string
	MountLabel           string
}

// projectNameUnique creates a reasonably unique project name based on the hashed
// path of the project. This prevents collisions of projects with identical dir names
// in different paths. ie (~/dev/project1 vs ~/prod/project1)
func projectNameUnique() (string, error) {
	projectName := config.CFG.ProjectName.GetString()

	pwd, err := fileutil.GetWorkingDir()
	if err != nil {
		return "", errors.Wrap(err, "error retrieving working directory")
	}

	// #nosec
	b := md5.Sum([]byte(pwd))
	s := fmt.Sprintf("%x", b[:])

	return projectName + "_" + s[0:6], nil
}

// repositoryName creates an airflow repository name
func repositoryName(name string) string {
	return fmt.Sprintf("%s/%s", name, componentName)
}

// imageName creates an airflow image name
func imageName(name, tag string) string {
	return fmt.Sprintf("%s:%s", repositoryName(name), tag)
}

// imageBuild builds the airflow project
func imageBuild(path, imageName string) error {
	// Change to location of Dockerfile
	err := os.Chdir(path)
	if err != nil {
		return err
	}

	// Build image
	err = docker.Exec("build", "-t", imageName, ".")
	if err != nil {
		return errors.Wrapf(err, "command 'docker build -t %s failed", imageName)
	}

	return nil
}

// generateConfig generates the docker-compose config
func generateConfig(projectName, airflowHome, envFile string) (string, error) {
	tmpl, err := template.New("yml").Parse(include.Composeyml)
	if err != nil {
		return "", errors.Wrap(err, "failed to generate config")
	}

	envExists, err := fileutil.Exists(envFile)
	if err != nil {
		return "", errors.Wrapf(err, messages.EnvPath, envFile)
	}

	if envFile != "" {
		if !envExists {
			fmt.Printf(messages.EnvNotFound, envFile)
			envFile = ""
		} else {
			fmt.Printf(messages.EnvFound, envFile)
			envFile = fmt.Sprintf("env_file: %s", envFile)
		}
	}

	cfg := ComposeConfig{
		PostgresUser:         config.CFG.PostgresUser.GetString(),
		PostgresPassword:     config.CFG.PostgresPassword.GetString(),
		PostgresHost:         config.CFG.PostgresHost.GetString(),
		PostgresPort:         config.CFG.PostgresPort.GetString(),
		AirflowImage:         imageName(projectName, "latest"),
		AirflowHome:          airflowHome,
		AirflowUser:          "astro",
		AirflowWebserverPort: config.CFG.WebserverPort.GetString(),
		AirflowEnvFile:       envFile,
		MountLabel:           "z",
	}

	buff := new(bytes.Buffer)
	err = tmpl.Execute(buff, cfg)
	if err != nil {
		return "", errors.Wrap(err, "failed to generate config")
	}

	return buff.String(), nil
}

func checkServiceState(serviceState, expectedState string) bool {
	scrubbedState := strings.Split(serviceState, " ")[0]
	return scrubbedState == expectedState
}

// createProject creates project with yaml config as context
func createProject(projectName, airflowHome, envFile string) (p.APIProject, error) {
	// Generate the docker-compose yaml
	yaml, err := generateConfig(projectName, airflowHome, envFile)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create project")
	}
	composeCtx := p.Context{
		ComposeBytes:  [][]byte{[]byte(yaml)},
		ProjectName:   projectName,
		LoggerFactory: logger.NewColorLoggerFactory(),
	}

	// No need to stat then read, just try to read and ignore ENOENT error
	composeFile := "docker-compose.override.yml"
	composeBytes, err := ioutil.ReadFile(composeFile)
	if err != nil && !os.IsNotExist(err) {
		return nil, errors.Wrapf(err, "Failed to open the compose file: %s", composeFile)
	}
	if err == nil {
		composeCtx.ComposeBytes = append(composeCtx.ComposeBytes, composeBytes)

		// Even though these won't be loaded (as we have provided ComposeBytes) we
		// need to specify this so that relative volume paths are resolved by
		// libcompose
		composeCtx.ComposeFiles = []string{composeFile}
	}

	// Create the project
	return dockercompose.NewProject(&ctx.Context{Context: composeCtx}, nil)
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

// Start starts a local airflow development cluster
func Start(airflowHome, dockerfile, envFile string) error {
	// Get project name from config
	projectName, err := projectNameUnique()
	replacer := strings.NewReplacer("_", "", "-", "")
	strippedProjectName := replacer.Replace(projectName)

	if err != nil {
		return errors.Wrap(err, "error retrieving working directory")
	}

	airflowDockerVersion, err := airflowVersionFromDockerFile(airflowHome, dockerfile)
	if err != nil {
		return errors.Wrap(err, "error parsing airflow version from dockerfile")
	}

	// Create a libcompose project
	project, err := createProject(projectName, airflowHome, envFile)
	if err != nil {
		return errors.Wrap(err, messages.ErrComposeCreate)
	}

	// Get project containers
	psInfo, err := project.Ps(context.Background())
	if err != nil {
		return errors.Wrap(err, messages.ErrComposeStatusCheck)
	}

	if len(psInfo) > 0 {
		// Ensure project is not already running
		for _, info := range psInfo {
			if checkServiceState(info["State"], dockerStateUp) {
				return errors.New("cannot start, project already running")
			}
		}

		err = imageBuild(airflowHome, imageName(projectName, "latest"))
		if err != nil {
			return err
		}

		err = project.Up(context.Background(), options.Up{})
		if err != nil {
			return errors.Wrap(err, messages.ErrComposeRecreate)
		}
	} else {
		// Build this project image
		err = imageBuild(airflowHome, imageName(projectName, "latest"))
		if err != nil {
			return err
		}

		// Start up our project
		err = project.Up(context.Background(), options.Up{})
		if err != nil {
			return errors.Wrap(err, messages.ErrComposeRecreate)
		}
	}

	psInfo, err = project.Ps(context.Background())
	if err != nil {
		return errors.Wrap(err, messages.ErrComposeStatusCheck)
	}

	fileState, err := fileutil.Exists("airflow_settings.yaml")
	if err != nil {
		return errors.Wrap(err, messages.SettingsPath)
	}

	if fileState {
		for _, info := range psInfo {
			if strings.Contains(info["Name"], strippedProjectName) &&
				strings.Contains(info["Name"], "webserver") {
				settings.ConfigSettings(info["Id"], airflowDockerVersion)
			}
		}
	}

	parts := strings.Split(config.CFG.WebserverPort.GetString(), ":")
	fmt.Printf(messages.ComposeLinkWebserver+"\n", parts[len(parts)-1])
	fmt.Printf(messages.ComposeLinkPostgres+"\n", config.CFG.PostgresPort.GetString())
	fmt.Printf(messages.ComposeUserPassword + "\n")

	return nil
}

// Kill stops a local airflow development cluster
func Kill(airflowHome string) error {
	// Get project name from config
	projectName, err := projectNameUnique()
	if err != nil {
		return errors.Wrap(err, "error retrieving working directory")
	}

	// Create a libcompose project
	project, err := createProject(projectName, airflowHome, "")
	if err != nil {
		return errors.Wrap(err, messages.ErrComposeCreate)
	}

	// Shut down our project
	err = project.Down(context.Background(), options.Down{RemoveVolume: true, RemoveOrphans: true})
	if err != nil {
		return errors.Wrap(err, messages.ErrComposeStop)
	}

	return nil
}

// Logs out airflow webserver or scheduler logs
func Logs(airflowHome string, webserver, scheduler, follow bool) error {
	s := make([]string, 0)

	// Get project name from config
	projectName, err := projectNameUnique()
	if err != nil {
		return err
	}

	// Create libcompose project
	project, err := createProject(projectName, airflowHome, "")
	if err != nil {
		return errors.Wrap(err, messages.ErrComposeCreate)
	}

	psInfo, err := project.Ps(context.Background())
	if err != nil {
		return errors.Wrap(err, messages.ErrComposeStatusCheck)
	}

	if len(psInfo) == 0 {
		return errors.Wrap(err, "cannot view logs, project not running")
	}

	if scheduler {
		s = append(s, "scheduler")
	}
	if webserver {
		s = append(s, "webserver")
	}

	err = project.Log(context.Background(), follow, s...)
	if err != nil {
		return err
	}

	return nil
}

// Stop a running docker project
func Stop(airflowHome string) error {
	// Get project name from config
	projectName, err := projectNameUnique()
	if err != nil {
		return errors.Wrap(err, "error retrieving working directory")
	}

	// Create a libcompose project
	project, err := createProject(projectName, airflowHome, "")
	if err != nil {
		return errors.Wrap(err, messages.ErrComposeCreate)
	}

	// Pause our project
	err = project.Stop(context.Background(), projectStopTimeout)
	if err != nil {
		return errors.Wrap(err, messages.ErrComposePause)
	}

	return nil
}

// PS prints the running airflow containers
func PS(airflowHome string) error {
	// Get project name from config
	projectName, err := projectNameUnique()
	if err != nil {
		return errors.Wrap(err, "error retrieving working directory")
	}

	// Create a libcompose project
	project, err := createProject(projectName, airflowHome, "")
	if err != nil {
		return errors.Wrap(err, messages.ErrComposeCreate)
	}

	// List project containers
	psInfo, err := project.Ps(context.Background())
	if err != nil {
		return errors.Wrap(err, messages.ErrComposeStatusCheck)
	}

	// Columns for table
	infoColumns := []string{"Name", "State", "Ports"}

	// Create a new tabwriter
	tw := new(tabwriter.Writer)
	tw.Init(os.Stdout, 0, 8, 2, '\t', tabwriter.AlignRight) // nolint:gomnd

	// Append data to table
	fmt.Fprintln(tw, strings.Join(infoColumns, "\t"))
	for _, info := range psInfo {
		data := []string{}
		for _, lbl := range infoColumns {
			data = append(data, info[lbl])
		}
		fmt.Fprintln(tw, strings.Join(data, "\t"))
	}

	// Flush to stdout
	return tw.Flush()
}

// getWebServerContainerID return webserver container id
func getWebServerContainerID(airflowHome string) (string, error) {
	projectName, err := projectNameUnique()
	if err != nil {
		return "", err
	}

	project, err := createProject(projectName, airflowHome, "")
	if err != nil {
		return "", errors.Wrap(err, messages.ErrComposeCreate)
	}

	psInfo, err := project.Ps(context.Background())
	if err != nil {
		return "", errors.Wrap(err, messages.ErrComposeStatusCheck)
	}

	replacer := strings.NewReplacer("_", "", "-", "")
	strippedProjectName := replacer.Replace(projectName)

	for _, info := range psInfo {
		if strings.Contains(info["Name"], strippedProjectName) &&
			strings.Contains(info["Name"], "webserver") {
			return info["Id"], nil
		}
	}
	return "", err
}

// Run creates using docker exec
// inspired from https://github.com/docker/cli/tree/master/cli/command/container
func Run(airflowHome string, args []string, user string) error {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return err
	}

	execConfig := &types.ExecConfig{
		AttachStdout: true,
		Cmd:          args,
	}
	if user != "" {
		execConfig.User = user
	}

	fmt.Printf("Running: %s\n", strings.Join(args, " "))
	containerID, err := getWebServerContainerID(airflowHome)
	if err != nil {
		return err
	}

	response, err := cli.ContainerExecCreate(context.Background(), containerID, *execConfig)
	if err != nil {
		return errors.New("airflow is not running, Start it with 'astro airflow start'")
	}

	execID := response.ID
	if execID == "" {
		return errors.New("exec ID is empty")
	}

	execStartCheck := types.ExecStartCheck{
		Detach: execConfig.Detach,
	}

	resp, _ := cli.ContainerExecAttach(context.Background(), execID, execStartCheck)

	return docker.ExecPipe(resp, os.Stdin, os.Stdout, os.Stderr)
}

// Deploy pushes a new docker image
func Deploy(path, name, wsID string, prompt bool) error {
	if wsID == "" {
		return errors.New("no workspace id provided")
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
		return errors.New("no domain set, re-authenticate")
	}

	// Use config deployment if provided
	if name == "" {
		name = config.CFG.ProjectDeployment.GetProjectString()
	}

	if name != "" && !deploymentNameExists(name, deployments) {
		return errors.New(messages.ErrHoustonDeploymentName)
	}

	// Prompt user for deployment if no deployment passed in
	if name == "" || prompt {
		if len(deployments) == 0 {
			return errors.New(messages.ErrNoHoustonDeployment)
		}

		fmt.Printf(messages.HoustonDeploymentHeader, cloudDomain)
		fmt.Println(messages.HoustonSelectDeploymentPrompt)

		deployMap := map[string]houston.Deployment{}
		for i := range deployments {
			d := deployments[i]
			index := i + 1
			tab.AddRow([]string{strconv.Itoa(index), d.Label, d.ReleaseName, currentWorkspace.Label, d.ID}, false)

			deployMap[strconv.Itoa(index)] = d
		}

		tab.Print(os.Stdout)
		choice := input.Text("\n> ")
		selected, ok := deployMap[choice]
		if !ok {
			return errors.New(messages.HoustonInvalidDeploymentKey)
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
	fmt.Println(repositoryName(name))

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

func buildPushDockerImage(c config.Context, name, path, nextTag, cloudDomain string) error {
	// We use latest and keep this tag around after deployments to keep subsequent deploys quick
	deployImage := imageName(name, "latest")

	// Build our image
	fmt.Println(messages.ComposeImageBuildingPrompt)

	// parse dockerfile
	cmds, err := docker.ParseFile(filepath.Join(path, "Dockerfile"))
	if err != nil {
		return errors.Wrapf(err, "failed to parse dockerfile: %s", filepath.Join(path, "Dockerfile"))
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
	diReq := houston.Request{
		Query: houston.DeploymentInfoRequest,
	}
	diResp, err := diReq.Do()
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
	err = imageBuild(path, deployImage)
	if err != nil {
		return err
	}
	registry := "registry." + cloudDomain
	remoteImage := fmt.Sprintf("%s/%s",
		registry, imageName(name, nextTag))
	err = docker.Exec("tag", deployImage, remoteImage)
	if err != nil {
		return errors.Wrapf(err, "command 'docker tag %s %s' failed", deployImage, remoteImage)
	}

	// Push image to registry
	fmt.Println(messages.ComposePushingImagePrompt)
	err = docker.ExecPush(registry, c.Token, remoteImage)
	if err != nil {
		return errors.Wrapf(err, "command 'docker push %s' failed", remoteImage)
	}
	// Delete the image tags we just generated
	err = docker.Exec("rmi", remoteImage)
	if err != nil {
		return errors.Wrapf(err, "command 'docker rmi %s' failed", remoteImage)
	}
	return nil
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

func airflowVersionFromDockerFile(airflowHome, dockerfile string) (uint64, error) {
	// parse dockerfile
	cmd, err := docker.ParseFile(filepath.Join(airflowHome, dockerfile))
	if err != nil {
		return 0, errors.Wrapf(err, "failed to parse dockerfile: %s", filepath.Join(airflowHome, dockerfile))
	}

	_, airflowTag := docker.GetImageTagFromParsedFile(cmd)

	semVer, err := semver.NewVersion(airflowTag)
	if err != nil {
		return defaultAirflowVersion, nil // Default to Airflow 1 if the user has a custom image without a semVer tag
	}

	return semVer.Major(), nil
}
