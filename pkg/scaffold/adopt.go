package scaffold

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
)

// Adopting a pyproject.toml that is already there is what lets `astro init` run
// in the repo the dags already live in. The edit is surgical: it adds
// [tool.astro], fills in the [project] keys packaging requires, adds the
// Airflow dependency when nothing pins one, and leaves the rest of the file —
// its comments, its key order, its other tools' settings — untouched.

// adopt returns the manifest already in dir with [tool.astro] added. It writes
// nothing: Run puts every file on disk, so a failure here leaves the manifest
// as its author left it. It refuses only a manifest carrying the section
// already — that directory is an Astro project, and re-initializing it would
// overwrite the pin.
func adopt(dir string, data []byte, opts Options, res *Result) (out []byte, labels []string, pin manifestFacts, err error) {
	path := filepath.Join(dir, manifest.Marker)
	ed, err := tomledit.NewSurgical(data)
	if err != nil {
		return nil, nil, pin, fmt.Errorf("parsing %s: %w", path, err)
	}
	if _, ok := ed.Get([]string{"tool", "astro"}); ok {
		return nil, nil, pin, fmt.Errorf("%s %w; edit that manifest instead of re-initializing", dir, ErrAlreadyAstroProject)
	}
	// [project] and [tool.astro] are what make the directory an Astro project,
	// so they lead the manifest the way [project] leads a pyproject.toml
	// everywhere else, rather than trailing every tool's own section.
	if err := ed.EnsureTablesAtTop([][]string{{"project"}, {"tool", "astro"}}); err != nil {
		return nil, nil, pin, err
	}

	deps := asStrings(mustGet(ed, "project", "dependencies"))
	version, defaulted := resolveAirflowVersion(opts.AirflowVersion, deps)
	pin = manifestFacts{
		defaultedPin: defaulted,
		// A manifest naming Airflow without a clean == pin — a range, a
		// wildcard — states a version this cannot read, so the default that
		// lands instead may move the project a whole generation.
		namesAirflow: defaulted && pinsAirflow(deps),
		dynamicDeps:  slices.Contains(asStrings(mustGet(ed, "project", "dynamic")), "dependencies"),
	}

	// The manifest's own validation requires a [project] name, and a repo that
	// was never a Python package often has no [project] table at all.
	if name := opts.Name; name != "" {
		if err := ed.Set([]string{"project", "name"}, name); err != nil {
			return nil, nil, pin, err
		}
	} else if v, ok := ed.Get([]string{"project", "name"}); !ok || v == "" {
		if err := ed.Set([]string{"project", "name"}, deriveName(dir)); err != nil {
			return nil, nil, pin, err
		}
	}
	if err := ensureProjectVersion(ed); err != nil {
		return nil, nil, pin, err
	}

	added, err := ensureAirflowDependency(ed, version, pin.dynamicDeps)
	if err != nil {
		return nil, nil, pin, err
	}
	if err := ed.Set([]string{"tool", "astro", "airflow"}, version); err != nil {
		return nil, nil, pin, err
	}

	out, err = ed.Bytes()
	if err != nil {
		return nil, nil, pin, err
	}
	m, err := manifest.Parse(out)
	if err != nil {
		return nil, nil, pin, withPath(err, path)
	}

	res.Name = m.Project.Name
	res.AirflowVersion = version
	res.Adopted = true
	// Returned rather than appended to a list beside the change: these lines
	// describe the manifest write, so they belong to it.
	labels = []string{manifest.Marker + " (added [tool.astro])"}
	if added != "" {
		labels = append(labels, manifest.Marker+" (added "+added+" to dependencies)")
	}
	return out, labels, pin, nil
}

// ensureAirflowDependency adds the requirement that installs the Airflow the
// manifest pins, and reports what it added. A manifest that pins no Airflow
// cannot start, so this is the same guarantee the greenfield path gives.
//
// A manifest listing dependencies under project.dynamic supplies them from
// somewhere else — a requirements.txt, through a build backend — and PEP 621
// forbids a static array beside it, so that manifest keeps its own arrangement
// and the pin goes on the hand-off list instead.
func ensureAirflowDependency(ed tomledit.Editor, version string, dynamic bool) (added string, err error) {
	if dynamic {
		return "", nil
	}
	// Re-read the array: its length is the index an append writes to, so it
	// has to be the length as it stands now, not as it stood earlier.
	deps, ok := ed.Get([]string{"project", "dependencies"})
	if list := asStrings(deps); pinsAirflow(list) {
		return "", nil
	} else if ok {
		err = ed.Set([]string{"project", "dependencies", strconv.Itoa(len(list))}, airflowRequirement(version))
	} else {
		err = ed.Set([]string{"project", "dependencies"}, []any{airflowRequirement(version)})
	}
	if err != nil {
		return "", err
	}
	return airflowRequirement(version), nil
}

// defaultProjectVersion is what a manifest with no version gets. It is a
// placeholder for packaging's sake: an Astro project is deployed, not
// published, so nothing reads it back.
const defaultProjectVersion = "0.1.0"

// ensureProjectVersion writes [project] version when the manifest has none.
// PEP 621 requires one beside the name, and uv refuses to build the
// environment without it, while this package's own validation does not ask for
// it — so a manifest missing it would parse here and fail at the first
// `astro local start`. A version listed in project.dynamic is supplied by a
// build backend at build time, which satisfies PEP 621, so that manifest is
// left alone.
func ensureProjectVersion(ed tomledit.Editor) error {
	if v, ok := ed.Get([]string{"project", "version"}); ok && v != "" {
		return nil
	}
	if slices.Contains(asStrings(mustGet(ed, "project", "dynamic")), "version") {
		return nil
	}
	return ed.Set([]string{"project", "version"}, defaultProjectVersion)
}

// withPath names the manifest in a validation or parse failure. Both error
// shapes carry the path once it is set, so setting it beats wrapping — a wrap
// would name pyproject.toml twice.
func withPath(err error, path string) error {
	var parse *manifest.ParseError
	var invalid *manifest.ValidationError
	switch {
	case errors.As(err, &parse):
		parse.Path = path
	case errors.As(err, &invalid):
		invalid.Path = path
	}
	return err
}

// mustGet returns the value at key, or nil when the key is absent, for the
// reads whose only question is what the value holds.
func mustGet(ed tomledit.Editor, key ...string) any {
	v, _ := ed.Get(key)
	return v
}

// asStrings returns the string members of a decoded TOML array, ignoring any
// entry of another type.
func asStrings(v any) []string {
	list, _ := v.([]any)
	out := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
