// Package scaffold makes a directory an `astro init` project: the
// pyproject.toml manifest — written fresh, or adopted where one is already
// there — the standard directories, .gitignore, and AGENTS.md (with CLAUDE.md
// as a symlink to it outside Windows). It follows the layer rules in
// docs/v2-architecture.md: it returns data and errors, never prints, never
// exits.
package scaffold

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
)

// DefaultAirflowVersion is the Airflow version a new project pins when
// --airflow-version is not given. Partial on purpose: resolution to a
// concrete release happens at start time, so new projects track patches.
const DefaultAirflowVersion = "3.1"

// manifestKeyAirflow is the [tool.astro] key carrying the Airflow pin. Both
// manifest arms write it and the starter DAG's import rule is checked against
// it, which is three spellings of one name — the drift the file-name constants
// below are kept for.
const manifestKeyAirflow = "airflow"

// Options adjust what Run scaffolds.
type Options struct {
	// Name is the [project] name. Empty derives it from the directory name.
	Name string
	// AirflowVersion is the [tool.astro] airflow pin. Empty means
	// DefaultAirflowVersion.
	AirflowVersion string
	// GOOS overrides runtime.GOOS, so tests can check the Windows layout
	// (no CLAUDE.md symlink) from any host.
	GOOS string
}

// Result reports what Run did. It is the `astro init` output payload in
// both text and json mode.
type Result struct {
	Dir            string   `json:"dir"`
	Name           string   `json:"name"`
	AirflowVersion string   `json:"airflow"`
	Created        []string `json:"created"`
	// Skipped lists entries that already existed and were left untouched.
	Skipped []string `json:"skipped,omitempty"`
	// Updated lists files this run changed rather than created.
	Updated []string `json:"updated,omitempty"`
	// Adopted reports that the manifest was already there and gained a
	// [tool.astro] section, rather than being written by this run. A status
	// bool a json consumer reads, so it stays present when false.
	Adopted bool `json:"adopted"`
	// Notes lists the files init found but did not read, and where their
	// contents belong. It is the work left to do by hand.
	Notes []string `json:"notes,omitempty"`
}

// ErrAlreadyAstroProject reports a directory whose pyproject.toml already
// carries [tool.astro]. It is the one shape Run refuses: every other
// directory is either scaffolded or adopted. Callers branch with errors.Is.
var ErrAlreadyAstroProject = errors.New("is already an Astro project")

// Project files are the user's own; world-readable is right (never the v1
// helpers' 0o777 — an earlier fix).
const (
	dirPerm  = 0o755
	filePerm = 0o644
)

// projectHasNoDags reports whether the project has no DAGs yet: either no dags
// directory at all, or one holding nothing that could be a DAG.
//
// It answers conservatively, because the two mistakes do not cost the same.
// Skipping the example leaves someone without a file they could write in a
// minute. Writing it can put a file somewhere that is not ours, or fail a run
// that had no reason to fail — so anything this cannot read as a plain,
// DAG-less directory counts as a project that already has DAGs.
//
// That is why `dags` present as anything other than a real directory — a
// regular file, a symlink, an entry that will not stat — means skip, and why
// nothing here returns an error. A regular file named `dags` used to scaffold
// fine, and turning it into a hard failure of the whole run would be a
// regression for a state nobody asked us to police. The symlink case matters on
// its own: os.ReadDir follows one, so the example would be written THROUGH it,
// landing outside the directory the preview showed and Change.resolve undertakes
// not to leave. A dangling symlink is worse still, reading as empty right up
// until the write fails half way through a run.
//
// Only the directory's own entries are counted, never a recursive walk: walking
// a large tree to decide whether to write one small file is the wrong trade.
// That makes an empty `dags/archive/` read as a project WITH DAGs. It is the
// wrong answer for that one shape, and it is the conservative one.
func projectHasNoDags(dir string) bool {
	path := filepath.Join(dir, dagsDir)
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return true
	case err != nil || !info.IsDir():
		return false
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !isDagsPlaceholder(e.Name()) {
			return false
		}
	}
	return true
}

// isDagsPlaceholder reports whether an entry in dags/ is bookkeeping rather than
// a DAG.
//
// Git cannot track an empty directory, so a repository that committed an empty
// dags/ carries a .gitkeep inside it — which makes "a clone with a placeholder
// in it" the single most common shape of a project with no DAGs, and counting
// that entry would deny the example to exactly the projects it is for.
// .DS_Store arrives from opening the folder in Finder; the .gitignore this
// package writes lists it for that reason.
func isDagsPlaceholder(name string) bool {
	return strings.HasPrefix(name, ".") || name == "__pycache__"
}

// starterDagSuits reports whether the starter DAG can run on the Airflow this
// project pins.
//
// The example imports airflow.sdk, which is the Airflow 3 Task SDK and does not
// exist before it: Airflow 2 spells the same two decorators airflow.decorators,
// which is why this repo's v1 templates keep a per-major pair
// (pkg/airflowrt/include/airflow2 beside .../airflow3). The pin is not always 3.
// pickAirflowVersion reads it from --airflow-version, the manifest, a Dockerfile
// runtime tag or requirements.txt, and any of those can say 2 — adopting a v1
// project is the ordinary way it happens.
//
// So a project pinning Airflow 2 gets no example, deliberately. A DAG that
// cannot import is the first-run failure this whole file exists to avoid, and it
// is worse than an empty dags directory rather than better — the same trade the
// example's third-party imports were already decided on. An Airflow 2 variant
// would differ by one import line, but nothing has asked for one, and a second
// copy of the file earns its keep only once something does.
//
// A pin this cannot parse is treated as suitable. It has already been through
// pickAirflowVersion by the time it arrives, so an unreadable one means the
// manifest is unusual in ways this decision has no business adjudicating.
func starterDagSuits(airflowVersion string) bool {
	major, _, _ := strings.Cut(airflowVersion, ".")
	n, err := strconv.Atoi(major)
	if err != nil {
		return true
	}
	return n >= firstAirflowWithTaskSDK
}

// firstAirflowWithTaskSDK is the Airflow major that introduced airflow.sdk, the
// one import the starter DAG makes.
const firstAirflowWithTaskSDK = 3

// templateFile is one scaffolded file: where it goes, and what goes in it.
type templateFile struct{ name, content string }

// fileExampleDag is the starter DAG's path. It sits under the dags directory
// projectDirs creates, and planFiles writes files after directories for that
// reason.
//
// A new project had an empty dags/ until this landed: v1's scaffold wrote an
// example and pkg/scaffold did not carry it over, so `astro init` followed by
// `astro local start` gave you an Airflow with nothing in it. No decision was
// recorded against having one, so this reads as an omission rather than a
// choice.
const fileExampleDag = dagsDir + "/exampledag.py"

// Names written in more than one place, kept as constants so the spellings
// never drift. gitignore.go uses these too — it arrived with its own
// .gitignore and 0o644 constants, which is the drift this comment forbids.
const (
	fileGitignore = ".gitignore"
	fileAgents    = "AGENTS.md"
	fileClaude    = "CLAUDE.md"
)

// projectDirs are the standard project directories, in creation order.
var projectDirs = []string{dagsDir, "include", "plugins", "tests"}

// dagsDir is named because three things key off it: projectDirs creates it,
// projectHasNoDags decides the starter DAG from what is in it, and
// fileExampleDag is a path inside it.
const dagsDir = "dags"

// windowsOS is the GOOS whose layout skips the CLAUDE.md symlink.
const windowsOS = "windows"

// manifestFacts is what the hand-off list needs to know about the manifest,
// beyond the files sitting beside it.
type manifestFacts struct {
	// defaultedPin reports that nothing named an Airflow version, so the pin
	// is the CLI's default.
	defaultedPin bool
	// namesAirflow reports that the manifest names apache-airflow in a shape
	// no version could be read out of — a range, a wildcard. With a defaulted
	// pin that means the project may have just moved an Airflow generation.
	namesAirflow bool
	// dynamicDeps reports that dependencies are declared dynamic, so the
	// Airflow requirement could not be added beside them.
	dynamicDeps bool
	// carriedNotes is what building the manifest could not carry, discovered
	// while building it rather than while reading the v1 files: extras on an
	// apache-airflow requirement that the generated pin does not reproduce.
	carriedNotes []string
	// carriedLabels describes what the manifest write absorbed, for the
	// Result's lists. The adopt arm returns its own labels directly; the
	// greenfield arm cannot, because its label is fixed by Plan, so it reports
	// them here instead.
	carriedLabels []string
}

// Run makes dir an Astro project, creating dir if needed. A directory with no
// pyproject.toml is scaffolded; one that has a pyproject.toml without
// [tool.astro] is adopted, so `astro init` runs in an Airflow repo as it
// stands. Either way, files already there are kept, and what Run could not
// carry over is reported in Notes.
//
// Run is Plan followed by Apply, which is what a command wants: nobody is going
// to review a change set at a terminal that has already asked for it. A caller
// that shows the change set to a person first calls the two halves itself.
func Run(dir string, opts Options) (*Result, error) {
	cs, err := Plan(dir, opts)
	if err != nil {
		return nil, err
	}
	return cs.Apply()
}

// Plan works out what making dir an Astro project would do, and returns it
// without touching the project.
//
// The split exists because this package's output lands in someone's repository.
// A command can reasonably scaffold on request, but Astro Desktop offers to
// convert a project the user already has, and O3 requires it to show the diff
// first — which is impossible if the only way to learn what a run does is to
// let it happen. Plan reads the project (it has to: an adopted manifest is
// computed from the one already there) and writes nothing.
func Plan(dir string, opts Options) (*Changeset, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", dir, err)
	}

	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	marker := filepath.Join(abs, manifest.Marker)

	// What the v1 files say, read before either arm, because both need it: a
	// greenfield manifest is BUILT from them and an adopted one is extended
	// with them. This is the difference between init-in-an-existing-project
	// converting it and init leaving a hand-off list beside files nobody read.
	v1, err := readV1Project(abs)
	if err != nil {
		return nil, err
	}

	// A manifest already there is adopted; its absence is the greenfield path.
	// Both arms settle the manifest and write nothing.
	cs := &Changeset{Result: Result{Dir: abs}}
	var out []byte
	var manifestLabels []string
	var pin manifestFacts
	data, readErr := os.ReadFile(marker)
	switch {
	case readErr == nil:
		out, manifestLabels, pin, err = adopt(abs, data, opts, v1, &cs.Result)
	case errors.Is(readErr, os.ErrNotExist):
		// No labels from this arm: a scaffolded manifest is created rather than
		// edited, so its one line is the filename, supplied below.
		out, pin, err = scaffoldManifest(abs, opts, v1, &cs.Result)
	default:
		err = fmt.Errorf("reading %s: %w", marker, readErr)
	}
	if err != nil {
		return nil, err
	}

	if err := planFiles(abs, goos != windowsOS, cs); err != nil {
		return nil, err
	}

	// The manifest goes LAST, and the ordering is the reason Apply walks a
	// slice rather than a map. A manifest carrying [tool.astro] is the one
	// thing that makes a rerun refuse, so a run that dies part-way through is
	// safe to repeat only while the manifest is still absent.
	manifestChange := Change{Kind: UpdateFile, Path: manifest.Marker, Content: out, Labels: manifestLabels}
	if !cs.Adopted {
		manifestChange.Kind = CreateFile
		manifestChange.Labels = append([]string{manifest.Marker}, pin.carriedLabels...)
	}
	cs.Changes = append(cs.Changes, manifestChange)

	// Created and Updated are derived from the changes rather than appended
	// beside them, so a change that is performed but unreported — or reported
	// but not performed — cannot be constructed.
	cs.report()
	// The v1 notes lead: they are about the files this run just read, so they
	// describe what it could not carry. leftovers is about files it did not read
	// at all, which is a weaker statement and belongs after.
	cs.Notes = slices.Concat(v1.notes, pin.carriedNotes, leftovers(abs, cs.AirflowVersion, pin, v1))
	return cs, nil
}

// scaffoldManifest renders the manifest for a directory that has none, and
// records on the Result what it chose. It returns the manifest rather than
// writing it, so write puts every file on disk in one place.
func scaffoldManifest(dir string, opts Options, v1 *v1Project, res *Result) ([]byte, manifestFacts, error) {
	name := opts.Name
	if name == "" {
		name = deriveName(dir)
	}
	version, defaulted := pickAirflowVersion(opts.AirflowVersion, nil, v1)
	pyproject, notes, err := renderPyproject(name, version, v1)
	if err != nil {
		return nil, manifestFacts{}, err
	}
	res.Name, res.AirflowVersion = name, version
	return pyproject, manifestFacts{
		defaultedPin:  defaulted,
		carriedNotes:  notes,
		carriedLabels: carriedLabels(v1),
	}, nil
}

// carriedLabels describes what a greenfield manifest absorbed from the v1 files.
//
// Without this the common case said nothing. A real v1 project has no
// pyproject.toml, so it takes the greenfield arm, where Plan hardcodes the
// manifest's label to the filename — so `astro init` printed "pyproject.toml"
// and never mentioned that thirty requirement lines and a list of apt packages
// had just been moved into it. The rarer adopt arm did say so.
func carriedLabels(v1 *v1Project) []string {
	var out []string
	if n := len(v1.dependencies); n > 0 {
		out = append(out, manifest.Marker+" (carried "+strconv.Itoa(n)+" from requirements.txt into dependencies)")
	}
	if len(v1.packages) > 0 {
		out = append(out, manifest.Marker+" (carried packages.txt into packages)")
	}
	if v1.airflow != "" {
		out = append(out, manifest.Marker+" (read airflow = "+v1.airflow+" from the Dockerfile)")
	}
	return out
}

// pickAirflowVersion resolves the pin from every source that can state one, in
// precedence order, and reports whether the answer is only the default.
//
//	Options.AirflowVersion
//	  → an apache-airflow pin in the MANIFEST's [project.dependencies]
//	    → the Dockerfile's runtime tag
//	      → an apache-airflow pin in requirements.txt
//	        → DefaultAirflowVersion
//
// Two of those orderings were wrong before, and both produced a project pinned a
// whole Airflow generation from where it actually was.
//
// The manifest's own pin now outranks the Dockerfile. Folding the Dockerfile
// into resolveAirflowVersion's flag slot put it above everything, so adopting a
// manifest pinning apache-airflow==2.9.1 in a directory with a stale
// runtime:3.1-12 Dockerfile wrote airflow = "3.1" beside a dependency list still
// saying 2.9.1 — a manifest contradicting itself, with the image built for one
// and the venv installing the other. Nothing cross-validates the two. A pin its
// author wrote in the manifest is the strongest statement short of an explicit
// flag.
//
// And requirements.txt is consulted on BOTH paths. The adopt arm passed the
// manifest's dependencies and never looked at v1's, so the same project answered
// differently depending on whether an unrelated pyproject.toml happened to
// exist: greenfield read the requirements pin, adopt defaulted and then dropped
// the pin during the merge.
//
// The caller's option stays on top, which is what lets Plan stay offline: an
// Airflow 2 tag names no minor, so a caller that wants the exact one resolves it
// through the release index and passes it here.
//
// The Dockerfile sits above a requirements.txt pin because the image tag is what
// the project runs today, while a pin in requirements.txt is what pip was asked
// to install INTO that image.
func pickAirflowVersion(flag string, manifestDeps []string, v1 *v1Project) (version string, defaulted bool) {
	if flag != "" {
		return flag, false
	}
	if v, ok := pinFromDeps(manifestDeps); ok {
		return v, false
	}
	if v1.airflow != "" {
		return v1.airflow, false
	}
	if v, ok := pinFromDeps(v1.dependencies); ok {
		return v, false
	}
	return DefaultAirflowVersion, true
}

// renderPyproject builds the greenfield manifest. It fills the template
// through the surgical editor, so the name, the pin, and the dependency are
// quoted the way the manifest expects, then round-trips through
// manifest.Parse, so every scaffolded project is guaranteed to load. An
// invalid --name or --airflow-version surfaces here as the manifest's own
// validation error. [project.dependencies] carries the Airflow the project
// pins, derived from the same version that fills [tool.astro].airflow, so
// init → start needs no hand-edit.
func renderPyproject(name, version string, v1 *v1Project) (pyproject []byte, notes []string, err error) {
	tmpl := "[project]\n" +
		"name = 'astro-project'\n" +
		"version = '" + defaultProjectVersion + "'\n" +
		"requires-python = '>=3.10'\n" +
		"dependencies = []\n\n" +
		"[tool.astro]\n" +
		"airflow = '" + DefaultAirflowVersion + "'\n"
	ed, err := tomledit.NewSurgical([]byte(tmpl))
	if err != nil {
		return nil, nil, err
	}
	if err := ed.Set([]string{"project", "name"}, name); err != nil {
		return nil, nil, err
	}
	if err := ed.Set([]string{"tool", "astro", manifestKeyAirflow}, version); err != nil {
		return nil, nil, err
	}
	// The Airflow requirement leads, then whatever requirements.txt carried,
	// deduplicated by distribution name the way the adopt arm does.
	//
	// The dedup is not tidiness. A requirements.txt naming one distribution
	// twice — "pandas==1.5.0" early and "pandas==2.1.0" later, or "Flask" and
	// "flask", which are the same PEP 503 name — produced two entries for it.
	// manifest.Parse accepts that, then uv intersects the specifiers and the
	// environment is unsatisfiable at the first start. Only the adopt arm
	// guarded against it, and greenfield is the arm a real v1 project takes.
	deps := []any{airflowRequirement(version)}
	seen := map[string]bool{airflowDist: true}
	for _, d := range v1.dependencies {
		name := distName(d)
		if seen[name] {
			// An apache-airflow entry is where `version` came from, so the
			// generated requirement above already says it.
			if name == airflowDist {
				notes = append(notes, airflowExtrasNote(d)...)
			}
			continue
		}
		seen[name] = true
		deps = append(deps, d)
	}
	if err := ed.Set([]string{"project", "dependencies"}, deps); err != nil {
		return nil, nil, err
	}
	if len(v1.packages) > 0 {
		if err := ed.Set([]string{"tool", "astro", "packages"}, asAny(v1.packages)); err != nil {
			return nil, nil, err
		}
	}
	data, err := ed.Bytes()
	if err != nil {
		return nil, nil, err
	}
	if _, err := manifest.Parse(data); err != nil {
		return nil, nil, err
	}
	return data, notes, nil
}

// planFiles works out the scaffold half of a run: the standard directories, the
// template files, the .env rule, and the CLAUDE.md symlink. It decides
// everything by reading and appends the operations to cs, writing nothing.
//
// The decisions were previously made as each file was written, which is why the
// preview did not exist: "does .gitignore already cover .env" was answered
// inside the call that healed it.
func planFiles(dir string, withSymlink bool, cs *Changeset) error {
	for _, d := range projectDirs {
		if _, err := os.Lstat(filepath.Join(dir, d)); err == nil {
			cs.Skipped = append(cs.Skipped, d+"/")
			continue
		}
		cs.Changes = append(cs.Changes, Change{Kind: CreateDir, Path: d, Labels: []string{d + "/"}})
	}

	files := []templateFile{
		{fileGitignore, gitignoreTemplate},
		{fileAgents, agentsContent()},
	}
	// The starter DAG is for a project that has none, which is not the same as
	// a project that lacks a file called exampledag.py.
	//
	// Skip-existing answers the second question, and would drop an example into
	// a repo full of real pipelines just because nothing there happened to carry
	// that name. Adoption is the common case for init in an existing repo, so
	// that would be clutter in someone's dags directory far more often than it
	// would be a helpful first DAG.
	if starterDagSuits(cs.AirflowVersion) && projectHasNoDags(dir) {
		files = append(files, templateFile{fileExampleDag, exampleDag})
	}
	for _, f := range files {
		if _, err := os.Lstat(filepath.Join(dir, f.name)); err == nil {
			cs.Skipped = append(cs.Skipped, f.name)
			continue
		}
		cs.Changes = append(cs.Changes, Change{
			Kind: CreateFile, Path: f.name, Content: []byte(f.content), Labels: []string{f.name},
		})
	}

	// Local env values must never be committed, so a .gitignore lacking the rule
	// gets it added. Computed here rather than performed, so the bytes can be
	// shown before they land.
	//
	// Run unconditionally, deliberately. Guarding it on "we are not writing the
	// template" reads like an optimization and is a dependency: it makes the
	// heal rely on gitignoreTemplate containing a .env line, so an edit to that
	// template would ship every new project with .env tracked by git and nothing
	// would notice. Unguarded, planEnvIgnored reads whatever is on disk — which
	// during Plan is still the pre-scaffold state — and returns nil when there
	// is nothing to do.
	healed, err := planEnvIgnored(dir)
	if err != nil {
		return err
	}
	if healed != nil {
		cs.Changes = append(cs.Changes, *healed)
	}

	if !withSymlink {
		return nil
	}
	if _, err := os.Lstat(filepath.Join(dir, fileClaude)); err == nil {
		cs.Skipped = append(cs.Skipped, fileClaude)
		return nil
	}
	cs.Changes = append(cs.Changes, Change{
		Kind: CreateSymlink, Path: fileClaude, Target: fileAgents,
		Labels: []string{fileClaude + " -> " + fileAgents},
	})
	return nil
}

// deriveName turns a directory basename into a valid [project] name (PEP
// 508): letters lower, invalid runes become "-", separators neither lead,
// trail, nor repeat.
func deriveName(dir string) string {
	var b strings.Builder
	sep := true // true also strips leading separators
	for _, r := range strings.ToLower(filepath.Base(dir)) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			sep = false
		case r == '-' || r == '_' || r == '.':
			if !sep {
				b.WriteRune(r)
				sep = true
			}
		default:
			if !sep {
				b.WriteByte('-')
				sep = true
			}
		}
	}
	name := strings.TrimRight(b.String(), "-_.")
	if name == "" {
		return "astro-project"
	}
	return name
}

// leftovers reports the files init found, did not read, and cannot carry over
// on its own, with where each one belongs. Reading a Dockerfile means guessing
// what its RUN lines were for, so init names it and stops there. The list is
// the hand-off: what a person, or the agent working with them, does next.
func leftovers(dir, version string, facts manifestFacts, v1 *v1Project) []string {
	// requirements.txt, packages.txt and the Dockerfile are READ now, so they
	// are gone from this list: whatever they could not carry is a note from the
	// reader that says which line and why, which is strictly better than
	// "move its pins" about a file that was mostly carried.
	checks := []struct{ file, note string }{
		{"airflow_settings.yaml", "move its connections, variables, and pools into [tool.astro]"},
		{filepath.Join(".astro", "config.yaml"), "move the Deployments it names into deployments under [tool.astro]"},
		{"docker-compose.yml", "not read — `astro local start` replaces it"},
		{"docker-compose.yaml", "not read — `astro local start` replaces it"},
		{"docker-compose.override.yml", "not read — move any service your dags need into your own setup"},
		{"docker-compose.override.yaml", "not read — move any service your dags need into your own setup"},
	}
	var out []string
	for _, c := range checks {
		if _, err := os.Stat(filepath.Join(dir, c.file)); err != nil {
			continue
		}
		out = append(out, c.file+": "+c.note)
	}
	// A pin nobody chose leads the list. Most repos state the Airflow they run
	// in a Dockerfile image tag, and most of those are on 2.x, so the default
	// is the likeliest way this ends up a project that cannot start.
	//
	// v1.statedVersion is now what gates this rather than the presence of a
	// Dockerfile or a requirements.txt. Presence used to stand in for "we did
	// not read it", and that is no longer true: a Dockerfile whose tag we read
	// leaves defaultedPin false, and one whose tag we could not read has
	// already said so in its own note. What is left for this warning is the
	// case where a file named a version and we still ended up defaulting.
	if facts.defaultedPin && (v1.statedVersion || facts.namesAirflow) {
		out = append([]string{"airflow = '" + DefaultAirflowVersion + "' is the default, not this project's version: " +
			"set it from the Airflow this project already names"}, out...)
	}
	// Dependencies declared dynamic are supplied from somewhere this cannot
	// reach, so the requirement that installs Airflow has to be put there.
	// The version this run actually pinned, not the default. Telling someone to
	// install apache-airflow==3.1.* under a manifest this run pinned to "2" is
	// advice that installs the wrong Airflow generation and contradicts the file
	// it was printed beside.
	if facts.dynamicDeps {
		out = append(out, "dependencies are dynamic, so the Airflow pin was not added: put "+
			airflowRequirement(version)+" wherever this project lists its dependencies")
		if len(v1.dependencies) > 0 {
			out = append(out, "requirements.txt: dependencies are dynamic, so its "+
				strconv.Itoa(len(v1.dependencies))+" requirements were not carried either")
		}
	}
	return out
}
