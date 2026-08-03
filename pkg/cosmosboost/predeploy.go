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

// EnsureClean removes the Cosmos Boost artifacts earlier deploys left under
// path, and fails when their absence cannot be guaranteed. Consumers cannot
// tell fresh output from stale, so a deploy must not proceed while a stale
// artifact may still be in the payload.
func EnsureClean(path string) error {
	if err := removeArtifacts([]string{path}); err != nil {
		return fmt.Errorf("%w; remove the reported files (or run 'astro dbt cleanup %s') and retry", err, path)
	}
	return nil
}

// BestEffortPreDeploy runs the Cosmos Boost pre-deploy step over path ahead of
// a deploy. Stamping is best-effort — failures are warnings, never errors —
// because without a sidecar the plugin simply falls back to hashing at parse
// time. Callers must run EnsureClean first: writing nothing is safe, leaving
// something stale is not.
func BestEffortPreDeploy(path string) {
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
