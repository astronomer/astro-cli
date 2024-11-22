package runtimes

type DockerRuntime struct{}

func (p DockerRuntime) Initialize() error {
	if !isMac() {
		return nil
	}
	return InitializeDocker()
}
