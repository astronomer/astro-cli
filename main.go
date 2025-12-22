package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/astronomer/astro-cli/cmd"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/plugin"
	"github.com/spf13/afero"
)

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --version
//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --config ./astro-client-core/api.cfg.yaml ../astro/apps/core/docs/public/v1alpha1/public_v1alpha1.yaml
//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --config ./astro-client-iam-core/api.cfg.yaml ../astro/apps/core/docs/iam/v1beta1/iam_v1beta1.yaml
//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --config ./astro-client-platform-core/api.cfg.yaml ../astro/apps/core/docs/platform/v1beta1/platform_v1beta1.yaml

func main() {
	// TODO: Remove this when version logic is implemented
	fs := afero.NewOsFs()
	config.InitConfig(fs)

	rootCmd := cmd.NewRootCmd()

	// Silence errors so we can handle plugin fallback without Cobra printing the error first
	rootCmd.SilenceErrors = true

	if err := rootCmd.Execute(); err != nil {
		// Check if this is an unknown command error and try to execute as plugin
		if isUnknownCommandError(err) && len(os.Args) > 1 {
			if tryExecutePlugin(os.Args[1:]) {
				return // Plugin executed successfully
			}
		}
		// Plugin not found or other error, print the original error
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// platform specific terminal initialization:
	// this should run for all commands,
	// for most of the architectures there's no requirements:
	ansi.InitConsole()
}

// isUnknownCommandError checks if the error indicates an unknown command
func isUnknownCommandError(err error) bool {
	errMsg := err.Error()
	return strings.Contains(errMsg, "unknown command") ||
		strings.Contains(errMsg, "unknown subcommand")
}

// tryExecutePlugin attempts to find and execute a plugin for the given arguments
func tryExecutePlugin(args []string) bool {
	pluginPath, pluginArgs, err := plugin.FindPlugin(args)
	if err != nil {
		// No plugin found, let Cobra handle the error
		return false
	}

	// Execute the plugin
	if err := plugin.ExecutePlugin(pluginPath, pluginArgs); err != nil {
		fmt.Fprintf(os.Stderr, "Error executing plugin: %v\n", err)
		os.Exit(1)
	}

	return true
}
