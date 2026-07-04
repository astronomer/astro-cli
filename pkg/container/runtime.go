package container

import "errors"

const containerRuntimeNotFoundErrMsg = notFoundMsg

// ContainerRuntime is the lifecycle a caller drives around a container engine:
// Initialize brings the engine up (auto-starting Docker on Mac, or ensuring the
// Podman astro-machine), Configure points the current process at the running
// engine, ConfigureOrKill does the same but tears the machine down when nothing
// is running, and Kill stops/removes the astro-machine when appropriate. For
// Docker/OrbStack all but Initialize are no-ops; the Podman implementation is
// the *Manager.
type ContainerRuntime interface {
	Initialize() error
	Configure() error
	ConfigureOrKill() error
	Kill() error
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
