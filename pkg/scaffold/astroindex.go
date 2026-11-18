package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// uvIndexSettings are the uv settings that add an index or replace the
// default one, in a uv.toml or in [tool.uv].
var uvIndexSettings = []string{"index", "index-url", "extra-index-url", "find-links"}

// uvIndexEnv are the environment variables that do the same.
var uvIndexEnv = []string{"UV_DEFAULT_INDEX", "UV_INDEX_URL", "UV_INDEX", "UV_EXTRA_INDEX_URL", "UV_FIND_LINKS"}

// getenv and uvConfigFiles are where the index reasons look outside the
// project, replaced in tests so a developer's own uv settings do not leak in.
var (
	getenv        = os.Getenv
	uvConfigFiles = userUVConfigFiles
)

// uv asks a project's other indexes before the default and stops at the first
// that has a package, and settings outside the project replace the default,
// so with any of the settings below a mirror of PyPI can answer for
// apache-airflow and the +astro build is never found. Two helpers tell the
// ones in the project, which travel with the committed pyproject.toml, from
// the ones on this machine, which do not: a write the user asked for
// (init, an upgrade) heeds both, and a start heeds only the first, so it never
// rewrites a committed file because of one machine's settings.

// ProjectIndexReason says why Astronomer's index cannot be the default in the
// project in dir, from what the project itself holds, or "" when nothing does:
//
//   - a [[tool.uv.index]] entry of its own that is not explicit (an explicit
//     one is only asked for the packages that name it, so it cannot answer for
//     apache-airflow), default or not, or [tool.uv] index-url,
//     extra-index-url or find-links;
//   - a uv.toml beside the manifest that sets an index.
func ProjectIndexReason(dir string, ed tomledit.Editor) string {
	current, _ := ed.Get(indexKey)
	entries, _ := current.([]any)
	for _, e := range entries {
		entry, _ := e.(map[string]any)
		if entry["name"] == astroIndexName {
			continue
		}
		if explicit, _ := entry["explicit"].(bool); explicit {
			continue
		}
		name, _ := entry["name"].(string)
		if name == "" {
			return manifest.Marker + " lists an index of its own"
		}
		return fmt.Sprintf("%s lists its own index, %s", manifest.Marker, name)
	}
	for _, key := range uvIndexSettings[1:] {
		if _, ok := ed.Get([]string{"tool", "uv", key}); ok {
			return fmt.Sprintf("%s sets [tool.uv] %s", manifest.Marker, key)
		}
	}
	path := filepath.Join(dir, "uv.toml")
	if key := uvConfigIndexSetting(path); key != "" {
		return fmt.Sprintf("%s sets %s", path, key)
	}
	return ""
}

// MachineIndexReason says why Astronomer's index may not be found on this
// machine, or "" when nothing here says so. These settings are not in the
// project, so they say nothing about another machine:
//
//   - UV_DEFAULT_INDEX, UV_INDEX_URL, UV_INDEX, UV_EXTRA_INDEX_URL or
//     UV_FIND_LINKS set where the CLI runs;
//   - the uv.toml UV_CONFIG_FILE names, or the user's or the system's, that
//     sets an index;
//   - ASTRO_AIRFLOW_INDEX_URL at an address other than Astronomer's, a mirror
//     that may carry only Astronomer's builds.
//
// Settings only another machine has (a CI job's environment) cannot be seen
// here; the manifest reference says to keep the index explicit there.
func MachineIndexReason() string {
	for _, name := range uvIndexEnv {
		if getenv(name) != "" {
			return name + " is set"
		}
	}
	files := uvConfigFiles()
	if path := strings.TrimSpace(getenv("UV_CONFIG_FILE")); path != "" {
		files = append([]string{path}, files...)
	}
	for _, path := range files {
		if key := uvConfigIndexSetting(path); key != "" {
			return fmt.Sprintf("%s sets %s", path, key)
		}
	}
	if u := strings.TrimSpace(getenv(runtimeversions.IndexURLEnv)); u != "" && strings.TrimSuffix(u, "/")+"/" != runtimeversions.DefaultIndexURL {
		return runtimeversions.IndexURLEnv + " points at another index"
	}
	return ""
}

// uvConfigIndexSetting is the first index setting the uv.toml at path holds,
// or "" when it holds none or cannot be read.
func uvConfigIndexSetting(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var settings map[string]any
	if toml.Unmarshal(data, &settings) != nil {
		return ""
	}
	if i := slices.IndexFunc(uvIndexSettings, func(key string) bool { _, ok := settings[key]; return ok }); i >= 0 {
		return uvIndexSettings[i]
	}
	return ""
}

// userUVConfigFiles are the user's and the system's uv.toml, where uv reads
// them.
func userUVConfigFiles() []string {
	var files []string
	if runtime.GOOS == "windows" {
		if appData := getenv("APPDATA"); appData != "" {
			files = append(files, filepath.Join(appData, "uv", "uv.toml"))
		}
		if programData := getenv("PROGRAMDATA"); programData != "" {
			files = append(files, filepath.Join(programData, "uv", "uv.toml"))
		}
		return files
	}
	if xdg := getenv("XDG_CONFIG_HOME"); xdg != "" {
		files = append(files, filepath.Join(xdg, "uv", "uv.toml"))
	} else if home, err := os.UserHomeDir(); err == nil {
		files = append(files, filepath.Join(home, ".config", "uv", "uv.toml"))
	}
	return append(files, "/etc/uv/uv.toml")
}
