// Package runtimes is a thin adapter over the shared pkg/container module. The
// container-runtime logic (engine resolution, Docker auto-start, the Podman
// astro-machine lifecycle) lives in github.com/astronomer/astro-cli/pkg/container
// so it can be shared with other tools (e.g. astro-desktop). This package wires
// that logic to astro-cli's global config singleton and CLI spinner, preserving
// the call surface the rest of the CLI already depends on.
package runtimes

import (
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/container"
)

// ContainerRuntime is the lifecycle interface the CLI command hooks drive. It is
// an alias for the shared container.ContainerRuntime so existing call sites
// (cmd/airflow.go, cmd/airflow_hooks.go) keep compiling unchanged.
type ContainerRuntime = container.ContainerRuntime

// OSChecker reports the host operating system. Alias for container.OSChecker so
// callers (EnsureRuntime's Windows plugins-dir handling) keep compiling.
type OSChecker = container.OSChecker

// cliConfig builds a container.Config from astro-cli's global config singleton.
func cliConfig() container.Config {
	return container.Config{
		Binary:        config.CFG.DockerCommand.GetString(),
		MachineMemory: config.CFG.MachineMemory.GetString(),
		MachineCPU:    config.CFG.MachineCPU.GetString(),
	}
}

// GetContainerRuntimeBinary returns the resolved engine binary ("docker" or
// "podman"). It is a var so unit tests can replace it (see
// airflow/docker_registry_test.go).
var GetContainerRuntimeBinary = func() (string, error) {
	return container.GetContainerRuntimeBinary(cliConfig())
}

// GetContainerRuntime resolves the host engine and returns the matching runtime,
// wired to the CLI spinner so the "Astro uses containers…" UX is preserved.
func GetContainerRuntime() (ContainerRuntime, error) {
	return container.GetContainerRuntime(cliConfig(), newSpinnerFeedback())
}

// IsPodman reports whether the given binary name is podman.
func IsPodman(binaryName string) bool {
	return container.IsPodman(binaryName)
}

// CreateOSChecker returns the default OSChecker.
func CreateOSChecker() OSChecker {
	return container.CreateOSChecker()
}
