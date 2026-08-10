package cmd

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/astro-client-v1"
	apcCmd "github.com/astronomer/astro-cli/cmd/apc"
	cloudCmd "github.com/astronomer/astro-cli/cmd/cloud"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/version"
)

// SetupLogging is a pre-run hook shared between APC & cloud
// setting up log verbosity.
func SetupLogging(_ *cobra.Command, _ []string) error {
	return apcCmd.SetUpLogs(os.Stdout, verboseLevel)
}

// CreateRootPersistentPreRunE takes clients as arguments and returns a cobra
// pre-run hook that sets up the context and checks for the latest version.
func CreateRootPersistentPreRunE(astroV1Client astrov1.APIClient) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		// Check for latest version
		if config.CFG.UpgradeMessage.GetBool() {
			// create http client with 3 second timeout, setting an aggressive timeout since its not mandatory to get a response in each command execution
			httpClient := &http.Client{Timeout: 3 * time.Second}

			// compare current version to latest
			err := version.CompareVersions(cmd.Context(), httpClient)
			if err != nil {
				apcCmd.InitDebugLogs = append(apcCmd.InitDebugLogs, "Error comparing CLI versions: "+err.Error())
			}
		}
		if context.IsCloudContext() {
			err := cloudCmd.Setup(cmd, astroV1Client)
			if err != nil {
				if strings.Contains(err.Error(), "token is invalid or malformed") {
					return errors.New("API Token is invalid or malformed") //nolint
				}
				if strings.Contains(err.Error(), "the API token given has expired") {
					return errors.New("API Token is expired") //nolint
				}
				apcCmd.InitDebugLogs = append(apcCmd.InitDebugLogs, "Error during cmd setup: "+err.Error())
			}
		}
		apcCmd.PrintDebugLogs()
		return nil
	}
}
