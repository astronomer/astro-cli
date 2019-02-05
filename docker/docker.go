package docker

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/pkg/errors"
)

const (
	// Docker is the docker command.
	Docker = "docker"
)

// Exec executes a docker command
func Exec(args ...string) error {
	_, lookErr := exec.LookPath(Docker)
	if lookErr != nil {
		return errors.Wrap(lookErr, "failed to find the docker binary")
	}

	cmd := exec.Command(Docker, args...)
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	if cmdErr := cmd.Run(); cmdErr != nil {
		return errors.Wrapf(cmdErr, "failed to execute cmd")
	}

	return nil
}

// ExecLogin executes a docker login
// Is a workaround for submitting password to stdin without prompting user again
func ExecLogin(registry, username, password string) error {
	_, lookErr := exec.LookPath(Docker)
	if lookErr != nil {
		panic(lookErr)
	}

	loginCmd := fmt.Sprintf("echo '%s' | docker login %s -u %s --password-stdin", password, registry, username)
	cmd := exec.Command("/usr/bin/env", "sh", "-c", loginCmd)

	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// AirflowCommand is the main method of interaction with Airflow
func AirflowCommand(id string, airflowCommand string) string {
	fmt.Println(airflowCommand)
	cmd := exec.Command("docker", "exec", "-it", id, "bash", "-c", airflowCommand)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()

	if err != nil {
		errors.Wrapf(err, "error encountered")
	}

	stringOut := string(out)

	return stringOut
}
