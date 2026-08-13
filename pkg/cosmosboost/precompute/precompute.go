package precompute

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// The two kinds of unit a run processes.
const (
	kindProject  = "project"
	kindManifest = "manifest"
)

// Result records what happened for one unit of work: either a dbt project
// directory or a standalone manifest.json.
type Result struct {
	Kind     string        // "project" or "manifest"
	Path     string        // project directory, or manifest.json path
	Hash     string        // version hash (empty if Err != nil or Skipped)
	Files    int           // files hashed (1 for a manifest)
	Bytes    int64         // total bytes hashed
	Duration time.Duration // time spent on this unit
	Skipped  bool          // a manifest.json that isn't a dbt manifest (no sidecar written)
	Warning  string        // non-fatal note (sidecar still written), e.g. an unresolved template
	Err      error         // non-nil if hashing, writing the sidecar, or writing the slim manifest failed
}

// Summary is the structured outcome of a precompute run. It backs both the
// human-readable report and the tracking of this step's deploy-time overhead.
type Summary struct {
	Duration time.Duration
	Results  []Result
}

// Options selects which artifacts a run writes; the zero value writes only the
// hash sidecars.
type Options struct {
	// SlimManifest also writes a slim, field-filtered copy of each discovered
	// manifest.json (see buildSlimManifest) next to its sidecar.
	SlimManifest bool
}

// Run finds every dbt project (a directory with dbt_project.yml) and standalone
// dbt manifest.json under the given roots, and writes a .astro/dbt_metadata.json
// hash sidecar next to each. Units are processed concurrently — one worker each,
// bounded by GOMAXPROCS — and each is hashed over sorted input, so results are
// deterministic with no cross-worker coordination.
//
// Per-unit failures are best-effort: a unit that fails is recorded in its Result
// and does not stop the others. Run only returns a non-nil error for a top-level
// problem, such as a root that cannot be walked. version is recorded in each
// sidecar's generated_by.
//
// With opts.SlimManifest set, every discovered manifest.json also gets a slim,
// field-filtered copy (see buildSlimManifest) written next to its sidecar, for
// the Cosmos Boost plugin to load in place of the full manifest at DAG-parse
// time. A manifest.json sitting directly in a project's root is excluded from
// discovery (see findManifests) and so gets no slim copy either.
func Run(roots []string, version string, opts Options) (Summary, error) {
	start := time.Now()

	projectDirs := map[string]bool{}
	for _, root := range roots {
		found, err := findProjects(root)
		if err != nil {
			return Summary{}, fmt.Errorf("scanning %q for dbt projects: %w", root, err)
		}
		for _, d := range found {
			projectDirs[d] = true
		}
	}

	manifests := map[string]bool{}
	for _, root := range roots {
		found, err := findManifests(root, projectDirs)
		if err != nil {
			return Summary{}, fmt.Errorf("scanning %q for manifests: %w", root, err)
		}
		for _, m := range found {
			manifests[m] = true
		}
	}

	type unit struct{ kind, path string }
	var units []unit
	for d := range projectDirs {
		units = append(units, unit{kindProject, d})
	}
	for m := range manifests {
		units = append(units, unit{kindManifest, m})
	}
	// Composite sort key: path first, kind as the tiebreaker. NUL sorts below
	// every other byte, so prefix relationships between paths are preserved.
	sort.Slice(units, func(i, j int) bool {
		return units[i].path+"\x00"+units[i].kind < units[j].path+"\x00"+units[j].kind
	})

	results := make([]Result, len(units))
	sem := make(chan struct{}, max(1, runtime.GOMAXPROCS(0)))
	var wg sync.WaitGroup
	for i, u := range units {
		wg.Add(1)
		sem <- struct{}{} // acquire a worker slot
		go func(i int, u unit) {
			defer wg.Done()
			defer func() { <-sem }() // release the slot
			if u.kind == kindProject {
				results[i] = processProject(u.path, version)
			} else {
				results[i] = processManifest(u.path, version, opts)
			}
		}(i, u)
	}
	wg.Wait()

	return Summary{Duration: time.Since(start), Results: results}, nil
}

// processProject hashes one dbt project directory and writes its sidecar. It reads
// dbt_project.yml once (readDbtConfig) and threads the result through hashing and the
// templated-packages warning, so the file isn't parsed more than once per project.
func processProject(dir, version string) Result {
	start := time.Now()
	cfg := readDbtConfig(dir)
	hash, files, totalBytes, err := hashProject(dir, cfg)
	r := Result{Kind: kindProject, Path: dir, Hash: hash, Files: files, Bytes: totalBytes, Duration: time.Since(start)}
	if err != nil {
		r.Err = err
		return r
	}
	if len(cfg.templatedSettings) > 0 {
		r.Warning = strings.Join(cfg.templatedSettings, ", ") +
			" in dbt_project.yml hold unresolved Jinja templates; using the dbt default directories for exclusion (the real ones may add cache churn)"
	}
	r.Err = writeSidecar(dir, algoProjectTree, hash, version, nil)
	r.Duration = time.Since(start)
	return r
}

// processManifest hashes one manifest.json and writes a sidecar next to it,
// plus a slim, field-filtered copy of the manifest when opts asks for one (see
// buildSlimManifest). A file that isn't a dbt manifest is skipped (nothing is
// written) so unrelated manifest.json files in the project aren't stamped.
func processManifest(path, version string, opts Options) Result {
	start := time.Now()
	doc, bytes, isDbt, err := readManifestDoc(path)
	var hash string
	var slimData []byte
	if err == nil && isDbt {
		if opts.SlimManifest {
			// Marshal before hashDocument mutates doc: the slim manifest shares
			// doc's nested values, so only turning it into bytes here decouples
			// the two. It holds JSON-native types only, so this cannot fail.
			slimData, _ = json.Marshal(buildSlimManifest(doc, version))
		}
		hash = hashDocument(doc)
	}
	r := Result{Kind: kindManifest, Path: path, Hash: hash, Files: 1, Bytes: bytes, Duration: time.Since(start)}
	switch {
	case err != nil:
		r.Err = err
	case !isDbt:
		r.Skipped = true
	default:
		dir := filepath.Dir(path)
		// The sidecar goes last: it carries the filtered_manifest pointer, so it
		// must never exist before the file it points at. Stopping short of it
		// looks like "nothing was stamped", which BestEffortPreDeploy treats as
		// safe.
		var filtered *FilteredManifest
		if slimData != nil {
			if r.Err = writeArtifact(dir, slimManifestName, slimData); r.Err == nil {
				filtered = &FilteredManifest{
					Schema:  slimSchemaVersion,
					Path:    slimManifestName,
					Version: ProjectVersion{Algo: algoFilteredManifest, Hash: sha256Hex(slimData)},
				}
			}
		}
		if r.Err == nil {
			r.Err = writeSidecar(dir, algoManifestJSON, hash, version, filtered)
		}
	}
	return r
}

// CountFailed returns the number of units that errored.
func (s Summary) CountFailed() int {
	n := 0
	for _, r := range s.Results {
		if r.Err != nil {
			n++
		}
	}
	return n
}

// CountSkipped returns the number of units skipped (manifest.json files that aren't
// dbt manifests).
func (s Summary) CountSkipped() int {
	n := 0
	for _, r := range s.Results {
		if r.Skipped {
			n++
		}
	}
	return n
}

// WriteReport prints a short, human-readable report of the run.
// Per-entry glyphs, shared by every command's WriteReport (see also
// CleanupSummary.WriteReport in cleanup.go) so the two reports keep one
// convention: acted on, deliberately left alone, failed, and a note attached to
// an entry that otherwise succeeded.
const (
	glyphDone = "✓"
	glyphLeft = "⊘"
	glyphFail = "✗"
	glyphNote = "⚠"
)

func (s Summary) WriteReport(w io.Writer) {
	stamped := len(s.Results) - s.CountFailed() - s.CountSkipped()
	fmt.Fprintf(w, "cosmos boost pre-deploy: %d stamped, %d skipped, %d failed in %s total (incl. discovery)\n",
		stamped, s.CountSkipped(), s.CountFailed(), s.Duration.Round(time.Microsecond))
	for _, r := range s.Results {
		switch {
		case r.Err != nil:
			fmt.Fprintf(w, "  %s %-8s %s  (%v)\n", glyphFail, r.Kind, r.Path, r.Err)
		case r.Skipped:
			fmt.Fprintf(w, "  %s %-8s %s  (not a dbt manifest)\n", glyphLeft, r.Kind, r.Path)
		default:
			fmt.Fprintf(w, "  %s %-8s %s  hash=%s files=%d bytes=%d %s\n",
				glyphDone, r.Kind, r.Path, shortHash(r.Hash), r.Files, r.Bytes, r.Duration.Round(time.Microsecond))
			if r.Warning != "" {
				fmt.Fprintf(w, "    %s %s\n", glyphNote, r.Warning)
			}
		}
	}
}

// shortHashLen is how much of the hash the human-readable report shows.
const shortHashLen = 12

func shortHash(hash string) string {
	if len(hash) > shortHashLen {
		return hash[:shortHashLen]
	}
	return hash
}
