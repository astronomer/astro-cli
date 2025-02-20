package runtimes

import (
	"errors"
	"time"

	"github.com/briandowns/spinner"
	"github.com/docker/docker/client"
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

// CreateDockerRuntime creates a new DockerRuntime with the provided DockerEngine and OSChecker.
func CreateDockerRuntime(engine DockerEngine, osChecker OSChecker) DockerRuntime {
	return DockerRuntime{Engine: engine, OSChecker: osChecker}
}

// CreateDockerRuntimeWithDefaults creates a new DockerRuntime with the default DockerEngine and OSChecker.
func CreateDockerRuntimeWithDefaults() DockerRuntime {
	return DockerRuntime{Engine: GetDockerEngine(CreateHostInspectorWithDefaults()), OSChecker: CreateOSChecker()}
}

// GetDockerEngine returns the appropriate DockerEngine based on the binary discovered.
func GetDockerEngine(interrogator HostInterrogator) DockerEngine {
	engine, err := GetDockerEngineBinary(interrogator)
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

// GetDockerEngineBinary returns the first Docker binary found in the $PATH environment variable.
// This is used to determine which Engine to use. Eg: orbctl or docker.
func GetDockerEngineBinary(inspector HostInterrogator) (string, error) {
	// If the orbctl binary is found, it means orbstack is installed. The docker binary would also exist
	// in this case, but we use this as a signal to use the orbstack engine. We'll still end up
	// shelling our docker commands out using the docker binary.
	binaries := []string{orbctl, docker}

	// Get the $PATH environment variable.
	pathEnv := inspector.GetEnvVar("PATH")
	for _, binary := range binaries {
		if found := FindBinary(pathEnv, binary, inspector); found {
			return binary, nil
		}
	}

	// If no binary is found, we just return our default docker binary.
	// The higher level check for the container runtime binary will handle the user-facing error.
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
	s := spinner.New(spinnerCharSet, spinnerRefresh)
	s.Suffix = " " + containerRuntimeInitMessage
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

func (rt DockerRuntime) NewDockerClient() (*client.Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}
	return cli, nil
}
