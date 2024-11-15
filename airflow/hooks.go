package airflow

import (
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/spf13/cobra"
	"runtime"
)

func PreRunHook(cmd *cobra.Command, args []string) error {
	if err := utils.EnsureProjectDir(cmd, args); err != nil {
		return err
	}

	containerRuntime, err := GetContainerRuntimeBinary()
	if err != nil {
		return err
	}

	if containerRuntime == dockerCmd && runtime.GOOS == "darwin" {
		err := startDocker()
		if err != nil {
			return err
		}
	}

	if containerRuntime == podmanCmd {
		if err := InitPodmanMachineCMD(); err != nil {
			return err
		}
	}

	return nil
}

func StopPreRunHook(cmd *cobra.Command, args []string) error {
	if err := utils.EnsureProjectDir(cmd, args); err != nil {
		return err
	}

	containerRuntime, err := GetContainerRuntimeBinary()
	if err != nil {
		return err
	}

	if containerRuntime == podmanCmd {
		if err := SetPodmanDockerHost(); err != nil {
			return err
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
