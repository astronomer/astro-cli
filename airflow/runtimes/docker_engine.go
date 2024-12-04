package runtimes

const (
	defaultTimeoutSeconds = 60
	tickNum               = 500
	open                  = "open"
	timeoutErrMsg         = "timed out waiting for docker"
	dockerOpenNotice      = "We couldn't start the docker engine automatically. Please start it manually and try again."
)

// DockerEngine is a struct that contains the functions needed to initialize Docker.
// The concrete implementation that we use is DefaultDockerEngine below.
// When running the tests, we substitute the default implementation with a mock implementation.
type DockerEngine interface {
	IsRunning() (string, error)
	Start() (string, error)
}

// DefaultDockerEngine is the default implementation of DockerEngine.
// The concrete functions defined here are called from the InitializeDocker function below.
type DefaultDockerEngine struct{}

func (d DefaultDockerEngine) IsRunning() (string, error) {
	checkDockerCmd := Command{
		Command: docker,
		Args:    []string{"ps"},
	}
	return checkDockerCmd.Execute()
}

func (d DefaultDockerEngine) Start() (string, error) {
	openDockerCmd := Command{
		Command: open,
		Args:    []string{"-a", docker},
	}
	return openDockerCmd.Execute()
}
