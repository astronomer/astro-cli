package runtimes

import (
	"fmt"
)

const (
	projectNotRunningErrMsg = "this astro project is not running"
)

type PodmanRuntime struct{}

func (p PodmanRuntime) Initialize() error {
	// If we're in podman mode, and DOCKER_HOST is not already set
	// we need to initialize our astro machine.
	// If DOCKER_HOST is already set, we assume the user already has a
	// workflow with podman that we don't want to interfere with.
	if IsDockerHostSet() {
		return nil
	}
	return InitializeMachine()
}

func (p PodmanRuntime) Configure() error {
	// If we're in podman mode, and DOCKER_HOST is not already set
	// we need to set things up for our astro machine.
	// If DOCKER_HOST is already set, we assume the user already has a
	// workflow with podman that we don't want to interfere with.
	if IsDockerHostSet() {
		return nil
	}

	// If the astro machine is running, we just configure it
	// for usage, so the regular compose commands can carry out.
	if AstroMachineIsRunning() {
		return GetAndConfigureMachineForUsage(podmanMachineName)
	}

	// Otherwise, we return an error indicating that the project isn't running.
	return fmt.Errorf(projectNotRunningErrMsg)
}

func (p PodmanRuntime) ConfigureOrKill() error {
	// If we're in podman mode, and DOCKER_HOST is not already set
	// we need to set things up for our astro machine.
	// If DOCKER_HOST is already set, we assume the user already has a
	// workflow with podman that we don't want to interfere with.
	if IsDockerHostSet() {
		return nil
	}

	// If the astro machine is running, we just configure it
	// for usage, so the regular compose kill can carry out.
	// We follow up with a machine kill in the post run hook.
	if AstroMachineIsRunning() {
		return GetAndConfigureMachineForUsage(podmanMachineName)
	}

	// The machine is already not running,
	// so we can just ensure its fully killed.
	if err := StopAndKillMachine(); err != nil {
		return err
	}

	// We also return an error indicating that you can't kill
	// a project that isn't running.
	return fmt.Errorf(projectNotRunningErrMsg)
}

func (p PodmanRuntime) Kill() error {
	// If we're in podman mode, and DOCKER_HOST is set to the astro machine (in the pre-run hook),
	// we'll ensure that the machine is killed.
	if !isWindows() {
		if !IsDockerHostSetToAstroMachine() {
			return nil
		}
	}
	return StopAndKillMachine()
}
