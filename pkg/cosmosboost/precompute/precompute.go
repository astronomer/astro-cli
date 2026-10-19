package precompute

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
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
// directory or a standalone manifest.
type Result struct {
	Kind     string        // "project" or "manifest"
	Path     string        // project directory, or manifest path
	Hash     string        // version hash (empty if Err != nil or Skipped)
	Files    int           // files hashed (1 for a manifest)
	Bytes    int64         // total bytes hashed
	Duration time.Duration // time spent on this unit
	Skipped  bool          // a manifest-like file that isn't a dbt manifest (no sidecar produced)
	Warning  string        // non-fatal note (sidecar still produced), e.g. an unresolved template
	Err      error         // non-nil if reading or hashing the unit failed (no artifacts then)
	// Artifacts are the files this unit produced: its slim manifests when
	// asked for, and its sidecar - except that manifests sharing a directory
	// share one sidecar, carried by the first of them (see manifestGroup).
	// Empty when Err or Skipped.
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
	// dbt manifest (see buildSlimManifest) next to its sidecar.
	SlimManifest bool
}

type unit struct{ kind, path string }

// discover finds every dbt project (a directory with dbt_project.yml) and
// standalone manifest candidate (isManifestCandidateName) under roots, in a
// fixed order. It fails only when a
// root cannot be walked.
func discover(roots []string) ([]unit, error) {
	projectDirs := map[string]bool{}
	for _, root := range roots {
		found, err := findProjects(root)
		if err != nil {
			return nil, fmt.Errorf("scanning %q for dbt projects: %w", root, err)
		}
		for _, d := range found {
			projectDirs[d] = true
		}
	}

	manifests := map[string]bool{}
	for _, root := range roots {
		found, err := findManifests(root, projectDirs)
		if err != nil {
			return nil, fmt.Errorf("scanning %q for manifests: %w", root, err)
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
	return units, nil
}

// Run finds every dbt project (a directory with dbt_project.yml) and every
// standalone dbt manifest under the given roots, and produces a
// .astro/dbt_metadata.json hash sidecar for each directory holding one, in
// memory (Result.Artifacts): nothing under the roots is written. Units are
// computed concurrently - one worker each, bounded by GOMAXPROCS - and each
// is hashed over sorted input, so results are deterministic.
//
// Per-unit failures are best-effort: a unit that fails is recorded in its
// Result and does not stop the others. Run only returns a non-nil error for a
// top-level problem, such as a root that cannot be walked. version is
// recorded in each sidecar's generated_by.
//
// With opts.SlimManifest set, every dbt manifest also gets a slim,
// field-filtered copy (see buildSlimManifest), named after it (slimNameFor),
// for the .astro/ beside it, for the Cosmos Boost plugin to load in place of
// the full manifest at DAG-parse time. That includes those in a project's
// own root, which are not discovery units of their own and are handled by
// processProject.
//
// Manifests sharing a directory share its .astro/, so their results are
// assembled per directory once all are computed (manifestGroup): one sidecar
// names them all. Each project owns its directory, so needs no such step.
func Run(roots []string, version string, opts Options) (Summary, error) {
	start := time.Now()
	units, err := discover(roots)
	if err != nil {
		return Summary{}, err
	}
	results := make([]Result, len(units))
	computations := make([]manifestComputation, len(units))
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
				computations[i] = computeManifest(u.path, version, opts)
			}
		}(i, u)
	}
	wg.Wait()

	// units is sorted by path, so each directory's indexes are too.
	byDir := map[string][]int{}
	for i, u := range units {
		if u.kind == kindManifest {
			byDir[filepath.Dir(u.path)] = append(byDir[filepath.Dir(u.path)], i)
		}
	}
	for dir, idxs := range byDir {
		manifestGroup(dir, idxs, units, computations, results, version)
	}

	return Summary{Duration: time.Since(start), Results: results}, nil
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

	// A manifest in the project root is not a unit of its own - its .astro/
	// is this project's, so its sidecar would collide with the one produced
	// below - and findManifests skips it for that reason. Slim every one here
	// instead, leaving the project's own hash as the anchor.
	var manifests map[string]ManifestVersion
	if opts.SlimManifest {
		var slims []Artifact
		manifests, slims, r.Warning = slimProjectRootManifests(dir, version, r.Warning)
		r.Artifacts = append(r.Artifacts, slims...)
	}

	r.Artifacts = append(r.Artifacts, sidecarArtifact(dir, algoProjectTree, hash, version, manifests))
	if r.Err = checkSidecarDir(dir); r.Err != nil {
		r.Artifacts = nil
	}
	r.Duration = time.Since(start)
	return r
}

// slimProjectRootManifests produces a slim manifest for each dbt manifest
// directly in project dir, and the sidecar entries describing them. A
// candidate it cannot read costs only that one candidate, as a note added to
// warning: the project keeps its sidecar, and every other candidate its slim
// manifest.
func slimProjectRootManifests(dir, version, warning string) (manifests map[string]ManifestVersion, slims []Artifact, notes string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, joinNotes(warning, "could not scan for manifests to slim: "+err.Error())
	}
	manifests = map[string]ManifestVersion{}
	for _, e := range entries {
		if e.IsDir() || !isManifestCandidateName(e.Name()) {
			continue
		}
		doc, _, isDbt, readErr := readManifestDoc(filepath.Join(dir, e.Name()))
		if readErr != nil {
			warning = joinNotes(warning, e.Name()+": could not read as a manifest ("+readErr.Error()+")")
			continue
		}
		if !isDbt {
			continue
		}
		// Marshal before hashDocument mutates doc (see computeManifest).
		data, _ := json.Marshal(buildSlimManifest(doc, version)) //nolint:errcheck // JSON-native types only, so this cannot fail
		slim, entry := slimManifestArtifact(dir, e.Name(), data)
		slims = append(slims, slim)
		manifests[e.Name()] = ManifestVersion{
			Version: ProjectVersion{Algo: algoManifestJSON, Hash: hashDocument(doc)},
			Slim:    entry,
		}
	}
	return manifests, slims, warning
}

// manifestComputation is what one manifest unit produces before its
// directory's results are assembled (manifestGroup).
type manifestComputation struct {
	start    time.Time
	bytes    int64
	isDbt    bool
	hash     string
	slimData []byte
	err      error
}

// computeManifest reads and hashes one manifest-like file, and builds its
// slim copy when opts asks for one. A file that isn't actually a dbt manifest
// is left unhashed, so an unrelated *.json file whose name happens to contain
// "manifest" isn't stamped.
func computeManifest(path, version string, opts Options) manifestComputation {
	c := manifestComputation{start: time.Now()}
	doc, bytes, isDbt, err := readManifestDoc(path)
	c.bytes, c.isDbt, c.err = bytes, isDbt, err
	if err != nil || !isDbt {
		return c
	}
	if opts.SlimManifest {
		// Marshal before hashDocument mutates doc: the slim manifest shares
		// doc's nested values, so only turning it into bytes here decouples
		// the two. It holds JSON-native types only, so this cannot fail.
		c.slimData, _ = json.Marshal(buildSlimManifest(doc, version)) //nolint:errcheck // see above: JSON-native types only, so this cannot fail
	}
	c.hash = hashDocument(doc)
	return c
}

// manifestGroup sets the Result of each manifest unit in dir - idxs, all of
// them, in path order - and produces their artifacts: each dbt manifest's
// slim manifest on its own Result, and dir's one sidecar, naming every one,
// on the first of them. Each producing a sidecar would put two files at one
// bundle path. A unit that failed or isn't a dbt manifest is left out of the
// sidecar, and costs its siblings nothing.
func manifestGroup(dir string, idxs []int, units []unit, computations []manifestComputation, results []Result, version string) {
	manifests := map[string]ManifestVersion{}
	var stamped []int
	for _, i := range idxs {
		path, c := units[i].path, computations[i]
		r := Result{Kind: kindManifest, Path: path, Files: 1, Bytes: c.bytes}
		switch {
		case c.err != nil:
			r.Err = c.err
		case !c.isDbt:
			r.Skipped = true
		default:
			r.Hash = c.hash
			name := filepath.Base(path)
			var slim *SlimManifest
			if c.slimData != nil {
				var a Artifact
				a, slim = slimManifestArtifact(dir, name, c.slimData)
				r.Artifacts = append(r.Artifacts, a)
			}
			manifests[name] = ManifestVersion{Version: ProjectVersion{Algo: algoManifestJSON, Hash: c.hash}, Slim: slim}
			stamped = append(stamped, i)
		}
		r.Duration = time.Since(c.start)
		results[i] = r
	}
	if len(stamped) == 0 {
		return
	}
	if err := checkSidecarDir(dir); err != nil {
		for _, i := range stamped {
			results[i].Err, results[i].Artifacts = err, nil
		}
		return
	}
	// The sidecar's top-level Version mirrors the first manifest by name, for
	// a reader that doesn't yet look at Manifests: stable across runs.
	first := manifests[filepath.Base(units[stamped[0]].path)]
	r := &results[stamped[0]]
	r.Artifacts = append(r.Artifacts, sidecarArtifact(dir, first.Version.Algo, first.Version.Hash, version, manifests))
}

// joinNotes appends add to existing, semicolon-separated.
func joinNotes(existing, add string) string {
	if existing == "" {
		return add
	}
	return existing + "; " + add
}

// slimManifestArtifact is the slim manifest of manifestFilename in dir
// (slimNameFor), holding data, and the sidecar entry describing it. data
// must already be marshaled, so a caller that later mutates the source doc
// cannot leak into it (see computeManifest).
func slimManifestArtifact(dir, manifestFilename string, data []byte) (Artifact, *SlimManifest) {
	name := slimNameFor(manifestFilename)
	return Artifact{Path: filepath.Join(dir, sidecarDir, name), Data: data}, &SlimManifest{
		Schema:  slimSchemaVersion,
		Path:    name,
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

// CountSkipped returns the number of units skipped (manifest-like files that
// aren't dbt manifests).
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
