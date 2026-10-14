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

// Bundle is what a deploy bundle of a directory changes for Cosmos Boost, as
// an overlay on the tarball (Overlay): the directory itself is never written.
//
// Whatever the setting says, every .astro/dbt_metadata.json and
// .astro/manifest.slim.json that an earlier Astro CLI wrote into the tree (its
// own producer marker says so, airflowrt.CosmosBoostFileWrittenByAstro) is
// left out, judged inside the tarball's own walk so the files left out are
// exactly the ones it would have carried. The plugin cannot tell a fresh
// sidecar from a stale one, and this CLI never writes there.
//
// With the setting on (cosmos_boost.pre_deploy), the pre-deploy step runs over
// the directory and its sidecars are added at the same paths, with the same
// contents, the plugin has always read: every dbt project (dbt_project.yml)
// gets a .astro/dbt_metadata.json carrying its content hash, which the plugin
// uses as a cache version key at parse time instead of hashing the project
// tree itself, and every standalone dbt manifest.json gets a hash sidecar too,
// plus a slim, field-filtered copy for the plugin to load in place of the full
// manifest. That step is best-effort - a failure is a warning, never an error
// - because without a sidecar the plugin simply falls back to hashing at parse
// time.
//
// A file from an unrecognized producer is the user's, and ships untouched
// when the step is off. With it on, Err refuses the deploy: the plugin would
// consume that file's version.hash all the same, and this CLI will not judge
// a file it did not write fresh.
type Bundle struct {
	Overlay fileutil.TarOverlay
	enabled bool
	foreign []string
}

// NewBundle prepares the overlay for a bundle of root, generating the
// sidecars into it when enabled. A root that cannot be read generates none,
// and is left for the tarball to deal with as it always has.
func NewBundle(root string, enabled bool) *Bundle {
	b := &Bundle{enabled: enabled}
	b.Overlay = fileutil.TarOverlay{Skip: b.skip, Extra: map[string][]byte{}}
	if !enabled {
		return b
	}
	if _, err := os.Stat(root); err != nil {
		return b
	}
	artifacts, err := preDeploy(root)
	for _, a := range artifacts {
		rel, relErr := filepath.Rel(root, a.Path)
		if relErr != nil {
			err = errors.Join(err, relErr)
			continue
		}
		b.Overlay.Extra[filepath.ToSlash(rel)] = a.Data
	}
	if err != nil {
		fmt.Printf("Warning: the Cosmos Boost pre-deploy step failed, continuing deploy: %s\n", err)
		return b
	}
	fmt.Println("Cosmos Boost pre-deploy step complete")
	return b
}

// skip is Overlay.Skip: it leaves out each Cosmos Boost file an Astro CLI
// wrote into the tree, and notes every other one for Err. Only files at a
// sidecar's path are read.
func (b *Bundle) skip(rel, file string) (bool, error) {
	if !airflowrt.IsCosmosBoostFile(rel) {
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
	b.foreign = append(b.foreign, file)
	return false, nil
}

// Err, once the tarball is written, refuses an enabled deploy that found a
// Cosmos Boost file from an unrecognized producer, which the plugin would read
// as current. nil otherwise.
func (b *Bundle) Err() error {
	if !b.enabled || len(b.foreign) == 0 {
		return nil
	}
	return fmt.Errorf("found Cosmos Boost file(s) from an unrecognized producer: %s; the deploy cannot prove they are fresh and will not ship them for the Cosmos Boost plugin to read. Remove them and retry", strings.Join(b.foreign, ", "))
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
