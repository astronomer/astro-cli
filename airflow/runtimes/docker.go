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

type DockerInitializer struct {
	CheckDockerCmd func() (string, error)
	OpenDockerCmd  func() (string, error)
}

var DefaultDockerInitializer = DockerInitializer{
	CheckDockerCmd: func() (string, error) {
		checkDockerCmd := Command{
			Command: docker,
			Args:    []string{"ps"},
		}
		return checkDockerCmd.Execute()
	},
	OpenDockerCmd: func() (string, error) {
		openDockerCmd := Command{
			Command: open,
			Args:    []string{"-a", docker},
		}
		return openDockerCmd.Execute()
	},
}

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
		return fmt.Errorf(dockerOpenNotice)
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
