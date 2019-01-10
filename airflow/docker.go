package airflow

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/docker/libcompose/cli/logger"
	dockercompose "github.com/docker/libcompose/docker"
	"github.com/docker/libcompose/docker/ctx"
	"github.com/docker/libcompose/project"
	"github.com/docker/libcompose/project/options"
	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v2"

	"github.com/astronomer/astro-cli/airflow/include"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/docker"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/messages"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
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
		Header:         []string{"#", "LABEL", "RELEASE NAME", "WORKSPACE", "DEPLOYMENT UUID"},
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
}

// SettingsConfig is input data to generate connections, pools, and variables
type SettingsConfig struct {
	Airflow struct {
		Connections []struct {
			ConnID       string `yaml:"conn_id"`
			ConnType     string `yaml:"conn_type"`
			ConnHost     string `yaml:"conn_host"`
			ConnSchema   string `yaml:"conn_schema"`
			ConnLogin    string `yaml:"conn_login"`
			ConnPassword string `yaml:"conn_password"`
			ConnPort     int    `yaml:"conn_port"`
			ConnUri      string `yaml:"conn_uri"`
			ConnExtra    string `yaml:"conn_extra"`
		} `yaml:"connections"`
		Pools []struct {
			PoolName        string `yaml:"pool_name"`
			PoolSlot        int    `yaml:"pool_slot"`
			PoolDescription string `yaml:"pool_description"`
		} `yaml:"pools"`
		Variables []struct {
			VariableName  string `yaml:"variable_name"`
			VariableValue string `yaml:"variable_value"`
		} `yaml:"variables"`
	} `yaml:"airflow"`
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
	}

	buff := new(bytes.Buffer)
	err = tmpl.Execute(buff, config)
	if err != nil {
		return "", errors.Wrap(err, "failed to generate config")
	}

	return buff.String(), nil
}

func generateSettings() (SettingsConfig, error) {
	var settings SettingsConfig
	file := filepath.Join(config.WorkingPath, "settings.yaml")

	buff, err := ioutil.ReadFile(file)

	if err != nil {
		return settings, errors.Wrap(err, "failed to read settings file")
	}

	err = yaml.Unmarshal(buff, &settings)

	if err != nil {
		return settings, errors.Wrap(err, "failed to generate settings")
	}

	return settings, nil
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
	if envFile == "" {
		envFile = ".env"
	}
	// Generate the docker-compose yaml
	yaml, err := generateConfig(projectName, airflowHome, envFile)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create project")
	}

	// Create the project
	project, err := dockercompose.NewProject(&ctx.Context{
		Context: project.Context{
			ComposeBytes:  [][]byte{[]byte(yaml)},
			ProjectName:   projectName,
			LoggerFactory: logger.NewColorLoggerFactory(),
		},
	}, nil)

	return project, err
}

// Start starts a local airflow development cluster
func Start(airflowHome string, envFile string) error {
	// Get project name from config
	projectName := config.CFG.ProjectName.GetString()

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
	settings, err := generateSettings()
	if err != nil {
		return errors.Wrap(err, "failed to create project")
	}
	psInfo, err = project.Ps(context.Background())
	if err != nil {
		return errors.Wrap(err, messages.COMPOSE_STATUS_CHECK_ERROR)
	}
	for _, info := range psInfo {
		if strings.Contains(info["Name"], "scheduler") {
			for _, conn := range settings.Airflow.Connections {

				if len(conn.ConnID) == 0 {
					fmt.Print("Skipping Connection: Conn ID must be specified.")
				} else if len(conn.ConnType) == 0 && len(conn.ConnUri) == 0 {
					fmt.Printf("Skipping %s: ConnType or ConnUri must be specified.", conn.ConnID)
				} else {
					airflowCommand := fmt.Sprintf("airflow connections -a --conn_id \"%s\" --conn_type \"%s\" --conn_uri \"%s\" --conn_extra \"%s\" --conn_host  \"%s\" --conn_login \"%s\" --conn_password \"%s\" --conn_schema \"%s\" --conn_port \"%v\"", conn.ConnID, conn.ConnType, conn.ConnUri, conn.ConnExtra, conn.ConnHost, conn.ConnLogin, conn.ConnPassword, conn.ConnSchema, conn.ConnPort)
					cmd := exec.Command("docker", "exec", "-it", info["Id"], "bash", "-c", airflowCommand)
					cmd.Stdin = os.Stdin
					cmd.Stderr = os.Stderr
					if cmdErr := cmd.Run(); cmdErr != nil {
						return errors.Wrapf(cmdErr, "Error adding %s Connection", conn.ConnID)
					} else {
						fmt.Printf("Added Connection: %s\n", conn.ConnID)
					}
				}
			}
			for _, variable := range settings.Airflow.Variables {
				if len(variable.VariableName) == 0 && len(variable.VariableValue) > 0 {
					fmt.Print("Skipping Variable Creation: No Variable Name Specified.")

				} else {
					airflowCommand := fmt.Sprintf("airflow variables -s \"%s\" \"%s\"", variable.VariableName, variable.VariableValue)
					cmd := exec.Command("docker", "exec", "-it", info["Id"], "bash", "-c", airflowCommand)
					cmd.Stdin = os.Stdin
					cmd.Stderr = os.Stderr
					if cmdErr := cmd.Run(); cmdErr != nil {
						return errors.Wrapf(cmdErr, "Error adding %s Variable", variable.VariableName)
					} else {
						fmt.Printf("Added Variable: %s\n", variable.VariableName)
					}
				}
			}
			for _, pool := range settings.Airflow.Pools {
				if len(pool.PoolName) == 0 && pool.PoolSlot > 0 {
					fmt.Print("Skipping Pool Creation: No Pool Name Specified.")
				} else {
					airflowCommand := fmt.Sprintf("airflow pool -s \"%s\" \"%v\" \"%s\"", pool.PoolName, pool.PoolSlot, pool.PoolDescription)
					cmd := exec.Command("docker", "exec", "-it", info["Id"], "bash", "-c", airflowCommand)
					cmd.Stdin = os.Stdin
					cmd.Stderr = os.Stderr
					if cmdErr := cmd.Run(); cmdErr != nil {
						return errors.Wrapf(cmdErr, "Error adding %s Pool", pool.PoolName)
					} else {
						fmt.Printf("Added Pool: %s\n", pool.PoolName)
					}
				}
			}
		}
	}
	fmt.Printf(messages.COMPOSE_LINK_WEBSERVER+"\n", config.CFG.WebserverPort.GetString())
	fmt.Printf(messages.COMPOSE_LINK_POSTGRES+"\n", config.CFG.PostgresPort.GetString())
	return nil
}

// Kill stops a local airflow development cluster
func Kill(airflowHome string) error {
	// Get project name from config
	projectName := config.CFG.ProjectName.GetString()

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
	projectName := config.CFG.ProjectName.GetString()

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
	projectName := config.CFG.ProjectName.GetString()

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
	projectName := config.CFG.ProjectName.GetString()

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

// Deploy pushes a new docker image
func Deploy(path, name, wsId string, prompt bool) error {
	if len(wsId) == 0 {
		return errors.New("no workspace uuid provided")
	}

	// Validate workspace
	wsReq := houston.Request{
		Query:     houston.WorkspacesGetRequest,
		Variables: map[string]interface{}{"workspaceUuid": wsId},
	}

	wsResp, err := wsReq.Do()
	if err != nil {
		return err
	}

	if len(wsResp.Data.GetWorkspaces) == 0 {
		return fmt.Errorf("no workspaces with uuid (%s) found", wsId)
	}

	w := wsResp.Data.GetWorkspaces[0]

	// Get Deployments from workspace UUID
	deReq := houston.Request{
		Query:     houston.DeploymentsGetRequest,
		Variables: map[string]interface{}{"workspaceUuid": w.Uuid},
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
			tab.AddRow([]string{strconv.Itoa(index), d.Label, d.ReleaseName, w.Label, d.Id}, false)

			deployMap[strconv.Itoa(index)] = d
		}

		tab.Print()
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
	err = docker.Exec("push", remoteImage)
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
