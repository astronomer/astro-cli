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
func adopt(dir string, data []byte, opts Options, v1 *v1Project, res *Result) (out []byte, labels []string, pin manifestFacts, err error) {
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
	version, defaulted := pickAirflowVersion(opts.AirflowVersion, deps, v1)
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

	// requirements.txt merges into the dependencies already declared, by
	// distribution name. A manifest that names a package is the authority on how
	// it is pinned: the author wrote that specifier, and a requirements.txt in
	// the same repo is the thing being retired, so "add what is missing" is the
	// only merge that cannot silently change a pin someone chose.
	migrated, err := mergeDependencies(ed, v1.dependencies, pin.dynamicDeps, &pin.migrationNotes)
	if err != nil {
		return nil, nil, pin, err
	}

	added, err := ensureAirflowDependency(ed, version, pin.dynamicDeps)
	if err != nil {
		return nil, nil, pin, err
	}
	// packages.txt is carried unconditionally, and there is no "unless one is
	// already there" case to handle.
	//
	// There was one, and it was dead code. A [tool.astro].packages key cannot
	// exist without [tool.astro] existing, and a manifest carrying that table
	// has already been refused above with ErrAlreadyAstroProject — so the guard
	// could never run, and the test named for it exercised [tool.other] and
	// asserted the list WAS carried. A branch no input can reach, described by a
	// comment claiming it was a deliberate decision.
	migratedPackages := false
	if len(v1.packages) > 0 {
		if err := ed.Set([]string{"tool", "astro", "packages"}, asAny(v1.packages)); err != nil {
			return nil, nil, pin, err
		}
		migratedPackages = true
	}
	if err := ed.Set([]string{"tool", "astro", manifestKeyAirflow}, version); err != nil {
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
	if migrated > 0 {
		labels = append(labels, manifest.Marker+" (migrated "+strconv.Itoa(migrated)+" from requirements.txt into dependencies)")
	}
	if migratedPackages {
		labels = append(labels, manifest.Marker+" (migrated packages.txt into packages)")
	}
	return out, labels, pin, nil
}

// mergeDependencies appends the requirements a manifest does not already name,
// and reports how many it added.
//
// Matching is by distribution name, not by the whole specifier, because the
// point is to avoid declaring the same package twice — PEP 621 has no rule that
// forbids it, and pip resolves duplicates by intersecting them, so two entries
// for one package is a resolution puzzle rather than an error. Anything already
// named is left exactly as the manifest's author wrote it.
//
// A manifest declaring dependencies dynamic carries nothing: PEP 621 forbids a
// static array beside it, so the requirements stay where they are and the
// existing dynamic-deps note covers it.
func mergeDependencies(ed tomledit.Editor, reqs []string, dynamic bool, extras *[]string) (int, error) {
	if dynamic || len(reqs) == 0 {
		return 0, nil
	}
	existing, ok := ed.Get([]string{"project", "dependencies"})
	have := map[string]bool{}
	// Which names the MANIFEST already had, kept apart from the ones this loop
	// adds, because the two produce different notes: one is the manifest
	// outranking the file, the other is the file contradicting itself.
	fromManifest := map[string]bool{}
	for _, d := range asStrings(existing) {
		have[distName(d)] = true
		fromManifest[distName(d)] = true
	}

	var add []string
	for _, r := range reqs {
		name := distName(r)
		// Airflow is not carried here: ensureAirflowDependency writes the
		// requirement that matches the pin, and this would race it.
		if name == airflowDist {
			// Where the pin came from, so ensureAirflowDependency writes the
			// equivalent requirement. Its extras are not equivalent, though.
			*extras = append(*extras, airflowExtrasNote(r)...)
			continue
		}
		if have[name] {
			// Dropped, and said so. "Anything already named is left exactly as
			// the manifest's author wrote it" is a defensible merge rule only
			// while the losing specifier survives somewhere — and it does not,
			// once requirements.txt is retired. Naming the file is also what
			// keeps planRetirements from deleting it.
			if fromManifest[name] {
				*extras = append(*extras, "requirements.txt: "+r+
					" was not carried, because pyproject.toml already pins "+name+" and the manifest wins")
			} else {
				*extras = append(*extras, "requirements.txt: "+r+
					" names a distribution listed earlier in the same file, so this specifier was not carried")
			}
			continue
		}
		have[name] = true
		add = append(add, r)
	}
	if len(add) == 0 {
		return 0, nil
	}

	// No dependencies key at all: write the whole array. Otherwise append, and
	// the index to append at is the length as it stands now — the same reason
	// ensureAirflowDependency re-reads.
	if !ok {
		if err := ed.Set([]string{"project", "dependencies"}, asAny(add)); err != nil {
			return 0, err
		}
		return len(add), nil
	}
	at := len(asStrings(existing))
	for i, r := range add {
		if err := ed.Set([]string{"project", "dependencies", strconv.Itoa(at + i)}, r); err != nil {
			return 0, err
		}
	}
	return len(add), nil
}

// asAny turns a string list into the []any a TOML array wants.
func asAny(list []string) []any {
	out := make([]any, len(list))
	for i, s := range list {
		out[i] = s
	}
	return out
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
