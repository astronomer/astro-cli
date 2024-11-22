package runtimes

type PodmanRuntime struct{}

func (p PodmanRuntime) Initialize() error {
	return nil
}
