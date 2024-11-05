package airflow

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	semver "github.com/Masterminds/semver/v3"
	airflowTypes "github.com/astronomer/astro-cli/airflow/types"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	"github.com/astronomer/astro-cli/cloud/deployment"
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
	"github.com/sirupsen/logrus"
)

const (
	RuntimeImageLabel              = "io.astronomer.docker.runtime.version"
	AirflowImageLabel              = "io.astronomer.docker.airflow.version"
	componentName                  = "airflow"
	podman                         = "podman"
	dockerStateUp                  = "running"
	dockerExitState                = "exited"
	defaultAirflowVersion          = uint64(0x2) //nolint:gomnd
	triggererAllowedRuntimeVersion = "4.0.0"
	triggererAllowedAirflowVersion = "2.2.0"
	pytestDirectory                = "tests"
	OpenCmd                        = "open"
	dockerCmd                      = "docker"
	registryUsername               = "cli"
	unknown                        = "unknown"
	major                          = "major"
	patch                          = "patch"
	minor                          = "minor"
	partsNum                       = 2

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
	errNoFile                  = errors.New("file specified does not exist")
	errSettingsPath            = "error looking for settings.yaml"
	errCustomImageDoesNotExist = errors.New("The custom image provided either does not exist or Docker is unable to connect to the repository")

	initSettings      = settings.ConfigSettings
	exportSettings    = settings.Export
	envExportSettings = settings.EnvExport

	openURL    = browser.OpenURL
	timeoutNum = 60
	tickNum    = 500

	majorUpdatesAirflowProviders    = []string{}
	minorUpdatesAirflowProviders    = []string{}
	patchUpdatesAirflowProviders    = []string{}
	unknownUpdatesAirflowProviders  = []string{}
	removedPackagesAirflowProviders = []string{}
	addedPackagesAirflowProviders   = []string{}
	majorUpdates                    = []string{}
	minorUpdates                    = []string{}
	patchUpdates                    = []string{}
	unknownUpdates                  = []string{}
	removedPackages                 = []string{}
	addedPackages                   = []string{}
	airflowUpdate                   = []string{}
	titles                          = []string{
		"Apache Airflow Update:\n",
		"Airflow Providers Unknown Updates:\n",
		"Airflow Providers Major Updates:\n",
		"Airflow Providers Minor Updates:\n",
		"Airflow Providers Patch Updates:\n",
		"Added Airflow Providers:\n",
		"Removed Airflow Providers:\n",
		"Unknown Updates:\n",
		"Major Updates:\n",
		"Minor Updates:\n",
		"Patch Updates:\n",
		"Added Packages:\n",
		"Removed Packages:\n",
	}
	composeOverrideFilename = "docker-compose.override.yml"

	stopPostgresWaitTimeout = 10 * time.Second
	stopPostgresWaitTicker  = 1 * time.Second
)

// ComposeConfig is input data to docker compose yaml template
type ComposeConfig struct {
	PytestFile            string
	PostgresUser          string
	PostgresPassword      string
	PostgresHost          string
	PostgresPort          string
	PostgresRepository    string
	PostgresTag           string
	AirflowEnvFile        string
	AirflowImage          string
	AirflowHome           string
	AirflowUser           string
	AirflowWebserverPort  string
	AirflowExposePort     bool
	MountLabel            string
	SettingsFile          string
	SettingsFileExist     bool
	DuplicateImageVolumes bool
	TriggererEnabled      bool
	ProjectName           string
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
		logrus.Fatalf("error creating compose client %s", err)
	}

	err = dockerCli.Initialize(flags.NewClientOptions())
	if err != nil {
		logrus.Fatalf("error init compose client %s", err)
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
//
//nolint:gocognit
func (d *DockerCompose) Start(imageName, settingsFile, composeFile, buildSecretString string, noCache, noBrowser bool, waitTime time.Duration, envConns map[string]astrocore.EnvironmentObjectConnection) error {
	// check if docker is up for macOS
	if runtime.GOOS == "darwin" && config.CFG.DockerCommand.GetString() == dockerCmd {
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
		if !config.CFG.DisableAstroRun.GetBool() {
			// add astro-run-dag package
			err = fileutil.AddLineToFile("./requirements.txt", "astro-run-dag", "# This package is needed for the astro run command. It will be removed before a deploy")
			if err != nil {
				fmt.Printf("Adding 'astro-run-dag' package to requirements.txt unsuccessful: %s\nManually add package to requirements.txt", err.Error())
			}
		}
		imageBuildErr := d.imageHandler.Build(d.dockerfile, buildSecretString, airflowTypes.ImageBuildConfig{Path: d.airflowHome, Output: true, NoCache: noCache})
		if !config.CFG.DisableAstroRun.GetBool() {
			// remove astro-run-dag from requirments.txt
			err = fileutil.RemoveLineFromFile("./requirements.txt", "astro-run-dag", " # This package is needed for the astro run command. It will be removed before a deploy")
			if err != nil {
				fmt.Printf("Removing line 'astro-run-dag' package from requirements.txt unsuccessful: %s\n", err.Error())
			}
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
	project, err := createDockerProject(d.projectName, d.airflowHome, d.envFile, "", settingsFile, composeFile, imageLabels)
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

	fmt.Println("\n\nAirflow is starting up!")

	airflowDockerVersion, err := d.checkAiflowVersion()
	if err != nil {
		return err
	}

	// Airflow webserver should be hosted at localhost
	// from the perspective of the CLI running on the host machine.
	webserverPort := config.CFG.WebserverPort.GetString()
	healthURL := fmt.Sprintf("http://localhost:%s/health", webserverPort)

	// Check the health of the webserver, up to the timeout.
	// If we fail to get a 200 status code, we'll return an error message.
	err = checkWebserverHealth(healthURL, waitTime)
	if err != nil {
		return err
	}

	// If we've successfully gotten a healthcheck response, print the status.
	err = printStatus(settingsFile, envConns, project, d.composeService, airflowDockerVersion, noBrowser)
	if err != nil {
		return err
	}

	return nil
}

func (d *DockerCompose) ComposeExport(settingsFile, composeFile string) error {
	// Get project containers
	_, err := d.composeService.Ps(context.Background(), d.projectName, api.PsOptions{
		All: true,
	})
	if err != nil {
		return err
	}

	// get image lables
	imageLabels, err := d.imageHandler.ListLabels()
	if err != nil {
		return err
	}

	// Generate the docker-compose yaml
	yaml, err := generateConfig(d.projectName, d.airflowHome, d.envFile, "", settingsFile, imageLabels)
	if err != nil {
		return errors.Wrap(err, "failed to create Compose file")
	}

	// write the yaml to a file
	err = os.WriteFile(composeFile, []byte(yaml), 0o666) //nolint:gosec, gomnd
	if err != nil {
		return errors.Wrap(err, "failed to write to compose file")
	}

	return nil
}

// Stop a running docker project
func (d *DockerCompose) Stop(waitForExit bool) error {
	imageLabels, err := d.imageHandler.ListLabels()
	if err != nil {
		return err
	}

	// Create a compose project
	project, err := createDockerProject(d.projectName, d.airflowHome, d.envFile, "", "", "", imageLabels)
	if err != nil {
		return errors.Wrap(err, composeCreateErrMsg)
	}

	// Pause our project
	err = d.composeService.Stop(context.Background(), project, api.StopOptions{})
	if err != nil {
		return errors.Wrap(err, composePauseErrMsg)
	}

	if !waitForExit {
		return nil
	}

	// Adding check on wether all containers have exited or not, because in case of restart command with immediate start after stop execution,
	// in windows machine it take a fraction of second for container to be in exited state, after docker compose completes the stop command execution
	// causing the dev start for airflow to fail
	timeout := time.After(stopPostgresWaitTimeout)
	ticker := time.NewTicker(stopPostgresWaitTicker)
	for {
		select {
		case <-timeout:
			logrus.Debug("timed out waiting for postgres container to be in exited state")
			return nil
		case <-ticker.C:
			psInfo, _ := d.composeService.Ps(context.Background(), d.projectName, api.PsOptions{
				All: true,
			})
			for i := range psInfo {
				// we only need to check for postgres container state, since all other containers depends on postgres container
				// so docker compose will ensure that postgres container going in shutting down phase only after all other containers have exited
				if strings.Contains(psInfo[i].Name, PostgresDockerContainerName) {
					if psInfo[i].State == dockerExitState {
						logrus.Debug("postgres container reached exited state")
						return nil
					}
					logrus.Debugf("postgres container is still in %s state, waiting for it to be in exited state", psInfo[i].State)
				}
			}
		}
	}
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
func (d *DockerCompose) Pytest(pytestFile, customImageName, deployImageName, pytestArgsString, buildSecretString string) (string, error) {
	// deployImageName may be provided to the function if it is being used in the deploy command
	if deployImageName == "" {
		// build image
		if customImageName == "" {
			err := d.imageHandler.Build(d.dockerfile, buildSecretString, airflowTypes.ImageBuildConfig{Path: d.airflowHome, Output: true})
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
	pytestArgs := strings.Fields(pytestArgsString)

	// Determine pytest file
	if pytestFile != ".astro/test_dag_integrity_default.py" {
		if !strings.Contains(pytestFile, pytestDirectory) {
			pytestFile = pytestDirectory + "/" + pytestFile
		} else if pytestFile == "" {
			pytestFile = pytestDirectory + "/"
		}
	}

	// run pytests
	exitCode, err := d.imageHandler.Pytest(pytestFile, d.airflowHome, d.envFile, "", pytestArgs, false, airflowTypes.ImageBuildConfig{Path: d.airflowHome, Output: true})
	if err != nil {
		return exitCode, err
	}
	if strings.Contains(exitCode, "0") { // if the error code is 0 the pytests passed
		return "", nil
	}
	return exitCode, errors.New("something went wrong while Pytesting your DAGs")
}

func (d *DockerCompose) UpgradeTest(newAirflowVersion, deploymentID, newImageName, customImage, buildSecretString string, conflictTest, versionTest, dagTest bool, astroPlatformCore astroplatformcore.CoreClient) error {
	// figure out which tests to run
	if !versionTest && !dagTest {
		versionTest = true
		dagTest = true
	}
	var deploymentImage string
	// if custom image is used get new Airflow version
	if customImage != "" {
		err := d.imageHandler.DoesImageExist(customImage)
		if err != nil {
			return errCustomImageDoesNotExist
		}
		newAirflowVersion = strings.SplitN(customImage, ":", partsNum)[1]
	}
	// if user supplies deployment id pull down current image
	if deploymentID != "" {
		err := d.pullImageFromDeployment(deploymentID, astroPlatformCore)
		if err != nil {
			return err
		}
	} else {
		// build image for current Airflow version to get current Airflow version
		fmt.Println("\nBuilding image for current Airflow version")
		imageBuildErr := d.imageHandler.Build(d.dockerfile, buildSecretString, airflowTypes.ImageBuildConfig{Path: d.airflowHome, Output: true})
		if imageBuildErr != nil {
			return imageBuildErr
		}
	}
	// get current Airflow version
	currentAirflowVersion, err := d.imageHandler.GetLabel(deploymentImage, RuntimeImageLabel)
	if err != nil {
		return err
	}
	if currentAirflowVersion == "" {
		currentAirflowVersion, err = d.imageHandler.GetLabel(deploymentImage, AirflowImageLabel)
		if err != nil {
			return err
		}
	}
	// create test home directory
	testHomeDirectory := "upgrade-test-" + currentAirflowVersion + "--" + newAirflowVersion

	destFolder := filepath.Join(d.airflowHome, testHomeDirectory)
	var filePerms fs.FileMode = 0o755
	if err := os.MkdirAll(destFolder, filePerms); err != nil {
		return err
	}
	newDockerFile := destFolder + "/Dockerfile"

	// check for dependency conflicts
	if conflictTest {
		err = d.conflictTest(testHomeDirectory, newImageName, newAirflowVersion)
		if err != nil {
			return err
		}
	}
	if versionTest {
		err := d.versionTest(testHomeDirectory, currentAirflowVersion, deploymentImage, newDockerFile, newAirflowVersion, customImage, buildSecretString)
		if err != nil {
			return err
		}
	}
	var errorCode int
	if dagTest {
		errorCode, err = d.dagTest(testHomeDirectory, newAirflowVersion, newDockerFile, customImage, buildSecretString)
		if err != nil {
			return err
		}
	}
	fmt.Println("\nTest Summary:")
	fmt.Printf("\tUpgrade Test Results Directory: %s\n", testHomeDirectory)
	if conflictTest {
		fmt.Printf("\tDependency Conflict Test Results file: %s\n", "conflict-test-results.txt")
	}
	if versionTest {
		fmt.Printf("\tDependency Version Comparison Results file: %s\n", "dependency_compare.txt")
	}
	if dagTest {
		fmt.Printf("\tDAG Parse Test HTML Report: %s\n", "dag-test-report.html")
	}
	if errorCode == 1 {
		return errors.New("one of the tests run above failed")
	}

	return nil
}

func (d *DockerCompose) pullImageFromDeployment(deploymentID string, platformCoreClient astroplatformcore.CoreClient) error {
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}
	ws := c.Workspace
	currentDeployment, err := deployment.GetDeployment(ws, deploymentID, "", true, nil, platformCoreClient, nil)
	if err != nil {
		return err
	}
	deploymentImage := fmt.Sprintf("%s:%s", currentDeployment.ImageRepository, currentDeployment.ImageTag)
	token := c.Token
	fmt.Printf("\nPulling image from Astro Deployment %s\n\n", currentDeployment.Name)
	err = d.imageHandler.Pull(deploymentImage, registryUsername, token)
	if err != nil {
		return err
	}
	return nil
}

func (d *DockerCompose) conflictTest(testHomeDirectory, newImageName, newAirflowVersion string) error {
	fmt.Println("\nChecking your 'requirments.txt' for dependency conflicts with the new version of Airflow")
	fmt.Println("\nThis may take a few minutes...")

	// create files needed for conflict test
	err := initConflictTest(config.WorkingPath, newImageName, newAirflowVersion)
	defer os.Remove("conflict-check.Dockerfile")
	if err != nil {
		return err
	}

	exitCode, conflictErr := d.imageHandler.ConflictTest(d.airflowHome, testHomeDirectory, airflowTypes.ImageBuildConfig{Path: d.airflowHome, Output: true})
	if conflictErr != nil {
		return conflictErr
	}
	if strings.Contains(exitCode, "0") || exitCode == "" { // if the error code is 0 the pytests passed
		fmt.Println("There were no dependency conflicts found")
	} else {
		fmt.Println("\nSomething went wrong while compiling your dependencies check the logs above for conflicts")
		fmt.Println("If there are conflicts remove them from your 'requirments.txt' and rerun this test\nYou will see the best candidate in the 'conflict-test-results.txt' file")
		return err
	}
	return nil
}

func (d *DockerCompose) versionTest(testHomeDirectory, currentAirflowVersion, deploymentImage, newDockerFile, newAirflowVersion, customImage, buildSecretString string) error {
	fmt.Println("\nComparing dependency versions between current and upgraded environment")
	// pip freeze old Airflow image
	fmt.Println("\nObtaining pip freeze for current Airflow version")
	currentAirflowPipFreezeFile := d.airflowHome + "/" + testHomeDirectory + "/pip_freeze_" + currentAirflowVersion + ".txt"
	err := d.imageHandler.CreatePipFreeze(deploymentImage, currentAirflowPipFreezeFile)
	if err != nil {
		return err
	}

	// build image with the new airflow version
	err = upgradeDockerfile(d.dockerfile, newDockerFile, newAirflowVersion, customImage)
	if err != nil {
		return err
	}
	fmt.Println("\nBuilding image for new Airflow version")
	imageBuildErr := d.imageHandler.Build(newDockerFile, buildSecretString, airflowTypes.ImageBuildConfig{Path: d.airflowHome, Output: true})
	if imageBuildErr != nil {
		return imageBuildErr
	}
	// pip freeze new airflow image
	fmt.Println("\nObtaining pip freeze for new Airflow version")
	newAirflowPipFreezeFile := d.airflowHome + "/" + testHomeDirectory + "/pip_freeze_" + newAirflowVersion + ".txt"
	err = d.imageHandler.CreatePipFreeze("", newAirflowPipFreezeFile)
	if err != nil {
		return err
	}
	// compare pip freeze files
	fmt.Println("\nComparing pip freeze files")
	pipFreezeCompareFile := d.airflowHome + "/" + testHomeDirectory + "/dependency_compare.txt"
	err = CreateVersionTestFile(currentAirflowPipFreezeFile, newAirflowPipFreezeFile, pipFreezeCompareFile)
	if err != nil {
		return err
	}
	fmt.Printf("Pip Freeze comparison can be found at \n" + pipFreezeCompareFile)
	return nil
}

func (d *DockerCompose) dagTest(testHomeDirectory, newAirflowVersion, newDockerFile, customImage, buildSecretString string) (int, error) {
	fmt.Printf("\nChecking the DAGs in this project for errors against the new Airflow version %s\n", newAirflowVersion)

	// build image with the new runtime version
	err := upgradeDockerfile(d.dockerfile, newDockerFile, newAirflowVersion, customImage)
	if err != nil {
		return 1, err
	}

	reqFile := d.airflowHome + "/requirements.txt"
	// add pytest-html to the requirements
	err = fileutil.AddLineToFile(reqFile, "pytest-html", "# This package is needed for the upgrade dag test. It will be removed once the test is over")
	if err != nil {
		fmt.Printf("Adding 'pytest-html' package to requirements.txt unsuccessful: %s\nManually add package to requirements.txt", err.Error())
	}
	fmt.Println("\nBuilding image for new Airflow version")
	imageBuildErr := d.imageHandler.Build(newDockerFile, buildSecretString, airflowTypes.ImageBuildConfig{Path: d.airflowHome, Output: true})

	// remove pytest-html to the requirements
	err = fileutil.RemoveLineFromFile(reqFile, "pytest-html", " # This package is needed for the upgrade dag test. It will be removed once the test is over")
	if err != nil {
		fmt.Printf("Removing package 'pytest-html' from requirements.txt unsuccessful: %s\n", err.Error())
	}
	if imageBuildErr != nil {
		return 1, imageBuildErr
	}
	// check for file
	path := d.airflowHome + "/" + DefaultTestPath

	fileExist, err := util.Exists(path)
	if err != nil {
		return 1, err
	}
	if !fileExist {
		fmt.Println("\nThe file " + path + " which is needed for the parse test does not exist. Please run `astro dev init` to create it")
		return 1, err
	}
	// run parse test
	pytestFile := DefaultTestPath
	// create html report
	htmlReportArgs := "--html=dag-test-report.html --self-contained-html"
	// compare pip freeze files
	fmt.Println("\nRunning DAG parse test with the new Airflow version")
	exitCode, err := d.imageHandler.Pytest(pytestFile, d.airflowHome, d.envFile, testHomeDirectory, strings.Fields(htmlReportArgs), true, airflowTypes.ImageBuildConfig{Path: d.airflowHome, Output: true})
	if err != nil {
		if strings.Contains(exitCode, "1") { // exit code is 1 meaning tests failed
			fmt.Println("See above for errors detected in your DAGs")
			return 1, nil
		} else {
			return 1, errors.Wrap(err, "something went wrong while parsing your DAGs")
		}
	} else {
		fmt.Println("\n" + ansi.Green("✔") + " no errors detected in your DAGs ")
	}
	return 0, nil
}

func upgradeDockerfile(oldDockerfilePath, newDockerfilePath, newTag, newImage string) error { //nolint:gocognit
	// Read the content of the old Dockerfile
	content, err := os.ReadFile(oldDockerfilePath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	var newContent strings.Builder
	if newImage == "" {
		for _, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "FROM quay.io/astronomer/astro-runtime:") {
				// Replace the tag on the matching line
				parts := strings.SplitN(line, ":", partsNum)
				if len(parts) == partsNum {
					line = parts[0] + ":" + newTag
				}
			}
			if strings.HasPrefix(strings.TrimSpace(line), "FROM quay.io/astronomer/ap-airflow:") {
				isRuntime, err := isRuntimeVersion(newTag)
				if err != nil {
					logrus.Debug(err)
				}
				if isRuntime {
					// Replace the tag on the matching line
					parts := strings.SplitN(line, "/", partsNum)
					if len(parts) >= partsNum {
						line = parts[0] + "/astronomer/astro-runtime:" + newTag
					}
				} else {
					// Replace the tag on the matching line
					parts := strings.SplitN(line, ":", partsNum)
					if len(parts) == partsNum {
						line = parts[0] + ":" + newTag
					}
				}
			}
			newContent.WriteString(line + "\n") // Add a newline after each line
		}
	} else {
		for _, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "FROM ") {
				// Replace the tag on the matching line
				parts := strings.SplitN(line, " ", partsNum)
				if len(parts) == partsNum {
					line = parts[0] + " " + newImage
				}
			}
			newContent.WriteString(line + "\n") // Add a newline after each line
		}
	}

	// Write the new content to the new Dockerfile
	err = os.WriteFile(newDockerfilePath, []byte(newContent.String()), 0o600) //nolint:gomnd
	if err != nil {
		return err
	}

	return nil
}

func isRuntimeVersion(versionStr string) (bool, error) {
	// Parse the version string
	v, err := semver.NewVersion(versionStr)
	if err != nil {
		return false, err
	}

	// Runtime versions start 4.0.0 to not get confused with Airflow versions that are currently in 2.X.X
	referenceVersion := semver.MustParse("4.0.0")

	// Compare the parsed version with the reference version
	return v.Compare(referenceVersion) > 0, nil
}

func CreateVersionTestFile(beforeFile, afterFile, outputFile string) error {
	// Open the before file for reading
	before, err := os.Open(beforeFile)
	if err != nil {
		return err
	}
	defer before.Close()

	// Open the after file for reading
	after, err := os.Open(afterFile)
	if err != nil {
		return err
	}
	defer after.Close()

	// Create the output file for writing
	output, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer output.Close()

	// Create a map to store versions by package name
	pgkVersions := make(map[string][2]string)

	// Read versions from the before file and store them in the map
	beforeScanner := bufio.NewScanner(before)
	for beforeScanner.Scan() {
		line := beforeScanner.Text()
		parts := strings.Split(line, "==")
		if len(parts) == partsNum {
			pkg := parts[0]
			ver := parts[1]
			pgkVersions[pkg] = [2]string{ver, ""}
		}
	}

	// Read versions from the after file and update the map with the new versions
	afterScanner := bufio.NewScanner(after)
	for afterScanner.Scan() {
		line := afterScanner.Text()
		parts := strings.Split(line, "==")
		if len(parts) == partsNum {
			pkg := parts[0]
			ver := parts[1]
			if v, ok := pgkVersions[pkg]; ok {
				v[1] = ver
				pgkVersions[pkg] = v
			} else {
				pgkVersions[pkg] = [2]string{"", ver}
			}
		}
	}
	// Iterate over the versions map and categorize the changes
	err = iteratePkgMap(pgkVersions)
	if err != nil {
		return err
	}
	// sort lists into alphabetical order
	sort.Strings(unknownUpdatesAirflowProviders)
	sort.Strings(majorUpdatesAirflowProviders)
	sort.Strings(minorUpdatesAirflowProviders)
	sort.Strings(patchUpdatesAirflowProviders)
	sort.Strings(addedPackagesAirflowProviders)
	sort.Strings(removedPackagesAirflowProviders)
	sort.Strings(unknownUpdates)
	sort.Strings(majorUpdates)
	sort.Strings(minorUpdates)
	sort.Strings(patchUpdates)
	sort.Strings(addedPackages)
	sort.Strings(removedPackages)
	pkgLists := [][]string{
		airflowUpdate,
		unknownUpdatesAirflowProviders,
		majorUpdatesAirflowProviders,
		minorUpdatesAirflowProviders,
		patchUpdatesAirflowProviders,
		addedPackagesAirflowProviders,
		removedPackagesAirflowProviders,
		unknownUpdates,
		majorUpdates,
		minorUpdates,
		patchUpdates,
		addedPackages,
		removedPackages,
	}

	// Write the categorized updates to the output file
	writer := bufio.NewWriter(output)

	for i, title := range titles {
		writeToCompareFile(title, pkgLists[i], writer)
	}

	// Flush the buffer to ensure all data is written to the file
	writer.Flush()
	return nil
}

func iteratePkgMap(pgkVersions map[string][2]string) error { //nolint:gocognit
	// Iterate over the versions map and categorize the changes
	for pkg, ver := range pgkVersions {
		beforeVer := ver[0]
		afterVer := ver[1]
		if beforeVer != "" && afterVer != "" && beforeVer != afterVer {
			change, updateType, err := checkVersionChange(beforeVer, afterVer)
			if err != nil {
				if err.Error() == "Invalid Semantic Version" {
					updateType = unknown
				} else {
					return err
				}
			}
			if !change {
				updateType = unknown
			}
			pkgUpdate := pkg + " " + beforeVer + " >> " + afterVer

			// Categorize the packages based on the update type
			categorizeAirflowProviderPackage(pkg, pkgUpdate, updateType)
		}
		switch {
		case strings.Contains(pkg, "apache-airflow-providers-"):
			if beforeVer != "" && afterVer == "" {
				pkgUpdate := pkg + "==" + beforeVer
				removedPackagesAirflowProviders = append(removedPackagesAirflowProviders, pkgUpdate)
			}
			if beforeVer == "" && afterVer != "" {
				pkgUpdate := pkg + "==" + afterVer
				addedPackagesAirflowProviders = append(addedPackagesAirflowProviders, pkgUpdate)
			}
		default:
			if beforeVer != "" && afterVer == "" {
				pkgUpdate := pkg + "==" + beforeVer
				removedPackages = append(removedPackages, pkgUpdate)
			}
			if beforeVer == "" && afterVer != "" {
				pkgUpdate := pkg + "==" + afterVer
				addedPackages = append(addedPackages, pkgUpdate)
			}
		}
	}
	return nil
}

func categorizeAirflowProviderPackage(pkg, pkgUpdate, updateType string) {
	// Categorize the packages based on the update type
	switch {
	case strings.Contains(pkg, "apache-airflow-providers-"):
		switch updateType {
		case major:
			majorUpdatesAirflowProviders = append(majorUpdatesAirflowProviders, pkgUpdate)
		case minor:
			minorUpdatesAirflowProviders = append(minorUpdatesAirflowProviders, pkgUpdate)
		case patch:
			patchUpdatesAirflowProviders = append(patchUpdatesAirflowProviders, pkgUpdate)
		case unknown:
			unknownUpdatesAirflowProviders = append(unknownUpdatesAirflowProviders, pkgUpdate)
		}
	case pkg == "apache-airflow":
		airflowUpdate = append(airflowUpdate, pkgUpdate)
	default:
		switch updateType {
		case major:
			majorUpdates = append(majorUpdates, pkgUpdate)
		case minor:
			minorUpdates = append(minorUpdates, pkgUpdate)
		case patch:
			patchUpdates = append(patchUpdates, pkgUpdate)
		case unknown:
			unknownUpdates = append(unknownUpdates, pkgUpdate)
		}
	}
}

func writeToCompareFile(title string, pkgList []string, writer *bufio.Writer) {
	if len(pkgList) > 0 {
		_, err := writer.WriteString(title)
		if err != nil {
			logrus.Debug(err)
		}
		for _, pkg := range pkgList {
			_, err = writer.WriteString(pkg + "\n")
			if err != nil {
				logrus.Debug(err)
			}
		}
		_, err = writer.WriteString("\n")
		if err != nil {
			logrus.Debug(err)
		}
	}
}

func checkVersionChange(before, after string) (change bool, updateType string, err error) {
	beforeVer, err := semver.NewVersion(before)
	if err != nil {
		return false, "", err
	}

	afterVer, err := semver.NewVersion(after)
	if err != nil {
		return false, "", err
	}

	switch {
	case afterVer.Major() > beforeVer.Major():
		return true, "major", nil
	case afterVer.Minor() > beforeVer.Minor():
		return true, "minor", nil
	case afterVer.Patch() > beforeVer.Patch():
		return true, "patch", nil
	default:
		return false, "", nil
	}
}

func (d *DockerCompose) Parse(customImageName, deployImageName, buildSecretString string) error {
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
	exitCode, err := d.Pytest(pytestFile, customImageName, deployImageName, "", buildSecretString)
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
	dockerCommand := config.CFG.DockerCommand.GetString()
	err = cmdExec(dockerCommand, os.Stdout, os.Stderr, "exec", "-it", containerName, "bash")
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

	if envExport {
		err = envExportSettings(containerID, envFile, airflowDockerVersion, connections, variables)
		if err != nil {
			return err
		}
		fmt.Println("\nAirflow objects exported to env file")
		return nil
	}

	fileState, err := fileutil.Exists(settingsFile, nil)
	if err != nil {
		return errors.Wrap(err, errSettingsPath)
	}
	if !fileState {
		return errNoFile
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

	err = initSettings(containerID, settingsFile, nil, airflowDockerVersion, connections, variables, pools)
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

func (d *DockerCompose) RunDAG(dagID, settingsFile, dagFile, executionDate string, noCache, taskLogs bool) error {
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
					err = d.imageHandler.Run(dagID, d.envFile, settingsFile, psInfo[i].Name, dagFile, executionDate, taskLogs)
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
	err = d.imageHandler.Build(d.dockerfile, "", airflowTypes.ImageBuildConfig{Path: d.airflowHome, Output: true, NoCache: noCache})
	if err != nil {
		return err
	}

	err = d.imageHandler.Run(dagID, d.envFile, settingsFile, "", dagFile, executionDate, taskLogs)
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
var createDockerProject = func(projectName, airflowHome, envFile, buildImage, settingsFile, composeFile string, imageLabels map[string]string) (*types.Project, error) {
	// Generate the docker-compose yaml
	var yaml string
	var err error
	if composeFile == "" {
		yaml, err = generateConfig(projectName, airflowHome, envFile, buildImage, settingsFile, imageLabels)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create project")
		}
	} else {
		yaml, err = fileutil.ReadFileToString(composeFile)
		if err != nil {
			return nil, errors.Wrap(err, "failed to read compose file")
		}
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

func printStatus(settingsFile string, envConns map[string]astrocore.EnvironmentObjectConnection, project *types.Project, composeService api.Service, airflowDockerVersion uint64, noBrowser bool) error {
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
				err = initSettings(psInfo[i].ID, settingsFile, envConns, airflowDockerVersion, true, true, true)
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
	dockerCommand := config.CFG.DockerCommand.GetString()

	buf := new(bytes.Buffer)
	err := cmdExec(dockerCommand, buf, buf, "ps")
	if err != nil {
		// open docker
		fmt.Println("\nDocker is not running. Starting up the Docker engine…")
		err = cmdExec(OpenCmd, buf, os.Stderr, "-a", dockerCmd)
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
	dockerCommand := config.CFG.DockerCommand.GetString()

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
			err := cmdExec(dockerCommand, buf, buf, "ps")
			if err != nil {
				continue
			} else {
				return nil
			}
		}
	}
}
