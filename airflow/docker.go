package airflow

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/astronomer/astro-cli/airflow/runtimes"
	airflowTypes "github.com/astronomer/astro-cli/airflow/types"
	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/docker"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/logger"
	"github.com/astronomer/astro-cli/pkg/spinner"
	"github.com/astronomer/astro-cli/pkg/util"
	"github.com/astronomer/astro-cli/settings"
	"github.com/compose-spec/compose-go/v2/interpolation"
	"github.com/compose-spec/compose-go/v2/loader"
	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/flags"
	"github.com/docker/compose/v2/cmd/formatter"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/compose"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/versions"
	"github.com/pelletier/go-toml/v2"
	"github.com/pkg/browser"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	RuntimeImageLabel                     = "io.astronomer.docker.runtime.version"
	AirflowImageLabel                     = "io.astronomer.docker.airflow.version"
	componentName                         = "airflow"
	dockerStateUp                         = "running"
	dockerExitState                       = "exited"
	defaultAirflowVersion                 = uint64(0x2) //nolint:mnd
	triggererAllowedRuntimeVersion        = "4.0.0"
	triggererAllowedAirflowVersion        = "2.2.0"
	pytestDirectory                       = "tests"
	registryUsername                      = "cli"
	unknown                               = "unknown"
	major                                 = "major"
	patch                                 = "patch"
	minor                                 = "minor"
	partsNum                              = 2
	ruffFilePermission                    = 0o600
	airflowMajorVersion2           uint64 = 2
	airflowMajorVersion3           uint64 = 3

	composeCreateErrMsg      = "error creating docker-compose project"
	composeStatusCheckErrMsg = "error checking docker-compose status"
	composeRecreateErrMsg    = "error building, (re)creating or starting project containers"
	composePauseErrMsg       = "Error pausing project containers"
	composeStopErrMsg        = "Error stopping and removing containers"

	composeLinkUIMsg        = "Airflow UI: %s"
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

	openURL = browser.OpenURL

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
	PytestFile               string
	PostgresUser             string
	PostgresPassword         string
	PostgresHost             string
	PostgresPort             string
	PostgresRepository       string
	PostgresTag              string
	AirflowEnvFile           string
	AirflowImage             string
	AirflowHome              string
	AirflowUser              string
	AirflowWebserverPort     string
	AirflowAPIServerPort     string
	AirflowExposePort        bool
	MountLabel               string
	SettingsFile             string
	SettingsFileExist        bool
	DuplicateImageVolumes    bool
	TriggererEnabled         bool
	ProjectName              string
	AuthCredentialsDirectory string
}

type DockerCompose struct {
	airflowHome      string
	projectName      string
	envFile          string
	dockerfile       string
	composeService   DockerComposeAPI
	cliClient        DockerCLIClient
	imageHandler     ImageHandler
	ruffImageHandler ImageHandler
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
	ruffImageHandler := DockerImageInit(config.CFG.RuffImage.GetString())

	// Route output streams according to verbosity.
	var stdout, stderr io.Writer
	if logger.IsLevelEnabled(logrus.DebugLevel) {
		stdout = os.Stdout
		stderr = os.Stderr
	} else {
		stdout = io.Discard
		stderr = io.Discard
	}

	dockerCli, err := command.NewDockerCli(command.WithOutputStream(stdout), command.WithErrorStream(stderr))
	if err != nil {
		logger.Fatalf("error creating compose client %s", err)
	}

	err = dockerCli.Initialize(flags.NewClientOptions())
	if err != nil {
		logger.Fatalf("error init compose client %s", err)
	}

	composeService := compose.NewComposeService(dockerCli)

	return &DockerCompose{
		airflowHome:      airflowHome,
		projectName:      projectName,
		envFile:          envFile,
		dockerfile:       dockerfile,
		composeService:   composeService,
		cliClient:        dockerCli.Client(),
		imageHandler:     imageHandler,
		ruffImageHandler: ruffImageHandler,
	}, nil
}

// Start starts a local airflow development cluster
//
//nolint:gocognit
func (d *DockerCompose) Start(imageName, settingsFile, composeFile, buildSecretString string, noCache, noBrowser bool, waitTime time.Duration, envConns map[string]astrocore.EnvironmentObjectConnection) error {
	// Build this project image
	if imageName == "" {
		if !config.CFG.DisableAstroRun.GetBool() {
			// add astro-run-dag package
			err := fileutil.AddLineToFile("./requirements.txt", "astro-run-dag", "# This package is needed for the astro run command. It will be removed before a deploy")
			if err != nil {
				fmt.Printf("Adding 'astro-run-dag' package to requirements.txt unsuccessful: %s\nManually add package to requirements.txt", err.Error())
			}
		}
		imageBuildErr := d.imageHandler.Build(d.dockerfile, buildSecretString, airflowTypes.ImageBuildConfig{Path: d.airflowHome, NoCache: noCache})
		if !config.CFG.DisableAstroRun.GetBool() {
			// remove astro-run-dag from requirments.txt
			err := fileutil.RemoveLineFromFile("./requirements.txt", "astro-run-dag", " # This package is needed for the astro run command. It will be removed before a deploy")
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

	s := spinner.NewSpinner("Project is starting up…")
	if !logger.IsLevelEnabled(logrus.DebugLevel) {
		s.Start()
		defer s.Stop()
	}

	// Create a compose project
	project, err := createDockerProject(d.projectName, d.airflowHome, d.envFile, "", settingsFile, composeFile, imageLabels)
	if err != nil {
		return errors.Wrap(err, composeCreateErrMsg)
	}

	// Start up our project
	err = d.composeService.Up(context.Background(), project, api.UpOptions{
		Create: api.CreateOptions{
			QuietPull: logger.IsLevelEnabled(logrus.DebugLevel),
		},
		Start: api.StartOptions{
			Project: project,
		},
	})
	if err != nil {
		return errors.Wrap(err, composeRecreateErrMsg)
	}

	airflowDockerVersion, err := d.checkAirflowVersion()
	if err != nil {
		return err
	}

	// If we're logging the output, only start the spinner for waiting on the webserver healthcheck.
	s.Start()
	defer s.Stop()

	airflowMajorVersion := airflowversions.AirflowMajorVersionForRuntimeVersion(imageLabels[runtimeVersionLabelName])
	var healthURL, healthComponent string
	switch airflowMajorVersion {
	case "3":
		healthURL = fmt.Sprintf("http://localhost:%s/api/v2/monitor/health", config.CFG.APIServerPort.GetString())
		healthComponent = "api-server"
	case "2":
		healthURL = fmt.Sprintf("http://localhost:%s/health", config.CFG.WebserverPort.GetString())
		healthComponent = "webserver"
	}

	// Check the health of the webserver, up to the timeout.
	// If we fail to get a 200 status code, we'll return an error message.
	err = checkWebserverHealth(healthURL, waitTime, healthComponent)
	if err != nil {
		return err
	}

	spinner.StopWithCheckmark(s, "Project started")

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
	composeFilePerms := 0o666
	err = os.WriteFile(composeFile, []byte(yaml), fs.FileMode(composeFilePerms))
	if err != nil {
		return errors.Wrap(err, "failed to write to compose file")
	}

	return nil
}

// Stop a running docker project
func (d *DockerCompose) Stop(waitForExit bool) error {
	s := spinner.NewSpinner("Stopping project…")
	if !logger.IsLevelEnabled(logrus.DebugLevel) {
		s.Start()
		defer s.Stop()
	}

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
	err = d.composeService.Stop(context.Background(), project.Name, api.StopOptions{})
	if err != nil {
		return errors.Wrap(err, composePauseErrMsg)
	}

	if !waitForExit {
		spinner.StopWithCheckmark(s, "Project stopped")
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
			logger.Debug("timed out waiting for postgres container to be in exited state")
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
						logger.Debug("postgres container reached exited state")
						spinner.StopWithCheckmark(s, "Project stopped")
						return nil
					}
					logger.Debugf("postgres container is still in %s state, waiting for it to be in exited state", psInfo[i].State)
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
	tw.Init(os.Stdout, 0, 8, 2, '\t', tabwriter.AlignRight) //nolint:mnd

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
	s := spinner.NewSpinner("Killing project…")
	if !logger.IsLevelEnabled(logrus.DebugLevel) {
		s.Start()
		defer s.Stop()
	}

	// Killing an already killed project produces an unsightly warning,
	// so we briefly switch to a higher level before running the kill command.
	// We then swap back to the original level.
	originalLevel := logrus.GetLevel()
	logrus.SetLevel(logrus.ErrorLevel)

	// Shut down our project
	err := d.composeService.Down(context.Background(), d.projectName, api.DownOptions{Volumes: true, RemoveOrphans: true})
	if err != nil {
		return errors.Wrap(err, composeStopErrMsg)
	}
	logrus.SetLevel(originalLevel)

	spinner.StopWithCheckmark(s, "Project killed")

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

	consumer := formatter.NewLogConsumer(context.Background(), os.Stdout, os.Stderr, true, false, true)

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
	execConfig := &container.ExecOptions{
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

	execStartCheck := container.ExecStartOptions{
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
			err := d.imageHandler.Build(d.dockerfile, buildSecretString, airflowTypes.ImageBuildConfig{Path: d.airflowHome})
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
	exitCode, err := d.imageHandler.Pytest(pytestFile, d.airflowHome, d.envFile, "", pytestArgs, false, airflowTypes.ImageBuildConfig{Path: d.airflowHome})
	if err != nil {
		return exitCode, err
	}
	if strings.Contains(exitCode, "0") { // if the error code is 0 the pytests passed
		return "", nil
	}
	return exitCode, errors.New("something went wrong while Pytesting your DAGs")
}

func (d *DockerCompose) UpgradeTest(newVersion, deploymentID, customImage, buildSecretString string, versionTest, dagTest, lintTest, includeLintDeprecations bool, lintConfigFile string, astroPlatformCore astroplatformcore.CoreClient) error { //nolint:gocognit,gocyclo
	// figure out which tests to run
	if !versionTest && !dagTest && !lintTest {
		versionTest = true
		dagTest = true
		lintTest = true
	}
	var deploymentImage string
	// if custom image is used get new Airflow version
	if customImage != "" {
		err := d.imageHandler.DoesImageExist(customImage)
		if err != nil {
			return errCustomImageDoesNotExist
		}
		newVersion = strings.SplitN(customImage, ":", partsNum)[1]
	}
	// if user supplies deployment id pull down current image
	if deploymentID != "" {
		err := d.pullImageFromDeployment(deploymentID, astroPlatformCore)
		if err != nil {
			return err
		}
	} else {
		// build image for current Airflow version to get current Airflow version
		fmt.Println("\nBuilding image for current version")
		imageBuildErr := d.imageHandler.Build(d.dockerfile, buildSecretString, airflowTypes.ImageBuildConfig{Path: d.airflowHome})
		if imageBuildErr != nil {
			return imageBuildErr
		}
	}

	// get the current version from the image labels
	currentVersion, err := d.imageHandler.GetLabel(deploymentImage, RuntimeImageLabel)
	if err != nil {
		return err
	}
	if currentVersion == "" {
		currentVersion, err = d.imageHandler.GetLabel(deploymentImage, AirflowImageLabel)
		if err != nil {
			return err
		}
	}

	// create test home directory
	testHomeDirectory := "upgrade-test-" + currentVersion + "--" + newVersion

	destFolder := filepath.Join(d.airflowHome, testHomeDirectory)
	var filePerms fs.FileMode = 0o755
	if err := os.MkdirAll(destFolder, filePerms); err != nil {
		return err
	}
	newDockerFile := destFolder + "/Dockerfile"

	if versionTest {
		err := d.versionTest(testHomeDirectory, currentVersion, deploymentImage, newDockerFile, newVersion, customImage, buildSecretString)
		if err != nil {
			return err
		}
	}

	var failed bool

	if dagTest {
		dagTestPassed, err := d.dagTest(testHomeDirectory, newVersion, newDockerFile, customImage, buildSecretString)
		if err != nil {
			return err
		}
		failed = failed || !dagTestPassed
	}

	if lintTest && airflowversions.AirflowMajorVersionForRuntimeVersion(newVersion) != "3" {
		fmt.Println("")
		fmt.Println("Skipping ruff linter because not testing upgrade to Airflow 3.x")
		lintTest = false
	}

	if lintTest {
		fmt.Println("")
		fmt.Println("Running ruff linter")

		lintTestPassed, err := d.lintTest(testHomeDirectory, includeLintDeprecations, lintConfigFile)
		if err != nil {
			return err
		}
		failed = failed || !lintTestPassed
	}

	fmt.Println("\nTest Summary:")
	fmt.Printf("\tUpgrade Test Results Directory: %s\n", testHomeDirectory)
	if versionTest {
		fmt.Printf("\tDependency Version Comparison Results file: %s\n", "dependency_compare.txt")
	}
	if dagTest {
		fmt.Printf("\tDAG Parse Test HTML Report: %s\n", "dag-test-report.html")
	}
	if lintTest {
		fmt.Printf("\tRuff Linter Results: %s\n", "ruff-lint-results.txt")
	}
	if failed {
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

func (d *DockerCompose) versionTest(testHomeDirectory, currentVersion, deploymentImage, newDockerFile, newVersion, customImage, buildSecretString string) error {
	fmt.Println("\nComparing dependency versions between current and upgraded environment")
	// pip freeze old Airflow image
	fmt.Println("\nObtaining pip freeze for current version")
	currentAirflowPipFreezeFile := d.airflowHome + "/" + testHomeDirectory + "/pip_freeze_" + currentVersion + ".txt"
	err := d.imageHandler.CreatePipFreeze(deploymentImage, currentAirflowPipFreezeFile)
	if err != nil {
		return err
	}

	// build image with the new airflow version
	err = upgradeDockerfile(d.dockerfile, newDockerFile, newVersion, customImage)
	if err != nil {
		return err
	}
	fmt.Println("\nBuilding image for new version")
	imageBuildErr := d.imageHandler.Build(newDockerFile, buildSecretString, airflowTypes.ImageBuildConfig{Path: d.airflowHome})
	if imageBuildErr != nil {
		return imageBuildErr
	}
	// pip freeze new airflow image
	fmt.Println("\nObtaining pip freeze for new version")
	newAirflowPipFreezeFile := filepath.Join(d.airflowHome, testHomeDirectory, "pip_freeze_"+newVersion+".txt")
	err = d.imageHandler.CreatePipFreeze("", newAirflowPipFreezeFile)
	if err != nil {
		return err
	}
	// compare pip freeze files
	fmt.Println("\nComparing pip freeze files")
	pipFreezeCompareFile := filepath.Join(d.airflowHome, testHomeDirectory, "dependency_compare.txt")
	err = CreateVersionTestFile(currentAirflowPipFreezeFile, newAirflowPipFreezeFile, pipFreezeCompareFile)
	if err != nil {
		return err
	}
	fmt.Printf("Pip Freeze comparison can be found at %s", pipFreezeCompareFile)
	return nil
}

func (d *DockerCompose) dagTest(testHomeDirectory, newVersion, newDockerFile, customImage, buildSecretString string) (bool, error) {
	fmt.Printf("\nChecking the DAGs in this project for errors against the new Airflow version %s\n", newVersion)

	// build image with the new runtime version
	err := upgradeDockerfile(d.dockerfile, newDockerFile, newVersion, customImage)
	if err != nil {
		return false, err
	}

	reqFile := d.airflowHome + "/requirements.txt"
	// add pytest-html to the requirements
	err = fileutil.AddLineToFile(reqFile, "pytest-html", "# This package is needed for the upgrade dag test. It will be removed once the test is over")
	if err != nil {
		fmt.Printf("Adding 'pytest-html' package to requirements.txt unsuccessful: %s\nManually add package to requirements.txt", err.Error())
	}
	fmt.Println("\nBuilding image for new version")
	imageBuildErr := d.imageHandler.Build(newDockerFile, buildSecretString, airflowTypes.ImageBuildConfig{Path: d.airflowHome})

	// remove pytest-html to the requirements
	err = fileutil.RemoveLineFromFile(reqFile, "pytest-html", " # This package is needed for the upgrade dag test. It will be removed once the test is over")
	if err != nil {
		fmt.Printf("Removing package 'pytest-html' from requirements.txt unsuccessful: %s\n", err.Error())
	}
	if imageBuildErr != nil {
		return false, imageBuildErr
	}
	// check for file
	path := d.airflowHome + "/" + DefaultTestPath

	fileExist, err := util.Exists(path)
	if err != nil {
		return false, err
	}
	if !fileExist {
		fmt.Println("\nThe file " + path + " which is needed for the parse test does not exist. Please run `astro dev init` to create it")
		return false, err
	}
	// run parse test
	pytestFile := DefaultTestPath
	// create html report
	htmlReportArgs := "--html=dag-test-report.html --self-contained-html"
	// compare pip freeze files
	fmt.Println("\nRunning DAG parse test with the new Airflow version")
	exitCode, err := d.imageHandler.Pytest(pytestFile, d.airflowHome, d.envFile, testHomeDirectory, strings.Fields(htmlReportArgs), true, airflowTypes.ImageBuildConfig{Path: d.airflowHome})
	if err != nil {
		if strings.Contains(exitCode, "1") { // exit code is 1 meaning tests failed
			fmt.Println("See above for errors detected in your DAGs")
			return false, nil
		} else {
			return false, errors.Wrap(err, "something went wrong while parsing your DAGs")
		}
	} else {
		fmt.Println("\n" + ansi.Green("✔") + " No errors detected in your DAGs ")
	}
	return true, nil
}

func (d *DockerCompose) lintTest(testHomeDirectory string, includeDeprecations bool, configFile string) (bool, error) {
	var err error
	if configFile == "" {
		configFile, err = createTempRuffConfigFile(includeDeprecations)
		if err != nil {
			return false, err
		}
		defer os.Remove(configFile)
	} else {
		configFile = filepath.Join(config.WorkingPath, configFile)
		if _, err := os.Stat(configFile); os.IsNotExist(err) {
			return false, fmt.Errorf("the specified lint config file '%s' does not exist", configFile)
		}
	}

	// Mount the project and the ruff config file
	mountDirs := map[string]string{
		config.WorkingPath: "/app/project",
		configFile:         "/app/ruff.toml",
	}

	// Pull the ruff image to get the latest image for the "latest" tag
	err = d.ruffImageHandler.Pull("", "", "")
	if err != nil {
		return false, err
	}

	// Run ruff with the config file and the project directory
	ruffArgs := []string{"check", "--config", "/app/ruff.toml", "/app/project"}
	var buf bytes.Buffer
	bufWriter := io.MultiWriter(os.Stdout, &buf)
	ruffErr := d.ruffImageHandler.RunCommand(ruffArgs, mountDirs, bufWriter, bufWriter)

	// Write the ruff output to a results file in the test directory
	resultsFile := filepath.Join(d.airflowHome, testHomeDirectory, "ruff-lint-results.txt")
	resultsErr := os.WriteFile(resultsFile, buf.Bytes(), ruffFilePermission)
	if resultsErr != nil {
		return false, fmt.Errorf("failed to write ruff output to file: %w", resultsErr)
	}

	return ruffErr == nil, nil
}

func createTempRuffConfigFile(includeDeprecations bool) (string, error) {
	type LintConfig struct {
		Preview bool     `toml:"preview"`
		Select  []string `toml:"select"`
		Ignore  []string `toml:"ignore"`
	}
	type RsetConfig struct {
		Lint LintConfig `toml:"lint"`
	}
	ruffConfig := RsetConfig{
		Lint: LintConfig{
			Preview: true,
			Select:  []string{"AIR30"},
			Ignore:  []string{"ALL"},
		},
	}
	if includeDeprecations {
		ruffConfig.Lint.Select = append(ruffConfig.Lint.Select, "AIR31")
	}

	// create temporary ruff config file
	tempFile, err := os.CreateTemp("", "ruff-airflow3-*.toml")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary ruff config file: %w", err)
	}

	// Encode the struct to TOML and write to the file
	encoder := toml.NewEncoder(tempFile)
	encoder.SetIndentTables(true)
	if err := encoder.Encode(ruffConfig); err != nil {
		return "", fmt.Errorf("failed to encode or write to temporary ruff config file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return "", fmt.Errorf("failed to close temporary ruff config file: %w", err)
	}

	return tempFile.Name(), nil
}

func upgradeDockerfile(oldDockerfilePath, newDockerfilePath, newTag, customImage string) error { //nolint:gocognit
	var newImage string
	switch airflowversions.AirflowMajorVersionForRuntimeVersion(newTag) {
	case "3":
		newImage = "astrocrpublic.azurecr.io/runtime"
	case "2":
		newImage = "quay.io/astronomer/astro-runtime"
	}

	// Read the content of the old Dockerfile
	content, err := os.ReadFile(oldDockerfilePath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	var newContent strings.Builder
	if customImage == "" {
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "FROM quay.io/astronomer/astro-runtime") ||
				strings.HasPrefix(line, "FROM astrocrpublic.azurecr.io/runtime") {
				if strings.HasPrefix(line, fmt.Sprintf("FROM %s", newImage)) {
					parts := strings.SplitN(line, ":", partsNum)
					if len(parts) == partsNum {
						line = parts[0] + ":" + newTag
					}
				} else {
					line = fmt.Sprintf("FROM %s:%s", newImage, newTag)
				}
			}
			if strings.HasPrefix(strings.TrimSpace(line), "FROM quay.io/astronomer/ap-airflow:") {
				isRuntime, err := isRuntimeVersion(newTag)
				if err != nil {
					logger.Debug(err)
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
					line = parts[0] + " " + customImage
				}
			}
			newContent.WriteString(line + "\n") // Add a newline after each line
		}
	}

	// Write the new content to the new Dockerfile
	err = os.WriteFile(newDockerfilePath, []byte(newContent.String()), 0o600) //nolint:mnd
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
			logger.Debug(err)
		}
		for _, pkg := range pkgList {
			_, err = writer.WriteString(pkg + "\n")
			if err != nil {
				logger.Debug(err)
			}
		}
		_, err = writer.WriteString("\n")
		if err != nil {
			logger.Debug(err)
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

	fmt.Println("Checking your DAGs for errors…")

	pytestFile := DefaultTestPath
	exitCode, err := d.Pytest(pytestFile, customImageName, deployImageName, "", buildSecretString)
	if err != nil {
		if strings.Contains(exitCode, "1") { // exit code is 1 meaning tests failed
			return errors.New("See above for errors detected in your DAGs")
		}
		return errors.Wrap(err, "something went wrong while parsing your DAGs")
	}
	fmt.Println(ansi.Green("✔") + " No errors detected in your DAGs ")
	return err
}

func (d *DockerCompose) Bash(component string) error {
	// exec into schedueler by default
	if component == "" {
		component = SchedulerDockerContainerName
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
		if strings.Contains(psInfo[i].Name, component) {
			containerName = psInfo[i].Name
		}
	}
	// exec into container
	containerRuntime, err := runtimes.GetContainerRuntimeBinary()
	if err != nil {
		return err
	}
	err = cmdExec(containerRuntime, os.Stdout, os.Stderr, "exec", "-it", containerName, "bash")
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
	airflowDockerVersion, err := d.checkAirflowVersion()
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
	airflowDockerVersion, err := d.checkAirflowVersion()
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
			(strings.Contains(psInfo[i].Name, WebserverDockerContainerName) ||
				strings.Contains(psInfo[i].Name, APIServerDockerContainerName)) {
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
					err = d.imageHandler.RunDAG(dagID, d.envFile, settingsFile, psInfo[i].Name, dagFile, executionDate, taskLogs)
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
	err = d.imageHandler.Build(d.dockerfile, "", airflowTypes.ImageBuildConfig{Path: d.airflowHome, NoCache: noCache})
	if err != nil {
		return err
	}

	err = d.imageHandler.RunDAG(dagID, d.envFile, settingsFile, "", dagFile, executionDate, taskLogs)
	if err != nil {
		return err
	}
	return nil
}

func (d *DockerCompose) checkAirflowVersion() (uint64, error) {
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
			fmt.Printf("unable to parse Airflow version, defaulting to major version 2, error: %s", err.Error())
		}
	} else {
		runtimeVersion, ok := imageLabels[runtimeVersionLabelName]
		if ok {
			airflowDockerVersion, err = strconv.ParseUint(airflowversions.AirflowMajorVersionForRuntimeVersion(runtimeVersion), 10, 64)
			if err != nil {
				fmt.Printf("unable to parse runtime version, defaulting to major version 2")
			}
		} else {
			fmt.Printf("unable to detect Airflow or runtime versions in image labels, defaulting to major version 2")
		}
	}

	return airflowDockerVersion, nil
}

// createProject creates project with yaml config as context
var createDockerProject = func(projectName, airflowHome, envFile, buildImage, settingsFile, composeFile string, imageLabels map[string]string) (*composetypes.Project, error) {
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

	var configs []composetypes.ConfigFile

	configs = append(configs, composetypes.ConfigFile{
		Filename: "compose.yaml",
		Content:  []byte(yaml),
	})

	composeBytes, err := os.ReadFile(composeOverrideFilename)
	if err != nil && !os.IsNotExist(err) {
		return nil, errors.Wrapf(err, "Failed to open the compose file: %s", composeOverrideFilename)
	}
	if err == nil {
		configs = append(configs, composetypes.ConfigFile{
			Filename: "docker-compose.override.yml",
			Content:  composeBytes,
		})
	}

	var loadOptions []func(*loader.Options)

	nameLoadOpt := func(opts *loader.Options) {
		opts.SetProjectName(normalizeName(projectName), true)
		opts.Interpolate = &interpolation.Options{
			LookupValue: os.LookupEnv,
		}
	}

	loadOptions = append(loadOptions, nameLoadOpt)

	project, err := loader.LoadWithContext(context.Background(), composetypes.ConfigDetails{
		ConfigFiles: configs,
		WorkingDir:  airflowHome,
	}, loadOptions...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load project")
	}

	// The latest versions of compose libs handle adding these labels at an outer layer,
	// near the cobra entrypoint. Without these labels, compose will lose track of the containers its
	// starting for a given project and downstream library calls will fail.
	for name, s := range project.Services {
		s.CustomLabels = map[string]string{
			api.ProjectLabel:     project.Name,
			api.ServiceLabel:     name,
			api.VersionLabel:     api.ComposeVersion,
			api.WorkingDirLabel:  project.WorkingDir,
			api.ConfigFilesLabel: strings.Join(project.ComposeFiles, ","),
			api.OneoffLabel:      "False",
		}
		project.Services[name] = s
	}
	return project, nil
}

func printStatus(settingsFile string, envConns map[string]astrocore.EnvironmentObjectConnection, project *composetypes.Project, composeService api.Service, airflowMajorVersion uint64, noBrowser bool) error {
	containers, err := composeService.Ps(context.Background(), project.Name, api.PsOptions{
		All: true,
	})
	if err != nil {
		return errors.Wrap(err, composeStatusCheckErrMsg)
	}

	settingsFileExists, err := fileutil.Exists(settingsFile, nil)
	if err != nil {
		return errors.Wrap(err, errSettingsPath)
	}
	if settingsFileExists || len(envConns) > 0 {
		for _, container := range containers { //nolint:gocritic
			if strings.Contains(container.Name, project.Name) &&
				(strings.Contains(container.Name, WebserverDockerContainerName) ||
					strings.Contains(container.Name, APIServerDockerContainerName)) {
				err = initSettings(container.ID, settingsFile, envConns, airflowMajorVersion, true, true, true)
				if err != nil {
					return err
				}
			}
		}
	}

	var port string
	switch airflowMajorVersion {
	case airflowMajorVersion2:
		port = config.CFG.WebserverPort.GetString()
	case airflowMajorVersion3:
		port = config.CFG.APIServerPort.GetString()
	}
	parts := strings.Split(port, ":")
	uiURL := "http://localhost:" + parts[len(parts)-1]
	bullet := ansi.Cyan("\u27A4") + " "
	fmt.Printf(bullet+composeLinkUIMsg+"\n", ansi.Bold(uiURL))
	fmt.Printf(bullet+composeLinkPostgresMsg+"\n", ansi.Bold("postgresql://localhost:"+config.CFG.PostgresPort.GetString()+"/postgres"))
	// The CLI configures Airflow 3 to run without UI credentials, so we don't want to print them out
	if airflowMajorVersion == airflowMajorVersion2 {
		fmt.Printf(bullet+composeUserPasswordMsg+"\n", ansi.Bold("admin:admin"))
	}
	fmt.Printf(bullet+postgresUserPasswordMsg+"\n", ansi.Bold("postgres:postgres"))
	if !(noBrowser || util.CheckEnvBool(os.Getenv("ASTRONOMER_NO_BROWSER"))) {
		err = openURL(uiURL)
		if err != nil {
			fmt.Println("\nUnable to open the Airflow UI, please visit the following link: " + uiURL)
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

	return airflowversions.CompareRuntimeVersions(runtimeVersion, triggererAllowedRuntimeVersion) >= 0, nil
}

func checkServiceState(serviceState, expectedState string) bool {
	scrubbedState := strings.Split(serviceState, " ")[0]
	return scrubbedState == expectedState
}
