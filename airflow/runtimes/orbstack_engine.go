package runtimes

// orbstackEngine is the default implementation of DockerEngine.
// The concrete functions defined here are called from the initializeDocker function below.
type orbstackEngine struct {
	dockerEngine
}

func (d orbstackEngine) Start() (string, error) {
	openDockerCmd := Command{
		Command: open,
		Args: []string{
			"-a",
			orbstack,
		},
	}
	return openDockerCmd.Execute()
}
