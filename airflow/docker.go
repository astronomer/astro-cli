package airflow

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"text/tabwriter"

	semver "github.com/Masterminds/semver/v3"
	"github.com/astronomer/astro-cli/airflow/include"
	airflowTypes "github.com/astronomer/astro-cli/airflow/types"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/docker"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/util"
	"github.com/astronomer/astro-cli/settings"
	composeInterp "github.com/compose-spec/compose-go/interpolation"
	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/command/inspect"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/flags"
	"github.com/docker/compose/v2/cmd/formatter"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/compose"
	docker_types "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/versions"
	"github.com/docker/docker/client"
	"github.com/pkg/errors"
)

const (
	componentName                  = "airflow"
	dockerStateUp                  = "running"
	defaultAirflowVersion          = uint64(0x2) // nolint:gomnd
	triggererAllowedRuntimeVersion = "4.0.0"
	triggererAllowedAirflowVersion = "2.2.0"

	composeCreateErrMsg      = "error creating docker-compose project"
	composeStatusCheckErrMsg = "error checking docker-compose status"
	composeRecreateErrMsg    = "error building, (re)creating or starting project containers"
	composePauseErrMsg       = "Error pausing project containers"
	composeStopErrMsg        = "Error stopping and removing containers"

	composeLinkWebserverMsg = "Airflow Webserver: %s"
	composeLinkPostgresMsg  = "Postgres Database: %s"
	composeUserPasswordMsg  = "The default Airflow UI credentials are: %s"
	postgresUserPasswordMsg = "The default Postgres DB credentials are: %s"

	envPathMsg     = "Error looking for \"%s\""
	envFoundMsg    = "Env file \"%s\" found. Loading...\n"
	envNotFoundMsg = "Env file \"%s\" not found. Skipping...\n"
)

var (
	errSettingsPath          = "error looking for settings.yaml"
	errComposeProjectRunning = errors.New("project is up and running")

	airflowSettingsFile = "airflow_settings.yaml"

	inspectContainer = inspect.Inspect
	initSettings     = settings.ConfigSettings
)

// ComposeConfig is input data to docker compose yaml template
type ComposeConfig struct {
	PytestFile           string
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
	TriggererEnabled     bool
}

type DockerCompose struct {
	airflowHome    string
	projectName    string
	envFile        string
	dockerfile     string
	composefile    string
	composeService DockerComposeAPI
	cliClient      DockerCLIClient
	imageHandler   ImageHandler
}

func DockerComposeInit(airflowHome, envFile, dockerfile, imageName string, isPyTestCompose bool) (*DockerCompose, error) {
	// Get project name from config
	projectName, err := ProjectNameUnique(isPyTestCompose)
	if err != nil {
		return nil, fmt.Errorf("error retrieving working directory: %w", err)
	}

	if imageName == "" {
		imageName = projectName
	}

	dockerClient, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("error initializing docker client: %w", err)
	}
	composeService := compose.NewComposeService(dockerClient, &configfile.ConfigFile{})
	imageHandler := DockerImageInit(ImageName(imageName, "latest"))

	composeFile := include.Composeyml
	if isPyTestCompose {
		composeFile = include.Pytestyml
	}

	dockerCli, err := command.NewDockerCli()
	if err != nil {
		log.Fatalf("error creating compose client %s", err)
	}

	err = dockerCli.Initialize(flags.NewClientOptions())
	if err != nil {
		log.Fatalf("error init compose client %s", err)
	}

	return &DockerCompose{
		airflowHome:    airflowHome,
		projectName:    projectName,
		envFile:        envFile,
		dockerfile:     dockerfile,
		composefile:    composeFile,
		composeService: composeService,
		cliClient:      dockerCli.Client(),
		imageHandler:   imageHandler,
	}, nil
}

// Start starts a local airflow development cluster
func (d *DockerCompose) Start(noCache bool) error {
	// Get project containers
	psInfo, err := d.composeService.Ps(context.Background(), d.projectName, api.PsOptions{
		All: true,
	})
	if err != nil {
		return errors.Wrap(err, composeCreateErrMsg)
	}
	if len(psInfo) > 0 {
		// Ensure project is not already running
		for i := range psInfo {
			if checkServiceState(psInfo[i].State, dockerStateUp) {
				return errors.New("cannot start, project already running")
			}
		}
	}

	// Build this project image
	err = d.imageHandler.Build(airflowTypes.ImageBuildConfig{Path: d.airflowHome, Output: true, NoCache: noCache})
	if err != nil {
		return err
	}

	imageLabels, err := d.imageHandler.ListLabels()
	if err != nil {
		return err
	}

	// Create a compose project
	project, err := createDockerProject(d.projectName, d.airflowHome, d.envFile, "", "", imageLabels, false)
	if err != nil {
		return errors.Wrap(err, composeCreateErrMsg)
	}

	// Start up our project
	err = d.composeService.Up(context.Background(), project, api.UpOptions{
		Create: api.CreateOptions{},
	})
	if err != nil {
		return errors.Wrap(err, composeRecreateErrMsg)
	}

	fmt.Println("\n\nAirflow is starting up! This might take a few minutes…")

	airflowDockerVersion := defaultAirflowVersion
	airflowVersion, ok := imageLabels[airflowVersionLabelName]
	if ok {
		if version := semver.MustParse(airflowVersion); version != nil {
			airflowDockerVersion = version.Major()
		}
	}

	err = checkWebserverHealth(project, d.composeService, airflowDockerVersion)
	if err != nil {
		return err
	}
	return nil
}

// Stop a running docker project
func (d *DockerCompose) Stop() error {
	imageLabels, err := d.imageHandler.ListLabels()
	if err != nil {
		return err
	}

	// Create a compose project
	project, err := createDockerProject(d.projectName, d.airflowHome, d.envFile, "", "", imageLabels, false)
	if err != nil {
		return errors.Wrap(err, composeCreateErrMsg)
	}

	// Pause our project
	err = d.composeService.Stop(context.Background(), project, api.StopOptions{})
	if err != nil {
		return errors.Wrap(err, composePauseErrMsg)
	}

	return nil
}

func (d *DockerCompose) PS() error {
	// List project containers
	psInfo, err := d.composeService.Ps(context.Background(), d.projectName, api.PsOptions{
		All: true,
	})
	if err != nil {
		return errors.Wrap(err, composeStatusCheckErrMsg)
	}

	// Columns for table
	infoColumns := []string{"Name", "State", "Ports"}

	// Create a new tabwriter
	tw := new(tabwriter.Writer)
	tw.Init(os.Stdout, 0, 8, 2, '\t', tabwriter.AlignRight) //nolint:gomnd

	// Append data to table
	fmt.Fprintln(tw, strings.Join(infoColumns, "\t"))
	for i := range psInfo {
		data := []string{psInfo[i].Name, psInfo[i].State}
		if len(psInfo[i].Publishers) != 0 {
			data = append(data, fmt.Sprint(psInfo[i].Publishers[0].PublishedPort))
		}
		fmt.Fprintln(tw, strings.Join(data, "\t"))
	}

	// Flush to stdout
	return tw.Flush()
}

// Kill stops a local airflow development cluster
func (d *DockerCompose) Kill() error {
	// Shut down our project
	err := d.composeService.Down(context.Background(), d.projectName, api.DownOptions{Volumes: true, RemoveOrphans: true})
	if err != nil {
		return errors.Wrap(err, composeStopErrMsg)
	}

	return nil
}

// Logs out airflow webserver or scheduler logs
func (d *DockerCompose) Logs(follow bool, containerNames ...string) error {
	psInfo, err := d.composeService.Ps(context.Background(), d.projectName, api.PsOptions{
		All: true,
	})
	if err != nil {
		return errors.Wrap(err, composeStatusCheckErrMsg)
	}
	if len(psInfo) == 0 {
		return errors.New("cannot view logs, project not running")
	}

	consumer := formatter.NewLogConsumer(context.Background(), os.Stdout, true, false)

	err = d.composeService.Logs(context.Background(), d.projectName, consumer, api.LogOptions{
		Services: containerNames,
		Follow:   follow,
	})
	if err != nil {
		return err
	}

	return nil
}

// Run creates using docker exec
// inspired from https://github.com/docker/cli/tree/master/cli/command/container
func (d *DockerCompose) Run(args []string, user string) error {
	execConfig := &docker_types.ExecConfig{
		AttachStdout: true,
		Cmd:          args,
	}
	if user != "" {
		execConfig.User = user
	}

	fmt.Printf("Running: %s\n", strings.Join(args, " "))
	containerID, err := d.getWebServerContainerID()
	if err != nil {
		return err
	}

	response, err := d.cliClient.ContainerExecCreate(context.Background(), containerID, *execConfig)
	if err != nil {
		fmt.Println(err)
		return errors.New("airflow is not running. To start a local Airflow environment, run 'astro dev start'")
	}

	execID := response.ID
	if execID == "" {
		return errors.New("exec ID is empty")
	}

	execStartCheck := docker_types.ExecStartCheck{
		Detach: execConfig.Detach,
	}

	resp, _ := d.cliClient.ContainerExecAttach(context.Background(), execID, execStartCheck)

	return docker.ExecPipe(resp, os.Stdin, os.Stdout, os.Stderr)
}

// Pytest creates and runs a container containing the users airflow image, requirments, packages, and volumes(DAGs folder, etc...)
// These containers runs pytest on a specified pytest file (pytestFile). This function is used in the dev parse and dev pytest commands
func (d *DockerCompose) Pytest(pytestFile, projectImageName string) (string, error) {
	// projectImageName may be provided to the function if it is being used in the deploy command
	if projectImageName == "" {
		var err error
		// get same image as what's used for other dev commands
		// This makes it so we don't have to rebuild this image if it was used in other commands
		projectImageName, err = ProjectNameUnique(false)
		if err != nil {
			return "", errors.Wrap(err, "error retrieving working directory")
		}

		projectImageName = ImageName(projectImageName, "latest")
		// build image
		err = d.imageHandler.Build(airflowTypes.ImageBuildConfig{Path: d.airflowHome, Output: true})
		if err != nil {
			return "", err
		}
	}

	imageLabels, err := d.imageHandler.ListLabels()
	if err != nil {
		return "", err
	}

	// Create a compose project
	project, err := createDockerProject(d.projectName, d.airflowHome, d.envFile, pytestFile, projectImageName, imageLabels, true)
	if err != nil {
		return "", errors.Wrap(err, composeCreateErrMsg)
	}

	consumer := formatter.NewLogConsumer(context.Background(), os.Stdout, true, true)

	// runs the equivalent of 'docker-compose up' on the file located at include/pytestyml(project)
	err = d.composeService.Up(context.Background(), project, api.UpOptions{
		Create: api.CreateOptions{
			IgnoreOrphans: true,
		},
		Start: api.StartOptions{
			Attach:   consumer,
			AttachTo: project.ServiceNames(), // streams logs
		},
	})
	if err != nil {
		return "", errors.Wrap(err, composeRecreateErrMsg)
	}

	defer func() {
		// runs the equivalent of 'docker-compose down' to kill compose project
		err = d.composeService.Down(context.Background(), d.projectName, api.DownOptions{Volumes: true, RemoveOrphans: true})
		if err != nil {
			log.Printf("%s: %s", composeStopErrMsg, err.Error())
		}
	}()

	// runs the equivalent of 'docker-compose ps' on the project to get the container name
	psInfo, err := d.composeService.Ps(context.Background(), project.Name, api.PsOptions{
		All: true,
	})
	if err != nil {
		return "", errors.Wrap(err, composeStatusCheckErrMsg)
	}

	// capture exit code
	refs := []string{}
	for i := range psInfo {
		if strings.Contains(psInfo[i].Name, "test-1") {
			refs = []string{psInfo[i].Name}
		}
	}

	if len(refs) == 0 {
		return "", errors.New("error finding the testing container")
	}

	ctx := context.Background()
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf) // creates writer to capture exit code
	getRefFunc := func(ref string) (interface{}, []byte, error) {
		return d.cliClient.ContainerInspectWithRaw(ctx, ref, false)
	}

	// runs the equivalent of 'docker container inspect --format '{{.State.ExitCode}}' <container name>' to get the exit code
	err = inspectContainer(w, refs, "{{.State.ExitCode}}", getRefFunc)
	if err != nil {
		return "", errors.Wrap(err, composeStopErrMsg)
	}
	// Flush() sends the output from the writer to buf
	w.Flush()

	if strings.Contains(buf.String(), "0") { // if the error code is 0 the pytests passed
		return "", nil
	}
	return buf.String(), errors.New("something went wrong while Pytesting your DAGs")
}

func (d *DockerCompose) Parse(buildImage string) error {
	// check for file
	path := d.airflowHome + "/" + DefaultTestPath

	fileExist, err := util.Exists(path)
	if err != nil {
		return err
	}
	if !fileExist {
		fmt.Println("\nThe file " + path + " which is needed for `astro dev parse` does not exist. Please run `astro dev init` to create it")

		return err
	}

	fmt.Println("\nChecking your DAGs for errors,\nthis might take a minute if you haven't run this command before…")

	pytestFile := DefaultTestPath
	exitCode, err := d.Pytest(pytestFile, buildImage)
	if err != nil {
		if strings.Contains(exitCode, "1") { // exit code is 1 meaning tests failed
			return errors.New("errors detected in your local DAGs are listed above")
		}
		return errors.Wrap(err, "something went wrong while parsing your DAGs")
	}
	fmt.Println("\nno errors detected in your local DAGs")
	return err
}

// getWebServerContainerId return webserver container id
func (d *DockerCompose) getWebServerContainerID() (string, error) {
	psInfo, err := d.composeService.Ps(context.Background(), d.projectName, api.PsOptions{
		All: true,
	})
	if err != nil {
		return "", errors.Wrap(err, composeStatusCheckErrMsg)
	}

	replacer := strings.NewReplacer("_", "", "-", "")
	strippedProjectName := replacer.Replace(d.projectName)
	for i := range psInfo {
		if strings.Contains(replacer.Replace(psInfo[i].Name), strippedProjectName) &&
			strings.Contains(psInfo[i].Name, WebserverDockerContainerName) {
			return psInfo[i].ID, nil
		}
	}
	return "", err
}

// createProject creates project with yaml config as context
var createDockerProject = func(projectName, airflowHome, envFile, pytestFile, buildImage string, imageLabels map[string]string, pytest bool) (*types.Project, error) {
	// Generate the docker-compose yaml
	yaml, err := generateConfig(projectName, airflowHome, envFile, pytestFile, buildImage, imageLabels, pytest)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create project")
	}

	var configs []types.ConfigFile

	configs = append(configs, types.ConfigFile{
		Filename: "compose.yaml",
		Content:  []byte(yaml),
	})

	if !pytest {
		composeFile := "docker-compose.override.yml"
		composeBytes, err := os.ReadFile(composeFile)
		if err != nil && !os.IsNotExist(err) {
			return nil, errors.Wrapf(err, "Failed to open the compose file: %s", composeFile)
		}
		if err == nil {
			configs = append(configs, types.ConfigFile{
				Filename: "docker-compose.override.yml",
				Content:  composeBytes,
			})
		}
	}

	var loadOptions []func(*loader.Options)

	nameLoadOpt := func(opts *loader.Options) {
		opts.Name = projectName
		opts.Name = normalizeName(opts.Name)
		opts.Interpolate = &composeInterp.Options{
			LookupValue: os.LookupEnv,
		}
	}

	loadOptions = append(loadOptions, nameLoadOpt)

	project, err := loader.Load(types.ConfigDetails{
		ConfigFiles: configs,
		WorkingDir:  airflowHome,
	}, loadOptions...)

	return project, err
}

var checkWebserverHealth = func(project *types.Project, composeService api.Service, airflowDockerVersion uint64) error {
	// check if webserver is healthy for user
	err := composeService.Events(context.Background(), project.Name, api.EventsOptions{
		Services: []string{WebserverDockerContainerName}, Consumer: func(event api.Event) error {
			marshal, err := json.Marshal(map[string]interface{}{
				"action": event.Status,
			})
			if err != nil {
				return err
			}

			if string(marshal) == `{"action":"health_status: healthy"}` {
				psInfo, err := composeService.Ps(context.Background(), project.Name, api.PsOptions{
					All: true,
				})
				if err != nil {
					return errors.Wrap(err, composeStatusCheckErrMsg)
				}

				fileState, err := fileutil.Exists(airflowSettingsFile, nil)
				if err != nil {
					return errors.Wrap(err, errSettingsPath)
				}

				if fileState {
					for i := range psInfo {
						if strings.Contains(psInfo[i].Name, project.Name) &&
							strings.Contains(psInfo[i].Name, WebserverDockerContainerName) {
							err = initSettings(psInfo[i].ID, airflowDockerVersion)
							if err != nil {
								return err
							}
						}
					}
				}

				fmt.Println("\nProject is running! All components are now available.")
				parts := strings.Split(config.CFG.WebserverPort.GetString(), ":")
				fmt.Printf("\n"+composeLinkWebserverMsg+"\n", ansi.Bold("http://localhost:"+parts[len(parts)-1]))
				fmt.Printf(composeLinkPostgresMsg+"\n", ansi.Bold("localhost:"+config.CFG.PostgresPort.GetString()+"/postgres"))
				fmt.Printf(composeUserPasswordMsg+"\n", ansi.Bold("admin:admin"))
				fmt.Printf(postgresUserPasswordMsg+"\n", ansi.Bold("postgres:postgres"))
				return errComposeProjectRunning
			}
			return nil
		},
	})
	if err != nil && !errors.Is(err, errComposeProjectRunning) {
		return err
	}
	return nil
}

// CheckTriggererEnabled checks if the airflow triggerer component should be enabled.
// for astro-runtime users: check if compatible runtime version
// for AC users, triggerer is only compatible with Airflow versions >= 2.2.0
// the runtime version and airflow version can be found as a label on the user's docker image
var CheckTriggererEnabled = func(imageLabels map[string]string) (bool, error) {
	airflowVersion, ok := imageLabels[airflowVersionLabelName]
	if ok {
		if versions.GreaterThanOrEqualTo(airflowVersion, triggererAllowedAirflowVersion) {
			return true, nil
		}
		return false, nil
	}

	runtimeVersion, ok := imageLabels[runtimeVersionLabelName]
	if !ok {
		// image doesn't have either runtime version or airflow version
		// we don't want to block the user's experience in case this happens, so we disable triggerer and warn error
		fmt.Println(warningTriggererDisabledNoVersionDetectedMsg)

		return false, nil
	}

	return versions.GreaterThanOrEqualTo(runtimeVersion, triggererAllowedRuntimeVersion), nil
}

func checkServiceState(serviceState, expectedState string) bool {
	scrubbedState := strings.Split(serviceState, " ")[0]
	return scrubbedState == expectedState
}
