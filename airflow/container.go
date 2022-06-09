package airflow

import (
	"bytes"
	"crypto/md5" // nolint:gosec
	"fmt"
	"html/template"
	"regexp"
	"strings"

	"github.com/astronomer/astro-cli/airflow/include"
	"github.com/astronomer/astro-cli/airflow/types"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/docker/client"
	"github.com/pkg/errors"
)

type ContainerHandler interface {
	Start(noCache bool) error
	Stop() error
	PS() error
	Kill() error
	Logs(follow bool, containerNames ...string) error
	Run(args []string, user string) error
	Pytest(pytestFile, projectImageName string) (string, error)
	Parse(buildImage string) error
}

// RegistryHandler defines methods require to handle all operations with registry
type RegistryHandler interface {
	Login(username, token string) error
}

// ImageHandler defines methods require to handle all operations on/for container images
type ImageHandler interface {
	Build(config types.ImageBuildConfig) error
	Push(registry, username, token, remoteImage string) error
	GetLabel(labelName string) (string, error)
	ListLabels() (map[string]string, error)
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

func ContainerHandlerInit(airflowHome, envFile, dockerfile, projectName string, isPyTestCompose bool) (ContainerHandler, error) {
	return DockerComposeInit(airflowHome, envFile, dockerfile, projectName, isPyTestCompose)
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
func ProjectNameUnique(pytest bool) (string, error) {
	projectName := config.CFG.ProjectName.GetString()

	pwd, err := fileutil.GetWorkingDir()
	if err != nil {
		return "", errors.Wrap(err, "error retrieving working directory")
	}

	// #nosec
	b := md5.Sum([]byte(pwd))
	s := fmt.Sprintf("%x", b[:])

	if pytest {
		return s[0:6], nil
	}
	return projectName + "_" + s[0:6], nil
}

func normalizeName(s string) string {
	r := regexp.MustCompile("[a-z0-9_-]")
	s = strings.ToLower(s)
	s = strings.Join(r.FindAllString(s, -1), "")
	return strings.TrimLeft(s, "_-")
}

// generateConfig generates the docker-compose config
func generateConfig(projectName, airflowHome, envFile, pytestFile, buildImage string, imageLabels map[string]string, pytest bool) (string, error) {
	var tmpl *template.Template
	var err error

	if pytest {
		tmpl, err = template.New("yml").Parse(include.Pytestyml)
	} else {
		tmpl, err = template.New("yml").Parse(include.Composeyml)
	}
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

	cfg := ComposeConfig{
		PytestFile:           pytestFile,
		PostgresUser:         config.CFG.PostgresUser.GetString(),
		PostgresPassword:     config.CFG.PostgresPassword.GetString(),
		PostgresHost:         config.CFG.PostgresHost.GetString(),
		PostgresPort:         config.CFG.PostgresPort.GetString(),
		AirflowImage:         airflowImage,
		AirflowHome:          airflowHome,
		AirflowUser:          "astro",
		AirflowWebserverPort: config.CFG.WebserverPort.GetString(),
		AirflowEnvFile:       envFile,
		MountLabel:           "z",
		TriggererEnabled:     triggererEnabled,
	}

	buff := new(bytes.Buffer)
	err = tmpl.Execute(buff, cfg)
	if err != nil {
		return "", errors.Wrap(err, "failed to generate config")
	}
	return buff.String(), nil
}
