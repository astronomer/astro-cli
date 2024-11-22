package container_runtime

type DockerRuntime struct{}

func (p DockerRuntime) Initialize() error {
	if !isMac() {
		return nil
	}
	return InitializeDocker()
}

func (p DockerRuntime) Configure() error {
	return nil
}

func (p DockerRuntime) ConfigureOrKill() error {
	return nil
}

func (p DockerRuntime) Kill() error {
	return nil
}
