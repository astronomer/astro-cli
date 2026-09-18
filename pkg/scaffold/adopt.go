package scaffold

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

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
// setProjectName gives an adopted manifest a [project] name, and reports what
// to tell somebody when the name it ends up with is not the one their v1
// project stated.
//
// A manifest that states a name keeps it: it has already said what the project
// is called, and --name is the only thing that overrules that. Everything else
// goes through chooseName, so the order --name, then the v1 config, then the
// directory is written in one place rather than half here.
func setProjectName(ed tomledit.Editor, dir string, opts Options, v1 *v1Project) (advisory string, err error) {
	raw, has := ed.Get([]string{"project", "name"})
	stated, _ := raw.(string)
	if opts.Name == "" && has && stated != "" {
		// Said twice and differently is worth one line. The manifest wins, and
		// somebody who has only ever seen the v1 name should not have to work
		// out where it went.
		if v1 != nil && v1.projectName != "" && sanitizeName(v1.projectName) != stated {
			return "kept the name " + stated + " from " + manifest.Marker +
				", not " + v1.projectName + " from " + v1ConfigRelPath, nil
		}
		return "", nil
	}
	chosen, advisory := chooseName(dir, opts, v1)
	if err := ed.Set([]string{"project", "name"}, chosen); err != nil {
		return "", err
	}
	return advisory, nil
}

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
		pinUnread:    pinnedPastTheManifest(deps, defaulted, opts.AirflowVersion != ""),
		// A manifest naming Airflow without a clean == pin — a range —
		// states a version this cannot read, so the default that lands
		// instead may move the project a whole generation. A wildcard is
		// read now, so it no longer reaches this.
		namesAirflow: defaulted && pinsAirflow(deps),
		dynamicDeps:  slices.Contains(asStrings(mustGet(ed, "project", "dynamic")), "dependencies"),
	}

	// The manifest's own validation requires a [project] name, and a repo that
	// was never a Python package often has no [project] table at all.
	nameAdvisory, err := setProjectName(ed, dir, opts, v1)
	if err != nil {
		return nil, nil, pin, err
	}
	pin.nameAdvisory = nameAdvisory
	// Read before ensureProjectKeys, which fills a missing one: after it, an
	// absent key and one this run just wrote look the same.
	pin.loosePython = statedPythonTooLoose(ed, version)
	requiresPythonLabel, err := ensureProjectKeys(ed, version)
	if err != nil {
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
	// A repo that already had a pyproject.toml can have a Dockerfile too, and
	// it is load-bearing there for the same reason it is in the greenfield arm.
	// Both arms declare it or the field would only be true of projects that
	// arrived one particular way.
	if err := setDockerfileDeclaration(ed, v1); err != nil {
		return nil, nil, pin, err
	}
	if err := setEnvDeclarations(ed, v1.envSchema); err != nil {
		return nil, nil, pin, err
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
	// Reported because it constrains the project: it decides which interpreters
	// uv may build the environment with, and a preview that leaves it out shows
	// a review with the most restrictive key missing.
	labels = appendLabel(labels, requiresPythonLabel)
	// Same reason as the greenfield arm's: this is the key that decides whether
	// the image is generated or built from the user's own file, so a preview
	// without it hides the most consequential thing the run did.
	if declaresDockerfile(v1) {
		labels = append(labels, manifest.Marker+" (declared "+fileDockerfile+" as this project's build)")
	}
	if v1.envSchema.declares() {
		labels = append(labels, manifest.Marker+" (migrated "+migratedFrom(v1)+" into [tool.astro.env])")
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
	at := arrayLen(existing)
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
	switch {
	case pinsAirflow(asStrings(deps)):
		return "", nil
	case ok:
		// arrayLen, not the count of readable entries: an index one short
		// overwrites the last element instead of following it.
		err = ed.Set([]string{"project", "dependencies", strconv.Itoa(arrayLen(deps))}, airflowRequirement(version))
	default:
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

// ensureProjectKeys fills the [project] keys this run is responsible for and
// the manifest does not already state. Both are "fill a gap, never overwrite":
// what the author wrote is what they meant.
//
// It returns a label for what it wrote, empty when it wrote nothing worth
// reporting. requires-python is reportable because it decides which
// interpreters the project may ever use; the version placeholder has no
// behavioral consequence, so it stays quiet.
func ensureProjectKeys(ed tomledit.Editor, version string) (label string, err error) {
	if err := ensureProjectVersion(ed); err != nil {
		return "", err
	}
	wrote, err := ensureRequiresPython(ed, version)
	if err != nil || !wrote {
		return "", err
	}
	return manifest.Marker + " (set requires-python to " + requiresPython(version) + ")", nil
}

// statedPythonTooLoose reports that the manifest already states a
// requires-python which still lets uv pick an interpreter the pinned Airflow
// cannot run under.
//
// "No upper bound" is the whole test, deliberately. Comparing PEP 440
// specifiers properly means implementing them, and the only case that matters
// is the common one: an author who wrote ">=3.9" years ago, against an Airflow
// 2 that stops at 3.11 or 3.12. A bound that is merely wrong — "<3.14" on an
// Airflow 2 — is rare enough to leave to the reader, and saying nothing about
// it is better than a comparison this package would get subtly wrong.
func statedPythonTooLoose(ed tomledit.Editor, version string) bool {
	if major, _, _ := strings.Cut(version, "."); major != "2" {
		return false
	}
	v, ok := ed.Get([]string{"project", "requires-python"})
	if !ok {
		return false
	}
	stated, _ := v.(string)
	return stated != "" && !strings.Contains(stated, "<")
}

// appendLabel adds a label unless it is empty, so a caller assembling a list
// does not need a branch per optional entry.
func appendLabel(labels []string, label string) []string {
	if label == "" {
		return labels
	}
	return append(labels, label)
}

// ensureRequiresPython states which interpreters the project supports, when
// the manifest does not already.
//
// Only when it does not. A requires-python its author wrote is their decision
// and this run cannot overrule it — but a manifest stating none leaves uv free
// to build the venv on the newest Python present, and for an Airflow 2 pin
// that is one Airflow 2 cannot run under. See requiresPython for what that
// failure looks like.
// It reports whether it wrote, so the run can say so: this key decides which
// interpreters the project may ever use, and a change performed but unreported
// cannot be reviewed.
func ensureRequiresPython(ed tomledit.Editor, version string) (wrote bool, err error) {
	if v, ok := ed.Get([]string{"project", "requires-python"}); ok && v != "" {
		return false, nil
	}
	// Declared dynamic means a build backend supplies it, and PEP 621 forbids
	// stating a dynamic field statically — writing one turns a buildable
	// project into one that errors at build time. ensureProjectVersion guards
	// the same way for the same reason.
	if slices.Contains(asStrings(mustGet(ed, "project", "dynamic")), "requires-python") {
		return false, nil
	}
	if err := ed.Set([]string{"project", "requires-python"}, requiresPython(version)); err != nil {
		return false, err
	}
	return true, nil
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

// arrayLen is how many elements a TOML array holds — the index an append has
// to write to.
//
// Not len(asStrings(v)), which is what both append sites used. asStrings drops
// anything that is not a string, so an array holding one non-string entry
// reported a length one short, the "append" landed on the last element and
// replaced it, and `astro init` silently deleted a dependency from the user's
// manifest — no error, and nothing on the hand-off list.
//
// An array like that is malformed for PEP 621, which requires strings, so it
// takes a hand-edited file to reach. That makes it rare rather than harmless:
// the run destroys something it could not read, which is the one thing a
// conversion must never do.
func arrayLen(v any) int {
	list, _ := v.([]any)
	return len(list)
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
