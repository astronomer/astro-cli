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
// them as constraints added to the project's pyproject.toml for that one lock
// (SeedManifest), and uv keeps every version it chose as the lock's
// preferences from then on. None of them stays in pyproject.toml; the
// manifest records only which build and Python the lock was seeded from
// (Seed, manifest.SeedRecord).
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

// seeds are the pins a seed holds the lock to, name==version, sorted by
// distribution: the pins less the ones it never states, a package the
// project gives a version the image's does not meet (own), which is its to
// choose; Airflow, the Task SDK and core, by name, which the Airflow build
// pins hold; and any other of Astronomer's own builds (an +astro local
// version), which come from an index the project reads for Airflow alone.
// The one filter for what a seed states and what drift counts (SeededPins).
func (p *RuntimePins) seeds(own projectRequirements) []string {
	var out []string
	for _, pin := range p.Pins {
		name := manifest.DistName(pin)
		if own.excludes(pin) || slices.Contains(astroDists, name) || strings.Contains(pin, "+astro.") {
			continue
		}
		out = append(out, pin)
	}
	slices.SortStableFunc(out, func(a, b string) int {
		return strings.Compare(manifest.DistName(a), manifest.DistName(b))
	})
	return out
}

// constraints are the seeds as the seed states them, each limited to the
// image's Python: a lockfile resolves for every Python requires-python
// allows, and the image's versions are of one (numpy 2.3 needs Python 3.11,
// so held on every Python it leaves a project that allows 3.10 nothing to
// lock there).
func (p *RuntimePins) constraints(own projectRequirements) []string {
	seeds := p.seeds(own)
	for i, pin := range seeds {
		seeds[i] = pin + "; python_version == '" + p.Python + "'"
	}
	return seeds
}

// Seed is the build and the Python a lock was seeded from, which
// SetAstroBuild records in the manifest (manifest.SeedRecord). A zero Seed
// takes the record out.
type Seed struct {
	Build, Python string
}

// AddSeedConstraints writes pins's constraints into the manifest in dir for
// the one lock that seeds the project's uv.lock: appended to [tool.uv]
// constraint-dependencies, after what is there, between the heading and end
// comments an older astro wrote its runtime pins between (legacyPins). So
// marked, constraints a killed process leaves behind are never read as the
// project's own, and the next write of the manifest takes them out. The
// caller puts the manifest back as it was once the lock is resolved
// (RestoreManifest). The write is EditManifest's, so a symlinked manifest
// stays a link and a read-only one is refused.
//
// n is how many constraints it added; none, with nothing written, means there
// is nothing to seed. A [tool.uv] written as an inline table cannot carry the
// comments, and is refused (tomledit.ErrCommentsUnsupported).
func AddSeedConstraints(dir string, wrap func(run func() error) error, pins *RuntimePins) (n int, err error) {
	err = EditManifest(dir, wrap, func(_ *manifest.Manifest, ed tomledit.Editor) error {
		lines, _ := tomledit.GetLines(ed, constraintsKey)
		added := pins.constraints(ownRequirements(ed, projectConstraints(lines)))
		if n = len(added); n == 0 {
			return nil
		}
		out := slices.Clone(legacyPins(lines))
		out = append(out, tomledit.Comment(seedingHeading(pins)))
		for _, c := range added {
			out = append(out, c)
		}
		out = append(out, tomledit.Comment(legacyPinsEnd))
		return setConstraints(ed, out)
	})
	if err != nil {
		return 0, err
	}
	return n, nil
}

// seedingHeading is the comment AddSeedConstraints opens its constraints
// with. It starts as an older astro's runtime pins heading does
// (legacyPinsHeadRe), so legacyPins reads what it heads as astro's.
func seedingHeading(pins *RuntimePins) string {
	return "Astro Runtime " + pins.Build + " (Python " + pins.Python + ") ships these versions; astro seeds uv.lock with them and takes them out again."
}

// SetSeedRecord records seed in the manifest in dir, as SetAstroBuildSeed
// does, and changes nothing else but taking out what an older astro wrote
// into constraint-dependencies: for a start that cannot reach Astronomer's
// index, which leaves the Airflow build as it is, and for the record of a
// seed once the lock holds it. nil keeps the record there. refused reports a
// record not written because the array cannot hold a comment (a [tool.uv]
// written as an inline table).
func SetSeedRecord(dir string, wrap func(run func() error) error, seed *Seed) (refused bool, err error) {
	err = EditManifest(dir, wrap, func(_ *manifest.Manifest, ed tomledit.Editor) error {
		lines, _ := tomledit.GetLines(ed, constraintsKey)
		var astroPins []any
		for _, v := range lines {
			if s, isString := v.(string); isString && manifest.AstroPin(s) {
				astroPins = append(astroPins, s)
			}
		}
		var editErr error
		refused, editErr = writeConstraints(ed, lines, withoutAstroManaged(lines), astroPins, seed)
		return editErr
	})
	return refused, err
}

// writeConstraints writes constraint-dependencies as the project's own lines,
// then astroPins, then the seed record: seed's when seed is not nil (none for a
// zero Seed), else the one lines holds. lines is the array as it stands.
// refused reports a record left out because the array cannot carry a comment
// (tomledit.ErrCommentsUnsupported); the rest is written.
func writeConstraints(ed tomledit.Editor, lines, own, astroPins []any, seed *Seed) (refused bool, err error) {
	var record []any
	switch {
	case seed == nil:
		record = seedRecordIn(lines)
	case seed.Build != "":
		record = []any{tomledit.Comment(manifest.SeedRecord(seed.Build, seed.Python))}
	}
	out := slices.Concat(own, astroPins, record)
	if reflect.DeepEqual(out, lines) {
		return false, nil
	}
	err = setConstraints(ed, out)
	if errors.Is(err, tomledit.ErrCommentsUnsupported) && len(record) > 0 {
		return true, setConstraints(ed, slices.Concat(own, astroPins))
	}
	return false, err
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
// the seed record, and the runtime pins an older astro wrote (legacyPins).
// What is left is the project's own, comments included.
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
// it lists in [project] dependencies, [project.optional-dependencies],
// [dependency-groups] and the [tool.uv] dev-dependencies older uv wrote, and
// its own constraint-dependencies; and the packages [tool.uv]
// override-dependencies names, which replace any other requirement.
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

// ownRequirements reads projectRequirements out of the manifest open in ed,
// with constraints, the project's own constraint-dependencies.
func ownRequirements(ed tomledit.Editor, constraints []any) projectRequirements {
	r := projectRequirements{requirements: map[string][]string{}, overrides: map[string]bool{}}
	addAll := func(list []any) {
		for _, v := range list {
			// A dependency group can include another, as a table; that one is
			// read where it is defined.
			if s, isString := v.(string); isString {
				name := manifest.DistName(s)
				r.requirements[name] = append(r.requirements[name], s)
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
		for _, group := range groups {
			list, _ := group.([]any)
			addAll(list)
		}
	}
	addAll(constraints)
	overrides, _ := ed.Get(overrideDepsKey)
	list, _ := overrides.([]any)
	for _, v := range list {
		if s, isString := v.(string); isString {
			r.overrides[manifest.DistName(s)] = true
		}
	}
	return r
}

// SeededPins are the pins a seed of the project in dir holds its lock to,
// name==version: the same filter the seed states them with (seeds), so a
// lock that differs from the image elsewhere differs by the project's own
// choice, which drift does not count. None when the manifest cannot be read.
func SeededPins(dir string, pins *RuntimePins) []string {
	src, err := os.ReadFile(filepath.Join(dir, manifest.Marker))
	if err != nil {
		return nil
	}
	ed, err := tomledit.NewSurgical(src)
	if err != nil {
		return nil
	}
	lines, _ := tomledit.GetLines(ed, constraintsKey)
	return pins.seeds(ownRequirements(ed, projectConstraints(lines)))
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
