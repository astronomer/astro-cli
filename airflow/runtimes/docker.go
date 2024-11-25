package runtimes

import (
	"fmt"
	"time"

	"github.com/briandowns/spinner"
)

const (
	defaultTimeoutSeconds = 60
	tickNum               = 500
	open                  = "open"
	timeoutErrMsg         = "timed out waiting for docker"
	dockerOpenNotice      = "We couldn't start the docker engine automatically. Please start it manually and try again."
)

// DockerInitializer is a struct that contains the functions needed to initialize Docker.
// The concrete implementation that we use is DefaultDockerInitializer below.
// When running the tests, we substitute the default implementation with a mock implementation.
type DockerInitializer interface {
	CheckDockerCmd() (string, error)
	OpenDockerCmd() (string, error)
}

// DefaultDockerInitializer is the default implementation of DockerInitializer.
// The concrete functions defined here are called from the InitializeDocker function below.
type DefaultDockerInitializer struct{}

func (d DefaultDockerInitializer) CheckDockerCmd() (string, error) {
	checkDockerCmd := Command{
		Command: docker,
		Args:    []string{"ps"},
	}
	return checkDockerCmd.Execute()
}

func (d DefaultDockerInitializer) OpenDockerCmd() (string, error) {
	openDockerCmd := Command{
		Command: open,
		Args:    []string{"-a", docker},
	}
	return openDockerCmd.Execute()
}

// InitializeDocker initializes the Docker runtime.
// It checks if Docker is running, and if it is not, it attempts to start it.
func InitializeDocker(d DockerInitializer, timeoutSeconds int) error {
	// Initialize spinner.
	timeout := time.After(time.Duration(timeoutSeconds) * time.Second)
	ticker := time.NewTicker(time.Duration(tickNum) * time.Millisecond)
	s := spinner.New(spinnerCharSet, spinnerRefresh)
	s.Suffix = containerRuntimeInitMessage
	defer s.Stop()

	// Execute `docker ps` to check if Docker is running.
	_, err := d.CheckDockerCmd()

	// If we didn't get an error, Docker is running, so we can return.
	if err == nil {
		return nil
	}

	// If we got an error, Docker is not running, so we attempt to start it.
	_, err = d.OpenDockerCmd()
	if err != nil {
		return fmt.Errorf(dockerOpenNotice) //nolint:stylecheck
	}

	// Wait for Docker to start.
	s.Start()
	for {
		select {
		case <-timeout:
			return fmt.Errorf(timeoutErrMsg)
		case <-ticker.C:
			_, err := d.CheckDockerCmd()
			if err != nil {
				continue
			}
			return nil
		}
	}
}
