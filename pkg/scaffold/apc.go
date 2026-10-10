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

// enclosing1xProject is the 1.x project abs is in, abs itself included, or
// "". The walk goes up to the filesystem root and stops at a directory that
// is an Astro project (its manifest loads), whose own business a 1.x project
// above it is not.
func enclosing1xProject(abs string) string {
	for dir := filepath.Clean(abs); ; {
		if Is1xProject(dir) {
			return dir
		}
		if _, err := manifest.Load(filepath.Join(dir, manifest.Marker)); err == nil {
			return ""
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// Is1xProject reports whether dir itself holds a 1.x project: a Dockerfile
// beside a .astro/ directory, and no pyproject.toml carrying [tool.astro]. A
// pyproject.toml that only configures tools (ruff, pytest) does not rule it
// out, and one that cannot be read or parsed counts as a manifest, as
// internal/project's HasManifest has it. It looks at dir alone, not above it.
//
// A .astro/config.yaml that is the CLI's own settings file, as the one in a
// home directory is, does not make dir one (isCLISettings), so a stray
// ~/Dockerfile does not turn home into a 1.x project, while a home directory
// that does hold one (HOME=/usr/local/airflow in a 1.x image) still is.
func Is1xProject(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, fileDockerfile)); err != nil {
		return false
	}
	if info, err := os.Stat(filepath.Join(dir, ".astro")); err != nil || !info.IsDir() {
		return false
	}
	if isCLISettings(filepath.Join(dir, ".astro", "config.yaml")) {
		return false
	}
	_, err := manifest.Load(filepath.Join(dir, manifest.Marker))
	return errors.Is(err, manifest.ErrNotFound) || errors.Is(err, manifest.ErrNoAstroSection)
}

// cliSettingsKeys are top-level keys only the CLI's own settings file
// carries: the current context and the saved ones (config.CFG's context and
// contexts), and telemetry, which the first run writes.
var cliSettingsKeys = []string{"context", "contexts", "telemetry"}

// isCLISettings reports a config.yaml that is the CLI's own settings rather
// than a 1.x project's. A 1.x project's has a top-level project key (its
// name, as astro dev init writes it); the settings file has none, and has at
// least one of cliSettingsKeys. A file that is missing, unreadable or not a
// YAML mapping is not the settings file, so the directory stays a 1.x
// project: refusing a conversion wrongly costs less than allowing one.
func isCLISettings(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var top map[string]any
	if yaml.Unmarshal(data, &top) != nil {
		return false
	}
	if _, ok := top["project"]; ok {
		return false
	}
	for _, k := range cliSettingsKeys {
		if _, ok := top[k]; ok {
			return true
		}
	}
	return false
}
