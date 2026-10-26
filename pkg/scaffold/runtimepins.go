package scaffold

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// RuntimePins are the versions the image of one Astro Runtime build ships,
// which a project's uv.lock is seeded with: the lock is resolved once with
// them as constraints, in a scratch copy of the project (SeedManifest), and uv
// keeps every version it chose as the lock's preferences from then on. None
// of them is written into pyproject.toml; the manifest records only which
// build and Python the lock was seeded from (Seed, manifest.SeedRecord), as
// the first comment of the astro-runtime dependency group (RuntimeGroup).
//
// Seeded so, every package the project's lockfile shares with the image is at
// the image's version on the image's Python, and standalone mode, `uv run`
// and the lockfile a build installs from all resolve what a deployment runs.
// A constraint adds no package, so the image's packages the project does not
// use stay out of the lock.
type RuntimePins struct {
	// Build and Python are the image's: "3.3-9" and "3.12".
	Build, Python string
	// Pins are the image's distributions, name==version, as the CLI's
	// runtimecatalog.RuntimePins reads them from the build's freeze. None
	// means there is nothing to seed, and the record of an earlier seed is
	// taken out.
	Pins []string
}

// constraints are the pins as the scratch copy of a seed states them, less
// the ones it never states: a package the project gives a version the
// image's does not meet (own), which is its to choose; Airflow, the Task SDK
// and core, which the Airflow build pins hold; and any other of Astronomer's
// own builds (an +astro local version), which come from an index the project
// reads for Airflow alone. Each is limited to the image's Python: a lockfile
// resolves for every Python requires-python allows, and the image's versions
// are of one (numpy 2.3 needs Python 3.11, so held on every Python it leaves
// a project that allows 3.10 nothing to lock there).
func (p *RuntimePins) constraints(own projectRequirements) []string {
	var out []string
	for _, pin := range p.Pins {
		name := manifest.DistName(pin)
		if own.excludes(pin) || slices.Contains(astroDists, name) || strings.Contains(pin, "+astro.") {
			continue
		}
		out = append(out, pin+"; python_version == '"+p.Python+"'")
	}
	slices.SortStableFunc(out, func(a, b string) int {
		return strings.Compare(manifest.DistName(a), manifest.DistName(b))
	})
	return out
}

// Seed is the build and the Python a lock was seeded from, which
// SetAstroBuild records in the manifest (manifest.SeedRecord), as the first
// comment of the astro-runtime dependency group. A zero Seed takes the record
// out.
type Seed struct {
	Build, Python string
}

// ErrSeedWorkspace reports a project that is a uv workspace, whose lock
// covers members a scratch copy of the root does not have.
var ErrSeedWorkspace = errors.New("this project is a uv workspace, whose lock astro does not seed")

// SeedManifest is the manifest in dir as the scratch copy that seeds its lock
// resolves it: pyproject.toml with pins's constraints added to [tool.uv]
// constraint-dependencies, and every [tool.uv.sources] path made absolute,
// since the copy sits elsewhere. n is how many constraints it added; none
// means there is nothing to seed. The project's own file is not touched.
func SeedManifest(dir string, pins *RuntimePins) (out []byte, n int, err error) {
	src, err := os.ReadFile(filepath.Join(dir, manifest.Marker))
	if err != nil {
		return nil, 0, err
	}
	ed, err := tomledit.NewSurgical(src)
	if err != nil {
		return nil, 0, err
	}
	if _, ok := ed.Get([]string{"tool", "uv", "workspace"}); ok {
		return nil, 0, ErrSeedWorkspace
	}
	lines, _ := tomledit.GetLines(ed, constraintsKey)
	project := projectConstraints(lines)
	added := pins.constraints(ownRequirements(ed, project))
	if len(added) == 0 {
		return nil, 0, nil
	}
	values, _ := ed.Get(constraintsKey)
	current, _ := values.([]any)
	for _, c := range added {
		current = append(current, c)
	}
	// The copy is thrown away, so the comments the array holds need not be
	// kept: a plain Set of the values writes them as uv reads them.
	if err := ed.Set(constraintsKey, current); err != nil {
		return nil, 0, err
	}
	if err := absoluteSourcePaths(ed, dir); err != nil {
		return nil, 0, err
	}
	out, err = ed.Bytes()
	return out, len(added), err
}

// absoluteSourcePaths makes every relative path a [tool.uv.sources] entry
// names absolute against dir. A source given as a list of alternatives is
// left as it is; a lock of it may then fail in the copy, which leaves the
// project's lock unseeded rather than wrong.
func absoluteSourcePaths(ed tomledit.Editor, dir string) error {
	sources, _ := ed.Get(sourcesKey)
	table, _ := sources.(map[string]any)
	for pkg, v := range table {
		entry, _ := v.(map[string]any)
		path, isString := entry["path"].(string)
		if !isString || filepath.IsAbs(path) {
			continue
		}
		key := append(slices.Clone(sourcesKey), pkg, "path")
		if err := ed.Set(key, filepath.ToSlash(filepath.Join(dir, filepath.FromSlash(path)))); err != nil {
			return err
		}
	}
	return nil
}

// SetRuntimeGroup writes group as the astro-runtime dependency group of the
// manifest in dir and seed as its record, as SetAstroBuild does, and changes
// nothing else but taking out what an older astro wrote into
// constraint-dependencies: for a start that cannot reach Astronomer's index,
// which leaves the Airflow build as it is, and for the record of a seed once
// the lock holds it. nil group keeps the group less what the project now
// lists; nil seed keeps the record. refused reports a group not written
// because its table cannot hold the comments that head it (an inline
// [dependency-groups]).
func SetRuntimeGroup(dir string, wrap func(run func() error) error, group *RuntimeGroup, seed *Seed) (refused bool, err error) {
	err = EditManifest(dir, wrap, func(_ *manifest.Manifest, ed tomledit.Editor) error {
		lines, _ := tomledit.GetLines(ed, constraintsKey)
		var astroPins []any
		for _, v := range lines {
			if s, isString := v.(string); isString && manifest.AstroPin(s) {
				astroPins = append(astroPins, s)
			}
		}
		if err := writeConstraints(ed, lines, withoutAstroManaged(lines), astroPins); err != nil {
			return err
		}
		var editErr error
		refused, editErr = setRuntimeGroup(ed, group, seed)
		return editErr
	})
	return refused, err
}

// writeConstraints writes constraint-dependencies as the project's own lines,
// then astroPins. lines is the array as it stands.
func writeConstraints(ed tomledit.Editor, lines, own, astroPins []any) error {
	out := slices.Concat(own, astroPins)
	if reflect.DeepEqual(out, lines) {
		return nil
	}
	return setConstraints(ed, out)
}

// seedRecordIn is the seed record among lines, as a one-line list, or none.
func seedRecordIn(lines []any) []any {
	for _, l := range lines {
		if c, isComment := l.(tomledit.Comment); isComment {
			if _, _, ok := manifest.IsSeedRecord(string(c)); ok {
				return []any{c}
			}
		}
	}
	return nil
}

// withoutAstroManaged is constraint-dependencies without what astro writes
// there: the constraints that hold Airflow to a build (manifest.AstroPin),
// and what an older astro wrote there, a seed record and the runtime pins
// (legacyPins). What is left is the project's own, comments included.
func withoutAstroManaged(lines []any) []any {
	var out []any
	for _, v := range legacyPins(lines) {
		switch v := v.(type) {
		case string:
			if manifest.AstroPin(v) {
				continue
			}
		case tomledit.Comment:
			if _, _, ok := manifest.IsSeedRecord(string(v)); ok {
				continue
			}
		}
		out = append(out, v)
	}
	return out
}

// projectConstraints are the project's own constraints among lines, the
// values withoutAstroManaged leaves.
func projectConstraints(lines []any) []any {
	var out []any
	for _, v := range withoutAstroManaged(lines) {
		if s, isString := v.(string); isString {
			out = append(out, s)
		}
	}
	return out
}

// An older astro wrote the runtime pins themselves into constraint-dependencies,
// between a heading comment and an end comment:
//
//	# Astro Runtime 3.3-9 (Python 3.12) ships these versions, so the lockfile resolves to what the image runs.
//	# astro writes them. To choose a version yourself, give the package a bound the image's version doesn't meet.
//	"a2wsgi==1.10.10; python_version == '3.12'",
//	…
//	# End of what astro writes. Constraints below this line are yours.
//
// A seeded lock replaces them, and the next write takes them out.
var (
	legacyPinsHeadRe = regexp.MustCompile(`^Astro Runtime \S+ \(Python \d+\.\d+\) ships these versions`)
	legacyPinRe      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*==[^\s;,@]+(\s*;\s*python_version\s*==\s*['"]\d+\.\d+['"])?$`)
)

const legacyPinsEnd = "End of what astro writes. Constraints below this line are yours."

// legacyPins is lines without an older astro's runtime pins: from the heading
// comment to the end comment, or, with no end comment, through the comments
// after the heading and the name==version lines after them.
func legacyPins(lines []any) []any {
	start := slices.IndexFunc(lines, func(l any) bool {
		c, isComment := l.(tomledit.Comment)
		return isComment && legacyPinsHeadRe.MatchString(string(c))
	})
	if start < 0 {
		return lines
	}
	end := slices.IndexFunc(lines[start:], func(l any) bool {
		c, isComment := l.(tomledit.Comment)
		return isComment && string(c) == legacyPinsEnd
	})
	if end >= 0 {
		return slices.Concat(lines[:start], lines[start+end+1:])
	}
	i := start + 1
	for i < len(lines) {
		if _, isComment := lines[i].(tomledit.Comment); !isComment {
			break
		}
		i++
	}
	for i < len(lines) {
		if s, isString := lines[i].(string); !isString || !legacyPinRe.MatchString(strings.TrimSpace(s)) {
			break
		}
		i++
	}
	return slices.Concat(lines[:start], lines[i:])
}

// Where a project names packages beyond [project] dependencies: tables of
// lists (its extras, and its dependency groups, the dev group `uv add --dev`
// writes among them) and lists (uv's overrides, and the dev dependencies
// older uv wrote).
var (
	optionalDepsKey     = []string{"project", "optional-dependencies"}
	dependencyGroupsKey = []string{"dependency-groups"}
	overrideDepsKey     = []string{"tool", "uv", "override-dependencies"}
	uvDevDepsKey        = []string{"tool", "uv", "dev-dependencies"}
)

// projectRequirements are what the project says of the versions of the
// packages it names, by distribution (manifest.DistName): every requirement
// it lists (listedRequirements), and its own constraint-dependencies; and the
// packages [tool.uv] override-dependencies names, which replace any other
// requirement.
type projectRequirements struct {
	requirements map[string][]string
	overrides    map[string]bool
}

// excludes reports whether the project chooses a version of pin's package
// that pin's does not meet, so seeding with the pin would leave uv nothing
// to lock: an override of the package, or a requirement whose specifier
// excludes the pinned version (manifest.RequirementAdmits). "pandas" and
// "pandas>=2.3", as `uv add pandas` writes it, admit pandas 2.3.3 and keep
// the pin; "pandas>=3" leaves it out. A requirement that cannot be read for
// certain leaves it out too, which leaves uv to resolve as it would without it.
func (r projectRequirements) excludes(pin string) bool {
	name := manifest.DistName(pin)
	if r.overrides[name] {
		return true
	}
	head, _, _ := strings.Cut(pin, ";")
	_, version, ok := strings.Cut(head, "==")
	if !ok {
		return true
	}
	for _, req := range r.requirements[name] {
		if admits, known := manifest.RequirementAdmits(req, version); !known || !admits {
			return true
		}
	}
	return false
}

// listedRequirements are the requirements the project lists to install, by
// distribution: [project] dependencies, [project.optional-dependencies],
// [dependency-groups] but the astro-runtime group astro writes
// (RuntimeGroup), and the [tool.uv] dev-dependencies older uv wrote.
func listedRequirements(ed tomledit.Editor) map[string][]string {
	listed := map[string][]string{}
	addAll := func(list []any) {
		for _, v := range list {
			// A dependency group can include another, as a table; that one is
			// read where it is defined.
			if s, isString := v.(string); isString {
				name := manifest.DistName(s)
				listed[name] = append(listed[name], s)
			}
		}
	}
	for _, key := range [][]string{projectDepsKey, uvDevDepsKey} {
		value, _ := ed.Get(key)
		list, _ := value.([]any)
		addAll(list)
	}
	for _, key := range [][]string{optionalDepsKey, dependencyGroupsKey} {
		tables, _ := ed.Get(key)
		groups, _ := tables.(map[string]any)
		for name, group := range groups {
			// The runtime group is astro's list of what the image has, not the
			// project's: counted, it would leave every package in it to the
			// project to version, and out of the seed.
			if slices.Equal(key, dependencyGroupsKey) && name == manifest.RuntimeGroupName {
				continue
			}
			list, _ := group.([]any)
			addAll(list)
		}
	}
	return listed
}

// listedPackages are the distributions listedRequirements names.
func listedPackages(ed tomledit.Editor) map[string]bool {
	listed := map[string]bool{}
	for name := range listedRequirements(ed) {
		listed[name] = true
	}
	return listed
}

// ownRequirements reads projectRequirements out of the manifest open in ed,
// with constraints, the project's own constraint-dependencies.
func ownRequirements(ed tomledit.Editor, constraints []any) projectRequirements {
	r := projectRequirements{requirements: listedRequirements(ed), overrides: map[string]bool{}}
	for _, v := range constraints {
		if s, isString := v.(string); isString {
			name := manifest.DistName(s)
			r.requirements[name] = append(r.requirements[name], s)
		}
	}
	overrides, _ := ed.Get(overrideDepsKey)
	list, _ := overrides.([]any)
	for _, v := range list {
		if s, isString := v.(string); isString {
			r.overrides[manifest.DistName(s)] = true
		}
	}
	return r
}

// Excludes reports whether the project in dir chooses a version of pin's
// package (name==version) that pin's does not meet: the packages a seed
// leaves to the project, so a lock that differs from the image there differs
// by the project's own choice. false when the manifest cannot be read.
func Excludes(dir string, pins []string) map[string]bool {
	out := map[string]bool{}
	src, err := os.ReadFile(filepath.Join(dir, manifest.Marker))
	if err != nil {
		return out
	}
	ed, err := tomledit.NewSurgical(src)
	if err != nil {
		return out
	}
	lines, _ := tomledit.GetLines(ed, constraintsKey)
	own := ownRequirements(ed, projectConstraints(lines))
	for _, pin := range pins {
		if own.excludes(pin) {
			out[manifest.DistName(pin)] = true
		}
	}
	return out
}

// ErrNoRuntimeBuild reports a project whose image is no Astro Runtime build
// the catalog has: a declared Dockerfile on another image, or an Airflow pin
// no published build carries. Such a project's lock is not seeded.
var ErrNoRuntimeBuild = errors.New("this project builds on no Astro Runtime the catalog lists")

// errRuntimeUnknown reports a project whose build, or whose image's Python,
// cannot be told here: no catalog, or a Dockerfile whose FROM is built from a
// build argument or pinned by digest. Its lock and its record are left as
// they are.
var errRuntimeUnknown = errors.New("which Astro Runtime this project builds on cannot be told")

// RuntimeBuildFor is the Astro Runtime build a project's image is built from,
// and the Python that image runs: the build its lock is seeded from. It is
// named in one place, by the first of these that applies:
//
//   - a declared Dockerfile's FROM: its exact build, or for a floating tag
//     ("3.3", "12") the newest published build the tag serves; the Python
//     its tag names, or else the build's own;
//   - [tool.astro] runtime;
//   - the Airflow pin: the newest published build carrying a release the pin
//     covers, an exact patch when the pin names one. That is the release
//     runtimeversions.LookupAstroBuild holds Airflow to, so the seed and
//     the Airflow pin are of one release.
//
// For the last two the Python is the one the project's standalone venv and
// its generated image run (runtimeversions.ProjectPython, which the venv's
// StandalonePython and the image's RuntimeImageForPython both follow), so the
// seed holds the Python the project runs: for a patch pin, whose build is not
// the series' newest, that can be a Python the build is not the default of,
// and the caller reads the build's freeze for it. Where ProjectPython has no
// answer (an Airflow 2 pin, no requires-python), it is the build's own
// (BuildPython). python is "" when the build is known and its Python is not:
// an Airflow 3 build without a catalog.
//
// ErrNoRuntimeBuild means the project has no build; any other error means it
// cannot be told, as without a catalog (c nil) and no build named. One
// answer for every caller that asks which image a project builds on: the
// seed, and the Airflow lookup a declared Dockerfile's build names.
func RuntimeBuildFor(dir string, m *manifest.Manifest, c *runtimeversions.Catalog) (build, python string, err error) {
	if path := dockerfilePath(dir, m); path != "" {
		return dockerfileRuntimeBuild(airflowrt.ReadDeclaredBase(path), c)
	}
	airflow := m.Airflow()
	build = strings.TrimSpace(airflow.Runtime)
	if build == "" {
		if c == nil {
			return "", "", errRuntimeUnknown
		}
		var ok bool
		if build, ok = c.NewestPublishedRuntimeFor(airflow.Pin); !ok || airflow.Pin == "" {
			return "", "", ErrNoRuntimeBuild
		}
	}
	if c != nil {
		python, _, err := runtimeversions.ProjectPython(airflow.Pin, airflow.Runtime, m.Project.RequiresPython, func() *runtimeversions.Catalog { return c })
		if err == nil && python != "" {
			return build, python, nil
		}
	}
	return build, BuildPython(build, m.Project.RequiresPython, c), nil
}

// BuildPython is the Python a generated image of build runs for a project's
// requires-python (runtimeversions.Runtime.ImagePython): the build's default
// when requires-python admits it, else the newest it ships that it admits. An
// Airflow 2 build lists no Python, and runs its own. "" when it cannot be
// told, or the build ships none requires-python admits, which refuses an
// image of it.
func BuildPython(build, requiresPython string, c *runtimeversions.Catalog) string {
	if c != nil {
		if r, ok := c.Runtime(build); ok {
			if python, decided := r.ImagePython(requiresPython); decided {
				return python
			}
		}
	}
	return imagePython(build, "", c)
}

// dockerfileRuntimeBuild is RuntimeBuildFor for a declared Dockerfile's base.
func dockerfileRuntimeBuild(base airflowrt.DeclaredBase, c *runtimeversions.Catalog) (build, python string, err error) {
	if !base.Known || base.Tag == "" {
		return "", "", errRuntimeUnknown
	}
	version := base.RuntimeVersion()
	if version == "" {
		return "", "", ErrNoRuntimeBuild
	}
	tag, ok := manifest.ParseRuntimeTag(version)
	if !ok {
		return "", "", errRuntimeUnknown
	}
	build = version
	if !tag.Build {
		if c == nil {
			return "", "", errRuntimeUnknown
		}
		if build, ok = c.NewestPublishedRuntimeTagged(version); !ok {
			return "", "", ErrNoRuntimeBuild
		}
	}
	_, tagPython := airflowrt.ParseRuntimeTagPython(base.Tag)
	return build, imagePython(build, tagPython, c), nil
}

// imagePython is the Python the image of build runs: tagPython when its tag
// named one, else the build's own (runtimeversions.ImagePythonOf), which is ""
// for an Airflow 3 build without a catalog.
func imagePython(build, tagPython string, c *runtimeversions.Catalog) string {
	if tagPython != "" {
		return tagPython
	}
	return runtimeversions.ImagePythonOf(c, build)
}
