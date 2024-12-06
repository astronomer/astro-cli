package runtimes

// PodmanRuntime is a concrete implementation of the ContainerRuntime interface.
// When the podman binary is chosen, this implementation is used.
type PodmanRuntime struct{}

// Initialize initializes the Podman runtime.
func (p PodmanRuntime) Initialize() error {
	return nil
}
