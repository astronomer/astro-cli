package precompute

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// cleanupSkipDirs are not traversed below the walked root. Discovery never
// writes sidecars under them (both discovery walks skip these names at any
// depth), and generated directories like a root-owned logs/ left by a
// container bind mount may not even be readable - aborting a deploy over a
// directory we never write to helps nobody. The root itself is exempt,
// mirroring discovery: a project that happens to live in a directory named
// logs or dbt_packages can be stamped, so it must be cleanable too. .git is
// skipped unconditionally - VCS internals are never ours to mutate. target/
// is deliberately absent: a compiled manifest's sidecar lives at
// target/.astro/.
var cleanupSkipDirs = map[string]bool{
	"logs":             true,
	defaultPackagesDir: true,
}

// CleanupResult records what happened to one sidecar found during Cleanup.
type CleanupResult struct {
	Path string // sidecar file path
	Kept bool   // left in place: the file was not written by this tool
	Err  error  // non-nil if removal failed
}

// CleanupSummary is the structured outcome of an cleanup run.
type CleanupSummary struct {
	Duration time.Duration
	Results  []CleanupResult
}

// Cleanup removes every .astro/dbt_metadata.json sidecar under the given
// roots that this tool wrote, along with any slim manifest (manifest.slim.json)
// written next to it, pruning each containing .astro directory when removal
// leaves it empty. A dbt_metadata.json whose generated_by.application is not
// ours — or that isn't valid JSON — is left in place and reported as kept, so
// cleanup never deletes a file some other tool owns.
//
// A manifest.slim.json found WITHOUT a sidecar beside it — left behind by an
// older astro-cli that deleted only the sidecar, or by EnsureClean leaving a
// foreign sidecar in place — is not otherwise reachable by anything walking
// for dbt_metadata.json, so it is checked and removed on its own terms
// instead, via its own _generated_by marker (see removeOrphanSlimManifest).
// When a sidecar IS present, that sidecar's own removal owns the slim file
// next to it (removeSidecar), so it isn't visited twice here.
//
// Per-file removal failures are recorded in their Result and do not stop the
// others; like Run, a non-nil error is returned only for a top-level problem
// such as a root that cannot be walked.
func Cleanup(roots []string) (CleanupSummary, error) {
	start := time.Now()

	seen := map[string]bool{}
	var results []CleanupResult
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == gitDir || (path != root && cleanupSkipDirs[d.Name()]) {
					return filepath.SkipDir
				}
				return nil
			}
			if filepath.Base(filepath.Dir(path)) != sidecarDir {
				return nil
			}
			switch d.Name() {
			case sidecarName:
				recordOnce(seen, &results, path, func(p string) (CleanupResult, bool) {
					return removeSidecar(p), true
				})
			case slimManifestName:
				// The sidecar, if present, owns removing this file (see
				// removeSidecar); only handle it here when there is none.
				sidecarPath := filepath.Join(filepath.Dir(path), sidecarName)
				if _, statErr := os.Lstat(sidecarPath); statErr == nil {
					return nil
				}
				recordOnce(seen, &results, path, removeOrphanSlimManifest)
			}
			return nil
		})
		if err != nil {
			return CleanupSummary{}, fmt.Errorf("scanning %q for sidecars: %w", root, err)
		}
	}

	return CleanupSummary{Duration: time.Since(start), Results: results}, nil
}

// recordOnce runs remove(path) and appends its result, deduplicated by
// canonical path so overlapping roots naming the same tree differently — a
// relative name and an absolute one, or a symlink and its target — report
// each file once rather than once per spelling. remove's ok return lets a
// caller signal "nothing to report" (see removeOrphanSlimManifest) without
// that turning into a phantom entry in results.
func recordOnce(seen map[string]bool, results *[]CleanupResult, path string, remove func(string) (CleanupResult, bool)) {
	key := canonicalPath(path)
	if seen[key] { // overlapping roots: report each file once
		return
	}
	seen[key] = true
	if result, ok := remove(path); ok {
		*results = append(*results, result)
	}
}

// canonicalPath returns a path suitable for identifying one file across roots
// that spell it differently: absolute first, then symlinks resolved. Both steps
// are needed. Abs alone leaves a symlinked directory and its target looking like
// different files, and EvalSymlinks alone preserves relativity, so a relative
// root and an absolute one would still not compare equal. Each step is skipped
// if it fails, leaving Clean as the floor.
func canonicalPath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return filepath.Clean(path)
}

// removeSidecar deletes one sidecar after checking this tool wrote it, along
// with any slim manifest sitting next to it, then prunes the containing
// .astro directory if removal left it empty. Only files this tool could have
// written are ever removed — anything else under .astro (e.g. an Astro
// project's config.yaml) is never touched.
func removeSidecar(path string) CleanupResult {
	data, err := os.ReadFile(path)
	if err != nil {
		return CleanupResult{Path: path, Err: err}
	}
	var meta Metadata
	if json.Unmarshal(data, &meta) != nil || meta.GeneratedBy.Application != application {
		return CleanupResult{Path: path, Kept: true}
	}
	// Removed before the sidecar: on any failure but ENOENT, return here so
	// the sidecar survives and Cleanup can still find (and retry) it later.
	if slimErr := os.Remove(filepath.Join(filepath.Dir(path), slimManifestName)); slimErr != nil && !os.IsNotExist(slimErr) {
		return CleanupResult{Path: path, Err: slimErr}
	}
	if err := os.Remove(path); err != nil {
		return CleanupResult{Path: path, Err: err}
	}
	_ = os.Remove(filepath.Dir(path)) // rmdir; succeeds only when empty
	return CleanupResult{Path: path}
}

// removeOrphanSlimManifest removes a manifest.slim.json found with no sidecar
// beside it (see Cleanup), after checking its own _generated_by marker names
// this tool — the same provenance check removeSidecar applies to a sidecar,
// since there is no sidecar here to anchor the check to. ok is false when the
// file is already gone by the time this runs (e.g. its sidecar was found and
// removed, taking this file with it, after WalkDir had already queued this
// entry) — in that case there is nothing to report.
func removeOrphanSlimManifest(path string) (result CleanupResult, ok bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return CleanupResult{}, false
		}
		return CleanupResult{Path: path, Err: err}, true
	}
	var marker struct {
		GeneratedBy GeneratedBy `json:"_generated_by"`
	}
	if json.Unmarshal(data, &marker) != nil || marker.GeneratedBy.Application != application {
		return CleanupResult{Path: path, Kept: true}, true
	}
	if err := os.Remove(path); err != nil {
		return CleanupResult{Path: path, Err: err}, true
	}
	_ = os.Remove(filepath.Dir(path)) // rmdir; succeeds only when empty
	return CleanupResult{Path: path}, true
}

// CountKept returns the number of sidecars left in place because this tool
// did not write them.
func (s CleanupSummary) CountKept() int {
	n := 0
	for _, r := range s.Results {
		if r.Kept {
			n++
		}
	}
	return n
}

// CountFailed returns the number of sidecars that could not be removed.
func (s CleanupSummary) CountFailed() int {
	n := 0
	for _, r := range s.Results {
		if r.Err != nil {
			n++
		}
	}
	return n
}

// WriteReport prints a short, human-readable report of the run. It follows the
// same shape as Summary.WriteReport (precompute.go): a counts line, then one
// line per entry using the shared glyphs. The columns differ because the entries
// do — a cleanup has nothing to say about hashes or bytes.
func (s CleanupSummary) WriteReport(w io.Writer) {
	removed := len(s.Results) - s.CountKept() - s.CountFailed()
	fmt.Fprintf(w, "cosmos boost cleanup: %d removed, %d kept, %d failed in %s\n",
		removed, s.CountKept(), s.CountFailed(), s.Duration.Round(time.Microsecond))
	for _, r := range s.Results {
		switch {
		case r.Err != nil:
			fmt.Fprintf(w, "  %s %s  (%v)\n", glyphFail, r.Path, r.Err)
		case r.Kept:
			fmt.Fprintf(w, "  %s %s  (unrecognized producer; left in place)\n", glyphLeft, r.Path)
		default:
			fmt.Fprintf(w, "  %s %s\n", glyphDone, r.Path)
		}
	}
}
