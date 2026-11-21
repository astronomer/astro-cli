package cmd

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	apcCmd "github.com/astronomer/astro-cli/cmd/apc"
	astroCmd "github.com/astronomer/astro-cli/cmd/astro"
	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/version"
)

var astroSetup = astroCmd.Setup

// releaseCheckFile holds the upgrade check's last answer, in the home config
// directory.
const releaseCheckFile = "release-check.json"

// pendingUpdateCheck is the run's release lookup, started by the pre-run and
// collected by PrintUpdateNotice.
var pendingUpdateCheck *version.UpdateCheck

// updateNoticeWanted is every reason the run may show an upgrade notice at
// all: the upgrade_message setting on, NO_UPDATE_NOTIFIER unset, stderr a
// terminal (where a person reads it), no CI, and not --output json.
func updateNoticeWanted(cmd *cobra.Command) bool {
	if !config.CFG.UpgradeMessage.GetBool() || os.Getenv("NO_UPDATE_NOTIFIER") != "" {
		return false
	}
	if !input.StderrIsTerminal() || input.InCI() {
		return false
	}
	if f := cmd.Flags().Lookup("output"); f != nil && f.Value.String() == string(cliout.FormatJSON) {
		return false
	}
	return true
}

// PrintUpdateNotice prints the upgrade notice on stderr, after the command's
// own output, when the lookup the pre-run started has found a newer release.
// It never waits and never changes the exit status.
func PrintUpdateNotice() {
	check := pendingUpdateCheck
	pendingUpdateCheck = nil
	if input.StderrIsTerminal() {
		check.Print(os.Stderr, time.Now())
	}
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
		// ASTRO_DOMAIN names the host every API call of the run goes to (the
		// current domain, a token run's host), so one that is not a host
		// alone stops the run before anything is sent: carrying a user, a
		// port or a path, it would move the requests, and the token on them,
		// to another host.
		if d := os.Getenv("ASTRO_DOMAIN"); d != "" {
			if err := manifest.CheckDomain(d); err != nil {
				return fmt.Errorf("ASTRO_DOMAIN: %w", err)
			}
		}
		// Look for a newer release beside the command, never in front of it:
		// the answer is printed after the command's output (PrintUpdateNotice)
		// if it has come by then, and skipped if not.
		if updateNoticeWanted(cmd) {
			// An aggressive timeout, since an answer is not mandatory.
			httpClient := &http.Client{Timeout: 3 * time.Second}
			pendingUpdateCheck = version.StartUpdateCheck(cmd.Context(), httpClient, filepath.Join(config.HomeConfigPath, releaseCheckFile), time.Now())
		}
		var err error
		if context.IsCloudContext() {
			err = astroSetup(cmd, astroV1Client)
		}
		apcCmd.PrintDebugLogs()
		return err
	}
}
