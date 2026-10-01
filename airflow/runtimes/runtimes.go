// Package runtimes is a thin adapter over the shared pkg/container module. The
// container-runtime logic (engine resolution, Docker auto-start, reaching the
// user's Podman machine) lives in github.com/astronomer/astro-cli/pkg/container
// so it can be shared with other tools (e.g. Astro Desktop). This package wires
// that logic to astro-cli's global config singleton and CLI spinner, preserving
// the call surface the rest of the CLI already depends on.
package runtimes

import (
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/container"
)

// cliConfig builds a container.Config from astro-cli's global config singleton.
func cliConfig() container.Config {
	return container.Config{Binary: config.CFG.DockerCommand.GetString()}
}

// GetContainerRuntimeBinary returns the resolved engine binary ("docker" or
// "podman"). It is a var so unit tests can replace it (see
// airflow/docker_registry_test.go).
var GetContainerRuntimeBinary = func() (string, error) {
	return container.GetContainerRuntimeBinary(cliConfig())
}

// IsPodman reports whether the given binary name is podman.
func IsPodman(binaryName string) bool {
	return container.IsPodman(binaryName)
}
