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

const failedToCreatePluginsDir = "failed to create plugins directory: %w"

// noOpContainerRuntime is a ContainerRuntime that does nothing.
// It is used in standalone mode so that downstream hooks can call
// containerRuntime methods without nil-pointer guards.
type noOpContainerRuntime struct{}

func (n noOpContainerRuntime) Initialize() error      { return nil }
func (n noOpContainerRuntime) Configure() error       { return nil }
func (n noOpContainerRuntime) ConfigureOrKill() error { return nil }
func (n noOpContainerRuntime) Kill() error            { return nil }

// ConfigureContainerRuntime sets up the containerRuntime variable and is defined
// as a PersistentPreRunE hook for all astro dev sub-commands. The containerRuntime
// variable is then used in the following pre-run and post-run hooks defined here.
// In standalone mode a no-op runtime is assigned so that downstream hooks never
// encounter a nil containerRuntime.
func ConfigureContainerRuntime(_ *cobra.Command, _ []string) error {
	if isStandaloneMode() {
		containerRuntime = noOpContainerRuntime{}
		return nil
	}
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

	// Check if the OS is Windows and create the plugins project directory if it doesn't exist.
	// In Windows, the compose project will fail if the plugins directory doesn't exist, due
	// to the volume mounts we specify.
	osChecker := runtimes.CreateOSChecker()
	if osChecker.IsWindows() {
		pluginsDir := filepath.Join(config.WorkingPath, "plugins")
		if err := os.MkdirAll(pluginsDir, os.ModePerm); err != nil && !os.IsExist(err) {
			return fmt.Errorf(failedToCreatePluginsDir, err)
		}
	}

	// Initialize the runtime if it's not running.
	// In standalone mode this is a no-op via noOpContainerRuntime.
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
	return containerRuntime.Kill()
}
