package airflow

import (
	"errors"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/spf13/cobra"
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

	if containerRuntime == dockerCmd && utils.IsMac() {
		err := startDocker()
		if err != nil {
			return err
		}
	}

	if containerRuntime == podmanCmd {
		if err := InitPodmanMachine(); err != nil {
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

	if containerRuntime == podmanCmd {
		if IsPodmanMachineRunning() {
			return GetAndConfigureMachineForUsage()
		}
		return errors.New("this astro project is not running")
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

	if containerRuntime == podmanCmd {
		if IsPodmanMachineRunning() {
			return GetAndConfigureMachineForUsage()
		} else {
			if err := StopAndKillPodmanMachine(); err != nil {
				return err
			}
			return errors.New("this astro project is not running")
		}
	}
	return nil
}

func KillPostRunHook(cmd *cobra.Command, args []string) error {
	containerRuntime, err := GetContainerRuntimeBinary()
	if err != nil {
		return err
	}

	if containerRuntime == podmanCmd {
		if err := StopAndKillPodmanMachine(); err != nil {
			return err
		}
	}
	return nil
}
