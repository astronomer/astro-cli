package docker

import (
	"os"
	"os/exec"
)

const (
	// Docker is the docker command.
	Docker = "docker"
)

// Exec executes a docker command
func Exec(args ...string) {
	_, lookErr := exec.LookPath(Docker)
	if lookErr != nil {
		panic(lookErr)
	}

	cmd := exec.Command(Docker, args...)
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	if cmdErr := cmd.Run(); cmdErr != nil {
		panic(cmdErr)
	}
}
