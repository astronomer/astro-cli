package container

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

const projectNotRunningErrMsg = "this astro project is not running"

// Manager implements ContainerRuntime for the Podman case. Initialize ensures
// the astro-machine is up; Configure points this process at it; ConfigureOrKill
// configures when running and tears the machine down otherwise; Kill stops and
// removes the machine when DOCKER_HOST points at it.

// Initialize brings the astro-machine up AND points this process at it. It is
// the ContainerRuntime entry point (the dev-start pre-run hook): EnsureMachine
// starts the machine and sets it as the default podman connection, but the
// current process still needs DOCKER_HOST/CONTAINER_HOST set to its socket so the
// compose build/up reaches that machine instead of the default docker daemon
// (without this, `astro dev start` under podman fails with "pull access
// denied"). No-op for non-Podman engines or when the user already has DOCKER_HOST
// set — ConnectionEnv returns nil in both cases.
func (m *Manager) Initialize() error {
	if err := m.EnsureMachine(); err != nil {
		return err
	}
	env, err := m.ConnectionEnv()
	if err != nil {
		return err
	}
	for _, kv := range env {
		key, value, _ := strings.Cut(kv, "=")
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("error setting %s: %w", key, err)
		}
	}
	return nil
}

// Configure points the current process at the running astro-machine by setting
// DOCKER_HOST/CONTAINER_HOST and the podman default connection. If DOCKER_HOST
// is already set we leave the user's own workflow alone. If the machine isn't
// running we return projectNotRunningErrMsg.
func (m *Manager) Configure() error {
	if m.engine != Podman {
		return nil
	}
	if os.Getenv("DOCKER_HOST") != "" {
		return nil
	}
	if m.astroMachineIsRunning() {
		return m.configureMachineForUsage(podmanMachineName)
	}
	return errors.New(projectNotRunningErrMsg)
}

// ConfigureOrKill configures the machine for usage when it's running (the kill
// follows in a post-run hook), otherwise it ensures the machine is fully torn
// down and returns projectNotRunningErrMsg. A pre-set DOCKER_HOST short-circuits
// to a no-op.
func (m *Manager) ConfigureOrKill() error {
	if m.engine != Podman {
		return nil
	}
	if os.Getenv("DOCKER_HOST") != "" {
		return nil
	}
	if m.astroMachineIsRunning() {
		return m.configureMachineForUsage(podmanMachineName)
	}
	if err := m.stopAndKillMachine(); err != nil {
		return err
	}
	return errors.New(projectNotRunningErrMsg)
}

// Kill stops and removes the astro-machine, but only when DOCKER_HOST is pointed
// at it (set in the pre-run hook). For non-Podman engines, or any other
// DOCKER_HOST, it is a no-op.
func (m *Manager) Kill() error {
	if m.engine != Podman {
		return nil
	}
	if strings.Contains(os.Getenv("DOCKER_HOST"), podmanMachineName) {
		return m.stopAndKillMachine()
	}
	return nil
}

// astroMachineIsRunning reports whether the astro-machine exists and is running.
func (m *Manager) astroMachineIsRunning() bool {
	machine, err := m.astroMachine()
	if err != nil {
		return false
	}
	return machine != nil && machine.Running
}

// configureMachineForUsage inspects the named machine and sets DOCKER_HOST
// (and CONTAINER_HOST off Windows) to its socket, then sets it as the podman
// default connection. It shares its host/socket computation with ConnectionEnv
// via connectionEnvFor.
func (m *Manager) configureMachineForUsage(name string) error {
	machine, err := m.podman.InspectMachine(name)
	if err != nil {
		return err
	}
	if machine == nil {
		return fmt.Errorf("machine does not exist")
	}

	for _, kv := range connectionEnvFor(machine, m.host.isWindows()) {
		key, value, _ := strings.Cut(kv, "=")
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("error setting %s: %s", key, err)
		}
	}

	return m.podman.SetMachineAsDefault(machine.Name)
}

// stopAndKillMachine stops (if running and no other projects remain) and removes
// the astro-machine. It is a no-op when the machine doesn't exist.
func (m *Manager) stopAndKillMachine() error {
	machine, err := m.astroMachine()
	if err != nil {
		return err
	}
	if machine == nil {
		return nil
	}

	if machine.Running {
		containers, err := m.podman.ListContainers()
		if err != nil {
			return err
		}
		// At this point our own project has already been stopped; if any other
		// project is still running, leave the machine up.
		if m.hasRunningProjects(containers) {
			return nil
		}
		if err := m.podman.StopMachine(podmanMachineName); err != nil {
			return err
		}
	}

	return m.podman.RemoveMachine(podmanMachineName)
}

// connectionEnvFor computes the DOCKER_HOST (and, off Windows, CONTAINER_HOST)
// environment for a running machine. Shared by ConnectionEnv (returns the env)
// and configureMachineForUsage (sets the env), so the socket/scheme logic lives
// in one place.
func connectionEnvFor(machine *InspectedMachine, windows bool) []string {
	dockerHost := "unix://" + machine.ConnectionInfo.PodmanSocket.Path
	if windows {
		dockerHost = "npipe:////./pipe/podman-" + machine.Name
	}
	env := []string{"DOCKER_HOST=" + dockerHost}
	// CONTAINER_HOST routes native podman commands (e.g. `podman build`) to the
	// machine. The npipe:// scheme isn't supported by native podman on Windows,
	// where the default-connection setting handles routing instead.
	if !windows {
		env = append(env, "CONTAINER_HOST="+dockerHost)
	}
	return env
}
