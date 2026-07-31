package cosmosboost

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/astronomer/astro-cli/pkg/logger"
)

// PreDeploy runs the plugin's pre-deploy step over path.
func PreDeploy(path string) error {
	//nolint:gosec // BinaryPath() is a fixed path under the CLI's own bin dir; path is the deploy target
	out, err := exec.Command(BinaryPath(), "pre-deploy", path).CombinedOutput()
	// The helper's own report stays behind --verbosity debug and is deliberately
	// kept out of the returned error, which callers print as a warning.
	logger.Debugf("astro-cosmos-boost pre-deploy output:\n%s", strings.TrimSpace(string(out)))
	if err != nil {
		return fmt.Errorf("running astro-cosmos-boost pre-deploy: %w (re-run with --verbosity debug for details)", err)
	}
	return nil
}

// BestEffortCleanup asks the helper to remove what it left under path, so that
// neither a deploy with the feature disabled nor a failed pre-deploy step ships
// artifacts produced by an earlier deploy.
//
// It never downloads the helper: a machine with no helper installed has nothing
// for us to clean up, and pulling one down for a disabled feature would be
// wrong. Failures are warnings, because cleanup must not block a deploy.
func BestEffortCleanup(path string) {
	if _, err := os.Stat(BinaryPath()); err != nil {
		logger.Debugf("astro-cosmos-boost is not installed; nothing to clean up under %s", path)
		return
	}
	if err := execUninstall([]string{path}); err != nil {
		fmt.Printf("Warning: could not remove the Cosmos Boost artifacts: %s\n", err)
	}
}

// BestEffortPreDeploy runs the plugin's pre-deploy step over path ahead of a
// deploy. Failures are reported as warnings and never returned: an unavailable
// or failing helper must not block a deploy. What there is to do under path,
// and what the step produces, is the helper's business.
func BestEffortPreDeploy(path string) {
	if err := EnsureBinary(); err != nil {
		fmt.Printf("Warning: skipping the Cosmos Boost pre-deploy step: %s\n", err)
		return
	}
	// Clean up before running, so that a failure below cannot leave this deploy
	// carrying an earlier one's artifacts. The helper is installed by the line
	// above, so this delegates rather than doing nothing.
	if err := withUpdateRetry(func() error { return execUninstall([]string{path}) }); err != nil {
		fmt.Printf("Warning: could not remove the Cosmos Boost artifacts: %s\n", err)
	}
	if err := withUpdateRetry(func() error { return PreDeploy(path) }); err != nil {
		fmt.Printf("Warning: the Cosmos Boost pre-deploy step failed, continuing deploy: %s\n", err)
		return
	}
	fmt.Println("Cosmos Boost pre-deploy step complete")
}
