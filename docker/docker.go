package docker

import (
	"io"
	"log"
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

	cmd := exec.Command("docker", "login", registry, "-u", username, "--password-stdin")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		defer stdin.Close()
		io.WriteString(stdin, password)
	}()

	_, err = cmd.CombinedOutput()

	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	return err
}

// AirflowCommand is the main method of interaction with Airflow
func AirflowCommand(id string, airflowCommand string) string {

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
