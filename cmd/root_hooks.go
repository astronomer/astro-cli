package cmd

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

// SetupLogging is a pre-run hook shared between APC & cloud
// setting up log verbosity. Logs go to stderr: stdout is the command's output,
// which under --output json is one object a script parses.
func SetupLogging(_ *cobra.Command, _ []string) error {
	return apcCmd.SetUpLogs(os.Stderr, verboseLevel)
}

// readVerbosity sets verboseLevel from a removed command's raw arguments. A
// stub parses no flags (cliout.RemovedCommand), so cobra never sets the
// root's --verbosity for it; this reads it in the spellings pflag accepts for
// it, --verbosity=LEVEL and --verbosity LEVEL, stopping at the "--" that ends
// flags. The last one wins, as it would in pflag.
func readVerbosity(_ *cobra.Command, args []string) error {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			break
		}
		if level, ok := strings.CutPrefix(a, "--"+verbosityFlag+"="); ok {
			verboseLevel = level
		} else if a == "--"+verbosityFlag && i+1 < len(args) {
			i++
			verboseLevel = args[i]
		}
	}
	return nil
}

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
		if context.IsCloudContext() {
			if err := astroSetup(cmd, astroV1Client); err != nil {
				apcCmd.PrintDebugLogs()
				return err
			}
		}
		apcCmd.PrintDebugLogs()
		return nil
	}
}
