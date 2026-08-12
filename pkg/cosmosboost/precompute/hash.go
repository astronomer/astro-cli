package precompute

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// excludedDirs are directory names skipped during *project* discovery
// (findProjects in discover.go): generated/installed/secret content can't hold a
// project's dbt_project.yml. This includes target/, which is intentional here —
// manifest discovery (findManifests) uses its own manifestSkipDirs that keeps
// target/, so a compiled target/manifest.json is still found. Project hashing uses
// the more precise, root-relative excludedRelDirsFor instead.
var excludedDirs = map[string]bool{
	"target":           true,
	"logs":             true,
	defaultPackagesDir: true, // "dbt_packages"
	sidecarDir:         true, // ".astro"
	gitDir:             true, // VCS internals can't hold a real project
}

// excludedFiles are non-source files (matched at the project root only — see
// hashProject) that dbt/Cosmos ignore for project identity.
var excludedFiles = map[string]bool{
	"package-lock.yml": true, // dbt package lockfile
	"profiles.yml":     true, // dbt connection profiles (secret, env-specific)
}

// excludedRelDirsFor returns the project-root-relative directories to skip when
// hashing a project, derived from the (already-read) dbt config. Exclusions are
// root-relative (not name-based), so a custom packages-install-path like
// "vendor/packages" excludes exactly that directory and never a same-named source
// directory elsewhere (e.g. models/packages/). Mirrors cosmos/dbt/project.py::
// create_symlinks (whose ignore list is likewise project-root-relative), plus our
// own .astro/ output.
//
// The dbt defaults (target/, logs/, dbt_packages/) are always excluded; a custom
// target-path / log-path / packages-install-path from dbt_project.yml is excluded in
// addition, so generated output under a renamed output directory doesn't churn the
// hash on every compile.
func excludedRelDirsFor(cfg dbtConfig) map[string]bool {
	out := map[string]bool{
		"target":           true,
		"logs":             true,
		defaultPackagesDir: true,
		sidecarDir:         true,
	}
	for _, d := range []string{cfg.packagesInstallPath, cfg.targetPath, cfg.logPath} {
		if d != "" {
			out[d] = true
		}
	}
	return out
}

// gitDir is VCS metadata: never part of the deploy payload (bundle creation
// excludes it too), so it must not affect project identity. In linked worktrees
// and submodules .git is a pointer file rather than a directory, so both forms
// are skipped.
const gitDir = ".git"

// relPath is filepath.Rel behind a variable so tests can exercise the guard
// in hashProject's walk: WalkDir yields paths rooted at the walked directory,
// so Rel cannot otherwise be made to fail.
var relPath = filepath.Rel

// hashProject computes the "sha256-tree-v2" hash of a dbt project directory: the
// sha256 of the path-sorted list of "<relative-path>\x00<sha256(file)>\n" entries.
// cfg carries the dbt_project.yml directory settings that widen the exclusion set.
//
// Because the entries are sorted by relative path, the result depends only on the
// project's content, not on filesystem walk order — so it is stable across runs
// and unaffected by any concurrency in the caller. The value need not match
// Cosmos's own hash: the read-side treats it as an opaque version key.
func hashProject(dir string, cfg dbtConfig) (hash string, files int, totalBytes int64, err error) {
	excludedRel := excludedRelDirsFor(cfg)

	type entry struct{ rel, digest string }
	var entries []entry

	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := relPath(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel) // stable across operating systems

		if d.IsDir() {
			// Our own sidecar directory must never affect the project hash, at ANY
			// depth. A dbt manifest.json in a subdirectory (e.g. docs/manifest.json)
			// gets its own sidecar written into the walked tree (docs/.astro/), and
			// because manifest and project units run concurrently, hashing it would
			// make the project hash depend on that sidecar and vary between runs.
			// .astro is always our output, never dbt source, so skipping it by name
			// everywhere is safe (unlike target/logs, which are matched root-relative
			// to avoid over-excluding a same-named source dir).
			if d.Name() == sidecarDir || d.Name() == gitDir {
				return filepath.SkipDir
			}
			// Skip excluded directories by their root-relative path (never the root).
			if rel != "." && excludedRel[rel] {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == gitDir { // a worktree's or submodule's .git pointer file
			return nil
		}
		if excludedFiles[rel] { // root-level files only (rel has no slash)
			return nil
		}

		digest, n, err := hashFile(path)
		if err != nil {
			return err
		}
		entries = append(entries, entry{rel, digest})
		files++
		totalBytes += n
		return nil
	})
	if walkErr != nil {
		return "", 0, 0, walkErr
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].rel < entries[j].rel })

	h := sha256.New()
	for _, e := range entries {
		// NUL separates path from digest so the encoding is unambiguous.
		fmt.Fprintf(h, "%s\x00%s\n", e.rel, e.digest)
	}
	return hex.EncodeToString(h.Sum(nil)), files, totalBytes, nil
}

// volatileManifestKeys are dbt manifest metadata fields that change on every
// invocation even when the source is unchanged. They must be ignored, or every
// recompile would produce a new hash and defeat the cache.
var volatileManifestKeys = []string{"generated_at", "invocation_id", "invocation_started_at", "run_started_at"}

// stripEntityCreatedAt removes the dbt-owned created_at field from every
// resource entry in every top-level manifest collection (nodes, sources,
// macros, exposures, disabled, ...). dbt regenerates these timestamps on
// every full parse, so a CI-built manifest would never hash-match a locally
// built one. Only depth-2 entries are touched: a user-defined
// meta.created_at inside a resource sits deeper and is preserved. The
// disabled collection maps each name to a LIST of resources, so list
// elements are handled as well.
func stripEntityCreatedAt(doc map[string]any) {
	for _, v := range doc {
		collection, ok := v.(map[string]any)
		if !ok {
			continue
		}
		for _, e := range collection {
			switch entity := e.(type) {
			case map[string]any:
				delete(entity, "created_at")
			case []any:
				for _, item := range entity {
					if m, ok := item.(map[string]any); ok {
						delete(m, "created_at")
					}
				}
			}
		}
	}
}

// isDbtManifest reports whether doc looks like a dbt manifest. dbt always writes
// metadata.dbt_schema_version, which unrelated manifest.json files (web app/PWA,
// tooling, etc.) do not have — so we only stamp files that carry it.
func isDbtManifest(doc map[string]any) bool {
	meta, ok := doc["metadata"].(map[string]any)
	if !ok {
		return false
	}
	v, ok := meta["dbt_schema_version"].(string)
	return ok && v != ""
}

// readManifestDoc reads path and parses it as JSON. isDbt reports whether the
// file is actually a dbt manifest (see isDbtManifest). A file that is not
// valid JSON, or that lacks the dbt manifest shape, returns isDbt=false and
// doc=nil — not every manifest.json found in a project is one dbt produced
// (e.g. a web app's PWA manifest).
func readManifestDoc(path string) (doc map[string]any, bytes int64, isDbt bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, false, err
	}
	bytes = int64(len(data))

	if err := json.Unmarshal(data, &doc); err != nil {
		//nolint:nilerr // invalid JSON → not a dbt manifest: a skip, not an error (callers don't stamp it)
		return nil, bytes, false, nil
	}
	if !isDbtManifest(doc) {
		return nil, bytes, false, nil
	}
	return doc, bytes, true, nil
}

// hashDocument computes the "sha256-manifest-v2" hash of an already-parsed
// dbt manifest document, after removing the volatile fields dbt regenerates on
// every full parse, so the value is stable across recompiles of unchanged
// source. doc is mutated in place.
func hashDocument(doc map[string]any) string {
	if meta, ok := doc["metadata"].(map[string]any); ok {
		for _, k := range volatileManifestKeys {
			delete(meta, k)
		}
	}
	stripEntityCreatedAt(doc)
	// json.Marshal emits object keys in sorted order, so this is deterministic;
	// doc came from json.Unmarshal, so it holds only JSON-native types and
	// re-marshaling cannot fail.
	canonical, _ := json.Marshal(doc)
	return sha256Hex(canonical)
}

// hashManifest computes the "sha256-manifest-v2" hash of a dbt manifest.json.
// isDbt reports whether the file is actually a dbt manifest; a file that
// isn't is not stamped (callers skip it) — so unrelated manifest.json files
// in the project don't get a sidecar.
func hashManifest(path string) (hash string, isDbt bool, err error) {
	doc, _, isDbt, err := readManifestDoc(path)
	if err != nil || !isDbt {
		return "", isDbt, err
	}
	return hashDocument(doc), true, nil
}

// hashFile returns the hex sha256 of a file's contents and its size in bytes.
func hashFile(path string) (digest string, size int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	h := sha256.New()
	size, err = io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), size, nil
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
