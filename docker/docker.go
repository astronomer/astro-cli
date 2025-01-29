package docker

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/astronomer/astro-cli/airflow/runtimes"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/stdcopy"
)

const (
	// Docker is the docker command.
	Docker = "docker"
)

// AirflowCommand is the main method of interaction with Airflow
func AirflowCommand(id, airflowCommand string) (string, error) {
	containerRuntime, err := runtimes.GetContainerRuntimeBinary()
	if err != nil {
		return "", err
	}
	cmd := exec.Command(containerRuntime, "exec", "-it", id, "bash", "-c", airflowCommand)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("error encountered while running the airflow command: %w", err)
	}

	stringOut := string(out)
	return stringOut, nil
}

// ExecPipe does pipe stream into stdout/stdin and stderr
// so now we can pipe out during exec'ing any commands inside container
func ExecPipe(resp types.HijackedResponse, inStream io.Reader, outStream, errorStream io.Writer) error {
	var err error
	receiveStdout := make(chan error, 1)
	if outStream != nil || errorStream != nil {
		go func() {
			// always do this because we are never tty
			_, err = stdcopy.StdCopy(outStream, errorStream, resp.Reader)
			receiveStdout <- err
		}()
	}

	stdinDone := make(chan struct{})
	go func() {
		if inStream != nil {
			_, err := io.Copy(resp.Conn, inStream)
			if err != nil {
				fmt.Println("Error copying input stream: ", err.Error())
			}
		}

		err := resp.CloseWrite()
		if err != nil {
			fmt.Println("Error closing response body: ", err.Error())
		}
		close(stdinDone)
	}()

	select {
	case err := <-receiveStdout:
		if err != nil {
			return err
		}
	case <-stdinDone:
		if outStream != nil || errorStream != nil {
			if err := <-receiveStdout; err != nil {
				return err
			}
		}
	}

	return nil
}
