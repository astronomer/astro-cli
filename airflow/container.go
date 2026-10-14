package airflow

import (
	"github.com/docker/docker/client"

	"github.com/astronomer/astro-cli/airflow/types"
)

// RegistryHandler defines methods require to handle all operations with registry
type RegistryHandler interface {
	Login(username, token string) error
}

// ImageHandler defines methods require to handle all operations on/for container images
type ImageHandler interface {
	Build(dockerfile string, buildSecrets []string, config types.ImageBuildConfig) error
	Push(remoteImage, username, token string, getImageRepoSha bool) (string, error)
	GetLabel(altImageName, labelName string) (string, error)
	TagLocalImage(localImage string) error
}

type DockerRegistryAPI interface {
	client.APIClient
}

func RegistryHandlerInit(registry string) (RegistryHandler, error) {
	return DockerRegistryInit(registry)
}

func ImageHandlerInit(image string) ImageHandler {
	return DockerImageInit(image)
}
