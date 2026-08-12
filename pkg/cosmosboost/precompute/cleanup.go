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

// CleanupResult records what happened to one artifact found during Cleanup.
// Err always describes Path: each artifact is removed on its own terms, so a
// failure is never reported against a neighboring file's name.
type CleanupResult struct {
	Path string // artifact file path
	Kept bool   // left in place: the file was not written by this tool
	Err  error  // non-nil if removal failed
}

// CleanupSummary is the structured outcome of an cleanup run.
type CleanupSummary struct {
	Duration time.Duration
	Results  []CleanupResult
}

// Cleanup removes every .astro/dbt_metadata.json sidecar and every
// .astro/manifest.slim.json under the given roots that this tool wrote,
// pruning each containing .astro directory when removal leaves it empty. Each
// file is judged by its own producer marker: one whose marker is not ours — or
// that isn't valid JSON — is left in place and reported as kept, so cleanup
// never deletes a file some other tool owns.
//
// Checking the two independently is what makes the mixed states come out
// right, and neither is rare. A slim manifest outlives its sidecar whenever an
// older astro-cli's cleanup deleted only the sidecar it knew about, or
// EnsureClean left a foreign one in place; a sidecar we wrote can equally sit
// next to a manifest.slim.json we did not. Inferring either file's provenance
// from the other would delete a file we do not own in one direction and strand
// a stale artifact of ours — unremoved and unreported — in the other.
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
			// WalkDir reads a directory's entries in lexical order, so
			// dbt_metadata.json is handled before manifest.slim.json and the
			// .astro prune that finally succeeds is the slim manifest's.
			switch d.Name() {
			case sidecarName:
				recordOnce(seen, &results, path, removeSidecar)
			case slimManifestName:
				recordOnce(seen, &results, path, removeSlimManifest)
			}
			return nil
		})
		if err != nil {
			return CleanupSummary{}, fmt.Errorf("scanning %q for artifacts: %w", root, err)
		}
	}

	return CleanupSummary{Duration: time.Since(start), Results: results}, nil
}

// recordOnce runs remove(path) and appends its result, deduplicated by
// canonical path so overlapping roots naming the same tree differently — a
// relative name and an absolute one, or a symlink and its target — report
// each file once rather than once per spelling.
func recordOnce(seen map[string]bool, results *[]CleanupResult, path string, remove func(string) CleanupResult) {
	key := canonicalPath(path)
	if seen[key] { // overlapping roots: report each file once
		return
	}
	seen[key] = true
	*results = append(*results, remove(path))
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

// removeArtifact deletes one artifact after producedByUs confirms this tool
// wrote it, then prunes the containing .astro directory if removal left it
// empty. Only files this tool could have written are ever removed — anything
// else under .astro (e.g. an Astro project's config.yaml) is never touched.
// Both artifacts carry a producer marker and differ only in which field holds
// it, which is all producedByUs reads.
func removeArtifact(path string, producedByUs func(data []byte) bool) CleanupResult {
	data, err := os.ReadFile(path)
	if err != nil {
		return CleanupResult{Path: path, Err: err}
	}
	if !producedByUs(data) {
		return CleanupResult{Path: path, Kept: true}
	}
	if err := os.Remove(path); err != nil {
		return CleanupResult{Path: path, Err: err}
	}
	_ = os.Remove(filepath.Dir(path)) // rmdir; succeeds only when empty
	return CleanupResult{Path: path}
}

// removeSidecar removes one .astro/dbt_metadata.json, keeping it unless its
// generated_by names this tool.
func removeSidecar(path string) CleanupResult {
	return removeArtifact(path, func(data []byte) bool {
		var meta Metadata
		return json.Unmarshal(data, &meta) == nil && meta.GeneratedBy.Application == application
	})
}

// removeSlimManifest removes one .astro/manifest.slim.json, keeping it unless
// its own _generated_by marker names this tool. The marker read is the slim
// manifest's own and never the neighboring sidecar's: the plugin consumes a
// slim manifest just as directly as a sidecar, so a file another producer owns
// has to survive here even when the sidecar beside it is ours.
func removeSlimManifest(path string) CleanupResult {
	return removeArtifact(path, func(data []byte) bool {
		var marker struct {
			GeneratedBy GeneratedBy `json:"_generated_by"`
		}
		return json.Unmarshal(data, &marker) == nil && marker.GeneratedBy.Application == application
	})
}

// CountKept returns the number of artifacts left in place because this tool
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

// CountFailed returns the number of artifacts that could not be removed.
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
