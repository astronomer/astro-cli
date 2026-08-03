package precompute

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const defaultPackagesDir = "dbt_packages"

// dbtConfig holds the dbt_project.yml settings that decide which directories hold
// generated or installed output and must be excluded from a project's source hash.
// All paths are project-root-relative and already cleaned; an empty string means
// "unset" — the caller still excludes the dbt defaults (target/, logs/, dbt_packages/)
// unconditionally, so an unset field simply adds no extra exclusion.
type dbtConfig struct {
	packagesInstallPath string // packages-install-path override (default: dbt_packages/)
	targetPath          string // target-path override (default: target/)
	logPath             string // log-path override (default: logs/)
	templatedPackages   bool   // packages-install-path is an unresolved Jinja template
}

// readDbtConfig reads dbt_project.yml once and returns the directory settings that
// affect the project hash. It is best-effort: a missing file, a parse error, or an
// unset key yields the zero value for that field, because a project's hash must never
// fail over an optional setting. Reading here once (rather than per-lookup) keeps the
// single dbt_project.yml read on the caller's hot path.
//
// packages-install-path is special: a Jinja value (e.g. "{{ env_var('DBT_PKG_DIR') }}")
// is NOT rendered — that would need a Jinja engine, and the referenced env vars are
// typically only set where dbt runs (the Airflow runtime), not where this CLI runs
// (the deploy machine), so any value computed here would likely be wrong. Such a value
// is reported as unset with templatedPackages=true so the caller can warn; the real
// packages dir then isn't excluded (a noisier version key / cache churn, not a
// correctness problem). Rendering templates is left to a future PR if ever needed.
func readDbtConfig(projectDir string) dbtConfig {
	data, err := os.ReadFile(filepath.Join(projectDir, dbtProjectFile))
	if err != nil {
		return dbtConfig{}
	}
	var raw struct {
		PackagesInstallPath string `yaml:"packages-install-path"`
		TargetPath          string `yaml:"target-path"`
		LogPath             string `yaml:"log-path"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return dbtConfig{}
	}

	cfg := dbtConfig{
		targetPath: cleanRelDir(raw.TargetPath),
		logPath:    cleanRelDir(raw.LogPath),
	}
	switch {
	case raw.PackagesInstallPath == "":
		// unset → the caller excludes the default dbt_packages/
	case strings.Contains(raw.PackagesInstallPath, "{{"):
		cfg.templatedPackages = true // Jinja template, not rendered (see doc above)
	default:
		cfg.packagesInstallPath = cleanRelDir(raw.PackagesInstallPath)
	}
	return cfg
}

// cleanRelDir normalises a dbt_project.yml directory setting to a slash-separated,
// project-root-relative path, or "" if it is empty or escapes the project root
// (absolute or "../…"). A value that escapes the root can't be excluded by relative
// path, so we drop it rather than risk excluding something outside the project.
func cleanRelDir(p string) string {
	if p == "" {
		return ""
	}
	c := filepath.ToSlash(filepath.Clean(p))
	if c == "." || c == ".." || strings.HasPrefix(c, "../") || filepath.IsAbs(c) {
		return ""
	}
	return c
}
