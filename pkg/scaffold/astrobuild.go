package scaffold

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// The [tool.uv] settings that make uv install the Airflow a deployment runs:
// Astronomer's build of it, which differs from PyPI's release of the same
// number, from Astronomer's index, pinned the way the runtime image pins it.
//
// They are written into the project's own pyproject.toml because that is the
// only place `uv sync`, `uv lock`, `uv run` and `uv add` all read them. uv
// takes constraint-dependencies and sources from the project and from nothing
// else: not a --config-file, not a uv.toml, not UV_CONSTRAINT.
//
//	[tool.uv]
//	environments = [linux, arm64 macOS]
//	constraint-dependencies = ["apache-airflow==3.3.2+astro.1", "apache-airflow-task-sdk==1.3.2+astro.1"]
//	[[tool.uv.index]]  name = "astronomer", explicit = true
//	[tool.uv.sources]  apache-airflow, -core, -task-sdk = { index = "astronomer" }
//	[tool.uv.exclude-newer-package]  the same three = false
//
// The index is explicit, so it serves only the packages a source names, and
// every other package still resolves from PyPI. Sources apply only to direct
// dependencies, which is why apache-airflow-core and apache-airflow-task-sdk
// are listed in [project] dependencies beside the requirement.
//
// environments limits the lockfile to the platforms a project runs on. Locking
// for every platform lets one package's Intel-macOS-only marker (shap's
// numba<0.63) drag a shared dependency back to a release whose sdist does not
// build.
//
// The runtime image's constraints also pin SQLAlchemy, but nothing published
// says to which release short of pulling the image, so that line is left out
// rather than guessed.

// astroIndexName names the [[tool.uv.index]] entry this file writes.
const astroIndexName = "astronomer"

// AstroEnvironments are the platforms an Astro project's lockfile resolves
// for when the project names none: Linux, which the runtime image is, and
// Apple silicon macOS. Intel macOS is left out, and runs in Docker mode.
var AstroEnvironments = []string{
	"sys_platform == 'linux'",
	"sys_platform == 'darwin' and platform_machine == 'arm64'",
}

var (
	constraintsKey  = []string{"tool", "uv", "constraint-dependencies"}
	environmentsKey = []string{"tool", "uv", "environments"}
	indexKey        = []string{"tool", "uv", "index"}
	sourcesKey      = []string{"tool", "uv", "sources"}
	excludeNewerPkg = []string{"tool", "uv", "exclude-newer-package"}
	projectDepsKey  = []string{"project", "dependencies"}
	astroDists      = []string{runtimeversions.DistAirflow, runtimeversions.DistCore, runtimeversions.DistTaskSDK}
)

// SetAstroBuild writes b into the manifest in dir, through EditManifest: the
// [tool.uv] settings above, and the two distributions sources need listed. A
// zero b takes out what an earlier one wrote, for an Airflow Astronomer has no
// build of, which then comes from PyPI. It leaves every other [tool.uv] key,
// and every comment, as it was.
func SetAstroBuild(dir string, wrap func(run func() error) error, b runtimeversions.AstroBuild) error {
	return EditManifest(dir, wrap, func(before *manifest.Manifest, ed tomledit.Editor) error {
		return setAstroBuild(before, ed, b)
	})
}

func setAstroBuild(before *manifest.Manifest, ed tomledit.Editor, b runtimeversions.AstroBuild) error {
	dists := b.Dists()
	if b.Airflow == "" {
		dists = nil
	}
	if err := checkNotTheUsers(ed, dists); err != nil {
		return err
	}
	if err := setAstroPins(ed, b, dists); err != nil {
		return err
	}
	if err := setAstroSources(ed, dists); err != nil {
		return err
	}
	if err := setAstroIndex(ed, b); err != nil {
		return err
	}
	for _, d := range astroDists {
		key := append(slices.Clone(excludeNewerPkg), d)
		if v, ok := ed.Get(key); ok && v == false && !slices.Contains(dists, d) {
			ed.Delete(key)
		}
	}
	if v, ok := ed.Get(excludeNewerPkg); ok {
		if m, _ := v.(map[string]any); len(m) == 0 {
			ed.Delete(excludeNewerPkg)
		}
	}
	// Airflow 2 has no core or Task SDK distribution: a project moved back to
	// it loses the bare entries a build of Airflow 3 added, build or no build.
	if before.Airflow().Major() == "2" {
		dropBareDists(ed, runtimeversions.DistCore, runtimeversions.DistTaskSDK)
	}
	if len(dists) == 0 {
		// The platforms were narrowed for the build's lockfile; without one,
		// the ones this wrote go too, and a project's own stay.
		if v, ok := ed.Get(environmentsKey); ok && reflect.DeepEqual(v, asAny(AstroEnvironments)) {
			ed.Delete(environmentsKey)
		}
		return nil
	}
	if err := addDirectDists(before, ed, dists); err != nil {
		return err
	}
	if _, ok := ed.Get(environmentsKey); !ok {
		if err := ed.Set(environmentsKey, asAny(AstroEnvironments)); err != nil {
			return err
		}
	}
	// Astronomer's index shows upload times only in its page text, which uv
	// does not read, so under any
	// exclude-newer, the project's own or a UV_EXCLUDE_NEWER in the
	// environment, uv would treat every build as too new.
	for _, d := range dists {
		if err := setIfDifferent(ed, append(slices.Clone(excludeNewerPkg), d), false); err != nil {
			return err
		}
	}
	return nil
}

// ErrNotAstrosToWrite reports [tool.uv] settings the project wrote itself
// where SetAstroBuild would write its own: an index named "astronomer" at
// another address, or a source for a rebuilt distribution that names another
// index. They are left alone, and nothing is written.
var ErrNotAstrosToWrite = errors.New("pyproject.toml already sets this itself")

// checkNotTheUsers refuses to take over settings the project wrote.
func checkNotTheUsers(ed tomledit.Editor, dists []string) error {
	current, _ := ed.Get(indexKey)
	entries, _ := current.([]any)
	for _, e := range entries {
		entry, _ := e.(map[string]any)
		if url, _ := entry["url"].(string); entry["name"] == astroIndexName && url != runtimeversions.PublicIndexURL() {
			return fmt.Errorf("%w: [[tool.uv.index]] %q points at %s, not %s; rename it to let astro hold Airflow to Astronomer's build",
				ErrNotAstrosToWrite, astroIndexName, url, runtimeversions.PublicIndexURL())
		}
	}
	for _, d := range dists {
		current, ok := ed.Get(append(slices.Clone(sourcesKey), d))
		if ok && !reflect.DeepEqual(current, map[string]any{"index": astroIndexName}) {
			return fmt.Errorf("%w: [tool.uv.sources] %s names its own source; remove it to let astro hold Airflow to Astronomer's build",
				ErrNotAstrosToWrite, d)
		}
	}
	return nil
}

// setAstroPins replaces the constraints an earlier build wrote with b's,
// keeping the project's own. A constraint is the build's when it names one of
// the rebuilt distributions at an +astro version.
func setAstroPins(ed tomledit.Editor, b runtimeversions.AstroBuild, dists []string) error {
	current, _ := ed.Get(constraintsKey)
	list, _ := current.([]any)
	var kept []any
	for _, v := range list {
		if s, _ := v.(string); manifest.AstroPin(s) {
			continue
		}
		kept = append(kept, v)
	}
	if len(dists) > 0 {
		kept = append(kept, asAny(b.Pins())...)
	}
	if reflect.DeepEqual(kept, list) {
		return nil
	}
	if len(kept) == 0 {
		ed.Delete(constraintsKey)
		return nil
	}
	return ed.Set(constraintsKey, kept)
}

// setAstroSources points each rebuilt distribution at the index, and takes
// the index off one this build does not cover.
func setAstroSources(ed tomledit.Editor, dists []string) error {
	for _, d := range astroDists {
		key := append(slices.Clone(sourcesKey), d)
		current, _ := ed.Get(key)
		ours := reflect.DeepEqual(current, map[string]any{"index": astroIndexName})
		if slices.Contains(dists, d) {
			if !ours {
				if err := ReplaceTable(ed, key, map[string]any{"index": astroIndexName}); err != nil {
					return err
				}
			}
			continue
		}
		if ours {
			ed.Delete(key)
		}
	}
	if sources, ok := ed.Get(sourcesKey); ok {
		if m, isTable := sources.(map[string]any); isTable && len(m) == 0 {
			ed.Delete(sourcesKey)
		}
	}
	return nil
}

// setAstroIndex adds the index entry, or corrects its url, and removes it
// when b is zero.
func setAstroIndex(ed tomledit.Editor, b runtimeversions.AstroBuild) error {
	current, _ := ed.Get(indexKey)
	entries, _ := current.([]any)
	at := -1
	for i, e := range entries {
		if entry, _ := e.(map[string]any); entry["name"] == astroIndexName {
			at = i
		}
	}
	if b.Airflow == "" {
		if at >= 0 {
			ed.Delete(append(slices.Clone(indexKey), strconv.Itoa(at)))
			if len(entries) == 1 {
				ed.Delete(indexKey)
			}
		}
		return nil
	}
	values := map[string]any{"name": astroIndexName, "url": b.Index, "explicit": true}
	switch {
	case at >= 0:
	case len(entries) > 0 && ed.Set(append(slices.Clone(indexKey), strconv.Itoa(len(entries))), values) == nil:
		// An inline array, index = [{...}], takes the entry whole.
		return nil
	default:
		var err error
		if at, err = ed.AppendArrayTable(indexKey); err != nil {
			return err
		}
	}
	entry := append(slices.Clone(indexKey), strconv.Itoa(at))
	for _, kv := range []struct {
		key   string
		value any
	}{{"name", astroIndexName}, {"url", b.Index}, {"explicit", true}} {
		if err := setIfDifferent(ed, append(slices.Clone(entry), kv.key), kv.value); err != nil {
			return err
		}
	}
	return nil
}

// addDirectDists lists the rebuilt distributions the project does not name,
// bare: the requirement already states the version, and the pins hold them to
// the build.
func addDirectDists(before *manifest.Manifest, ed tomledit.Editor, dists []string) error {
	named := map[string]bool{}
	for _, dep := range before.Project.Dependencies {
		named[manifest.DistName(dep)] = true
	}
	current, _ := ed.Get(projectDepsKey)
	list, _ := current.([]any)
	n := len(list)
	for _, d := range dists {
		// apache-airflow itself is not added beside an apache-airflow-core
		// requirement: that project chose the distribution without providers.
		if named[d] || d == runtimeversions.DistAirflow {
			continue
		}
		if err := ed.Set(append(slices.Clone(projectDepsKey), strconv.Itoa(n)), d); err != nil {
			return err
		}
		n++
	}
	return nil
}

// dropBareDists removes the dependencies that name one of dists with no
// version, last first so the indexes still hold.
func dropBareDists(ed tomledit.Editor, dists ...string) {
	current, _ := ed.Get(projectDepsKey)
	deps, _ := current.([]any)
	for i := len(deps) - 1; i >= 0; i-- {
		spec, _ := deps[i].(string)
		if slices.Contains(dists, strings.TrimSpace(spec)) {
			ed.Delete(append(slices.Clone(projectDepsKey), strconv.Itoa(i)))
		}
	}
}

func setIfDifferent(ed tomledit.Editor, key []string, value any) error {
	if current, ok := ed.Get(key); ok && reflect.DeepEqual(current, value) {
		return nil
	}
	return ed.Set(key, value)
}
