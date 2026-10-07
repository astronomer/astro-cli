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

type unit struct{ kind, path string }

// Result records what happened for one unit of work: either a dbt project
// directory or a standalone manifest.
type Result struct {
	Kind     string        // "project" or "manifest"
	Path     string        // project directory, or manifest path
	Hash     string        // version hash (empty if Err != nil or Skipped)
	Files    int           // files hashed (1 for a manifest)
	Bytes    int64         // total bytes hashed
	Duration time.Duration // time spent on this unit
	Skipped  bool          // a manifest-like file that isn't a dbt manifest (no sidecar written)
	Warning  string        // non-fatal note (sidecar still written), e.g. an unresolved template
	Err      error         // non-nil if hashing, writing the sidecar, or writing a slim manifest failed
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
	// manifest (see buildSlimManifest) next to its sidecar.
	SlimManifest bool
}

// Run finds every dbt project (a directory with dbt_project.yml) and every
// standalone dbt manifest under the given roots, and writes a
// .astro/dbt_metadata.json hash sidecar next to each. version is recorded in
// each sidecar's generated_by.
//
// Per-unit failures are best-effort: a unit that fails is recorded in its
// Result and does not stop the others. Run only returns a non-nil error for a
// top-level problem, such as a root that cannot be walked.
//
// Manifest units are computed concurrently, then written in a second pass
// grouped by directory: two manifests sharing a directory (see
// isManifestCandidateName) share one dbt_metadata.json, so writing it twice
// from two goroutines would race. Project units need no such grouping - each
// owns its directory exclusively.
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

	byDir := map[string][]int{}
	for i, u := range units {
		if u.kind == kindManifest {
			byDir[filepath.Dir(u.path)] = append(byDir[filepath.Dir(u.path)], i)
		}
	}
	for dir, idxs := range byDir {
		writeManifestGroup(dir, idxs, units, computations, results, version)
	}

	return Summary{Duration: time.Since(start), Results: results}, nil
}

// processProject hashes one dbt project directory and writes its sidecar. It reads
// dbt_project.yml once (readDbtConfig) and threads the result through hashing and the
// templated-packages warning, so the file isn't parsed more than once per project.
func processProject(dir, version string, opts Options) Result {
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

	// A manifest-like file in the project root is not a unit of its own -
	// its .astro/ is this project's - so findManifests skips it. Slim every
	// one found directly here instead, leaving the project's own hash as the
	// anchor.
	manifests := map[string]ManifestVersion{}
	if opts.SlimManifest {
		entries, readErr := os.ReadDir(dir)
		if readErr != nil {
			r.Warning = joinNotes(r.Warning, "could not scan for manifests to slim: "+readErr.Error())
		} else {
			for _, e := range entries {
				if e.IsDir() || !isManifestCandidateName(e.Name()) {
					continue
				}
				doc, _, isDbt, readErr := readManifestDoc(filepath.Join(dir, e.Name()))
				if readErr != nil {
					r.Warning = joinNotes(r.Warning, e.Name()+": could not read as a manifest ("+readErr.Error()+")")
					continue
				}
				if !isDbt {
					continue
				}
				// Nothing mutates doc afterward here, unlike computeManifest.
				data, _ := json.Marshal(buildSlimManifest(doc, version))
				slim, writeErr := writeSlimManifest(dir, e.Name(), data)
				if writeErr != nil {
					r.Err = writeErr
					r.Duration = time.Since(start)
					return r
				}
				manifests[e.Name()] = ManifestVersion{
					Version: ProjectVersion{Algo: algoManifestJSON, Hash: hashDocument(doc)},
					Slim:    slim,
				}
			}
		}
	}

	r.Err = writeSidecar(dir, algoProjectTree, hash, version, manifests)
	r.Duration = time.Since(start)
	return r
}

// manifestComputation is what one manifest unit produces before any writes -
// Run groups these by directory so siblings share one sidecar write instead
// of racing each other for it (see writeManifestGroup).
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
	c.bytes = bytes
	c.isDbt = isDbt
	if err != nil {
		c.err = err
		return c
	}
	if !isDbt {
		return c
	}
	if opts.SlimManifest {
		// Marshal before hashDocument mutates doc: the slim manifest shares
		// doc's nested values, so only turning it into bytes here decouples
		// the two. It holds JSON-native types only, so this cannot fail.
		c.slimData, _ = json.Marshal(buildSlimManifest(doc, version))
	}
	c.hash = hashDocument(doc)
	return c
}

// writeManifestGroup writes the slim file for each manifest in idxs that
// computed successfully, then one shared sidecar naming all of them -
// idxs are every kindManifest unit found in dir, so this is the directory's
// only writer. Units whose computation failed, or weren't dbt manifests, get
// their Result set here too and are left out of the sidecar.
func writeManifestGroup(dir string, idxs []int, units []unit, computations []manifestComputation, results []Result, version string) {
	manifests := map[string]ManifestVersion{}
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
			var slim *SlimManifest
			if c.slimData != nil {
				slim, r.Err = writeSlimManifest(dir, filepath.Base(path), c.slimData)
			}
			if r.Err == nil {
				manifests[filepath.Base(path)] = ManifestVersion{
					Version: ProjectVersion{Algo: algoManifestJSON, Hash: c.hash},
					Slim:    slim,
				}
			}
		}
		r.Duration = time.Since(c.start)
		results[i] = r
	}

	if len(manifests) == 0 {
		return
	}
	// The sidecar's top-level Version mirrors one manifest for a reader that
	// doesn't yet look at Manifests; sorted keys make that choice stable
	// across runs rather than whichever unit's goroutine finished last.
	names := make([]string, 0, len(manifests))
	for name := range manifests {
		names = append(names, name)
	}
	sort.Strings(names)
	primary := manifests[names[0]]

	sidecarErr := writeSidecar(dir, primary.Version.Algo, primary.Version.Hash, version, manifests)
	if sidecarErr == nil {
		return
	}
	for _, i := range idxs {
		if results[i].Err == nil && !results[i].Skipped {
			results[i].Err = sidecarErr
		}
	}
}

// joinNotes appends add to existing, semicolon-separated.
func joinNotes(existing, add string) string {
	if existing == "" {
		return add
	}
	return existing + "; " + add
}

// writeSlimManifest writes data as the slim companion of manifestFilename
// (see slimNameFor) inside dir, and returns the sidecar entry describing it.
// data must already be marshaled, so a caller that later mutates the source
// doc cannot leak into it.
func writeSlimManifest(dir, manifestFilename string, data []byte) (*SlimManifest, error) {
	name := slimNameFor(manifestFilename)
	if err := writeArtifact(dir, name, data); err != nil {
		return nil, err
	}
	return &SlimManifest{
		Schema:  slimSchemaVersion,
		Path:    name,
		Version: ProjectVersion{Algo: algoFilteredManifest, Hash: sha256Hex(data)},
	}, nil
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

// CountSkipped returns the number of units skipped (manifest-like files that
// aren't dbt manifests).
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
