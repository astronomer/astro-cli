package airflow

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"html/template"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"

	"github.com/docker/libcompose/project"
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
	componentName     = "airflow"
	deployTagPrefix   = "cli-"
	dockerStateUp     = "Up"
	dockerStateExited = "Exited"
)

var (
	tab = printutil.Table{
		Padding:        []int{5, 30, 30, 50},
		DynamicPadding: true,
		Header:         []string{"#", "LABEL", "DEPLOYMENT NAME", "WORKSPACE", "DEPLOYMENT ID"},
	}
)

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
	os.Chdir(path)

	// Build image
	err := docker.Exec("build", "-t", imageName, ".")
	if err != nil {
		return errors.Wrapf(err, "command 'docker build -t %s failed", imageName)
	}

	return nil
}

// generateConfig generates the docker-compose config
func generateConfig(projectName, airflowHome string, envFile string) (string, error) {
	tmpl, err := template.New("yml").Parse(include.Composeyml)
	if err != nil {
		return "", errors.Wrap(err, "failed to generate config")
	}

	envExists, err := fileutil.Exists(envFile)

	if err != nil {
		return "", errors.Wrapf(err, messages.ENV_PATH, envFile)
	}

	if envFile != "" {
		if !envExists {
			fmt.Printf(messages.ENV_NOT_FOUND, envFile)
			envFile = ""
		} else {
			fmt.Printf(messages.ENV_FOUND, envFile)
			envFile = fmt.Sprintf("env_file: %s", envFile)
		}
	}

	config := ComposeConfig{
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
	err = tmpl.Execute(buff, config)
	if err != nil {
		return "", errors.Wrap(err, "failed to generate config")
	}

	return buff.String(), nil
}

func checkServiceState(serviceState, expectedState string) bool {
	scrubbedState := strings.Split(serviceState, " ")[0]

	if scrubbedState == expectedState {
		return true
	}

	return false
}

// createProject creates project with yaml config as context
func createProject(projectName, airflowHome string, envFile string) (project.APIProject, error) {
	// Generate the docker-compose yaml
	yaml, err := generateConfig(projectName, airflowHome, envFile)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create project")
	}
	composeCtx := project.Context{
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
	for _, deployment := range deployments {
		if deployment.ReleaseName == name {
			return true
		}
	}
	return false
}

// Start starts a local airflow development cluster
func Start(airflowHome string, envFile string) error {
	// Get project name from config
	projectName, err := projectNameUnique()
	replacer := strings.NewReplacer("_", "", "-", "")
	strippedProjectName := replacer.Replace(projectName)

	if err != nil {
		return errors.Wrap(err, "error retrieving working directory")
	}

	// Create a libcompose project
	project, err := createProject(projectName, airflowHome, envFile)
	if err != nil {
		return errors.Wrap(err, messages.COMPOSE_CREATE_ERROR)
	}

	// Get project containers
	psInfo, err := project.Ps(context.Background())
	if err != nil {
		return errors.Wrap(err, messages.COMPOSE_STATUS_CHECK_ERROR)
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
			return errors.Wrap(err, messages.COMPOSE_RECREATE_ERROR)
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
			return errors.Wrap(err, messages.COMPOSE_RECREATE_ERROR)
		}

	}

	psInfo, err = project.Ps(context.Background())
	if err != nil {
		return errors.Wrap(err, messages.COMPOSE_STATUS_CHECK_ERROR)
	}

	fileState, err := fileutil.Exists("airflow_settings.yaml")

	if err != nil {
		return errors.Wrap(err, messages.SETTINGS_PATH)
	}

	if fileState {
		for _, info := range psInfo {
			if strings.Contains(info["Name"], strippedProjectName) &&
				strings.Contains(info["Name"], "webserver") {
				settings.ConfigSettings(info["Id"])
			}
		}
	}

	parts := strings.Split(config.CFG.WebserverPort.GetString(), ":")
	fmt.Printf(messages.COMPOSE_LINK_WEBSERVER+"\n", parts[len(parts)-1])
	fmt.Printf(messages.COMPOSE_LINK_POSTGRES+"\n", config.CFG.PostgresPort.GetString())
	fmt.Printf(messages.COMPOSE_USER_PASSWORD + "\n")

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
		return errors.Wrap(err, messages.COMPOSE_CREATE_ERROR)
	}

	// Shut down our project
	err = project.Down(context.Background(), options.Down{RemoveVolume: true, RemoveOrphans: true})
	if err != nil {
		return errors.Wrap(err, messages.COMPOSE_STOP_ERROR)
	}

	return nil
}

// Logs out airflow webserver or scheduler logs
func Logs(airflowHome string, webserver, scheduler, follow bool) error {
	s := make([]string, 0)

	// Get project name from config
	projectName, err := projectNameUnique()

	// Create libcompose project
	project, err := createProject(projectName, airflowHome, "")
	if err != nil {
		return errors.Wrap(err, messages.COMPOSE_CREATE_ERROR)
	}

	psInfo, err := project.Ps(context.Background())
	if err != nil {
		return errors.Wrap(err, messages.COMPOSE_STATUS_CHECK_ERROR)
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
		return errors.Wrap(err, messages.COMPOSE_CREATE_ERROR)
	}

	// Pause our project
	err = project.Stop(context.Background(), 5)
	if err != nil {
		return errors.Wrap(err, messages.COMPOSE_PAUSE_ERROR)
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
		return errors.Wrap(err, messages.COMPOSE_CREATE_ERROR)
	}

	// List project containers
	psInfo, err := project.Ps(context.Background())
	if err != nil {
		return errors.Wrap(err, messages.COMPOSE_STATUS_CHECK_ERROR)
	}

	// Columns for table
	infoColumns := []string{"Name", "State", "Ports"}

	// Create a new tabwriter
	tw := new(tabwriter.Writer)
	tw.Init(os.Stdout, 0, 8, 2, '\t', tabwriter.AlignRight)

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

// getWebServerContainerId return webserver container id
func getWebServerContainerId(airflowHome string) (string, error) {
	projectName, err := projectNameUnique()

	project, err := createProject(projectName, airflowHome, "")
	if err != nil {
		return "", errors.Wrap(err, messages.COMPOSE_CREATE_ERROR)
	}

	psInfo, err := project.Ps(context.Background())

	if err != nil {
		return "", errors.Wrap(err, messages.COMPOSE_STATUS_CHECK_ERROR)
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
func Run(airflowHome string, args []string) error {
	cli, err := client.NewClientWithOpts(client.FromEnv)

	if err != nil {
		return err
	}

	execConfig := &types.ExecConfig{
		AttachStdout: true,
		Tty:          true,
		Cmd:          args,
	}
	fmt.Printf("Running: %s\n", strings.Join(args, " "))
	containerID, err := getWebServerContainerId(airflowHome)
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
		Tty:    execConfig.Tty,
	}

	resp, _ := cli.ContainerExecAttach(context.Background(), execID, execStartCheck)

	// Read stdout response from container
	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(resp.Reader)
	s := buf.String()
	fmt.Println(s)

	return err
}

// Deploy pushes a new docker image
func Deploy(path, name, wsId string, prompt bool) error {
	if len(wsId) == 0 {
		return errors.New("no workspace id provided")
	}

	// Validate workspace
	wsReq := houston.Request{
		Query:     houston.WorkspacesGetRequest,
		Variables: map[string]interface{}{"workspaceId": wsId},
	}

	wsResp, err := wsReq.Do()
	if err != nil {
		return err
	}

	if len(wsResp.Data.GetWorkspaces) == 0 {
		return fmt.Errorf("no workspaces with id (%s) found", wsId)
	}

	var currentWorkspace houston.Workspace
	for _, workspace := range wsResp.Data.GetWorkspaces {
		if workspace.Id == wsId {
			currentWorkspace = workspace
			break
		}
	}

	// Get Deployments from workspace ID
	deReq := houston.Request{
		Query:     houston.DeploymentsGetRequest,
		Variables: map[string]interface{}{"workspaceId": currentWorkspace.Id},
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
	if len(cloudDomain) == 0 {
		return errors.New("No domain set, re-authenticate.")
	}

	// Use config deployment if provided
	if len(name) == 0 {
		name = config.CFG.ProjectDeployment.GetProjectString()
	}

	if len(name) != 0 && !deploymentNameExists(name, deployments) {
		return errors.New(messages.HOUSTON_DEPLOYMENT_NAME_ERROR)
	}

	// Prompt user for deployment if no deployment passed in
	if len(name) == 0 || prompt {
		if len(deployments) == 0 {
			return errors.New(messages.HOUSTON_NO_DEPLOYMENTS_ERROR)
		}

		fmt.Printf(messages.HOUSTON_DEPLOYMENT_HEADER, cloudDomain)
		fmt.Println(messages.HOUSTON_SELECT_DEPLOYMENT_PROMPT)

		deployMap := map[string]houston.Deployment{}
		for i, d := range deployments {
			index := i + 1
			tab.AddRow([]string{strconv.Itoa(index), d.Label, d.ReleaseName, currentWorkspace.Label, d.Id}, false)

			deployMap[strconv.Itoa(index)] = d
		}

		tab.Print(os.Stdout)
		choice := input.InputText("\n> ")
		selected, ok := deployMap[choice]
		if !ok {
			return errors.New(messages.HOUSTON_INVALID_DEPLOYMENT_KEY)
		}
		name = selected.ReleaseName
	}

	nextTag := ""
	for _, deployment := range deployments {
		if deployment.ReleaseName == name {
			nextTag = deployment.DeploymentInfo.NextCli
		}
	}

	fmt.Printf(messages.HOUSTON_DEPLOYING_PROMPT, name)
	fmt.Println(repositoryName(name))

	// Build the image to deploy
	// We use latest and keep this tag around after deployments to keep subsequent deploys quick
	deployImage := imageName(name, "latest")

	// Build our image
	fmt.Println(messages.COMPOSE_IMAGE_BUILDING_PROMT)

	// parse dockerfile
	cmds, err := docker.ParseFile(filepath.Join(path, "Dockerfile"))
	if err != nil {
		return errors.Wrapf(err, "failed to parse dockerfile: %s", filepath.Join(path, "Dockerfile"))
	}

	image, tag := docker.GetImageTagFromParsedFile(cmds)
	if config.CFG.ShowWarnings.GetBool() && !validImageRepo(image) {
		i, _ := input.InputConfirm(fmt.Sprintf(messages.WARNING_INVALID_IMAGE_NAME, image))
		if !i {
			fmt.Println("Cancelling deploy...")
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
		i, _ := input.InputConfirm(fmt.Sprintf(messages.WARNING_INVALID_IMAGE_TAG, tag, validTags))
		if !i {
			fmt.Println("Cancelling deploy...")
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
	fmt.Println(messages.COMPOSE_PUSHING_IMAGE_PROMPT)
	err = docker.ExecPush(registry, c.Token, remoteImage)
	if err != nil {
		return errors.Wrapf(err, "command 'docker push %s' failed", remoteImage)
	}

	// Delete the image tags we just generated
	err = docker.Exec("rmi", remoteImage)
	if err != nil {
		return errors.Wrapf(err, "command 'docker rmi %s' failed", remoteImage)
	}

	fmt.Println("Deploy succeeded!")

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
