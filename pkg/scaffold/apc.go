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

// SwitchToAstro is how to make the current context Astro's, the one wording
// every account of a 1.x project under Astro Private Cloud gives. astro
// login names the domain: without one it logs back in to the current
// context, which is APC's.
const SwitchToAstro = "switch to an Astro context first (astro context switch astronomer.io, or astro login " +
	"astronomer.io to sign in to Astro)"

// Project1xUnderAPCMessage is the account of the 1.x project in dir under an
// Astro Private Cloud context, in plain text: why it stays as it is
// (Project1xUnderAPCReason), and how to convert it anyway. Plan's refusal
// gives it.
func Project1xUnderAPCMessage(dir string) string {
	return Project1xUnderAPCReason(dir) + ". To convert it anyway, for Astro or for local development only, " +
		SwitchToAstro + ", then run astro init in " + dir
}

// Project1xUnderAPCReason is why the 1.x project in dir stays as it is under
// an Astro Private Cloud context, the part every account of it shares; the
// CLI's own (internal/project) ends it with the way to convert anyway that
// fits how the context was chosen.
func Project1xUnderAPCReason(dir string) string {
	return dir + " holds a project made by Astro CLI 1.x (Dockerfile and .astro/), and the current context is " +
		"Astro Private Cloud, which deploys that layout and not yet pyproject.toml projects. Leave the project as " +
		"it is for now: Astro CLI 1.x keeps deploying it to Astro Private Cloud, and converting it will be " +
		"available once Astro Private Cloud deploys pyproject.toml projects"
}

// Find1xProject is the 1.x project dir is in, dir itself included, or "".
// The walk goes up from the cleaned absolute dir to the filesystem root, and
// stops at a directory that is an Astro project (HasManifest), whose own
// business a 1.x project above it is not; Plan then reports that manifest as
// it always has. dir need not exist yet.
func Find1xProject(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	for d := filepath.Clean(abs); ; {
		if HasManifest(d) {
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
// (has1xLayout), and no pyproject.toml carrying [tool.astro] (HasManifest).
// It is the one definition: the CLI's discovery and hints use it too
// (internal/project.Is1xProject). A pyproject.toml that only configures
// tools (ruff, pytest) does not rule it out. It looks at dir alone, not above
// it.
func Is1xProject(dir string) bool {
	return has1xLayout(dir) && !HasManifest(dir)
}

// has1xLayout reports the 1.x layout as Astro CLI 1.x itself knows a
// project: a Dockerfile, and a .astro/config.yaml (1.x's config.IsProjectDir,
// config/config.go, which its deploy requires through utils.EnsureProjectDir).
// 1.x tells its home directory apart by path; this tells the CLI's own
// settings file apart by content instead (isCLISettings), so a stray
// ~/Dockerfile beside them does not make home a 1.x project, while a home
// directory that holds one (HOME=/usr/local/airflow in a 1.x image) still is.
// A config.yaml that is empty or does not parse counts, as it does for 1.x.
func has1xLayout(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, fileDockerfile)); err != nil {
		return false
	}
	config := filepath.Join(dir, ".astro", "config.yaml")
	if info, err := os.Stat(config); err != nil || info.IsDir() {
		return false
	}
	return !isCLISettings(config)
}

// HasManifest reports a pyproject.toml whose [tool.astro] is there, loading
// or not: anything but no file and no [tool.astro] section. One that cannot
// be read or parsed counts, so a caller reports its error rather than
// treating the directory as no project.
func HasManifest(dir string) bool {
	_, err := manifest.Load(filepath.Join(dir, manifest.Marker))
	return !errors.Is(err, manifest.ErrNotFound) && !errors.Is(err, manifest.ErrNoAstroSection)
}

// cliSettingsKeys are top-level keys the CLI's own settings file has and a
// 1.x project's does not: the current context, the saved ones, and telemetry
// (config.CFG's context, contexts and telemetry.*).
var cliSettingsKeys = []string{"context", "contexts", "telemetry"}

// isCLISettings reports a config.yaml that is the CLI's own settings: a YAML
// mapping with no top-level project key (a 1.x project's always has one,
// astro dev init writes project.name) and at least one of cliSettingsKeys.
// Anything else, empty or unparseable included, is a project's.
func isCLISettings(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var top map[string]any
	if yaml.Unmarshal(data, &top) != nil || top == nil {
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
