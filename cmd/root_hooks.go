package cmd

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	cloudCmd "github.com/astronomer/astro-cli/cmd/cloud"
	softwareCmd "github.com/astronomer/astro-cli/cmd/software"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/version"
	"github.com/google/go-github/v48/github"
	"github.com/spf13/cobra"
)

// SetupLogging is a pre-run hook shared between software & cloud
// setting up log verbosity.
func SetupLogging(_ *cobra.Command, _ []string) error {
	return softwareCmd.SetUpLogs(os.Stdout, verboseLevel)
}

// CreateRootPersistentPreRunE takes clients as arguments and returns a cobra
// pre-run hook that sets up the context and checks for the latest version.
func CreateRootPersistentPreRunE(astroCoreClient astrocore.CoreClient, platformCoreClient astroplatformcore.CoreClient) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		// Check for latest version
		if config.CFG.UpgradeMessage.GetBool() {
			// create github client with 3 second timeout, setting an aggressive timeout since its not mandatory to get a response in each command execution
			githubClient := github.NewClient(&http.Client{Timeout: 3 * time.Second})
			// compare current version to latest
			err := version.CompareVersions(githubClient, "astronomer", "astro-cli")
			if err != nil {
				softwareCmd.InitDebugLogs = append(softwareCmd.InitDebugLogs, "Error comparing CLI versions: "+err.Error())
			}
		}
		if context.IsCloudContext() {
			err := cloudCmd.Setup(cmd, platformCoreClient, astroCoreClient)
			if err != nil {
				if strings.Contains(err.Error(), "token is invalid or malformed") {
					return errors.New("API Token is invalid or malformed") //nolint
				}
				if strings.Contains(err.Error(), "the API token given has expired") {
					return errors.New("API Token is expired") //nolint
				}
				softwareCmd.InitDebugLogs = append(softwareCmd.InitDebugLogs, "Error during cmd setup: "+err.Error())
			}
		}
		softwareCmd.PrintDebugLogs()
		return nil
	}
}
