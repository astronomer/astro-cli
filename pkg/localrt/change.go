package localrt

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// ChangeAction is what a running project needs after its manifest changed.
type ChangeAction string

const (
	// ChangeNone means nothing the running Airflow reads changed: a link, a
	// target, the project name.
	ChangeNone ChangeAction = "none"
	// ChangeHotInstall means the change is new requirements, and
	// Runtime.HotInstall of Change.Install makes them live without a restart.
	ChangeHotInstall ChangeAction = "hot-install"
	// ChangeRestart means the change reaches something only a start reads: the
	// Airflow version, the Python, how uv resolves, the env, the image, or a
	// package already loaded into the running Airflow.
	ChangeRestart ChangeAction = "restart"
)

// Change is ClassifyChange's answer.
type Change struct {
	Action ChangeAction
	// Install are the requirements to hot install, as the new manifest writes
	// them: every requirement of each distribution the new manifest added.
	// Empty unless Action is ChangeHotInstall.
	Install []string
	// Removed are requirements the new manifest no longer declares. A hot
	// install leaves them in the environment, since uninstalling under a live
	// scheduler is the reconcile HotInstall exists to avoid; they go at the
	// next restart. Reported whatever the Action, so a caller can say so.
	Removed []string
	// Reasons say why Action is ChangeRestart, one per thing that forced it,
	// in manifest order. Empty otherwise.
	Reasons []string
}

// ClassifyChange decides what a project running in mode needs after its
// manifest went from before to after: nothing, a hot install, or a restart.
//
// It lives here rather than with a file watcher because the answer is a
// property of the engines, not of whoever noticed the edit. The CLI's watch
// loop and Astro Desktop both ask it, and two copies of "what can be installed
// into a live Airflow" would drift until one of them swapped Airflow under a
// running scheduler.
//
// The rule is conservative. Only a distribution the old manifest did not name
// at all is hot installed: re-pinning one that is already installed makes uv
// replace it under the live scheduler, which may already have imported it,
// and a downgrade cannot be told from an upgrade without resolving. Anything
// that changes how the environment is resolved, rather than what it contains,
// is a restart too: a new constraint, override or index could make every
// installed package wrong, and a hot install only ever looks at the
// requirements it is handed.
//
// Requirements are compared by distribution and in a normalized form, so
// reordering them, changing a name's case or separators, or spacing them
// differently is not a change.
//
// before and after are manifests that loaded. A watcher whose file stops
// parsing keeps the last one that did; a nil either side is answered with a
// restart rather than a guess.
//
// Pure, and blind to the filesystem: a Dockerfile edited in place is not a
// manifest change, so a watcher that sees one asks for a restart itself.
func ClassifyChange(before, after *manifest.Manifest, mode Mode) Change {
	if before == nil || after == nil {
		return Change{Action: ChangeRestart, Reasons: []string{"the manifest could not be compared with the running one"}}
	}
	if mode == "" {
		mode = ModeStandalone
	}
	var c Change
	restart := func(why string) { c.Reasons = append(c.Reasons, why) }

	if before.Project.RequiresPython != after.Project.RequiresPython {
		restart("requires-python changed")
	}
	d := diffDependencies(before.Project.Dependencies, after.Project.Dependencies)
	c.Removed = d.removed
	if d.airflow {
		restart("the Airflow requirement changed")
	}
	if mode == ModeDocker {
		if len(d.added) > 0 || len(d.changed) > 0 || len(d.removed) > 0 {
			// The packages live in the image, so changing them is a rebuild.
			restart("dependencies changed, and docker mode installs them into the image")
		}
	} else {
		c.Reasons = append(c.Reasons, notHotInstallable(&d, &after.UV)...)
	}

	if !uvEqual(&before.UV, &after.UV) {
		restart("[tool.uv] changed how dependencies resolve")
	}
	if !reflect.DeepEqual(before.Astro.Env, after.Astro.Env) {
		restart("[tool.astro.env] changed")
	} else if len(after.Astro.Env) > 0 && !sameWorkspace(&before.Astro, &after.Astro) {
		// A source = "workspace" value is read from the workspace these name,
		// so a re-link changes the values without touching the env table.
		restart("[tool.astro] workspace, domain or organization changed, which env values are read from")
	}
	if !reflect.DeepEqual(before.Astro.Pools, after.Astro.Pools) {
		restart("[tool.astro.pools] changed")
	}
	if mode == ModeDocker {
		// Standalone has no image: it warns about packages and ignores the
		// Dockerfile, the runtime build and its secrets.
		b, a := &before.Astro, &after.Astro
		if !slices.Equal(b.Packages, a.Packages) {
			restart("[tool.astro] packages changed")
		}
		if b.Dockerfile != a.Dockerfile {
			restart("[tool.astro] dockerfile changed")
		}
		if b.Runtime != a.Runtime {
			restart("[tool.astro] runtime changed")
		}
		if !slices.Equal(b.BuildSecrets, a.BuildSecrets) {
			restart("[tool.astro] build-secrets changed")
		}
	}

	switch {
	case len(c.Reasons) > 0:
		c.Action = ChangeRestart
	case len(d.added) > 0:
		c.Action = ChangeHotInstall
		c.Install = d.added
	default:
		c.Action = ChangeNone
	}
	return c
}

// notHotInstallable are the reasons a standalone project's dependency changes
// cannot be hot installed: a re-pin of a package the running Airflow already
// has, and an added package the project sources from somewhere uv pip cannot
// see, since uv pip install reads no [tool.uv] table and would fetch whatever
// public package has that name instead.
func notHotInstallable(d *depDiff, uv *manifest.UV) []string {
	var reasons []string
	for _, dist := range d.changedDists {
		if namesAirflowRuntime(dist) {
			continue // the Airflow requirement is its own reason
		}
		reasons = append(reasons, fmt.Sprintf("the requirement on %s changed, and it is already installed in the running Airflow", dist))
	}
	for _, dist := range d.addedDists {
		if hasSource(uv, dist) {
			reasons = append(reasons, fmt.Sprintf("%s comes from [tool.uv.sources], which a hot install cannot read", dist))
		}
	}
	return reasons
}

// depDiff is diffDependencies' answer. added, changed and removed hold
// requirements as written; addedDists and changedDists the distributions
// behind the first two, sorted.
type depDiff struct {
	added, changed, removed  []string
	addedDists, changedDists []string
	// airflow reports that any of the three names Airflow itself.
	airflow bool
}

// diffDependencies compares two dependency lists by distribution: those only
// after names are added, those only before names are removed, and those both
// name but with different requirements are changed (as after writes them).
//
// By distribution rather than by line, so "pandas>=2" becoming "pandas>=2.2"
// is one changed requirement and not a removal beside an addition, and a
// distribution declared twice under different markers is compared as a whole.
func diffDependencies(before, after []string) depDiff {
	var d depDiff
	b, a := byDist(before), byDist(after)
	for _, dist := range slices.Sorted(maps.Keys(a)) {
		old, ok := b[dist]
		switch {
		case !ok:
			d.added = append(d.added, a[dist].written...)
			d.addedDists = append(d.addedDists, dist)
		case !slices.Equal(old.normalized, a[dist].normalized):
			d.changed = append(d.changed, a[dist].written...)
			d.changedDists = append(d.changedDists, dist)
		default:
			continue
		}
		d.airflow = d.airflow || namesAirflowRuntime(dist)
	}
	for _, dist := range slices.Sorted(maps.Keys(b)) {
		if _, ok := a[dist]; ok {
			continue
		}
		d.removed = append(d.removed, b[dist].written...)
		d.airflow = d.airflow || namesAirflowRuntime(dist)
	}
	return d
}

// distReqs are one distribution's requirements, as written and in the form
// they are compared in, each sorted by its normalized form.
type distReqs struct {
	written, normalized []string
}

// byDist groups requirements by distribution name, each group sorted so the
// order they are written in is not a change.
func byDist(reqs []string) map[string]distReqs {
	type req struct{ written, normalized string }
	groups := map[string][]req{}
	for _, r := range reqs {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		dist := manifest.DistName(r)
		groups[dist] = append(groups[dist], req{r, normalizeRequirement(r)})
	}
	out := make(map[string]distReqs, len(groups))
	for dist, rs := range groups {
		slices.SortFunc(rs, func(x, y req) int { return strings.Compare(x.normalized, y.normalized) })
		var g distReqs
		for _, r := range rs {
			g.written = append(g.written, r.written)
			g.normalized = append(g.normalized, r.normalized)
		}
		out[dist] = g
	}
	return out
}

// normalizeRequirement is a PEP 508 requirement in a form that two spellings
// of one requirement share: the name normalized as PEP 503 does, and the
// whitespace outside quoted marker values dropped, since PEP 508 gives it no
// meaning. "Pandas >= 2.1" and "pandas>=2.1" come out alike. It is a key to
// compare by, not a requirement to install.
func normalizeRequirement(req string) string {
	req = strings.TrimSpace(req)
	name := manifest.DistName(req)
	var rest string
	if i := strings.IndexAny(req, "[ \t<>=!~;@("); i >= 0 {
		rest = req[i:]
	}
	var sb strings.Builder
	sb.WriteString(name)
	var quote rune
	for _, r := range rest {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
		case r == ' ' || r == '\t':
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// namesAirflowRuntime reports a distribution that is part of the running
// Airflow: the two that state its version, and the task SDK pinned beside
// them. Installing a new one of any of them under a live scheduler is
// replacing Airflow, which is a restart. Providers are ordinary packages.
func namesAirflowRuntime(dist string) bool {
	return manifest.NamesAirflow(dist) || dist == "apache-airflow-task-sdk"
}

// hasSource reports whether [tool.uv.sources] names dist, by index or by URL.
// The tables are keyed as the project writes the name, so both sides are
// normalized.
func hasSource(uv *manifest.UV, dist string) bool {
	for _, m := range []map[string]string{uv.Sources, uv.DirectURLs} {
		for pkg := range m {
			if manifest.DistName(pkg) == dist {
				return true
			}
		}
	}
	// A source the typed fields skipped (a path, a workspace member, an index
	// the table does not define) is still a source uv pip will not see.
	sources, _ := uv.Raw["sources"].(map[string]any)
	for pkg := range sources {
		if manifest.DistName(pkg) == dist {
			return true
		}
	}
	return false
}

// sameWorkspace reports whether two [tool.astro] tables name one workspace to
// read env values from.
func sameWorkspace(b, a *manifest.Astro) bool {
	return b.Workspace == a.Workspace && b.Domain == a.Domain && b.Organization == a.Organization
}

// uvEqual reports whether two [tool.uv] tables resolve alike.
//
// The whole table is compared, not the fields astro reads: uv has many
// settings that change a resolution (override-dependencies, the indexes,
// prerelease, resolution, find-links) and a list of them here would fall
// behind uv's. The runtime record is compared besides, since it is written
// as comments the table does not carry.
func uvEqual(b, a *manifest.UV) bool {
	return b.RuntimeBuild == a.RuntimeBuild &&
		b.RuntimePython == a.RuntimePython &&
		reflect.DeepEqual(normalizeUV(b.Raw), normalizeUV(a.Raw))
}

// uvSetKeys are the [tool.uv] lists whose order uv does not read: each is a
// set of requirements or markers. The indexes are not among them, since uv
// tries those in the order they are written.
var uvSetKeys = []string{
	"constraint-dependencies", "override-dependencies", "build-constraint-dependencies",
	"dev-dependencies", "environments", "required-environments",
}

// normalizeUV is a [tool.uv] table with its set-like lists sorted and their
// requirements normalized, so reordering or respacing them is not a change.
// An empty table and none compare alike.
func normalizeUV(t map[string]any) map[string]any {
	if len(t) == 0 {
		return nil
	}
	out := maps.Clone(t)
	for _, k := range uvSetKeys {
		list, ok := t[k].([]any)
		if !ok {
			continue
		}
		norm := make([]string, 0, len(list))
		for _, v := range list {
			s, ok := v.(string)
			if !ok {
				// Not the shape uv documents: compare it as written.
				norm = nil
				break
			}
			norm = append(norm, normalizeRequirement(s))
		}
		if norm != nil {
			slices.Sort(norm)
			out[k] = norm
		}
	}
	return out
}
