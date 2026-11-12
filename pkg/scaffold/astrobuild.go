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
//	[[tool.uv.index]]  name = "astronomer", default = true
//
// The index is the project's default: it serves Astronomer's builds and
// passes every other package through from PyPI, so no source is needed. A
// project with a default index of its own (a mirror) keeps that one; there the
// index is explicit, each Airflow distribution gets a source naming it, and
// apache-airflow-core and apache-airflow-task-sdk are listed in [project]
// dependencies beside the requirement, since sources apply only to direct
// dependencies.
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
const astroIndexName = manifest.AstronomerIndexName

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
	return SetAstroBuildWith(dir, wrap, b, nil)
}

// RuntimeConstraints is what an Astro Runtime image's own install holds
// packages to (the runtime's constraints file), for one build and Python.
// Lines are its name==version lines as published.
type RuntimeConstraints struct {
	Build, Python string
	Lines         []string
}

// SetAstroBuildWith is SetAstroBuild that also writes rc: rc's lines other
// than the Airflow distributions' (which b's pins hold), under one comment
// recording rc's build and Python (manifest.RuntimeRecord), in place of the
// lines and the record an earlier rc wrote. A nil rc leaves those as they
// are; a zero one takes them out. Where the array cannot hold a comment (an
// inline [tool.uv]), no record and no lines are written.
func SetAstroBuildWith(dir string, wrap func(run func() error) error, b runtimeversions.AstroBuild, rc *RuntimeConstraints) error {
	_, err := SetAstroBuildOpts(dir, wrap, b, AstroBuildOptions{Constraints: rc})
	return err
}

// AstroBuildOptions are SetAstroBuildOpts's choices beyond the build.
type AstroBuildOptions struct {
	// Constraints is SetAstroBuildWith's rc.
	Constraints *RuntimeConstraints
	// MoveToDefaultIndex lets the write make Astronomer's index the project's
	// default where an earlier astro wrote it explicit, with a source per
	// Airflow distribution. Without it a project keeps the layout it has, so a
	// start never rewrites every package's registry in uv.lock unasked.
	MoveToDefaultIndex bool
	// HeedMachineSettings makes the settings on this machine (MachineIndexReason)
	// keep the index explicit, as the project's own (ProjectIndexReason)
	// always do. Init and the upgrade commands set it: they are runs the user
	// expects to change files. A start leaves it off, so one machine's
	// environment never rewrites a committed pyproject.toml.
	HeedMachineSettings bool
}

// The layouts of Astronomer's index: the [[tool.uv.index]] key that marks
// the entry.
const (
	AstroIndexDefault  = "default"
	AstroIndexExplicit = "explicit"
)

// AstroIndex is how the write left Astronomer's index.
type AstroIndex struct {
	// Layout is AstroIndexDefault, AstroIndexExplicit, or "" when no entry
	// was written (an Airflow Astronomer has no build of).
	Layout string
	// Reason says why the index is explicit when ProjectIndexReason or
	// MachineIndexReason gave one; "" when the project only kept the explicit
	// layout it had.
	Reason string
	// Moved reports that an explicit index became the default, which moves
	// every package's registry in uv.lock to Astronomer's index.
	Moved bool
}

// SetAstroBuildOpts is SetAstroBuildWith with opts, and says how it left
// Astronomer's index.
func SetAstroBuildOpts(dir string, wrap func(run func() error) error, b runtimeversions.AstroBuild, opts AstroBuildOptions) (AstroIndex, error) {
	var out AstroIndex
	err := EditManifest(dir, wrap, func(before *manifest.Manifest, ed tomledit.Editor) error {
		var err error
		out, err = setAstroBuild(dir, before, ed, b, opts)
		return err
	})
	return out, err
}

func setAstroBuild(dir string, before *manifest.Manifest, ed tomledit.Editor, b runtimeversions.AstroBuild, opts AstroBuildOptions) (AstroIndex, error) {
	rc := opts.Constraints
	dists := b.Dists()
	if b.Airflow == "" {
		dists = nil
	}
	if err := checkNotTheUsers(ed, dists); err != nil {
		return AstroIndex{}, err
	}
	if err := setAstroPins(ed, b, dists, rc); err != nil {
		return AstroIndex{}, err
	}
	// With Astronomer's index as the default, every package resolves from it
	// and no source is needed. It stays explicit, each Airflow distribution
	// routed to it, where another index could answer for Airflow first
	// (reason), and where an earlier astro wrote it explicit and the caller
	// did not ask to move it. The bare core and Task SDK entries a routed
	// layout listed stay when it moves: a project may have listed them itself.
	// The reason is looked for only when the entry is written and the layout
	// is not already decided: a start is a hot path.
	was := astroIndexLayout(ed)
	keep := was == AstroIndexExplicit && !opts.MoveToDefaultIndex
	reason := ""
	if b.Airflow != "" && !keep {
		reason = explicitReason(dir, ed, opts)
	}
	routed := reason != "" || keep
	if err := setAstroSources(ed, routedDists(routed, dists)); err != nil {
		return AstroIndex{}, err
	}
	if err := setAstroIndex(ed, b, routed); err != nil {
		return AstroIndex{}, err
	}
	out := AstroIndex{Layout: astroIndexLayout(ed)}
	if out.Layout == AstroIndexExplicit {
		out.Reason = reason
	}
	out.Moved = was == AstroIndexExplicit && out.Layout == AstroIndexDefault
	dropExcludeNewerOverrides(ed)
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
		return out, nil
	}
	if err := addDirectDists(before, ed, routedDists(routed, dists)); err != nil {
		return out, err
	}
	if _, ok := ed.Get(environmentsKey); !ok {
		if err := ed.Set(environmentsKey, asAny(AstroEnvironments)); err != nil {
			return out, err
		}
	}
	return out, nil
}

// explicitReason is why Astronomer's index must be explicit: what the project
// holds, and, for a run the user asked for, what this machine has.
func explicitReason(dir string, ed tomledit.Editor, opts AstroBuildOptions) string {
	if reason := ProjectIndexReason(dir, ed); reason != "" || !opts.HeedMachineSettings {
		return reason
	}
	return MachineIndexReason()
}

// dropExcludeNewerOverrides takes out the exclude-newer-package entries an
// earlier astro wrote, and the table once they leave it empty.
func dropExcludeNewerOverrides(ed tomledit.Editor) {
	// They were false for the Airflow distributions, when Astronomer's index
	// pages carried no upload times. They do now, so a cutoff applies to the
	// builds as it does to PyPI's files; the override goes where it is exactly
	// what was written, and a project's own entries stay.
	for _, d := range astroDists {
		key := append(slices.Clone(excludeNewerPkg), d)
		if v, ok := ed.Get(key); ok && v == false {
			ed.Delete(key)
		}
	}
	if v, ok := ed.Get(excludeNewerPkg); ok {
		if m, _ := v.(map[string]any); len(m) == 0 {
			ed.Delete(excludeNewerPkg)
		}
	}
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

// setAstroPins replaces the constraints an earlier build wrote with b's, and,
// when rc is not nil, the runtime's record and lines with rc's, keeping the
// project's own where they were. A constraint is the build's when it names
// one of the rebuilt distributions at an +astro version; the runtime's are the
// record comment and the exact pins right after it of a distribution the
// runtime constrains (manifest.SplitRuntimeBlock).
func setAstroPins(ed tomledit.Editor, b runtimeversions.AstroBuild, dists []string, rc *RuntimeConstraints) error {
	list, _ := tomledit.GetLines(ed, constraintsKey)
	own, block := manifest.SplitRuntimeBlock(list, rc.owns)
	var kept []any
	for _, v := range own {
		if s, _ := v.(string); manifest.AstroPin(s) {
			continue
		}
		kept = append(kept, v)
	}
	if len(dists) > 0 {
		kept = append(kept, asAny(b.Pins())...)
	}
	switch {
	case rc == nil:
		kept = append(kept, block...)
	case rc.Build != "":
		kept = append(kept, runtimeBlock(*rc)...)
	}
	if reflect.DeepEqual(kept, list) {
		return nil
	}
	if len(tomledit.Values(kept)) == 0 {
		ed.Delete(constraintsKey)
		return nil
	}
	err := tomledit.SetLines(ed, constraintsKey, kept)
	if errors.Is(err, tomledit.ErrCommentsUnsupported) {
		// An inline [tool.uv] holds no comment, so no record: without one
		// the lines could not be told from the project's own later.
		rest, _ := manifest.SplitRuntimeBlock(kept, rc.owns)
		return tomledit.SetLines(ed, constraintsKey, tomledit.Values(rest))
	}
	return err
}

// owns reports a distribution the runtime's block may hold: one a runtime's
// constraints file names besides Airflow's (manifest.RuntimeConstrained), or
// one rc's own lines name. A nil rc owns the former alone.
func (rc *RuntimeConstraints) owns(dist string) bool {
	if manifest.RuntimeConstrained(dist) {
		return true
	}
	if rc == nil {
		return false
	}
	for _, line := range rc.Lines {
		if manifest.DistName(line) == manifest.DistName(dist) && !slices.Contains(astroDists, manifest.DistName(dist)) {
			return true
		}
	}
	return false
}

// runtimeBlock is the record comment and rc's lines other than the Airflow
// distributions', which the build's +astro pins hold.
func runtimeBlock(rc RuntimeConstraints) []any {
	block := []any{tomledit.Comment(manifest.RuntimeRecord(rc.Build, rc.Python))}
	for _, line := range rc.Lines {
		if slices.Contains(astroDists, manifest.DistName(line)) {
			continue
		}
		block = append(block, line)
	}
	return block
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

// astroIndexLayout is how the project's [[tool.uv.index]] entry for
// Astronomer's index is marked: AstroIndexDefault, AstroIndexExplicit, or ""
// with no entry.
func astroIndexLayout(ed tomledit.Editor) string {
	current, _ := ed.Get(indexKey)
	entries, _ := current.([]any)
	for _, e := range entries {
		entry, _ := e.(map[string]any)
		if entry["name"] != astroIndexName {
			continue
		}
		if isDefault, _ := entry[AstroIndexDefault].(bool); isDefault {
			return AstroIndexDefault
		}
		return AstroIndexExplicit
	}
	return ""
}

// routedDists is dists when the Airflow distributions are routed to an
// explicit index, and none when the index is the default.
func routedDists(routed bool, dists []string) []string {
	if routed {
		return dists
	}
	return nil
}

// setAstroIndex adds the index entry, or corrects its url, and removes it
// when b is zero. The entry is the default index unless routed, when the
// project has a default of its own and the entry is explicit.
func setAstroIndex(ed tomledit.Editor, b runtimeversions.AstroBuild, routed bool) error {
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
	kind, other := AstroIndexDefault, AstroIndexExplicit
	if routed {
		kind, other = other, kind
	}
	values := map[string]any{"name": astroIndexName, "url": b.Index, kind: true}
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
	}{{"name", astroIndexName}, {"url", b.Index}, {kind, true}} {
		if err := setIfDifferent(ed, append(slices.Clone(entry), kv.key), kv.value); err != nil {
			return err
		}
	}
	if _, ok := ed.Get(append(slices.Clone(entry), other)); ok {
		ed.Delete(append(slices.Clone(entry), other))
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
