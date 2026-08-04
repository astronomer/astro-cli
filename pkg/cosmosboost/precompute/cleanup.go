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

// cleanupSkipDirs are never traversed. Discovery never writes sidecars under
// them (both discovery walks skip these names at any depth), and generated
// directories like a root-owned logs/ left by a container bind mount may not
// even be readable - aborting a deploy over a directory we never write to
// helps nobody. .git additionally must never be mutated. target/ is
// deliberately NOT here: a compiled manifest's sidecar lives at
// target/.astro/.
var cleanupSkipDirs = map[string]bool{
	gitDir:             true,
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
// roots that this tool wrote, pruning each containing .astro directory when
// removal leaves it empty. A dbt_metadata.json whose generated_by.application
// is not ours — or that isn't valid JSON — is left in place and reported as
// kept, so cleanup never deletes a file some other tool owns.
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
				if cleanupSkipDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if d.Name() != sidecarName || filepath.Base(filepath.Dir(path)) != sidecarDir {
				return nil
			}
			// Key on the canonical path, not the walked one. Roots that name the
			// same tree differently — a relative name and an absolute one, or a
			// symlink and its target — yield different strings for one file, and
			// a sidecar left in place (foreign, so not removed) would then be
			// found and reported once per spelling.
			key := canonicalPath(path)
			if seen[key] { // overlapping roots: report each sidecar once
				return nil
			}
			seen[key] = true
			results = append(results, removeSidecar(path))
			return nil
		})
		if err != nil {
			return CleanupSummary{}, fmt.Errorf("scanning %q for sidecars: %w", root, err)
		}
	}

	return CleanupSummary{Duration: time.Since(start), Results: results}, nil
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

// removeSidecar deletes one sidecar after checking this tool wrote it, then
// prunes the containing .astro directory if removal left it empty. Only the
// sidecar file is ever removed — anything else under .astro (e.g. an Astro
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
	if err := os.Remove(path); err != nil {
		return CleanupResult{Path: path, Err: err}
	}
	_ = os.Remove(filepath.Dir(path)) // rmdir; succeeds only when empty
	return CleanupResult{Path: path}
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
