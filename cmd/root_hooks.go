package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	cloudCmd "github.com/astronomer/astro-cli/cmd/cloud"
	softwareCmd "github.com/astronomer/astro-cli/cmd/software"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/keychain"
	"github.com/astronomer/astro-cli/version"
)

// SetupLogging is a pre-run hook shared between software & cloud
// setting up log verbosity.
func SetupLogging(_ *cobra.Command, _ []string) error {
	return softwareCmd.SetUpLogs(os.Stdout, verboseLevel)
}

// CreateRootPersistentPreRunE takes clients as arguments and returns a cobra
// pre-run hook that sets up the context and checks for the latest version.
func CreateRootPersistentPreRunE(storeErr error, store keychain.SecureStore, tokenHolder *httputil.TokenHolder, astroCoreClient astrocore.CoreClient, platformCoreClient astroplatformcore.CoreClient) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		// login/logout don't need existing credentials, skip auth setup
		if cmd.CalledAs() == "login" || cmd.CalledAs() == "logout" {
			return nil
		}

		if storeErr != nil {
			return fmt.Errorf("secure credential store unavailable: %w", storeErr)
		}

		// Check for latest version
		if config.CFG.UpgradeMessage.GetBool() {
			httpClient := &http.Client{Timeout: 3 * time.Second}
			err := version.CompareVersions(cmd.Context(), httpClient)
			if err != nil {
				softwareCmd.InitDebugLogs = append(softwareCmd.InitDebugLogs, "Error comparing CLI versions: "+err.Error())
			}
		}

		if migrated, err := config.MigrateLegacyCredentials(store); err != nil {
			softwareCmd.InitDebugLogs = append(softwareCmd.InitDebugLogs, "credential migration error: "+err.Error())
		} else if migrated > 0 {
			fmt.Printf("Migrated credentials for %d context(s) to your system's secure store.\n", migrated)
		}

		if context.IsCloudContext() {
			if err := handleCloudSetup(cmd, store, tokenHolder, platformCoreClient, astroCoreClient); err != nil {
				return err
			}
		} else {
			loadSoftwareToken(store, tokenHolder)
		}
		softwareCmd.PrintDebugLogs()
		return nil
	}
}

func handleCloudSetup(cmd *cobra.Command, store keychain.SecureStore, tokenHolder *httputil.TokenHolder, platformCoreClient astroplatformcore.CoreClient, astroCoreClient astrocore.CoreClient) error {
	err := cloudCmd.Setup(cmd, store, tokenHolder, platformCoreClient, astroCoreClient)
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "token is invalid or malformed") {
		return errors.New("API Token is invalid or malformed") //nolint
	}
	if strings.Contains(err.Error(), "the API token given has expired") {
		return errors.New("API Token is expired") //nolint
	}
	softwareCmd.InitDebugLogs = append(softwareCmd.InitDebugLogs, "Error during cmd setup: "+err.Error())
	return nil
}

func loadSoftwareToken(store keychain.SecureStore, tokenHolder *httputil.TokenHolder) {
	if store == nil {
		return
	}
	c, err := context.GetCurrentContext()
	if err != nil {
		return
	}
	if creds, credErr := store.GetCredentials(c.Domain); credErr == nil {
		tokenHolder.Set(creds.Token)
	}
}
