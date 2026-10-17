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

// ContainerRuntimeBinary is the engine binary every action here runs on: the
// tag and push, DockerLogin, and reading a local image's label. A caller that
// builds an image it then hands to this package (the remote-execution client
// deploy) builds on this binary too, so the build, the registry login and the
// push all reach the same engine.
func ContainerRuntimeBinary() (string, error) {
	return getContainerRuntimeBinary()
}
