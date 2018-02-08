package docker

import (
	"os/exec"
)

const (
	// CloudRegistry is the default astronomer registry.
	// XXX: This will change.
	CloudRegistry = "registry.gcp.astronomer.io"
	// Docker is the docker command.
	Docker = "docker"
)

// Exec executes a docker command
func Exec(args ...string) {
	_, lookErr := exec.LookPath(Docker)
	if lookErr != nil {
		panic(lookErr)
	}

	if cmdErr := exec.Command(Docker, args...).Run(); cmdErr != nil {
		panic(cmdErr)
	}
}
