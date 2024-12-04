package runtimes

import (
	"fmt"
	"time"

	"github.com/briandowns/spinner"
)

// DockerRuntime is a concrete implementation of the ContainerRuntime interface.
// When the docker binary is chosen, this implementation is used.
type DockerRuntime struct {
	Engine DockerEngine
}

func CreateDockerRuntime(engine DockerEngine) DockerRuntime {
	return DockerRuntime{Engine: engine}
}

// Initialize initializes the Docker runtime.
// We only attempt to initialize Docker on Mac today.
func (rt DockerRuntime) Initialize() error {
	if !isMac() {
		return nil
	}
	return rt.InitializeDocker(defaultTimeoutSeconds)
}

func (rt DockerRuntime) Configure() error {
	return nil
}

func (rt DockerRuntime) ConfigureOrKill() error {
	return nil
}

func (rt DockerRuntime) Kill() error {
	return nil
}

// InitializeDocker initializes the Docker runtime.
// It checks if Docker is running, and if it is not, it attempts to start it.
func (rt DockerRuntime) InitializeDocker(timeoutSeconds int) error {
	// Initialize spinner.
	timeout := time.After(time.Duration(timeoutSeconds) * time.Second)
	ticker := time.NewTicker(time.Duration(tickNum) * time.Millisecond)
	s := spinner.New(spinnerCharSet, spinnerRefresh)
	s.Suffix = containerRuntimeInitMessage
	defer s.Stop()

	// Execute `docker ps` to check if Docker is running.
	_, err := rt.Engine.IsRunning()

	// If we didn't get an error, Docker is running, so we can return.
	if err == nil {
		return nil
	}

	// If we got an error, Docker is not running, so we attempt to start it.
	_, err = rt.Engine.Start()
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
			_, err := rt.Engine.IsRunning()
			if err != nil {
				continue
			}
			return nil
		}
	}
}
