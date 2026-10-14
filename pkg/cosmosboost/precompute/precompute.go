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
	Skipped  bool          // a manifest.json that isn't a dbt manifest (no sidecar produced)
	Warning  string        // non-fatal note (sidecar still produced), e.g. an unresolved template
	Err      error         // non-nil if reading or hashing the unit failed (no artifacts then)
	// Artifacts are the files this unit produced: its sidecar, and the slim
	// manifest beside it when one was asked for. Empty when Err or Skipped.
	Artifacts []Artifact
}

// Artifact is one file a run produced, held in memory: Run writes nothing to
// disk. Path is where it belongs in the walked tree, <unit dir>/.astro/<name>,
// which is where the Cosmos Boost plugin looks for it in a deployed bundle; the
// caller decides how the bytes get there.
type Artifact struct {
	Path string
	Data []byte
}

// Summary is the structured outcome of a precompute run. It backs both the
// human-readable report and the tracking of this step's deploy-time overhead.
type Summary struct {
	Duration time.Duration
	Results  []Result
}

// Options selects which artifacts a run produces; the zero value produces only
// the hash sidecars.
type Options struct {
	// SlimManifest also produces a slim, field-filtered copy of each discovered
	// manifest.json (see buildSlimManifest) next to its sidecar.
	SlimManifest bool
}

// Units are what a run processes: every dbt project (a directory with
// dbt_project.yml) and standalone manifest.json under its roots, in a fixed
// order. Discover finds them; Units.Run processes them.
type Units struct {
	units []unit
}

type unit struct{ kind, path string }

// Discover finds the units under roots. It fails only when a root cannot be
// walked.
func Discover(roots []string) (Units, error) {
	projectDirs := map[string]bool{}
	for _, root := range roots {
		found, err := findProjects(root)
		if err != nil {
			return Units{}, fmt.Errorf("scanning %q for dbt projects: %w", root, err)
		}
		for _, d := range found {
			projectDirs[d] = true
		}
	}

	manifests := map[string]bool{}
	for _, root := range roots {
		found, err := findManifests(root, projectDirs)
		if err != nil {
			return Units{}, fmt.Errorf("scanning %q for manifests: %w", root, err)
		}
		for _, m := range found {
			manifests[m] = true
		}
	}

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
	return Units{units: units}, nil
}

// SidecarDirs are the .astro directories a run of u produces its artifacts
// in, one per unit, without reading any unit: a manifest.json that turns out
// not to be a dbt manifest is listed too, though nothing is produced for it.
func (u Units) SidecarDirs() []string {
	dirs := make([]string, 0, len(u.units))
	for _, un := range u.units {
		dir := un.path
		if un.kind == kindManifest {
			dir = filepath.Dir(un.path)
		}
		dirs = append(dirs, filepath.Join(dir, sidecarDir))
	}
	return dirs
}

// Run produces a .astro/dbt_metadata.json hash sidecar for each of u's units,
// in memory (Result.Artifacts): nothing is written. Units are processed
// concurrently — one worker each, bounded by GOMAXPROCS — and each is hashed
// over sorted input, so results are deterministic with no cross-worker
// coordination.
//
// Per-unit failures are best-effort: a unit that fails is recorded in its
// Result and does not stop the others. version is recorded in each sidecar's
// generated_by.
//
// With opts.SlimManifest set, every dbt manifest.json also gets a slim,
// field-filtered copy (see buildSlimManifest) for the .astro/ beside it, for
// the Cosmos Boost plugin to load in place of the full manifest at DAG-parse
// time. That includes one in a project's own root, which is not a discovery
// unit of its own and is handled by processProject.
func (u Units) Run(version string, opts Options) Summary {
	start := time.Now()
	units := u.units
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
				results[i] = processProject(u.path, version, opts)
			} else {
				results[i] = processManifest(u.path, version, opts)
			}
		}(i, u)
	}
	wg.Wait()

	return Summary{Duration: time.Since(start), Results: results}
}

// processProject hashes one dbt project directory and produces its sidecar. It reads
// dbt_project.yml once (readDbtConfig) and threads the result through hashing and the
// templated-packages warning, so the file isn't parsed more than once per project.
func processProject(dir, version string, opts Options) Result {
	start := time.Now()
	cfg := readDbtConfig(dir)
	hash, files, totalBytes, err := hashProject(dir, cfg)
	r := Result{Kind: kindProject, Path: dir, Hash: hash, Files: files, Bytes: totalBytes}
	if err != nil {
		r.Err = err
		r.Duration = time.Since(start)
		return r
	}
	if len(cfg.templatedSettings) > 0 {
		r.Warning = strings.Join(cfg.templatedSettings, ", ") +
			" in dbt_project.yml hold unresolved Jinja templates; using the dbt default directories for exclusion (the real ones may add cache churn)"
	}

	// A manifest.json in the project root is not a unit of its own - its .astro/
	// is this project's, so it would collide with the sidecar produced below -
	// and findManifests skips it for that reason. Slim it here instead, leaving
	// the project's own hash as the anchor the pointer hangs off.
	var filtered *FilteredManifest
	if opts.SlimManifest {
		if doc, _, isDbt, readErr := readManifestDoc(filepath.Join(dir, manifestFile)); readErr == nil && isDbt {
			// Nothing mutates doc afterward here, unlike processManifest.
			data, _ := json.Marshal(buildSlimManifest(doc, version)) //nolint:errcheck // JSON-native types only, so this cannot fail
			var slim Artifact
			slim, filtered = slimManifestArtifact(dir, data)
			r.Artifacts = append(r.Artifacts, slim)
		}
	}

	r.Artifacts = append(r.Artifacts, sidecarArtifact(dir, algoProjectTree, hash, version, filtered))
	if r.Err = checkArtifactPaths(dir, r.Artifacts); r.Err != nil {
		r.Artifacts = nil
	}
	r.Duration = time.Since(start)
	return r
}

// processManifest hashes one manifest.json and produces a sidecar for it, plus
// a slim, field-filtered copy of the manifest when opts asks for one (see
// buildSlimManifest). A file that isn't a dbt manifest is skipped (nothing is
// produced) so unrelated manifest.json files in the project aren't stamped.
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
			slimData, _ = json.Marshal(buildSlimManifest(doc, version)) //nolint:errcheck // see above: JSON-native types only, so this cannot fail
		}
		hash = hashDocument(doc)
	}
	r := Result{Kind: kindManifest, Path: path, Hash: hash, Files: 1, Bytes: bytes}
	dir := filepath.Dir(path)
	switch {
	case err != nil:
		r.Err = err
	case !isDbt:
		r.Skipped = true
	default:
		var filtered *FilteredManifest
		if slimData != nil {
			var slim Artifact
			slim, filtered = slimManifestArtifact(dir, slimData)
			r.Artifacts = append(r.Artifacts, slim)
		}
		r.Artifacts = append(r.Artifacts, sidecarArtifact(dir, algoManifestJSON, hash, version, filtered))
		if r.Err = checkArtifactPaths(dir, r.Artifacts); r.Err != nil {
			r.Artifacts = nil
		}
	}
	r.Duration = time.Since(start)
	return r
}

// slimManifestArtifact is dir's slim manifest holding data, and the sidecar
// pointer describing it. data must already be marshaled, so a caller that
// later mutates the source doc cannot leak into it (see processManifest).
func slimManifestArtifact(dir string, data []byte) (Artifact, *FilteredManifest) {
	return Artifact{Path: filepath.Join(dir, sidecarDir, slimManifestName), Data: data}, &FilteredManifest{
		Schema:  slimSchemaVersion,
		Path:    slimManifestName,
		Version: ProjectVersion{Algo: algoFilteredManifest, Hash: sha256Hex(data)},
	}
}

// Artifacts returns every artifact the run produced, from the units that did
// not fail, in the order of Results.
func (s Summary) Artifacts() []Artifact {
	var out []Artifact
	for i := range s.Results {
		r := &s.Results[i]
		if r.Err == nil {
			out = append(out, r.Artifacts...)
		}
	}
	return out
}

// CountFailed returns the number of units that errored.
func (s Summary) CountFailed() int {
	n := 0
	for i := range s.Results {
		r := &s.Results[i]
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
	for i := range s.Results {
		r := &s.Results[i]
		if r.Skipped {
			n++
		}
	}
	return n
}

// WriteReport prints a short, human-readable report of the run.
// Per-entry glyphs: acted on, deliberately left alone, failed, and a note
// attached to an entry that otherwise succeeded.
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
	for i := range s.Results {
		r := &s.Results[i]
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
