package cosmosboost

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/astronomer/astro-cli/pkg/cosmosboost/precompute"
	"github.com/astronomer/astro-cli/pkg/logger"
	"github.com/astronomer/astro-cli/version"
)

// PreDeploy runs the Cosmos Boost pre-deploy step over path: every dbt project
// (dbt_project.yml) and standalone dbt manifest.json under it gets a
// .astro/dbt_metadata.json sidecar carrying its content hash, which the Cosmos
// Boost plugin uses as a cache version key at parse time instead of hashing
// the project tree itself.
func PreDeploy(path string) error {
	summary, err := precompute.Run([]string{path}, version.CurrVersion)
	if err != nil {
		return fmt.Errorf("running the Cosmos Boost pre-deploy step: %w", err)
	}
	debugReport("pre-deploy", summary)
	if failed := summary.CountFailed(); failed > 0 {
		return fmt.Errorf("the Cosmos Boost pre-deploy step failed for %d unit(s) (re-run with --verbosity debug for details)", failed)
	}
	return nil
}

// removeArtifacts deletes the sidecars earlier pre-deploy runs wrote under
// roots. Files this integration did not write are left in place.
func removeArtifacts(roots []string) error {
	summary, err := precompute.Cleanup(roots)
	if err != nil {
		return fmt.Errorf("removing the Cosmos Boost artifacts: %w", err)
	}
	debugReport("cleanup", summary)
	if failed := summary.CountFailed(); failed > 0 {
		return fmt.Errorf("could not remove %d Cosmos Boost artifact(s) (re-run with --verbosity debug for details)", failed)
	}
	return nil
}

// BestEffortCleanup removes the Cosmos Boost artifacts under path, so that
// neither a deploy with the feature disabled nor a failed pre-deploy step
// ships artifacts produced by an earlier deploy. Failures are warnings,
// because cleanup must not block a deploy.
func BestEffortCleanup(path string) {
	if err := removeArtifacts([]string{path}); err != nil {
		fmt.Printf("Warning: could not remove the Cosmos Boost artifacts: %s\n", err)
	}
}

// BestEffortPreDeploy runs the Cosmos Boost pre-deploy step over path ahead of
// a deploy. Failures are reported as warnings and never returned: a failing
// pre-deploy step must not block a deploy. Without a sidecar the plugin simply
// falls back to hashing at parse time.
func BestEffortPreDeploy(path string) {
	// Clear earlier runs' artifacts first, so a failure below cannot leave
	// this deploy carrying a stale hash for since-edited content.
	BestEffortCleanup(path)
	if err := PreDeploy(path); err != nil {
		fmt.Printf("Warning: the Cosmos Boost pre-deploy step failed, continuing deploy: %s\n", err)
		return
	}
	fmt.Println("Cosmos Boost pre-deploy step complete")
}

// reporter is the common shape of the precompute summaries.
type reporter interface{ WriteReport(io.Writer) }

// debugReport keeps the step's detailed report behind --verbosity debug; the
// deploy output stays to one line either way.
func debugReport(step string, r reporter) {
	var buf bytes.Buffer
	r.WriteReport(&buf)
	logger.Debugf("cosmos boost %s report:\n%s", step, strings.TrimSpace(buf.String()))
}
