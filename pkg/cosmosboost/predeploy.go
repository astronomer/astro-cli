package cosmosboost

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/astronomer/astro-cli/pkg/cosmosboost/precompute"
	"github.com/astronomer/astro-cli/pkg/logger"
	"github.com/astronomer/astro-cli/pkg/util"
	"github.com/astronomer/astro-cli/version"
)

// slimManifestDisabledEnvVar opts out of the slim manifest without disabling
// the step. cosmos_boost.pre_deploy is the master switch; optimizations under
// it are on by default and disabled per-feature by env var, as the plugin does.
// Named for the disable so CheckEnvBool's "only an explicit truthy value
// counts" leaves the optimization on for an unset or misspelled value.
const slimManifestDisabledEnvVar = "ASTRO_COSMOS_BOOST_SLIM_MANIFEST_DISABLED"

func slimManifestEnabled() bool {
	return !util.CheckEnvBool(os.Getenv(slimManifestDisabledEnvVar))
}

// PreDeploy runs the Cosmos Boost pre-deploy step over path: every dbt project
// (dbt_project.yml) gets a .astro/dbt_metadata.json sidecar carrying its
// content hash, which the Cosmos Boost plugin uses as a cache version key at
// parse time instead of hashing the project tree itself. Every standalone dbt
// manifest.json gets a hash sidecar too, plus a slim, field-filtered copy for
// the plugin to load in place of the full manifest at DAG-parse time.
func PreDeploy(path string) error {
	opts := precompute.Options{SlimManifest: slimManifestEnabled()}
	summary, err := precompute.Run([]string{path}, version.CurrVersion, opts)
	if err != nil {
		return fmt.Errorf("running the Cosmos Boost pre-deploy step: %w", err)
	}
	debugReport("pre-deploy", summary)
	if failed := summary.CountFailed(); failed > 0 {
		return fmt.Errorf("the Cosmos Boost pre-deploy step failed for %d unit(s) (re-run with --verbosity debug for details)", failed)
	}
	return nil
}

// cleanupRoots deletes the sidecars earlier pre-deploy runs wrote under
// roots. Files this integration did not write are left in place; the summary
// is returned so callers with stricter needs can inspect what was kept.
func cleanupRoots(roots []string) (precompute.CleanupSummary, error) {
	summary, err := precompute.Cleanup(roots)
	if err != nil {
		return summary, fmt.Errorf("removing the Cosmos Boost artifacts: %w", err)
	}
	debugReport("cleanup", summary)
	if failed := summary.CountFailed(); failed > 0 {
		return summary, fmt.Errorf("could not remove %d Cosmos Boost artifact(s) (re-run with --verbosity debug for details)", failed)
	}
	return summary, nil
}

// removeArtifacts is cleanupRoots for callers that only need the error.
func removeArtifacts(roots []string) error {
	_, err := cleanupRoots(roots)
	return err
}

// EnsureClean removes the Cosmos Boost artifacts earlier deploys left under
// path, and fails when their absence cannot be guaranteed. Consumers cannot
// tell fresh output from stale, so a deploy must not proceed while a stale
// artifact may still be in the payload.
//
// That includes sidecars from an unrecognized producer: cleanup deliberately
// never deletes a file it does not own, but the plugin would consume its
// version.hash all the same if the deploy shipped it - so an enabled deploy
// stops and asks for manual removal instead. `astro dbt cleanup` keeps
// preserving such files.
func EnsureClean(path string) error {
	summary, err := cleanupRoots([]string{path})
	if err != nil {
		return fmt.Errorf("%w; remove the reported files (or run 'astro dbt cleanup %s') and retry", err, path)
	}
	var kept []string
	for _, r := range summary.Results {
		if r.Kept {
			kept = append(kept, r.Path)
		}
	}
	if len(kept) > 0 {
		return fmt.Errorf("found Cosmos Boost artifact(s) from an unrecognized producer: %s; the deploy cannot prove they are fresh, and cleanup will not delete files it does not own - remove them manually and retry", strings.Join(kept, ", "))
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
