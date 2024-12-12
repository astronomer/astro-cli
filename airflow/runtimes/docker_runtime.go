package runtimes

import (
	"errors"
	"time"

	"github.com/briandowns/spinner"
)

// DockerEngine is a struct that contains the functions needed to initialize Docker.
// The concrete implementation that we use is dockerEngine.
// When running the tests, we substitute the default implementation with a mock implementation.
type DockerEngine interface {
	IsRunning() (string, error)
	Start() (string, error)
}

// DockerRuntime is a concrete implementation of the ContainerRuntime interface.
// When the docker binary is chosen, this implementation is used.
type DockerRuntime struct {
	Engine    DockerEngine
	OSChecker OSChecker
}

func CreateDockerRuntime(engine DockerEngine, osChecker OSChecker) DockerRuntime {
	return DockerRuntime{Engine: engine, OSChecker: osChecker}
}

// Initialize initializes the Docker runtime.
// We only attempt to initialize Docker on Mac today.
func (rt DockerRuntime) Initialize() error {
	if !rt.OSChecker.IsMac() {
		return nil
	}
	return rt.initializeDocker(defaultTimeoutSeconds)
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

// initializeDocker initializes the Docker runtime.
// It checks if Docker is running, and if it is not, it attempts to start it.
func (rt DockerRuntime) initializeDocker(timeoutSeconds int) error {
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
		return errors.New(dockerOpenNotice) //nolint:staticcheck,stylecheck
	}

	// Wait for Docker to start.
	s.Start()
	for {
		select {
		case <-timeout:
			return errors.New(timeoutErrMsg)
		case <-ticker.C:
			_, err := rt.Engine.IsRunning()
			if err != nil {
				continue
			}
			return nil
		}
	}
}
