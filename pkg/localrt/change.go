package localrt

import (
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
	// ChangeHotInstall means the change is new or re-pinned requirements, and
	// Runtime.HotInstall of Change.Install makes it live without a restart.
	ChangeHotInstall ChangeAction = "hot-install"
	// ChangeRestart means the change reaches something only a start reads: the
	// Airflow version, the Python, how uv resolves, the env, the image.
	ChangeRestart ChangeAction = "restart"
)

// Change is ClassifyChange's answer.
type Change struct {
	Action ChangeAction
	// Install are the requirements to hot install, as the new manifest writes
	// them: every requirement of each distribution that was added or whose
	// requirement changed. Empty unless Action is ChangeHotInstall.
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
// The rule is conservative: anything that changes how the environment is
// resolved, rather than what it contains, is a restart. A new constraint or
// index could make every installed package wrong, and a hot install only ever
// looks at the requirements it is handed.
//
// Pure, and blind to the filesystem: a Dockerfile edited in place is not a
// manifest change, so a watcher that sees one asks for a restart itself.
func ClassifyChange(before, after *manifest.Manifest, mode Mode) Change {
	if mode == "" {
		mode = ModeStandalone
	}
	var c Change
	restart := func(why string) { c.Reasons = append(c.Reasons, why) }

	if before.Project.RequiresPython != after.Project.RequiresPython {
		restart("requires-python changed")
	}
	install, removed, airflow := diffDependencies(before.Project.Dependencies, after.Project.Dependencies)
	c.Removed = removed
	if airflow {
		restart("the Airflow requirement changed")
	}
	if mode == ModeDocker && (len(install) > 0 || len(removed) > 0) {
		// The packages live in the image, so changing them is a rebuild.
		restart("dependencies changed, and docker mode installs them into the image")
	}

	if !uvEqual(&before.UV, &after.UV) {
		restart("[tool.uv] changed how dependencies resolve")
	}
	if !reflect.DeepEqual(before.Astro.Env, after.Astro.Env) {
		restart("[tool.astro.env] changed")
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
	case len(install) > 0:
		c.Action = ChangeHotInstall
		c.Install = install
	default:
		c.Action = ChangeNone
	}
	return c
}

// diffDependencies compares two dependency lists by distribution, reporting
// the requirements of every distribution added or changed (from after), those
// of every distribution dropped (from before), and whether any of either names
// Airflow itself.
//
// By distribution rather than by line, so "pandas>=2" becoming "pandas>=2.2"
// is one changed requirement and not a removal beside an addition, and a
// distribution declared twice under different markers is compared as a whole.
func diffDependencies(before, after []string) (install, removed []string, airflow bool) {
	b, a := byDist(before), byDist(after)
	for _, dist := range slices.Sorted(maps.Keys(a)) {
		if slices.Equal(b[dist], a[dist]) {
			continue
		}
		install = append(install, a[dist]...)
		airflow = airflow || namesAirflowRuntime(dist)
	}
	for _, dist := range slices.Sorted(maps.Keys(b)) {
		if _, ok := a[dist]; ok {
			continue
		}
		removed = append(removed, b[dist]...)
		airflow = airflow || namesAirflowRuntime(dist)
	}
	return install, removed, airflow
}

// byDist groups requirements by distribution name, each group sorted so the
// order they are written in is not a change.
func byDist(reqs []string) map[string][]string {
	out := make(map[string][]string, len(reqs))
	for _, r := range reqs {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		dist := manifest.DistName(r)
		out[dist] = append(out[dist], r)
	}
	for _, rs := range out {
		slices.Sort(rs)
	}
	return out
}

// namesAirflowRuntime reports a distribution that is part of the running
// Airflow: the two that state its version, and the task SDK pinned beside
// them. Installing a new one of any of them under a live scheduler is
// replacing Airflow, which is a restart. Providers are ordinary packages.
func namesAirflowRuntime(dist string) bool {
	return manifest.NamesAirflow(dist) || dist == "apache-airflow-task-sdk"
}

// uvEqual reports whether two [tool.uv] tables resolve alike.
func uvEqual(b, a *manifest.UV) bool {
	return slices.Equal(b.ConstraintDependencies, a.ConstraintDependencies) &&
		slices.Equal(b.Environments, a.Environments) &&
		maps.Equal(b.Sources, a.Sources) &&
		maps.Equal(b.DirectURLs, a.DirectURLs) &&
		b.RuntimeBuild == a.RuntimeBuild &&
		b.RuntimePython == a.RuntimePython &&
		slices.Equal(b.RuntimeConstraints, a.RuntimeConstraints) &&
		b.ExcludeNewer == a.ExcludeNewer
}
