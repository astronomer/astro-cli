package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/astronomer/astro-cli/airflow/runtimes"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/config"
	"github.com/spf13/cobra"
)

// DoNothing is a persistent pre-run hook that does nothing.
// Used to clobber the standard astro dev persistent pre-run hook for select commands.
func DoNothing(_ *cobra.Command, _ []string) error {
	return nil
}

// ConfigureContainerRuntime sets up the containerRuntime variable.
// The containerRuntime variable is then used in the following pre-run and post-run hooks
// defined here.
func ConfigureContainerRuntime(_ *cobra.Command, _ []string) error {
	var err error
	containerRuntime, err = runtimes.GetContainerRuntime()
	if err != nil {
		return err
	}
	return nil
}

// EnsureRuntime is a pre-run hook that ensures that the project directory exists
// and starts the container runtime if necessary.
func EnsureRuntime(cmd *cobra.Command, args []string) error {
	if err := utils.EnsureProjectDir(cmd, args); err != nil {
		return err
	}

	if runtimes.IsWindows() {
		pluginsDir := filepath.Join(config.WorkingPath, "plugins")
		if _, err := os.Stat(pluginsDir); os.IsNotExist(err) {
			err := os.MkdirAll(pluginsDir, os.ModePerm)
			if err != nil {
				return fmt.Errorf("failed to create plugins directory: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("failed to check plugins directory: %w", err)
		}
	}
	// Initialize the runtime if it's not running.
	return containerRuntime.Initialize()
}

// SetRuntimeIfExists is a pre-run hook that ensures the project directory exists
// and sets the container runtime if its running, otherwise we bail with an error message.
func SetRuntimeIfExists(cmd *cobra.Command, args []string) error {
	if err := utils.EnsureProjectDir(cmd, args); err != nil {
		return err
	}
	return containerRuntime.Configure()
}

// KillPreRunHook sets the container runtime if its running,
// otherwise we bail with an error message.
func KillPreRunHook(cmd *cobra.Command, args []string) error {
	if err := utils.EnsureProjectDir(cmd, args); err != nil {
		return err
	}
	return containerRuntime.ConfigureOrKill()
}

// KillPostRunHook ensures that we stop and kill the
// podman machine once a project has been killed.
func KillPostRunHook(_ *cobra.Command, _ []string) error {
	// Kill the runtime.
	return containerRuntime.Kill()
}
