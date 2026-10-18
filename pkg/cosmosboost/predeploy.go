package cosmosboost

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/cosmosboost/precompute"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/logger"
	"github.com/astronomer/astro-cli/pkg/util"
	"github.com/astronomer/astro-cli/version"
)

// slimManifestEnvVar switches the slim manifest without disabling the step.
// deploy.cosmos_boost is the master switch; optimizations under it are on by
// default and switched per-feature by env var. The name matches the plugin's
// convention for its own optimizations.
const slimManifestEnvVar = "ASTRO_COSMOS_BOOST_SLIM_MANIFEST_ENABLED"

// slimManifestEnabled reports whether to produce the slim manifest: on when
// unset, otherwise only for a value util.CheckEnvBool reads as true. The plugin
// keeps an unrecognized value on; here it turns the optimization off, because
// reusing this repo's only bool-env helper beats hand-rolling a second parser.
func slimManifestEnabled() bool {
	value := os.Getenv(slimManifestEnvVar)
	return value == "" || util.CheckEnvBool(value)
}

// BundleOverlay is what a deploy bundle of root changes for Cosmos Boost, as
// an overlay on the tarball: root itself is never written.
//
// Whatever the setting says, the tarball's own walk (Skip) leaves out every
// .astro/dbt_metadata.json and .astro/manifest.slim.json that an earlier
// Astro CLI wrote into the tree as a regular file (its own producer marker
// says so, airflowrt.CosmosBoostFileWrittenByAstro), so the files left out
// are exactly ones the tarball would have carried. The plugin cannot tell a
// fresh sidecar from a stale one, and this CLI never writes there. Anything
// else at such a path - another producer's file, a link - is the user's and
// ships untouched.
//
// With the setting on (deploy.cosmos_boost), the pre-deploy step runs
// over root and its sidecars are added at the same paths, with the same
// contents, the plugin has always read, in place of whatever is there: every
// dbt project (dbt_project.yml) gets a .astro/dbt_metadata.json carrying its
// content hash, which the plugin uses as a cache version key at parse time
// instead of hashing the project tree itself, and every standalone dbt
// manifest.json gets a hash sidecar too, plus a slim, field-filtered copy for
// the plugin to load in place of the full manifest. That step is best-effort
// - a failure is a warning, never an error - because without a sidecar the
// plugin simply falls back to hashing at parse time. The one thing it refuses
// is to replace another producer's file: the plugin reads that path, and this
// CLI will not judge a file it did not write stale.
func BundleOverlay(root string, enabled bool) (fileutil.TarOverlay, error) {
	extra := map[string][]byte{}
	overlay := fileutil.TarOverlay{Extra: extra, Skip: func(rel, file string, info os.FileInfo) (bool, error) {
		if _, replaced := extra[rel]; replaced {
			return true, nil
		}
		return oldSidecar(rel, file, info)
	}}
	if !enabled {
		return overlay, nil
	}
	if _, err := os.Stat(root); err != nil {
		return overlay, nil //nolint:nilerr // the tarball reports a root it cannot read, as it always has
	}

	artifacts, stepErr := preDeploy(root)
	if err := refuseToReplaceForeign(artifacts); err != nil {
		return overlay, err
	}
	for _, a := range artifacts {
		rel, err := filepath.Rel(root, a.Path)
		if err != nil {
			stepErr = errors.Join(stepErr, err)
			continue
		}
		extra[filepath.ToSlash(rel)] = a.Data
	}
	if stepErr != nil {
		fmt.Printf("Warning: the Cosmos Boost pre-deploy step failed, continuing deploy: %s\n", stepErr)
		return overlay, nil
	}
	fmt.Println("Cosmos Boost pre-deploy step complete")
	return overlay, nil
}

// oldSidecar reports whether the entry at rel is a Cosmos Boost file an
// earlier Astro CLI wrote into the tree: a regular file at a sidecar's path
// whose own producer marker names this CLI. Only such a file is read.
func oldSidecar(rel, file string, info os.FileInfo) (bool, error) {
	if !info.Mode().IsRegular() || !airflowrt.IsCosmosBoostFile(rel) {
		return false, nil
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return false, err
	}
	if airflowrt.CosmosBoostFileWrittenByAstro(rel, data) {
		logger.Debugf("leaving out a Cosmos Boost file an earlier Astro CLI wrote into the project: %s", file)
		return true, nil
	}
	return false, nil
}

// refuseToReplaceForeign fails when a regular file another producer wrote
// sits where one of artifacts goes. Only those paths are read.
func refuseToReplaceForeign(artifacts []precompute.Artifact) error {
	var foreign []string
	for _, a := range artifacts {
		info, err := os.Lstat(a.Path)
		if err != nil || !info.Mode().IsRegular() {
			continue // absent, or a link or directory the artifact replaces
		}
		data, err := os.ReadFile(a.Path)
		if err != nil {
			return err
		}
		if !airflowrt.CosmosBoostFileWrittenByAstro(filepath.ToSlash(a.Path), data) {
			foreign = append(foreign, a.Path)
		}
	}
	if len(foreign) > 0 {
		return fmt.Errorf("found Cosmos Boost file(s) from an unrecognized producer: %s; the deploy cannot prove they are fresh and will not ship them for the Cosmos Boost plugin to read. Remove them and retry", strings.Join(foreign, ", "))
	}
	return nil
}

// preDeploy runs the Cosmos Boost pre-deploy step over path and returns the
// artifacts it produced, in memory. It fails when any unit failed, still
// returning the artifacts of the ones that did not.
func preDeploy(path string) ([]precompute.Artifact, error) {
	opts := precompute.Options{SlimManifest: slimManifestEnabled()}
	summary, err := precompute.Run([]string{path}, version.Current(), opts)
	if err != nil {
		return nil, fmt.Errorf("running the Cosmos Boost pre-deploy step: %w", err)
	}
	debugReport("pre-deploy", summary)
	if failed := summary.CountFailed(); failed > 0 {
		return summary.Artifacts(), fmt.Errorf("the Cosmos Boost pre-deploy step failed for %d unit(s) (re-run with --verbosity debug for details)", failed)
	}
	return summary.Artifacts(), nil
}

// reporter is the shape of the precompute summary.
type reporter interface{ WriteReport(io.Writer) }

// debugReport keeps the step's detailed report behind --verbosity debug; the
// deploy output stays to one line either way.
func debugReport(step string, r reporter) {
	var buf bytes.Buffer
	r.WriteReport(&buf)
	logger.Debugf("cosmos boost %s report:\n%s", step, strings.TrimSpace(buf.String()))
}
