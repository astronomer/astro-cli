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
	"strings"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
)

// DefaultAirflowVersion is the Airflow version a new project pins when
// --airflow-version is not given. Partial on purpose: resolution to a
// concrete release happens at start time, so new projects track patches.
const DefaultAirflowVersion = "3.1"

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

// Names written in more than one place, kept as constants so the spellings
// never drift. gitignore.go uses these too — it arrived with its own
// .gitignore and 0o644 constants, which is the drift this comment forbids.
const (
	fileGitignore = ".gitignore"
	fileAgents    = "AGENTS.md"
	fileClaude    = "CLAUDE.md"
)

// projectDirs are the standard project directories, in creation order.
var projectDirs = []string{"dags", "include", "plugins", "tests"}

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

	// A manifest already there is adopted; its absence is the greenfield path.
	// Both arms settle the manifest and write nothing.
	cs := &Changeset{Result: Result{Dir: abs}}
	var out []byte
	var manifestLabels []string
	var pin manifestFacts
	data, readErr := os.ReadFile(marker)
	switch {
	case readErr == nil:
		out, manifestLabels, pin, err = adopt(abs, data, opts, &cs.Result)
	case errors.Is(readErr, os.ErrNotExist):
		// No labels from this arm: a scaffolded manifest is created rather than
		// edited, so its one line is the filename, supplied below.
		out, pin, err = scaffoldManifest(abs, opts, &cs.Result)
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
		manifestChange.Labels = []string{manifest.Marker}
	}
	cs.Changes = append(cs.Changes, manifestChange)

	// Created and Updated are derived from the changes rather than appended
	// beside them, so a change that is performed but unreported — or reported
	// but not performed — cannot be constructed.
	cs.report()
	cs.Notes = leftovers(abs, pin)
	return cs, nil
}

// scaffoldManifest renders the manifest for a directory that has none, and
// records on the Result what it chose. It returns the manifest rather than
// writing it, so write puts every file on disk in one place.
func scaffoldManifest(dir string, opts Options, res *Result) ([]byte, manifestFacts, error) {
	name := opts.Name
	if name == "" {
		name = deriveName(dir)
	}
	version, defaulted := resolveAirflowVersion(opts.AirflowVersion, nil)
	pyproject, err := renderPyproject(name, version)
	if err != nil {
		return nil, manifestFacts{}, err
	}
	res.Name, res.AirflowVersion = name, version
	return pyproject, manifestFacts{defaultedPin: defaulted}, nil
}

// renderPyproject builds the greenfield manifest. It fills the template
// through the surgical editor, so the name, the pin, and the dependency are
// quoted the way the manifest expects, then round-trips through
// manifest.Parse, so every scaffolded project is guaranteed to load. An
// invalid --name or --airflow-version surfaces here as the manifest's own
// validation error. [project.dependencies] carries the Airflow the project
// pins, derived from the same version that fills [tool.astro].airflow, so
// init → start needs no hand-edit.
func renderPyproject(name, version string) ([]byte, error) {
	tmpl := "[project]\n" +
		"name = 'astro-project'\n" +
		"version = '" + defaultProjectVersion + "'\n" +
		"requires-python = '>=3.10'\n" +
		"dependencies = []\n\n" +
		"[tool.astro]\n" +
		"airflow = '" + DefaultAirflowVersion + "'\n"
	ed, err := tomledit.NewSurgical([]byte(tmpl))
	if err != nil {
		return nil, err
	}
	if err := ed.Set([]string{"project", "name"}, name); err != nil {
		return nil, err
	}
	if err := ed.Set([]string{"tool", "astro", "airflow"}, version); err != nil {
		return nil, err
	}
	if err := ed.Set([]string{"project", "dependencies", "0"}, airflowRequirement(version)); err != nil {
		return nil, err
	}
	data, err := ed.Bytes()
	if err != nil {
		return nil, err
	}
	if _, err := manifest.Parse(data); err != nil {
		return nil, err
	}
	return data, nil
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

	files := []struct{ name, content string }{
		{fileGitignore, gitignoreTemplate},
		{fileAgents, agentsContent()},
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
func leftovers(dir string, facts manifestFacts) []string {
	checks := []struct{ file, note string }{
		{"requirements.txt", "move its pins into [project.dependencies]"},
		{"packages.txt", "move its entries into packages under [tool.astro]"},
		{"airflow_settings.yaml", "move its connections, variables, and pools into [tool.astro]"},
		{filepath.Join(".astro", "config.yaml"), "move the Deployments it names into deployments under [tool.astro]"},
		{"Dockerfile", "not read — move what it installs into pyproject.toml"},
		{"docker-compose.yml", "not read — `astro local start` replaces it"},
		{"docker-compose.yaml", "not read — `astro local start` replaces it"},
		{"docker-compose.override.yml", "not read — move any service your dags need into your own setup"},
		{"docker-compose.override.yaml", "not read — move any service your dags need into your own setup"},
	}
	var out []string
	namesVersion := false
	for _, c := range checks {
		if _, err := os.Stat(filepath.Join(dir, c.file)); err != nil {
			continue
		}
		out = append(out, c.file+": "+c.note)
		// A Dockerfile image tag, or an apache-airflow pin in requirements.txt,
		// states the Airflow this project runs today.
		if c.file == "Dockerfile" || c.file == "requirements.txt" {
			namesVersion = true
		}
	}
	// A pin nobody chose leads the list. Most repos state the Airflow they run
	// in a Dockerfile image tag, and most of those are on 2.x, so the default
	// is the likeliest way this ends up a project that cannot start.
	if facts.defaultedPin && (namesVersion || facts.namesAirflow) {
		out = append([]string{"airflow = '" + DefaultAirflowVersion + "' is the default, not this project's version: " +
			"set it from the Airflow this project already names"}, out...)
	}
	// Dependencies declared dynamic are supplied from somewhere this cannot
	// reach, so the requirement that installs Airflow has to be put there.
	if facts.dynamicDeps {
		out = append(out, "dependencies are dynamic, so the Airflow pin was not added: put "+
			airflowRequirement(DefaultAirflowVersion)+" wherever this project lists its dependencies")
	}
	return out
}
