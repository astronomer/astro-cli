package scaffold

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// Astro Private Cloud deploys the 1.x layout (a Dockerfile and
// .astro/config.yaml), with Astro CLI 1.x, and not yet pyproject.toml
// projects, so a 1.x project converted under an APC context would stop
// deploying there, and a project scaffolded inside one would be deployed
// with it. Until APC deploys pyproject.toml projects, Plan refuses both when
// the caller sets Options.DeploysToAPC: astro init does, from the current
// context, and Astro Desktop is covered only if it sets the field too.

// ErrConvert1xUnderAPC is what a *Convert1xUnderAPCError matches with
// errors.Is: Plan or Run refused a directory in a 1.x project because
// Options.DeploysToAPC is set. Nothing was written.
var ErrConvert1xUnderAPC = errors.New("a 1.x project is not converted under an Astro Private Cloud context")

// Convert1xUnderAPCError is Plan's refusal under Options.DeploysToAPC of a
// directory that is, or is inside, the 1.x project in Dir. Its message is
// Project1xUnderAPCMessage(Dir).
type Convert1xUnderAPCError struct {
	Dir string
}

func (e *Convert1xUnderAPCError) Error() string { return Project1xUnderAPCMessage(e.Dir) }

// Is makes errors.Is(err, ErrConvert1xUnderAPC) match.
func (e *Convert1xUnderAPCError) Is(target error) bool { return target == ErrConvert1xUnderAPC }

// Project1xUnderAPCMessage is the one account of the 1.x project in dir under
// an Astro Private Cloud context, in plain text: why it stays as it is, and
// how to convert it anyway. Plan's refusal gives it, and so does every hint
// the CLI gives about a 1.x project under APC (internal/project).
func Project1xUnderAPCMessage(dir string) string {
	return dir + " holds a project made by Astro CLI 1.x (Dockerfile and .astro/), and the current context is " +
		"Astro Private Cloud, which deploys that layout and not yet pyproject.toml projects. Leave the project as " +
		"it is for now: Astro CLI 1.x keeps deploying it to Astro Private Cloud, and converting it will be " +
		"available once Astro Private Cloud deploys pyproject.toml projects. To convert it anyway, for Astro or for local " +
		"development only, switch to an Astro context first (astro context switch astronomer.io, or astro login " +
		"to sign in to Astro) and run astro init in " + dir
}

// Find1xProject is the 1.x project dir is in, dir itself included, or "".
// The walk goes up from the cleaned absolute dir to the filesystem root, and
// stops at a directory that is an Astro project (hasManifest: one whose
// [tool.astro] loads, or fails to), whose own business a 1.x project above
// it is not; Plan then reports that manifest as it always has. dir need not
// exist yet.
func Find1xProject(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	for d := filepath.Clean(abs); ; {
		if hasManifest(d) {
			return ""
		}
		if has1xLayout(d) {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}

// Is1xProject reports whether dir itself holds a 1.x project: the 1.x layout
// (has1xLayout), and no pyproject.toml carrying [tool.astro]. A
// pyproject.toml that only configures tools (ruff, pytest) does not rule it
// out, and one that cannot be read or parsed counts as a manifest, as
// internal/project's HasManifest has it. It looks at dir alone, not above it.
func Is1xProject(dir string) bool {
	return has1xLayout(dir) && !hasManifest(dir)
}

// has1xLayout reports a Dockerfile beside a .astro/ directory. In the home
// directory, and in ASTRO_HOME when it is set, .astro/ holds the CLI's own
// settings, so there it is a 1.x project's only when .astro/config.yaml
// parses with a top-level project key, which astro dev init always writes:
// a stray ~/Dockerfile beside the settings, missing, empty or corrupt, does
// not make home a 1.x project, and a home directory that holds one
// (HOME=/usr/local/airflow in a 1.x image) still is one.
func has1xLayout(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, fileDockerfile)); err != nil {
		return false
	}
	if info, err := os.Stat(filepath.Join(dir, ".astro")); err != nil || !info.IsDir() {
		return false
	}
	return !isCLIHome(dir) || namesAProject(filepath.Join(dir, ".astro", "config.yaml"))
}

// hasManifest reports a pyproject.toml whose [tool.astro] is there, loading
// or not: anything but no file and no [tool.astro] section.
func hasManifest(dir string) bool {
	_, err := manifest.Load(filepath.Join(dir, manifest.Marker))
	return !errors.Is(err, manifest.ErrNotFound) && !errors.Is(err, manifest.ErrNoAstroSection)
}

// isCLIHome reports the directory whose .astro/ holds the CLI's settings: the
// home directory, as the config package finds it (HOME, else the OS's), and
// ASTRO_HOME when set. Compared as cleaned paths, with the home resolved
// through symlinks once, so the walk resolves nothing per directory.
func isCLIHome(dir string) bool {
	home := os.Getenv("HOME")
	if home == "" {
		home, _ = os.UserHomeDir() //nolint:errcheck // no home is no match
	}
	dir = filepath.Clean(dir)
	for _, h := range []string{home, os.Getenv("ASTRO_HOME")} {
		if h == "" {
			continue
		}
		if dir == filepath.Clean(h) {
			return true
		}
		if resolved, err := filepath.EvalSymlinks(h); err == nil && dir == resolved {
			return true
		}
	}
	return false
}

// namesAProject reports a config.yaml that parses with a top-level project
// key, as a 1.x project's does.
func namesAProject(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var top map[string]any
	if yaml.Unmarshal(data, &top) != nil {
		return false
	}
	_, ok := top["project"]
	return ok
}
