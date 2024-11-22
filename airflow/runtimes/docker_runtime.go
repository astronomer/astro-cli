package runtimes

// DockerRuntime is a concrete implementation of the ContainerRuntime interface.
// When the docker binary is chosen, this implementation is used.
type DockerRuntime struct{}

// Initialize initializes the Docker runtime.
// We only attempt to initialize Docker on Mac today.
func (p DockerRuntime) Initialize() error {
	if !isMac() {
		return nil
	}
	return InitializeDocker(DefaultDockerInitializer, defaultTimeoutSeconds)
}
