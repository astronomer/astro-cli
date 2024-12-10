package runtimes

const (
	defaultTimeoutSeconds = 60
	tickNum               = 500
	open                  = "open"
	timeoutErrMsg         = "timed out waiting for docker"
	dockerOpenNotice      = "We couldn't start the docker engine automatically. Please start it manually and try again."
)

// dockerEngine is the default implementation of DockerEngine.
type dockerEngine struct{}

func (d dockerEngine) IsRunning() (string, error) {
	checkDockerCmd := Command{
		Command: docker,
		Args: []string{
			"ps",
		},
	}
	return checkDockerCmd.Execute()
}

func (d dockerEngine) Start() (string, error) {
	openDockerCmd := Command{
		Command: open,
		Args: []string{
			"-a",
			docker,
		},
	}
	return openDockerCmd.Execute()
}
