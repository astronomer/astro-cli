package cosmosboost

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/pkg/cosmosboost/precompute"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/logger"
	"github.com/astronomer/astro-cli/pkg/util"
	"github.com/astronomer/astro-cli/version"
)

// slimManifestEnvVar switches the slim manifest without disabling the step.
// cosmos_boost.pre_deploy is the master switch; optimizations under it are on by
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

// BundleOverlay is what a deploy bundle of root must change for Cosmos Boost,
// as an overlay on the tarball: root itself is never written.
//
// Whatever enabled says, every .astro/dbt_metadata.json and
// .astro/manifest.slim.json under root that an earlier astro-cli wrote into
// the project (its producer marker names this tool) is left out. The plugin
// cannot tell a fresh sidecar from a stale one, and this one never writes
// there, so nothing a deploy ships comes from the tree.
//
// With enabled set (cosmos_boost.pre_deploy), the pre-deploy step then runs
// over root and its sidecars are added to the bundle at the same paths, with
// the same contents, the plugin has always read: every dbt project
// (dbt_project.yml) gets a .astro/dbt_metadata.json carrying its content hash,
// which the plugin uses as a cache version key at parse time instead of
// hashing the project tree itself, and every standalone dbt manifest.json gets
// a hash sidecar too, plus a slim, field-filtered copy for the plugin to load
// in place of the full manifest. That step is best-effort - a failure is a
// warning, never an error - because without a sidecar the plugin simply falls
// back to hashing at parse time.
//
// A file from an unrecognized producer is the user's, and ships untouched
// when the step is off. With it on, the deploy fails instead: the plugin would
// consume that file's version.hash all the same, and this tool will not judge
// a file it did not write fresh.
func BundleOverlay(root string, enabled bool) (fileutil.TarOverlay, error) {
	overlay := fileutil.TarOverlay{Skip: map[string]bool{}, Extra: map[string][]byte{}}

	existing, err := precompute.FindExisting(root)
	if err != nil {
		return overlay, err
	}
	var foreign []string
	for _, e := range existing {
		if !e.Ours {
			foreign = append(foreign, e.Path)
			continue
		}
		rel, err := relSlash(root, e.Path)
		if err != nil {
			return overlay, err
		}
		logger.Debugf("leaving out a Cosmos Boost file an earlier Astro CLI wrote into the project: %s", e.Path)
		overlay.Skip[rel] = true
	}
	if !enabled {
		return overlay, nil
	}
	if len(foreign) > 0 {
		return overlay, fmt.Errorf("found Cosmos Boost file(s) from an unrecognized producer: %s; the deploy cannot prove they are fresh and will not ship them for the Cosmos Boost plugin to read. Remove them and retry", strings.Join(foreign, ", "))
	}

	artifacts, err := preDeploy(root)
	for _, a := range artifacts {
		rel, relErr := relSlash(root, a.Path)
		if relErr != nil {
			return overlay, relErr
		}
		overlay.Extra[rel] = a.Data
	}
	if err != nil {
		fmt.Printf("Warning: the Cosmos Boost pre-deploy step failed, continuing deploy: %s\n", err)
		return overlay, nil
	}
	fmt.Println("Cosmos Boost pre-deploy step complete")
	return overlay, nil
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

// relSlash is path relative to root, slash-separated, as TarOverlay keys it.
func relSlash(root, path string) (string, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
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
