// Package imagepush tags, builds and pushes a deploy's image, and logs in to
// the registry it goes to, through the container runtime: the image half of
// astro deploy for both control planes. It is shell code: it prints its
// progress and reads config/.
package imagepush

import (
	"github.com/docker/docker/client"
)

// RegistryHandler defines methods require to handle all operations with registry
type RegistryHandler interface {
	Login(username, token string) error
}

// ImageHandler defines methods require to handle all operations on/for container images
type ImageHandler interface {
	Build(dockerfile string, buildSecrets []string, config ImageBuildConfig) error
	Push(remoteImage, username, token string, getImageRepoSha bool) (string, error)
	GetLabel(altImageName, labelName string) (string, error)
	TagLocalImage(localImage string) error
}

// ImageBuildConfig defines options when building a container image
type ImageBuildConfig struct {
	Path            string
	TargetPlatforms []string
	NoCache         bool
	Labels          []string
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
