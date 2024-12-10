package runtimes

import (
	"fmt"
	"os"
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

func CreateDockerRuntimeWithDefaults() DockerRuntime {
	return DockerRuntime{Engine: GetDockerEngine(), OSChecker: CreateOSChecker()}
}

func GetDockerEngine() DockerEngine {
	engine, err := GetDockerEngineBinary()
	if err != nil {
		return new(dockerEngine)
	}

	// Return the appropriate container runtime based on the binary discovered.
	switch engine {
	case orbctl:
		return new(orbstackEngine)
	default:
		return new(dockerEngine)
	}
}

var GetDockerEngineBinary = func() (string, error) {
	binaries := []string{orbctl, docker}

	// Get the $PATH environment variable.
	pathEnv := os.Getenv("PATH")
	for _, binary := range binaries {
		if found := FindBinary(pathEnv, binary, CreateFileChecker(), CreateOSChecker()); found {
			return binary, nil
		}
	}

	return docker, nil
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
	s := spinner.New(SpinnerCharSet, SpinnerRefresh)
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
