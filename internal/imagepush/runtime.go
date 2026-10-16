package imagepush

import (
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/container"
)

// getContainerRuntimeBinary returns the resolved engine binary ("docker" or
// "podman") for the container.binary setting in the global config. It is a
// var so unit tests can replace it.
var getContainerRuntimeBinary = func() (string, error) {
	return container.GetContainerRuntimeBinary(container.Config{Binary: config.CFG.DockerCommand.GetString()})
}
