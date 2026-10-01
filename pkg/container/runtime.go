package container

import "errors"

const containerRuntimeNotFoundErrMsg = notFoundMsg

// ContainerRuntime is what a caller drives before using a container engine:
// Initialize brings the engine up (auto-starting Docker or OrbStack on Mac) or,
// for Podman, checks that the user's machine is running.
type ContainerRuntime interface {
	Initialize() error
}

// GetContainerRuntime resolves the host engine from cfg and returns the matching
// ContainerRuntime: a *DockerRuntime for Docker/OrbStack, or the *Manager for
// Podman. A nil Feedback is replaced with NoopFeedback. This is the CLI-facing
// entry point that astro-cli's adapter calls.
func GetContainerRuntime(cfg Config, fb Feedback) (ContainerRuntime, error) {
	if fb == nil {
		fb = NoopFeedback{}
	}
	engine, err := Resolve(cfg)
	if err != nil {
		return nil, err
	}
	switch engine {
	case Docker, Orbstack:
		return newDockerRuntime(engine, fb), nil
	case Podman:
		return NewManager(cfg, fb)
	default:
		return nil, errors.New(containerRuntimeNotFoundErrMsg)
	}
}
