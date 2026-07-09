package cosmosboost

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/pkg/logger"
)

const (
	sidecarDirName  = ".astro"
	sidecarFileName = "dbt_metadata.json"
)

// PreDeploy runs `astro-cosmos-boost pre-deploy <path>`, which discovers
// every dbt project (dbt_project.yml) and dbt manifest under path and writes
// a .astro/dbt_metadata.json sidecar beside each, carrying the content hash
// the parse-time consumer uses as its cache version key.
func PreDeploy(path string) error {
	out, err := exec.Command(BinaryPath(), "pre-deploy", path).CombinedOutput()
	logger.Debugf("astro-cosmos-boost pre-deploy output:\n%s", out)
	if err != nil {
		return fmt.Errorf("running astro-cosmos-boost pre-deploy: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// RemoveSidecars deletes every .astro/dbt_metadata.json under root and prunes
// the containing .astro directory when that leaves it empty. Only the sidecar
// file is ever removed — anything else under .astro (e.g. an Astro project's
// config.yaml) is never touched.
func RemoveSidecars(root string) (int, error) {
	removed := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || d.Name() != sidecarFileName || filepath.Base(filepath.Dir(path)) != sidecarDirName {
			return nil
		}
		if err := os.Remove(path); err != nil {
			return err
		}
		removed++
		_ = os.Remove(filepath.Dir(path)) // rmdir; succeeds only when empty
		return nil
	})
	return removed, err
}

// BestEffortCleanup removes any existing sidecars under path so a deploy with
// the feature disabled — or a failed stamping run — never ships a stale hash.
// The consumer treats the hash as an opaque cache key and cannot validate its
// freshness, so removal here is the safety mechanism; an absent sidecar just
// means the consumer falls back to hashing at parse time.
func BestEffortCleanup(path string) {
	removed, err := RemoveSidecars(path)
	if err != nil {
		fmt.Printf("Warning: could not remove dbt metadata sidecars: %s\n", err)
		return
	}
	if removed > 0 {
		fmt.Printf("Removed %d stale dbt metadata sidecar(s)\n", removed)
	}
}

// hasDbtContent reports whether anything under root looks like dbt content
// (a dbt_project.yml or a manifest.json). It is a cheap pre-check so a deploy
// with nothing to stamp never triggers a helper download; false positives
// (e.g. a web app's manifest.json) merely cost one helper run — the helper
// validates real dbt manifests itself.
func hasDbtContent(root string) bool {
	found := false
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // unreadable entries are simply skipped
		}
		if n := d.Name(); n == "dbt_project.yml" || n == "manifest.json" {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found
}

// BestEffortStamp stamps .astro/dbt_metadata.json sidecars under path ahead
// of a deploy. Failures are reported as warnings and never returned: when
// the sidecar is absent the consumer falls back to hashing the project at
// parse time, so an unavailable or failing helper must not block a deploy.
func BestEffortStamp(path string) {
	// Clear previous stamps first: if the helper fails below, a prior
	// deploy's sidecar must not ship with a hash that no longer matches
	// the (possibly edited) project content.
	BestEffortCleanup(path)

	// Nothing dbt-shaped to stamp → don't pull the helper onto this machine.
	if !hasDbtContent(path) {
		logger.Debugf("no dbt projects or manifests under %s; skipping pre-deploy stamping", path)
		return
	}

	if err := EnsureBinary(); err != nil {
		fmt.Printf("Warning: skipping the dbt pre-deploy stamping step: %s\n", err)
		return
	}
	if err := PreDeploy(path); err != nil {
		fmt.Printf("Warning: dbt pre-deploy stamping failed, continuing deploy: %s\n", err)
		return
	}
	fmt.Println("Pre-deploy: stamped dbt project metadata (.astro/dbt_metadata.json)")
}
