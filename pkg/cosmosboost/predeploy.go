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

// BundleOverlay is what a deploy bundle of root changes for Cosmos Boost, as
// an overlay on the tarball: root itself is never written.
//
// Whatever the setting says, the tarball's own walk (Skip) leaves out every
// .astro/dbt_metadata.json and .astro/manifest.slim.json that an earlier
// Astro CLI wrote into the tree (its own producer marker says so,
// airflowrt.CosmosBoostFileWrittenByAstro), so the files left out are exactly
// ones the tarball would have carried; and anything at such a path that is
// not a regular file, with a warning. The plugin cannot tell a fresh sidecar
// from a stale one, and this CLI never writes there. One from another
// producer is the user's file and ships untouched.
//
// With the setting on (cosmos_boost.pre_deploy), the pre-deploy step runs
// over root and its sidecars are added at the same paths, with the same
// contents, the plugin has always read: every dbt project (dbt_project.yml)
// gets a .astro/dbt_metadata.json carrying its content hash, which the plugin
// uses as a cache version key at parse time instead of hashing the project
// tree itself, and every standalone dbt manifest.json gets a hash sidecar too,
// plus a slim, field-filtered copy for the plugin to load in place of the full
// manifest. That step is best-effort - a failure is a warning, never an error
// - because without a sidecar the plugin simply falls back to hashing at parse
// time. Before it hashes anything, it refuses the deploy when another
// producer's file sits where it would put a sidecar: the plugin reads that
// directory, and this CLI will not judge a file it did not write fresh.
//
// A root that cannot be read gets no sidecars, and is left for the tarball to
// deal with as it always has.
func BundleOverlay(root string, enabled bool) (fileutil.TarOverlay, error) {
	overlay := fileutil.TarOverlay{Skip: skipOldSidecar, Extra: map[string][]byte{}}
	if !enabled {
		return overlay, nil
	}
	if _, err := os.Stat(root); err != nil {
		return overlay, nil //nolint:nilerr // the tarball reports a root it cannot read, as it always has
	}
	units, err := precompute.Discover([]string{root})
	if err != nil {
		fmt.Printf("Warning: the Cosmos Boost pre-deploy step failed, continuing deploy: %s\n", err)
		return overlay, nil
	}
	if err := refuseForeignSidecars(units.SidecarDirs()); err != nil {
		return overlay, err
	}

	artifacts, err := preDeploy(units)
	for _, a := range artifacts {
		rel, relErr := filepath.Rel(root, a.Path)
		if relErr != nil {
			err = errors.Join(err, relErr)
			continue
		}
		overlay.Extra[filepath.ToSlash(rel)] = a.Data
	}
	if err != nil {
		fmt.Printf("Warning: the Cosmos Boost pre-deploy step failed, continuing deploy: %s\n", err)
		return overlay, nil
	}
	fmt.Println("Cosmos Boost pre-deploy step complete")
	return overlay, nil
}

// skipOldSidecar is the overlay's Skip: it leaves out each Cosmos Boost file
// an Astro CLI wrote into the tree, and anything at a sidecar's path that is
// not a regular file. Only a regular file at a sidecar's path is read.
func skipOldSidecar(rel, file string, info os.FileInfo) (bool, error) {
	if !airflowrt.IsCosmosBoostFile(rel) {
		return false, nil
	}
	if !info.Mode().IsRegular() {
		fmt.Printf("Warning: leaving %s out of the bundle: Cosmos Boost keeps a sidecar file at that path, and this is not a regular file\n", file)
		return true, nil
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

// refuseForeignSidecars fails when a regular file another producer wrote sits
// where the pre-deploy step puts a sidecar, in one of dirs. Only those few
// paths are read.
func refuseForeignSidecars(dirs []string) error {
	var foreign []string
	for _, dir := range dirs {
		for _, name := range []string{airflowrt.CosmosBoostSidecar, airflowrt.CosmosBoostSlimManifest} {
			file := filepath.Join(dir, name)
			info, err := os.Lstat(file)
			if err != nil || !info.Mode().IsRegular() {
				continue // absent, or left out by skipOldSidecar
			}
			data, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			if !airflowrt.CosmosBoostFileWrittenByAstro(name, data) {
				foreign = append(foreign, file)
			}
		}
	}
	if len(foreign) > 0 {
		return fmt.Errorf("found Cosmos Boost file(s) from an unrecognized producer: %s; the deploy cannot prove they are fresh and will not ship them for the Cosmos Boost plugin to read. Remove them and retry", strings.Join(foreign, ", "))
	}
	return nil
}

// preDeploy runs the Cosmos Boost pre-deploy step over units and returns the
// artifacts it produced, in memory. It fails when any unit failed, still
// returning the artifacts of the ones that did not.
func preDeploy(units precompute.Units) ([]precompute.Artifact, error) {
	opts := precompute.Options{SlimManifest: slimManifestEnabled()}
	summary := units.Run(version.Current(), opts)
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
