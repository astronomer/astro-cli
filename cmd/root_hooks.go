package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/astro-client-v1"
	cloudCmd "github.com/astronomer/astro-cli/cmd/cloud"
	softwareCmd "github.com/astronomer/astro-cli/cmd/software"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/credentials"
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
func CreateRootPersistentPreRunE(storeErr error, store keychain.SecureStore, creds *credentials.CurrentCredentials, astroV1Client astrov1.APIClient) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		// logout doesn't need existing credentials, and still has local context
		// to reset even when the store is broken — let it through either way.
		if cmd.CalledAs() == "logout" {
			return nil
		}

		// Checked before login returns early: login doesn't need existing
		// credentials, but it does need somewhere to put new ones, and this is
		// where the reason it can't is worth saying.
		if storeErr != nil {
			return fmt.Errorf("secure credential store unavailable: %w", storeErr)
		}

		if cmd.CalledAs() == "login" {
			return nil
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
			// Deliberately doesn't name the destination: on a host with no OS
			// keyring this store is the plaintext fallback, and claiming a
			// "secure store" would be a lie. warnInsecureWrite says so instead.
			fmt.Printf("Moved credentials for %d context(s) out of config.yaml into the credential store.\n", migrated)
		}

		if context.IsCloudContext() {
			if err := handleCloudSetup(cmd, store, creds, astroV1Client); err != nil {
				return err
			}
		} else if err := loadStoredToken(store, creds); err != nil {
			return err
		}
		softwareCmd.PrintDebugLogs()
		return nil
	}
}

func handleCloudSetup(cmd *cobra.Command, store keychain.SecureStore, creds *credentials.CurrentCredentials, astroV1Client astrov1.APIClient) error {
	err := cloudCmd.Setup(cmd, store, creds, astroV1Client)
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

// loadStoredToken populates creds with the current context's stored token,
// without refreshing it or prompting for login. Used by Software contexts,
// which have no checkToken equivalent to do it for them, and by `astro dev`,
// whose pre-run hook shadows this one (see loadDevCredentials).
//
// It distinguishes "nothing stored" from "could not read what is stored". The
// former is ordinary — the user isn't logged in, so leave creds empty and let
// whatever needs auth say so. The latter must not be swallowed: a locked
// keychain, a denied ACL prompt after a binary update, or a dead D-Bus would
// otherwise leave creds empty and send unauthenticated requests,
// surfacing as a baffling 401 rather than the credential problem it is.
func loadStoredToken(store keychain.SecureStore, creds *credentials.CurrentCredentials) error {
	if store == nil {
		return nil
	}
	c, err := context.GetCurrentContext()
	if err != nil {
		// No context set yet, so there is nothing to load.
		return nil
	}
	keyCreds, err := store.GetCredentials(c.Domain)
	switch {
	case errors.Is(err, keychain.ErrNotFound):
		return nil
	case err != nil:
		return fmt.Errorf("could not read stored credentials for %s: %w", c.Domain, err)
	}
	creds.Set(keyCreds.Token)
	return nil
}
