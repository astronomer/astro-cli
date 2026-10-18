package scaffold

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// Find1xProject is the 1.x project dir is in, dir itself included, or "".
// The walk goes up from the cleaned absolute dir to the filesystem root, and
// stops at a directory that is an Astro project (HasManifest), whose own
// business a 1.x project above it is not; Plan then reports that manifest as
// it always has. dir need not exist yet. The astro dev stub names the root it
// finds, and the deploy's own walk (cmd/utils.Locate) stops where it does.
func Find1xProject(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	for d := filepath.Clean(abs); ; {
		if HasManifest(d) {
			return ""
		}
		if Has1xLayout(d) {
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
// (Has1xLayout), and no pyproject.toml carrying [tool.astro] (HasManifest).
// It is the one definition: the CLI's discovery and hints use it too
// (internal/project). A pyproject.toml that only configures
// tools (ruff, pytest) does not rule it out. It looks at dir alone, not above
// it.
func Is1xProject(dir string) bool {
	return Has1xLayout(dir) && !HasManifest(dir)
}

// Has1xLayout reports the 1.x layout as Astro CLI 1.x itself knows a
// project: a Dockerfile, and a .astro/config.yaml (1.x's config.IsProjectDir,
// config/config.go, which its deploy requires through utils.EnsureProjectDir).
// 1.x tells its home directory apart by path; this tells the CLI's own
// settings file apart by content instead (isCLISettings), so a stray
// ~/Dockerfile beside them does not make home a 1.x project, while a home
// directory that holds one (HOME=/usr/local/airflow in a 1.x image) still is.
// A config.yaml that is empty or does not parse counts, as it does for 1.x.
//
// It is Is1xProject's other half, for a walk that has already loaded dir's
// pyproject.toml and found no manifest there (ManifestPresent), so it need
// not load it again.
func Has1xLayout(dir string) bool {
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
	return ManifestPresent(err)
}

// ManifestPresent is HasManifest's answer from what manifest.Load returned
// for a directory's pyproject.toml, for a caller that keeps the manifest it
// loaded: anything but no file and no [tool.astro] section.
func ManifestPresent(loadErr error) bool {
	return !errors.Is(loadErr, manifest.ErrNotFound) && !errors.Is(loadErr, manifest.ErrNoAstroSection)
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
