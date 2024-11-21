package airflow

import (
	"errors"

	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/spf13/cobra"
)

const (
	projectNotRunningErrMsg = "this astro project is not running"
)

// EnsureRuntimePreRunHook ensures that the project directory exists
// and starts the container runtime if necessary.
func EnsureRuntimePreRunHook(cmd *cobra.Command, args []string) error {
	if err := utils.EnsureProjectDir(cmd, args); err != nil {
		return err
	}

	containerRuntime, err := GetContainerRuntimeBinary()
	if err != nil {
		return err
	}

	// If we're in docker mode and on a Mac, we attempt to start Docker,
	// if it's not already running.
	if containerRuntime == dockerCmd && utils.IsMac() {
		if err := startDocker(); err != nil {
			return err
		}
	}

	// If we're in podman mode, and DOCKER_HOST is not already set
	// we need to initialize our astro machine.
	// If DOCKER_HOST is already set, we assume the user already has a
	// workflow with podman that we don't want to interfere with.
	if containerRuntime == podmanCmd && !IsDockerHostSet() {
		if err := InitMachine(); err != nil {
			return err
		}
	}
	return nil
}

// SetRuntimeIfExistsPreRunHook sets the container runtime if its running,
// otherwise we bail with an error message.
func SetRuntimeIfExistsPreRunHook(cmd *cobra.Command, args []string) error {
	if err := utils.EnsureProjectDir(cmd, args); err != nil {
		return err
	}

	containerRuntime, err := GetContainerRuntimeBinary()
	if err != nil {
		return err
	}

	// If we're in podman mode, and DOCKER_HOST is not already set
	// we need to set things up for our astro machine.
	// If DOCKER_HOST is already set, we assume the user already has a
	// workflow with podman that we don't want to interfere with.
	if containerRuntime == podmanCmd && !IsDockerHostSet() {
		// If the astro machine is running, we just configure it
		// for usage, so the regular compose commands can carry out.
		if AstroMachineIsRunning() {
			return GetAndConfigureMachineForUsage(podmanMachineName)
		}

		// Otherwise, we return an error indicating that the project isn't running.
		return errors.New(projectNotRunningErrMsg)
	}
	return nil
}

// KillPreRunHook sets the container runtime if its running,
// otherwise we bail with an error message.
func KillPreRunHook(cmd *cobra.Command, args []string) error {
	if err := utils.EnsureProjectDir(cmd, args); err != nil {
		return err
	}

	containerRuntime, err := GetContainerRuntimeBinary()
	if err != nil {
		return err
	}

	// If we're in podman mode, and DOCKER_HOST is not already set
	// we need to set things up for our astro machine.
	// If DOCKER_HOST is already set, we assume the user already has a
	// workflow with podman that we don't want to interfere with.
	if containerRuntime == podmanCmd && !IsDockerHostSet() {
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
		return errors.New(projectNotRunningErrMsg)
	}
	return nil
}

// KillPostRunHook ensures that we stop and kill the
// podman machine once a project has been killed.
func KillPostRunHook(_ *cobra.Command, _ []string) error {
	containerRuntime, err := GetContainerRuntimeBinary()
	if err != nil {
		return err
	}

	// If we're in podman mode, and DOCKER_HOST is set to the astro machine (in the pre-run hook),
	// we'll ensure that the machine is killed.
	if containerRuntime == podmanCmd && IsDockerHostSetToAstroMachine() {
		if err := StopAndKillMachine(); err != nil {
			return err
		}
	}
	return nil
}
