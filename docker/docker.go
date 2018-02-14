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
func Exec(args ...string) string {
	_, lookErr := exec.LookPath(Docker)
	if lookErr != nil {
		panic(lookErr)
	}

	output, cmdErr := exec.Command(Docker, args...).Output()
	if cmdErr != nil {
		panic(cmdErr)
	}

	return string(output)
}
