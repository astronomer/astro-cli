package container_runtime

import (
	"fmt"
	"time"

	"github.com/briandowns/spinner"
)

const (
	timeoutNum       = 60
	tickNum          = 500
	open             = "open"
	timeoutErrMsg    = "timed out waiting for docker"
	dockerOpenNotice = "We couldn't start the docker engine automatically. Please start it manually and try again."
)

var (
	checkDockerCmd = Command{
		Command: docker,
		Args:    []string{"ps"},
	}
	openDockerCmd = Command{
		Command: open,
		Args:    []string{"-a", docker},
	}
)

func InitializeDocker() error {
	s := spinner.New(spinnerCharSet, spinnerRefresh)
	s.Suffix = containerRuntimeInitMessage
	defer s.Stop()

	// Execute `docker ps` to check if Docker is running.
	_, err := checkDockerCmd.Execute()

	// If we didn't get an error, Docker is running, so we can return.
	if err == nil {
		return nil
	}

	// If we got an error, Docker is not running, so we attempt to start it.
	_, err = openDockerCmd.Execute()
	if err != nil {
		return fmt.Errorf(dockerOpenNotice)
	}

	// Wait for Docker to start.
	s.Start()
	return WaitForDocker()
}

func WaitForDocker() error {
	timeout := time.After(time.Duration(timeoutNum) * time.Second)
	ticker := time.NewTicker(time.Duration(tickNum) * time.Millisecond)
	for {
		select {
		case <-timeout:
			return fmt.Errorf(timeoutErrMsg)
		case <-ticker.C:
			_, err := checkDockerCmd.Execute()
			if err != nil {
				continue
			}
			return nil
		}
	}
}
