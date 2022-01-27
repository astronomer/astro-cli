package airflow

import (
	"bytes"
	"crypto/md5" //nolint:gosec
	"errors"
	"fmt"
	"text/template"

	"github.com/astronomer/astro-cli/airflow/include"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/messages"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/docker/docker/api/types/versions"
)

var (
	errProjectAlreadyRunning   = errors.New("cannot start, project already running")
	errNoLogsProjectNotRunning = errors.New("cannot view logs, project not running")
	errAirflowNotRunning       = errors.New("airflow is not running, Start it with 'astro airflow start'")
	errEmptyExecID             = errors.New("exec ID is empty")
)

type Container string

const (
	DockerEngine Container = "docker"
	PodmanEngine Container = "podman"
)

// ContainerHandler defines methods require to handle all operations to run Airflow locally
type ContainerHandler interface {
	Start(dockerfile string) error
	Kill() error
	Logs(follow bool, containerNames ...string) error
	Stop() error
	PS() error
	Run(args []string, user string) error
	ExecCommand(containerID, command string) string
	GetContainerID(containerName string) (string, error)
}

// ImageHandler defines methods require to handle all operations on/for container images
type ImageHandler interface {
	Build(path string) error
	Push(serverAddress, token, remoteImage string) error
	GetImageLabels() (map[string]string, error)
}

// RegistryHandler defines methods require to handle all operations with registry
type RegistryHandler interface {
	Login(username, token string) error
}

// ComposeConfig is input data to docker compose yaml template
type ComposeConfig struct {
	PostgresUser           string
	PostgresPassword       string
	PostgresHost           string
	PostgresPort           string
	AirflowEnvFile         string
	AirflowImage           string
	AirflowHome            string
	AirflowUser            string
	AirflowWebserverPort   string
	MountLabel             string
	ProjectName            string
	TriggererEnabled       bool
	SchedulerContainerName string
	WebserverContainerName string
	TriggererContainerName string
}

func ContainerHandlerInit(airflowHome, envFile string) (ContainerHandler, error) {
	containerEngine := config.CFG.ContainerEngine.GetString()
	switch containerEngine {
	case string(DockerEngine):
		return DockerComposeInit(airflowHome, envFile)
	case string(PodmanEngine):
		return PodmanInit(airflowHome, envFile)
	default:
		return DockerComposeInit(airflowHome, envFile)
	}
}

func ImageHandlerInit(image string) (ImageHandler, error) {
	containerEngine := config.CFG.ContainerEngine.GetString()
	switch containerEngine {
	case string(DockerEngine):
		return DockerImageInit(image), nil
	case string(PodmanEngine):
		return PodmanImageInit(nil, image, nil) //nolint:staticcheck
	default:
		return DockerImageInit(image), nil
	}
}

func RegistryHandlerInit(registry string) (RegistryHandler, error) {
	containerEngine := config.CFG.ContainerEngine.GetString()
	switch containerEngine {
	case string(DockerEngine):
		return DockerRegistryInit(registry), nil
	case string(PodmanEngine):
		return PodmanRegistryInit(nil, registry) //nolint:staticcheck
	default:
		return DockerRegistryInit(registry), nil
	}
}

// generateConfig generates the docker-compose config
func generateConfig(projectName, airflowHome, envFile string, imageLabels map[string]string, containerEngine Container) (string, error) {
	var tmplFile string
	switch containerEngine {
	case DockerEngine:
		tmplFile = include.Composeyml
	case PodmanEngine:
		tmplFile = readPodConfigFile()
	default:
		tmplFile = include.Composeyml
	}

	tmpl, err := template.New("yml").Parse(tmplFile)
	if err != nil {
		return "", fmt.Errorf("failed to generate config: %w", err)
	}

	envFile, err = getFmtEnvFile(envFile, containerEngine)
	if err != nil {
		return "", err
	}

	var triggererEnabled bool
	airflowVersion := imageLabels[airflowVersionLabelName]
	if versions.GreaterThanOrEqualTo(airflowVersion, triggererAllowedAirflowVersion) {
		triggererEnabled = true
	}

	cfg := ComposeConfig{
		PostgresUser:           config.CFG.PostgresUser.GetString(),
		PostgresPassword:       config.CFG.PostgresPassword.GetString(),
		PostgresHost:           config.CFG.PostgresHost.GetString(),
		PostgresPort:           config.CFG.PostgresPort.GetString(),
		AirflowImage:           imageName(projectName, "latest"),
		AirflowHome:            airflowHome,
		AirflowUser:            "astro",
		AirflowWebserverPort:   config.CFG.WebserverPort.GetString(),
		AirflowEnvFile:         envFile,
		MountLabel:             "z",
		ProjectName:            sanitizeRepoName(projectName),
		TriggererEnabled:       triggererEnabled,
		SchedulerContainerName: config.CFG.SchedulerContainerName.GetString(),
		WebserverContainerName: config.CFG.WebserverContainerName.GetString(),
		TriggererContainerName: config.CFG.TriggererContainerName.GetString(),
	}

	buff := new(bytes.Buffer)
	err = tmpl.Execute(buff, cfg)
	if err != nil {
		return "", fmt.Errorf("failed to generate config: %w", err)
	}
	return buff.String(), nil
}

// projectNameUnique creates a reasonably unique project name based on the hashed
// path of the project. This prevents collisions of projects with identical dir names
// in different paths. ie (~/dev/project1 vs ~/prod/project1)
func projectNameUnique() (string, error) {
	projectName := config.CFG.ProjectName.GetString()

	pwd, err := fileutil.GetWorkingDir()
	if err != nil {
		return "", fmt.Errorf("error retrieving working directory: %w", err)
	}

	// #nosec
	b := md5.Sum([]byte(pwd))
	s := fmt.Sprintf("%x", b[:])

	return projectName + "_" + s[0:6], nil
}

func getFmtEnvFile(envFile string, containerEngine Container) (string, error) {
	if envFile == "" {
		return "", nil
	}

	envExists, err := fileutil.Exists(envFile)
	if err != nil {
		return "", fmt.Errorf("%s: %w", fmt.Sprintf(messages.EnvPath, envFile), err)
	}

	if !envExists {
		fmt.Printf(messages.EnvNotFound, envFile)
		return "", nil
	}

	switch containerEngine {
	case DockerEngine:
		return fmt.Sprintf("env_file: %s", envFile), nil
	case PodmanEngine:
		return fmtPodmanEnvVars(envFile)
	default:
		return "", nil
	}
}
