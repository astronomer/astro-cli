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

	"github.com/astronomer/astro-cli/internal/localenv"
	"github.com/astronomer/astro-cli/internal/project"
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
// never drift.
const (
	fileGitignore = ".gitignore"
	fileAgents    = "AGENTS.md"
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
func Run(dir string, opts Options) (*Result, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", dir, err)
	}

	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	marker := filepath.Join(abs, project.Marker)

	// A manifest already there is adopted; its absence is the greenfield path.
	// Both arms settle the manifest and write nothing.
	res := &Result{Dir: abs}
	var out []byte
	var pin manifestFacts
	data, readErr := os.ReadFile(marker)
	switch {
	case readErr == nil:
		out, pin, err = adopt(abs, data, opts, res)
	case errors.Is(readErr, os.ErrNotExist):
		out, pin, err = scaffoldManifest(abs, opts, res)
	default:
		err = fmt.Errorf("reading %s: %w", marker, readErr)
	}
	if err != nil {
		return nil, err
	}

	// The scaffold lands before the manifest. A manifest carrying [tool.astro]
	// is the one thing that makes a rerun refuse, so writing it last leaves a
	// run that failed part-way through safe to repeat.
	if err := write(abs, goos != windowsOS, res); err != nil {
		return nil, err
	}
	// Written in place, not through a temp file and a rename. This manifest is
	// often one this package did not create, and a rename replaces it: it
	// drops the file's mode, writes through a read-only bit that says don't,
	// and turns a symlinked manifest into a regular file. Every byte here has
	// already been through manifest.Parse, so what lands is a manifest that
	// loads.
	//nolint:gosec // G703: marker is the directory the user named for their own project, joined with a fixed filename — writing there is what init does
	if err := os.WriteFile(marker, out, filePerm); err != nil {
		return nil, fmt.Errorf("writing %s: %w", marker, err)
	}
	if !res.Adopted {
		res.Created = append(res.Created, project.Marker)
	}
	res.Notes = leftovers(abs, pin)
	return res, nil
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

// write puts the scaffold on disk — the directories, .gitignore, AGENTS.md and
// the symlink. Existing entries are kept and reported as skipped, so a rerun
// over a partial scaffold is safe. The manifest is Run's to write, on both
// paths, so it is not here.
func write(dir string, withSymlink bool, res *Result) error {
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return err
	}
	for _, d := range projectDirs {
		path := filepath.Join(dir, d)
		if _, err := os.Lstat(path); err == nil {
			res.Skipped = append(res.Skipped, d+"/")
			continue
		}
		if err := os.Mkdir(path, dirPerm); err != nil {
			return fmt.Errorf("creating %s: %w", d, err)
		}
		res.Created = append(res.Created, d+"/")
	}

	files := []struct{ name, content string }{
		{fileGitignore, gitignoreTemplate},
		{fileAgents, agentsContent()},
	}
	for _, f := range files {
		path := filepath.Join(dir, f.name)
		if _, err := os.Lstat(path); err == nil {
			res.Skipped = append(res.Skipped, f.name)
			continue
		}
		if err := os.WriteFile(path, []byte(f.content), filePerm); err != nil {
			return fmt.Errorf("creating %s: %w", f.name, err)
		}
		res.Created = append(res.Created, f.name)
	}

	// Ensure .gitignore covers .env even when it already existed and was kept
	// above (a fresh template already lists it, so this only heals a .gitignore
	// the repo already had). Local env values must never be committed.
	if added, err := localenv.EnsureEnvIgnored(dir); err != nil {
		return err
	} else if added {
		res.Updated = append(res.Updated, fileGitignore+" (added the .env rule)")
	}

	if !withSymlink {
		return nil
	}
	link := filepath.Join(dir, "CLAUDE.md")
	if _, err := os.Lstat(link); err == nil {
		res.Skipped = append(res.Skipped, "CLAUDE.md")
		return nil
	}
	if err := os.Symlink("AGENTS.md", link); err != nil {
		return fmt.Errorf("linking CLAUDE.md: %w", err)
	}
	res.Created = append(res.Created, "CLAUDE.md -> AGENTS.md")
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
