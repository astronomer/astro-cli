package airflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	semver "github.com/Masterminds/semver/v3"
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
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/flags"
	"github.com/docker/compose/v2/cmd/formatter"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/compose"
	docker_types "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/versions"
	"github.com/pkg/browser"
	"github.com/pkg/errors"
)

const (
	componentName                  = "airflow"
	dockerStateUp                  = "running"
	defaultAirflowVersion          = uint64(0x2) //nolint:gomnd
	triggererAllowedRuntimeVersion = "4.0.0"
	triggererAllowedAirflowVersion = "2.2.0"
	pytestDirectory                = "tests"
	OpenCmd                        = "open"

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
	errNoFile                = errors.New("file specified does not exist")
	errSettingsPath          = "error looking for settings.yaml"
	errComposeProjectRunning = errors.New("project is up and running")

	initSettings      = settings.ConfigSettings
	exportSettings    = settings.Export
	envExportSettings = settings.EnvExport

	openURL        = browser.OpenURL
	timeoutNum     = 60
	tickNum        = 500
	startupTimeout time.Duration
	isM1           = util.IsM1

	composeOverrideFilename = "docker-compose.override.yml"
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
	SettingsFile         string
	SettingsFileExist    bool
	TriggererEnabled     bool
	ProjectName          string
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

func DockerComposeInit(airflowHome, envFile, dockerfile, imageName string) (*DockerCompose, error) {
	// Get project name from config
	projectName, err := ProjectNameUnique()
	if err != nil {
		return nil, fmt.Errorf("error retrieving working directory: %w", err)
	}

	if imageName == "" {
		imageName = projectName
	}

	imageHandler := DockerImageInit(ImageName(imageName, "latest"))
	composeFile := Composeyml

	dockerCli, err := command.NewDockerCli()
	if err != nil {
		log.Fatalf("error creating compose client %s", err)
	}

	err = dockerCli.Initialize(flags.NewClientOptions())
	if err != nil {
		log.Fatalf("error init compose client %s", err)
	}

	composeService := compose.NewComposeService(dockerCli.Client(), &configfile.ConfigFile{})

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
func (d *DockerCompose) Start(imageName, settingsFile string, noCache, noBrowser bool, waitTime time.Duration) error {
	// check if docker is up for macOS
	if runtime.GOOS == "darwin" {
		err := startDocker()
		if err != nil {
			return err
		}
	}

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
	if imageName == "" {
		// add astro-run-dag package
		err = fileutil.AddLineToFile("./requirements.txt", "astro-run-dag", "# This package is needed for the astro run command. It will be removed before a deploy")
		if err != nil {
			fmt.Printf("Adding 'astro-run-dag' package to requirements.txt unsuccessful: %s\nManually add package to requirements.txt", err.Error())
		}
		imageBuildErr := d.imageHandler.Build(airflowTypes.ImageBuildConfig{Path: d.airflowHome, Output: true, NoCache: noCache})
		// remove astro-run-dag from requirments.txt
		err = fileutil.RemoveLineFromFile("./requirements.txt", "astro-run-dag", " # This package is needed for the astro run command. It will be removed before a deploy")
		if err != nil {
			fmt.Printf("Removing line 'astro-run-dag' package from requirements.txt unsuccessful: %s\n", err.Error())
		}
		if imageBuildErr != nil {
			return imageBuildErr
		}
	} else {
		// skip build if an imageName is passed
		err := d.imageHandler.TagLocalImage(imageName)
		if err != nil {
			return err
		}
	}

	imageLabels, err := d.imageHandler.ListLabels()
	if err != nil {
		return err
	}

	// Create a compose project
	project, err := createDockerProject(d.projectName, d.airflowHome, d.envFile, "", settingsFile, imageLabels)
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

	airflowDockerVersion, err := d.checkAiflowVersion()
	if err != nil {
		return err
	}

	startupTimeout = waitTime
	// check if user provided a waitTime
	// default is 1 minute
	if waitTime != 1*time.Minute {
		startupTimeout = waitTime
	} else if isM1(runtime.GOOS, runtime.GOARCH) {
		// user did not provide a waitTime
		// if running darwin/M1 architecture
		// we wait for a longer startup time
		startupTimeout = 5 * time.Minute
	}

	err = checkWebserverHealth(settingsFile, project, d.composeService, airflowDockerVersion, noBrowser, startupTimeout)
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
	project, err := createDockerProject(d.projectName, d.airflowHome, d.envFile, "", "", imageLabels)
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
func (d *DockerCompose) Pytest(pytestArgs []string, customImageName, deployImageName string) (string, error) {
	// deployImageName may be provided to the function if it is being used in the deploy command
	if deployImageName == "" {
		// build image
		if customImageName == "" {
			err := d.imageHandler.Build(airflowTypes.ImageBuildConfig{Path: d.airflowHome, Output: true})
			if err != nil {
				return "", err
			}
		} else {
			// skip build if an customImageName is passed
			err := d.imageHandler.TagLocalImage(customImageName)
			if err != nil {
				return "", err
			}
		}
	}

	// determine pytest args and file
	var pytestFile string
	if len(pytestArgs) > 0 {
		pytestFile = pytestArgs[0]
	}
	if len(strings.Fields(pytestFile)) > 1 {
		pytestArgs = strings.Fields(pytestFile)
		pytestFile = ""
	} else if len(pytestArgs) > 1 {
		pytestArgs = strings.Fields(pytestArgs[1])
	}

	// Determine pytest file
	if pytestFile != ".astro/test_dag_integrity_default.py" {
		if !strings.Contains(pytestFile, pytestDirectory) {
			pytestFile = pytestDirectory + "/" + pytestFile
		} else if pytestFile == "" {
			pytestFile = pytestDirectory + "/"
		}
	}

	// run pytests
	exitCode, err := d.imageHandler.Pytest(pytestFile, d.airflowHome, d.envFile, pytestArgs, airflowTypes.ImageBuildConfig{Path: d.airflowHome, Output: true})
	if err != nil {
		return exitCode, err
	}
	if strings.Contains(exitCode, "0") { // if the error code is 0 the pytests passed
		return "", nil
	}
	return exitCode, errors.New("something went wrong while Pytesting your DAGs")
}

func (d *DockerCompose) Parse(customImageName, deployImageName string) error {
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
	pytestArgs := []string{pytestFile}
	exitCode, err := d.Pytest(pytestArgs, customImageName, deployImageName)
	if err != nil {
		if strings.Contains(exitCode, "1") { // exit code is 1 meaning tests failed
			return errors.New("See above for errors detected in your DAGs")
		}
		return errors.Wrap(err, "something went wrong while parsing your DAGs")
	}
	fmt.Println("\n" + ansi.Green("✔") + " no errors detected in your DAGs ")
	return err
}

func (d *DockerCompose) Bash(container string) error {
	// exec into schedueler by default
	if container == "" {
		container = SchedulerDockerContainerName
	}

	// query for container names
	psInfo, err := d.composeService.Ps(context.Background(), d.projectName, api.PsOptions{
		All: true,
	})
	if err != nil {
		return errors.Wrap(err, composeStatusCheckErrMsg)
	}
	if len(psInfo) == 0 {
		return errors.New("cannot exec into container, project not running")
	}
	// find container name of specified container
	var containerName string
	for i := range psInfo {
		if strings.Contains(psInfo[i].Name, container) {
			containerName = psInfo[i].Name
		}
	}
	// exec into container
	err = cmdExec(DockerCmd, os.Stdout, os.Stderr, "exec", "-it", containerName, "bash")
	if err != nil {
		return err
	}
	return nil
}

func (d *DockerCompose) ExportSettings(settingsFile, envFile string, connections, variables, pools, envExport bool) error {
	// setup bools
	if !connections && !variables && !pools {
		connections = true
		variables = true
		pools = true
	}

	// Get project containers
	containerID, err := d.getWebServerContainerID()
	if err != nil {
		return err
	}

	// Get airflow version
	airflowDockerVersion, err := d.checkAiflowVersion()
	if err != nil {
		return err
	}

	fileState, err := fileutil.Exists(settingsFile, nil)
	if err != nil {
		return errors.Wrap(err, errSettingsPath)
	}
	if !fileState {
		return errNoFile
	}

	if envExport {
		err = envExportSettings(containerID, envFile, airflowDockerVersion, connections, variables)
		if err != nil {
			return err
		}
		fmt.Println("\nAirflow objects exported to env file")
		return nil
	}

	err = exportSettings(containerID, settingsFile, airflowDockerVersion, connections, variables, pools)
	if err != nil {
		return err
	}
	fmt.Println("\nAirflow objects exported to settings file")
	return nil
}

func (d *DockerCompose) ImportSettings(settingsFile, envFile string, connections, variables, pools bool) error {
	// setup bools
	if !connections && !variables && !pools {
		connections = true
		variables = true
		pools = true
	}

	// Get project containers
	containerID, err := d.getWebServerContainerID()
	if err != nil {
		return err
	}

	// Get airflow version
	airflowDockerVersion, err := d.checkAiflowVersion()
	if err != nil {
		return err
	}

	fileState, err := fileutil.Exists(settingsFile, nil)
	if err != nil {
		return errors.Wrap(err, errSettingsPath)
	}
	if !fileState {
		return errNoFile
	}

	err = initSettings(containerID, settingsFile, airflowDockerVersion, connections, variables, pools)
	if err != nil {
		return err
	}
	fmt.Println("\nAirflow objects created from settings file")
	return nil
}

func (d *DockerCompose) getWebServerContainerID() (string, error) {
	psInfo, err := d.composeService.Ps(context.Background(), d.projectName, api.PsOptions{
		All: true,
	})
	if err != nil {
		return "", errors.Wrap(err, composeStatusCheckErrMsg)
	}
	if len(psInfo) == 0 {
		return "", errors.New("project not running, run astro dev start to start project")
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

func (d *DockerCompose) RunDAG(dagID, settingsFile, dagFile string, noCache, taskLogs bool) error {
	// Get project containers
	psInfo, err := d.composeService.Ps(context.Background(), d.projectName, api.PsOptions{
		All: true,
	})
	if err != nil {
		return errors.Wrap(err, composeStatusCheckErrMsg)
	}
	if len(psInfo) > 0 {
		// In case the project is already running, run the dag test directly on the scheduler container
		for i := range psInfo {
			if checkServiceState(psInfo[i].State, dockerStateUp) {
				if strings.Contains(psInfo[i].Name, SchedulerDockerContainerName) {
					err = d.imageHandler.Run(dagID, d.envFile, settingsFile, psInfo[i].Name, dagFile, taskLogs)
					if err != nil {
						return err
					}
					return nil
				}
			}
		}
	}

	fmt.Println("Building image... For a faster 'astro run' experience run this command while Airflow is running with 'astro dev start'\n ")
	// add astro-run-dag
	err = fileutil.AddLineToFile("./requirements.txt", "astro-run-dag", "# This package is needed for the astro run command. It will be removed before a deploy")
	if err != nil {
		fmt.Printf("Adding 'astro-run-dag' package to requirements.txt unsuccessful: %s\nManually add package to requirements.txt", err.Error())
	}
	// add airflow db init
	err = fileutil.AddLineToFile("./Dockerfile", "RUN airflow db init", "")
	if err != nil {
		fmt.Printf("Adding line 'RUN airflow db init' to Dockerfile unsuccessful: %s\nYou may need to manually add this line for 'astro run' to work", err.Error())
	}
	defer func() {
		// remove airflow db init
		fileErr := fileutil.RemoveLineFromFile("./Dockerfile", "RUN airflow db init", "")
		if fileErr != nil {
			fmt.Printf("Removing line 'RUN airflow db init' from Dockerfile unsuccessful: %s\n", err.Error())
		}
		// remove astro-run-dag from requirments.txt
		err = fileutil.RemoveLineFromFile("./requirements.txt", "astro-run-dag", " # This package is needed for the astro run command. It will be removed before a deploy")
		if err != nil {
			fmt.Printf("Removing line 'astro-run-dag' package from requirements.txt unsuccessful: %s\n", err.Error())
		}
	}()
	err = d.imageHandler.Build(airflowTypes.ImageBuildConfig{Path: d.airflowHome, Output: true, NoCache: noCache})
	if err != nil {
		return err
	}

	err = d.imageHandler.Run(dagID, d.envFile, settingsFile, "", dagFile, taskLogs)
	if err != nil {
		return err
	}
	return nil
}

func (d *DockerCompose) checkAiflowVersion() (uint64, error) {
	imageLabels, err := d.imageHandler.ListLabels()
	if err != nil {
		return 0, err
	}

	airflowDockerVersion := defaultAirflowVersion
	airflowVersion, ok := imageLabels[airflowVersionLabelName]
	if ok {
		if version, err := semver.NewVersion(airflowVersion); err == nil {
			airflowDockerVersion = version.Major()
		} else {
			fmt.Printf("unable to parse airflow version, defaulting to major version 2, error: %s", err.Error())
		}
	}
	return airflowDockerVersion, nil
}

// createProject creates project with yaml config as context
var createDockerProject = func(projectName, airflowHome, envFile, buildImage, settingsFile string, imageLabels map[string]string) (*types.Project, error) {
	// Generate the docker-compose yaml
	yaml, err := generateConfig(projectName, airflowHome, envFile, buildImage, settingsFile, imageLabels)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create project")
	}

	var configs []types.ConfigFile

	configs = append(configs, types.ConfigFile{
		Filename: "compose.yaml",
		Content:  []byte(yaml),
	})

	composeBytes, err := os.ReadFile(composeOverrideFilename)
	if err != nil && !os.IsNotExist(err) {
		return nil, errors.Wrapf(err, "Failed to open the compose file: %s", composeOverrideFilename)
	}
	if err == nil {
		configs = append(configs, types.ConfigFile{
			Filename: "docker-compose.override.yml",
			Content:  composeBytes,
		})
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

var checkWebserverHealth = func(settingsFile string, project *types.Project, composeService api.Service, airflowDockerVersion uint64, noBrowser bool, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	// check if webserver is healthy for user
	err := composeService.Events(ctx, project.Name, api.EventsOptions{
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

				fileState, err := fileutil.Exists(settingsFile, nil)
				if err != nil {
					return errors.Wrap(err, errSettingsPath)
				}

				if fileState {
					for i := range psInfo {
						if strings.Contains(psInfo[i].Name, project.Name) &&
							strings.Contains(psInfo[i].Name, WebserverDockerContainerName) {
							err = initSettings(psInfo[i].ID, settingsFile, airflowDockerVersion, true, true, true)
							if err != nil {
								return err
							}
						}
					}
				}

				fmt.Println("\nProject is running! All components are now available.")
				parts := strings.Split(config.CFG.WebserverPort.GetString(), ":")
				webserverURL := "http://localhost:" + parts[len(parts)-1]
				fmt.Printf("\n"+composeLinkWebserverMsg+"\n", ansi.Bold(webserverURL))
				fmt.Printf(composeLinkPostgresMsg+"\n", ansi.Bold("localhost:"+config.CFG.PostgresPort.GetString()+"/postgres"))
				fmt.Printf(composeUserPasswordMsg+"\n", ansi.Bold("admin:admin"))
				fmt.Printf(postgresUserPasswordMsg+"\n", ansi.Bold("postgres:postgres"))
				if !(noBrowser || util.CheckEnvBool(os.Getenv("ASTRONOMER_NO_BROWSER"))) {
					err = openURL(webserverURL)
					if err != nil {
						fmt.Println("\nUnable to open the webserver URL, please visit the following link: " + webserverURL)
					}
				}
				return errComposeProjectRunning
			}

			return nil
		},
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			fmt.Printf("\n")
			return fmt.Errorf("there might be a problem with your project starting up. The webserver health check timed out after %s but your project will continue trying to start. Run 'astro dev logs --webserver | --scheduler' for details.\n\nTry again or use the --wait flag to increase the time out", timeout) //nolint:goerr113
		}
		if !errors.Is(err, errComposeProjectRunning) {
			return err
		}
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

func startDocker() error {
	buf := new(bytes.Buffer)
	err := cmdExec(DockerCmd, buf, buf, "ps")
	if err != nil {
		// open docker
		fmt.Println("\nDocker is not running. Starting up the Docker engine…")
		err = cmdExec(OpenCmd, buf, os.Stderr, "-a", "docker")
		if err != nil {
			return err
		}
		fmt.Println("\nIf you don't see Docker Desktop starting, exit this command and start it manually.")
		fmt.Println("If you don't have Docker Desktop installed, install it (https://www.docker.com/products/docker-desktop/) and try again.")
		fmt.Println("If you are using Colima or another Docker alternative, start the engine manually.")
		// poll for docker
		err = waitForDocker()
		if err != nil {
			return err
		}
	}
	return nil
}

func waitForDocker() error {
	buf := new(bytes.Buffer)
	timeout := time.After(time.Duration(timeoutNum) * time.Second)
	ticker := time.NewTicker(time.Duration(tickNum) * time.Millisecond)
	for {
		select {
		// Got a timeout! fail with a timeout error
		case <-timeout:
			return errors.New("timed out waiting for docker")
		// Got a tick, we should check if docker is up & running
		case <-ticker.C:
			buf.Reset()
			err := cmdExec(DockerCmd, buf, buf, "ps")
			if err != nil {
				continue
			} else {
				return nil
			}
		}
	}
}
