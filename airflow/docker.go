package airflow

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	dockercompose "github.com/docker/libcompose/docker"
	"github.com/docker/libcompose/docker/ctx"
	"github.com/docker/libcompose/project"
	"github.com/docker/libcompose/project/options"
	"github.com/pkg/errors"

	"github.com/astronomerio/astro-cli/airflow/include"
	"github.com/astronomerio/astro-cli/config"
	"github.com/astronomerio/astro-cli/docker"
	"github.com/astronomerio/astro-cli/houston"
	"github.com/astronomerio/astro-cli/messages"
	"github.com/astronomerio/astro-cli/pkg/httputil"
	"github.com/astronomerio/astro-cli/pkg/input"
)

const (
	componentName     = "airflow"
	deployTagPrefix   = "cli-"
	dockerStateUp     = "Up"
	dockerStateExited = "Exited"
)

var (
	http = httputil.NewHTTPClient()
	api  = houston.NewHoustonClient(http)
)

// ComposeConfig is input data to docker compose yaml template
type ComposeConfig struct {
	PostgresUser         string
	PostgresPassword     string
	PostgresHost         string
	PostgresPort         string
	AirflowImage         string
	AirflowHome          string
	AirflowUser          string
	AirflowWebserverPort string
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
func imageBuild(path, imageName string) {
	// Change to location of Dockerfile
	os.Chdir(path)

	// Build image
	docker.Exec("build", "-t", imageName, ".")
}

// generateConfig generates the docker-compose config
func generateConfig(projectName, airflowHome string) string {
	tmpl, err := template.New("yml").Parse(include.Composeyml)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	config := ComposeConfig{
		PostgresUser:         config.CFG.PostgresUser.GetString(),
		PostgresPassword:     config.CFG.PostgresPassword.GetString(),
		PostgresHost:         config.CFG.PostgresHost.GetString(),
		PostgresPort:         config.CFG.PostgresPort.GetString(),
		AirflowImage:         imageName(projectName, "latest"),
		AirflowHome:          airflowHome,
		AirflowUser:          "astro",
		AirflowWebserverPort: "8080",
	}

	buff := new(bytes.Buffer)
	err = tmpl.Execute(buff, config)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	return buff.String()
}

func checkServiceState(serviceState, expectedState string) bool {
	scrubbedState := strings.Split(serviceState, " ")[0]

	if scrubbedState == expectedState {
		return true
	}

	return false
}

// createProject creates project with yaml config as context
func createProject(projectName, airflowHome string) (project.APIProject, error) {
	// Generate the docker-compose yaml
	yaml := generateConfig(projectName, airflowHome)

	// Create the project
	project, err := dockercompose.NewProject(&ctx.Context{
		Context: project.Context{
			ComposeBytes: [][]byte{[]byte(yaml)},
			ProjectName:  projectName,
		},
	}, nil)

	return project, err
}

// Start starts a local airflow development cluster
func Start(airflowHome string) error {
	// Get project name from config
	projectName := config.CFG.ProjectName.GetString()

	// Create a libcompose project
	project, err := createProject(projectName, airflowHome)
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
				fmt.Println(messages.COMPOSE_PROJECT_RUNNING)
				os.Exit(1)
			}
		}

		err = project.Start(context.Background())
		if err != nil {
			return errors.Wrap(err, messages.COMPOSE_RECREATE_ERROR)
		}
	} else {
		// Build this project image
		imageBuild(airflowHome, imageName(projectName, "latest"))

		// Start up our project
		err = project.Up(context.Background(), options.Up{})
		if err != nil {
			return errors.Wrap(err, messages.COMPOSE_RECREATE_ERROR)
		}
	}
	fmt.Println(messages.COMPOSE_LINK_WEBSERVER)
	fmt.Println(messages.COMPOSE_LINK_POSTGRES)
	return nil
}

// Kill stops a local airflow development cluster
func Kill(airflowHome string) error {
	// Get project name from config
	projectName := config.CFG.ProjectName.GetString()

	// Create a libcompose project
	project, err := createProject(projectName, airflowHome)
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

// Stop a running docker project
func Stop(airflowHome string) error {
	// Get project name from config
	projectName := config.CFG.ProjectName.GetString()

	// Create a libcompose project
	project, err := createProject(projectName, airflowHome)
	if err != nil {
		return errors.Wrap(err, messages.COMPOSE_CREATE_ERROR)
	}

	// Pause our project
	err = project.Stop(context.Background(), 30)
	if err != nil {
		return errors.Wrap(err, messages.COMPOSE_PAUSE_ERROR)
	}

	return nil
}

// PS prints the running airflow containers
func PS(airflowHome string) error {
	// Get project name from config
	projectName := config.CFG.ProjectName.GetString()

	// Create a libcompose project
	project, err := createProject(projectName, airflowHome)
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

// Deploy pushes a new docker image
func Deploy(path, name, wsId string) error {
	deployments, err := api.GetDeployments(wsId)
	if err != nil {
		return err
	}

	if name == "" {
		if len(deployments) == 0 {
			return errors.New(messages.HOUSTON_NO_DEPLOYMENTS_ERROR)
		}

		cloudDomain := config.CFG.CloudDomain.GetString()
		fmt.Printf(messages.HOUSTON_DEPLOYMENT_HEADER, cloudDomain)
		fmt.Println(messages.HOUSTON_SELECT_DEPLOYMENT_PROMPT)

		deployMap := map[string]houston.Deployment{}
		for i, deployment := range deployments {
			index := i + 1
			deployMap[strconv.Itoa(index)] = deployment
			fmt.Printf("%d) %s (%s)\n", index, deployment.Label, deployment.ReleaseName)
		}

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
			nextTag = deployment.DeploymentInfo.Next
		}
	}

	fmt.Printf(messages.HOUSTON_DEPLOYING_PROMPT, name)
	fmt.Println(repositoryName(name))

	// Build the image to deploy
	// We use latest and keep this tag around after deployments to keep subsequent deploys quick
	deployImage := imageName(name, "latest")

	// Build our image
	fmt.Println(messages.COMPOSE_IMAGE_BUILDING_PROMT)
	imageBuild(path, deployImage)

	// Tag our build with remote registry and incremented tag
	// tag := fmt.Sprintf("%s%d", deployTagPrefix, highestTag+1)

	registry := "registry." + config.CFG.CloudDomain.GetString()

	remoteImage := fmt.Sprintf("%s/%s",
		registry, imageName(name, nextTag))
	docker.Exec("tag", deployImage, remoteImage)

	// Push image to registry
	fmt.Println(messages.COMPOSE_PUSHING_IMAGE_PROMPT)
	docker.Exec("push", remoteImage)

	// Delete the image tags we just generated
	docker.Exec("rmi", remoteImage)

	return nil
}
