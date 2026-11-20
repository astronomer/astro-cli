package cmd

import (
	"net/http"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	apcCmd "github.com/astronomer/astro-cli/cmd/apc"
	astroCmd "github.com/astronomer/astro-cli/cmd/astro"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/version"
)

var astroSetup = astroCmd.Setup

// releaseCheckFile holds the upgrade check's last answer, in the home config
// directory.
const releaseCheckFile = "release-check.json"

// CreateRootPersistentPreRunE takes clients as arguments and returns a cobra
// pre-run hook that sets up the context and checks for the latest version.
//
// The hook fails closed. An error from Astro's setup stops the command: a run
// that carried on without a login it could not set up would use the saved one
// (or none) and fail late, after doing work, or worse, act as someone the
// person did not mean. Setup decides what is soft, with typed errors: the
// version check below and a token renewal that failed while the access token
// still works never reach here as errors.
//
// Shell completion skips all of it. It runs on every TAB, and must not log in,
// refresh a token, call an API or draw a prompt.
func CreateRootPersistentPreRunE(astroV1Client astrov1.APIClient) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if astroCmd.IsCompletion(cmd) {
			return nil
		}
		// Check for latest version
		if config.CFG.UpgradeMessage.GetBool() {
			// create http client with 3 second timeout, setting an aggressive timeout since its not mandatory to get a response in each command execution
			httpClient := &http.Client{Timeout: 3 * time.Second}

			// compare current version to latest
			err := version.CompareVersions(cmd.Context(), httpClient, filepath.Join(config.HomeConfigPath, releaseCheckFile))
			if err != nil {
				apcCmd.InitDebugLogs = append(apcCmd.InitDebugLogs, "Error comparing CLI versions: "+err.Error())
			}
		}
		var err error
		if context.IsCloudContext() {
			err = astroSetup(cmd, astroV1Client)
		}
		apcCmd.PrintDebugLogs()
		return err
	}
}
