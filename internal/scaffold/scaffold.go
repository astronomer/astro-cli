// Package scaffold builds the greenfield `astro init` project: the
// pyproject.toml manifest, the standard directories, .gitignore, and
// AGENTS.md (with CLAUDE.md as a symlink to it outside Windows). It follows
// the layer rules in docs/v2-architecture.md: it returns data and errors,
// never prints, never exits.
package scaffold

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

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
}

// Refusal sentinels, wrapped with directory context by Run. Callers branch
// with errors.Is.
var (
	// ErrManifestExists reports a directory that already has a
	// pyproject.toml: an existing project, not a scaffolding target.
	ErrManifestExists = errors.New("already has a pyproject.toml")
	// ErrV1Project reports a directory holding an astro v1 project
	// (Dockerfile plus .astro/), which v2 cannot scaffold over.
	ErrV1Project = errors.New("holds an astro v1 project")
)

// Project files are the user's own; world-readable is right (never the v1
// helpers' 0o777 — an earlier fix).
const (
	dirPerm  = 0o755
	filePerm = 0o644
)

// projectDirs are the standard project directories, in creation order.
var projectDirs = []string{"dags", "include", "plugins", "tests"}

// Run scaffolds a project in dir, creating it if needed. It refuses a
// directory that already has a pyproject.toml or looks like a v1 astro
// project; anything else it fills in, keeping files that already exist.
func Run(dir string, opts Options) (*Result, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", dir, err)
	}
	if err := refuse(abs); err != nil {
		return nil, err
	}

	name := opts.Name
	if name == "" {
		name = deriveName(abs)
	}
	version := opts.AirflowVersion
	if version == "" {
		version = DefaultAirflowVersion
	}
	pyproject, err := renderPyproject(name, version)
	if err != nil {
		return nil, err
	}

	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	res := &Result{Dir: abs, Name: name, AirflowVersion: version}
	if err := write(abs, pyproject, goos != "windows", res); err != nil {
		return nil, err
	}
	return res, nil
}

// refuse rejects directories `astro init` must not touch.
func refuse(dir string) error {
	if _, err := os.Stat(filepath.Join(dir, project.Marker)); err == nil {
		return fmt.Errorf("%s %w; edit that manifest instead of re-initializing", dir, ErrManifestExists)
	}
	if _, err := os.Stat(filepath.Join(dir, "Dockerfile")); err != nil {
		return nil
	}
	if info, err := os.Stat(filepath.Join(dir, ".astro")); err == nil && info.IsDir() {
		return fmt.Errorf("%s %w (Dockerfile and .astro/); migration ships in a later release — use astro CLI 1.x with this project for now", dir, ErrV1Project)
	}
	return nil
}

// renderPyproject fills the static template: one tomledit pass sets the
// project name and the Airflow pin, and a manifest.Parse round-trip
// guarantees every scaffolded project loads — an invalid --name or
// --airflow-version surfaces here as the manifest's own validation error.
func renderPyproject(name, version string) (string, error) {
	ed, err := tomledit.NewSurgical([]byte(pyprojectTemplate))
	if err != nil {
		return "", err
	}
	if err := ed.Set([]string{"project", "name"}, name); err != nil {
		return "", err
	}
	if err := ed.Set([]string{"tool", "astro", "airflow"}, version); err != nil {
		return "", err
	}
	data, err := ed.Bytes()
	if err != nil {
		return "", err
	}
	if _, err := manifest.Parse(data); err != nil {
		return "", err
	}
	return string(data), nil
}

// write puts the scaffold on disk. Existing entries are kept and reported
// as skipped, so a rerun over a partial scaffold is safe.
func write(dir, pyproject string, withSymlink bool, res *Result) error {
	//nolint:gosec // G301: see dirPerm
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return err
	}
	for _, d := range projectDirs {
		path := filepath.Join(dir, d)
		if _, err := os.Lstat(path); err == nil {
			res.Skipped = append(res.Skipped, d+"/")
			continue
		}
		//nolint:gosec // G301: see dirPerm
		if err := os.Mkdir(path, dirPerm); err != nil {
			return fmt.Errorf("creating %s: %w", d, err)
		}
		res.Created = append(res.Created, d+"/")
	}

	files := []struct{ name, content string }{
		{project.Marker, pyproject},
		{".gitignore", gitignoreTemplate},
		{"AGENTS.md", agentsContent()},
	}
	for _, f := range files {
		path := filepath.Join(dir, f.name)
		if _, err := os.Lstat(path); err == nil {
			res.Skipped = append(res.Skipped, f.name)
			continue
		}
		//nolint:gosec // G306: see filePerm
		if err := os.WriteFile(path, []byte(f.content), filePerm); err != nil {
			return fmt.Errorf("creating %s: %w", f.name, err)
		}
		res.Created = append(res.Created, f.name)
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
