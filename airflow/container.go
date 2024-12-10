package airflow

import (
	"bytes"
	"crypto/md5" //nolint:gosec
	"fmt"
	"html/template"
	"regexp"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/airflow/types"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/util"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/docker/client"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type ContainerHandler interface {
	Start(imageName, settingsFile, composeFile, buildSecretString string, noCache, noBrowser bool, waitTime time.Duration, envConns map[string]astrocore.EnvironmentObjectConnection) error
	Stop(waitForExit bool) error
	PS() error
	Kill() error
	Logs(follow bool, containerNames ...string) error
	Run(args []string, user string) error
	Bash(container string) error
	RunDAG(dagID, settingsFile, dagFile, executionDate string, noCache, taskLogs bool) error
	ImportSettings(settingsFile, envFile string, connections, variables, pools bool) error
	ExportSettings(settingsFile, envFile string, connections, variables, pools, envExport bool) error
	ComposeExport(settingsFile, composeFile string) error
	Pytest(pytestFile, customImageName, deployImageName, pytestArgsString, buildSecretString string) (string, error)
	Parse(customImageName, deployImageName, buildSecretString string) error
	UpgradeTest(runtimeVersion, deploymentID, newImageName, customImageName, buildSecretString string, dependencyTest, versionTest, dagTest bool, astroPlatformCore astroplatformcore.ClientWithResponsesInterface) error
}

// RegistryHandler defines methods require to handle all operations with registry
type RegistryHandler interface {
	Login(username, token string) error
}

// ImageHandler defines methods require to handle all operations on/for container images
type ImageHandler interface {
	Build(dockerfile, buildSecretString string, config types.ImageBuildConfig) error
	Push(remoteImage, username, token string, getRemoteShaTag bool) (string, error)
	Pull(remoteImage, username, token string) error
	GetLabel(altImageName, labelName string) (string, error)
	DoesImageExist(image string) error
	ListLabels() (map[string]string, error)
	TagLocalImage(localImage string) error
	Run(dagID, envFile, settingsFile, containerName, dagFile, executionDate string, taskLogs bool) error
	Pytest(pytestFile, airflowHome, envFile, testHomeDirectory string, pytestArgs []string, htmlReport bool, config types.ImageBuildConfig) (string, error)
	ConflictTest(workingDirectory, testHomeDirectory string, buildConfig types.ImageBuildConfig) (string, error)
	CreatePipFreeze(altImageName, pipFreezeFile string) error
}

type DockerComposeAPI interface {
	api.Service
}

type DockerCLIClient interface {
	client.APIClient
}

type DockerRegistryAPI interface {
	client.CommonAPIClient
}

func ContainerHandlerInit(airflowHome, envFile, dockerfile, projectName string) (ContainerHandler, error) {
	return DockerComposeInit(airflowHome, envFile, dockerfile, projectName)
}

func RegistryHandlerInit(registry string) (RegistryHandler, error) {
	return DockerRegistryInit(registry)
}

func ImageHandlerInit(image string) ImageHandler {
	return DockerImageInit(image)
}

// ProjectNameUnique creates a reasonably unique project name based on the hashed
// path of the project. This prevents collisions of projects with identical dir names
// in different paths. ie (~/dev/project1 vs ~/prod/project1)
func ProjectNameUnique() (string, error) {
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

func normalizeName(s string) string {
	r := regexp.MustCompile("[a-z0-9_-]")
	s = strings.ToLower(s)
	s = strings.Join(r.FindAllString(s, -1), "")
	return strings.TrimLeft(s, "_-")
}

// generateConfig generates the docker-compose config
func generateConfig(projectName, airflowHome, envFile, buildImage, settingsFile string, imageLabels map[string]string) (string, error) {
	var tmpl *template.Template
	var err error
	tmpl, err = template.New("yml").Parse(Composeyml)
	if err != nil {
		return "", errors.Wrap(err, "failed to generate config")
	}

	envExists, err := fileutil.Exists(envFile, nil)
	if err != nil {
		return "", errors.Wrapf(err, envPathMsg, envFile)
	}

	if envFile != "" {
		if !envExists {
			fmt.Printf(envNotFoundMsg, envFile)
			envFile = ""
		} else {
			fmt.Printf(envFoundMsg, envFile)
			envFile = fmt.Sprintf("env_file: %s", envFile)
		}
	}

	triggererEnabled, err := CheckTriggererEnabled(imageLabels)
	if err != nil {
		fmt.Println("unable to check runtime version Triggerer is disabled")
	}

	airflowImage := ImageName(projectName, "latest")
	if buildImage != "" {
		airflowImage = buildImage
	}

	settingsFileExist, err := util.Exists("./" + settingsFile)
	if err != nil {
		log.Debug(err)
	}

	cfg := ComposeConfig{
		PostgresUser:          config.CFG.PostgresUser.GetString(),
		PostgresPassword:      config.CFG.PostgresPassword.GetString(),
		PostgresHost:          config.CFG.PostgresHost.GetString(),
		PostgresPort:          config.CFG.PostgresPort.GetString(),
		PostgresRepository:    config.CFG.PostgresRepository.GetString(),
		PostgresTag:           config.CFG.PostgresTag.GetString(),
		AirflowImage:          airflowImage,
		AirflowHome:           airflowHome,
		AirflowUser:           "astro",
		AirflowWebserverPort:  config.CFG.WebserverPort.GetString(),
		AirflowEnvFile:        envFile,
		AirflowExposePort:     config.CFG.AirflowExposePort.GetBool(),
		MountLabel:            "z",
		SettingsFile:          settingsFile,
		SettingsFileExist:     settingsFileExist,
		TriggererEnabled:      triggererEnabled,
		DuplicateImageVolumes: config.CFG.DuplicateImageVolumes.GetBool(),
		ProjectName:           projectName,
	}

	buff := new(bytes.Buffer)
	err = tmpl.Execute(buff, cfg)
	if err != nil {
		return "", errors.Wrap(err, "failed to generate config")
	}
	return buff.String(), nil
}
