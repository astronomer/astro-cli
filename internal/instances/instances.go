// Package instances answers "what am I acting on?" for every Airflow-facing
// command. It builds the project's named instance set — the manifest's
// deployment links plus the local Airflows running on this machine — applies
// one precedence rule to pick a winner, and opens a pkg/airflowapi transport
// to it (docs/v2-instances.md).
//
// The two halves are deliberately separate. Building the set and picking a
// winner touch nothing but the inputs handed in, so `astro use` and
// `astro instance list` render them with no network call at all; only
// Instance.Transport reaches out, and only for the one instance a command acts
// on. Nothing here prints, exits, or reads config: the session, env, and
// coordinate lookups arrive through Deps, and every failure travels as an
// error naming its cause and its fix. The AWS and Google credential chains are
// the exception, resolved through their own SDKs — see Deps.
package instances

import (
	"path/filepath"
	"sort"
	"strconv"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// LocalName is the reserved name of this project's own local Airflow. It is
// never declared anywhere: `astro local start` makes it exist and
// `astro use local` points back at it. It is the manifest's own constant, so
// the name this resolves and the name the manifest refuses cannot drift apart.
const LocalName = manifest.ReservedLinkName

// EnvVar is the ephemeral layer of the resolution rule. It is an env var
// rather than CLI-managed session state because the shell already owns session
// lifetime — one exported value makes this terminal the prod terminal.
const EnvVar = "ASTRO_INSTANCE"

// Kind is what an instance points at: the four manifest link kinds, plus the
// local Airflow, which no manifest declares.
type Kind string

const (
	KindAstro         = Kind(manifest.KindAstro)
	KindMWAA          = Kind(manifest.KindMWAA)
	KindComposer      = Kind(manifest.KindComposer)
	KindEndpoint      = Kind(manifest.KindEndpoint)
	KindLocal    Kind = "local"
)

// Source is where an instance came from: declared in the committed manifest,
// or discovered from this machine's runtime records.
type Source string

const (
	SourceManifest Source = "manifest"
	SourceRunning  Source = "running"
	// SourceURL is the stateless --url target: declared nowhere, discovered
	// from nothing, alive only for the command that named it.
	SourceURL Source = "url"
)

// Instance is one named Airflow a command can act on.
type Instance struct {
	Name   string
	Kind   Kind
	Source Source
	// Where is the coordinate as the manifest writes it ("deployment clm2xk…",
	// "environment orders-prod") or the URL of something already addressable.
	// It is display only and costs no lookup, so an unresolved coordinate shows
	// exactly as written.
	Where string
	// URL is the base URL when it is already known — an endpoint link's own
	// url, a local Airflow's port. Empty for a link whose coordinates must be
	// looked up first, which Instance.Transport does.
	URL string
	// Link is the manifest link this instance came from, including its auth
	// table. Zero for a discovered local Airflow.
	Link manifest.Link
	// TargetConfig is the link's [tool.astro.targets.<target>] section, plain
	// data as the manifest carries it. It travels with the instance because a
	// link names only its environment: which region that MWAA environment is
	// in, and which project and location that Composer environment is in, are
	// facts about the backend rather than the link, and the resolver needs
	// both halves to reach the Airflow. Read it through TargetString. Nil for a
	// link whose target declares no section, and read-only — it is the
	// manifest's own map rather than a copy of it.
	TargetConfig map[string]any
	// Project is the project root of a discovered local Airflow, empty for a
	// manifest link.
	Project string
	// AirflowMajor is the generation a discovered local Airflow was started
	// for, from its runtime record.
	AirflowMajor string
	// Own marks an instance belonging to the project in front of the user: its
	// own manifest links and its own local Airflow. Another project's running
	// Airflow is addressable and listed, but never wins by default and is never
	// offered in the prompt — defaulting to someone else's Airflow is how you
	// act on the wrong one.
	Own bool
	// Problem is why this instance cannot be reached by name, empty when it
	// can. A listed instance with a problem is shown and explained rather than
	// dropped, so nothing on the machine goes missing without a word.
	Problem string
}

// Local is one running local Airflow, as the caller discovered it. It mirrors
// the fields of a localstate record (internal/localstate) that resolution
// needs; the caller passes only the ones whose runtime is actually alive,
// because "running local Airflow" is a layer of the rule and a dead record is
// not one. ProjectPath must already be canonical — the caller resolves it, so
// this package touches no filesystem.
type Local struct {
	ProjectPath string
	Port        int
	// AirflowMajor is the generation this runtime was started for, from its
	// record. Empty on a record written before the field existed.
	AirflowMajor string
}

// Inputs is everything set building reads.
type Inputs struct {
	// ProjectPath is the project the set belongs to, canonical (symlinks
	// resolved) so it compares equal to the record paths in Running. The local
	// Airflow of this project is the one that takes the reserved name; every
	// other running Airflow is addressed by its own project's name.
	ProjectPath string
	// Manifest is the loaded pyproject.toml, nil outside a project.
	Manifest *manifest.Manifest
	// Running is the local Airflows alive on this machine.
	Running []Local
}

// Set is the named instances a project can act on, sorted by name.
type Set struct {
	items []Instance
	// shadowed are instances that exist but whose name another instance holds.
	// They are listed, with the problem that explains them, and are not
	// addressable by name — `astro instance list` promises every Airflow
	// running on this machine, and quietly dropping one breaks that promise.
	shadowed []Instance
	// defaultLink is the manifest's default link by name, "" when the manifest
	// names none. It comes from manifest.DefaultLink, the same call
	// `astro deploy` makes, so the two can never disagree about a project's
	// default.
	defaultLink string
}

// Build assembles the set: every manifest link, plus every running local
// Airflow.
//
// Naming is where the cases live. This project's own Airflow is always `local`
// — the manifest refuses that name, so nothing declared competes for it, and a
// foreign project whose directory happens to be called `local` does not take it
// either: the reserved name means "this machine, this project" or it means
// nothing. Another project's Airflow takes its directory name when nothing
// holds it; when something does — a link of that name, or another project with
// the same basename — it is shadowed: still listed, with the problem that says
// how to reach it, but not addressable by name.
func Build(in Inputs) Set {
	byName := map[string]Instance{}
	var shadowed []Instance
	defaultLink := ""
	if in.Manifest != nil {
		if name, _, ok := manifest.DefaultLink(in.Manifest.Astro.Deployments); ok {
			defaultLink = name
		}
		for name := range in.Manifest.Astro.Deployments {
			link := in.Manifest.Astro.Deployments[name]
			byName[name] = Instance{
				Name:         name,
				Kind:         Kind(link.Kind()),
				Source:       SourceManifest,
				Where:        linkWhere(link),
				URL:          link.URL,
				Link:         link,
				TargetConfig: in.Manifest.Astro.Targets[link.Target],
				Own:          true,
			}
		}
	}

	running := append([]Local(nil), in.Running...)
	sort.Slice(running, func(i, j int) bool { return running[i].ProjectPath < running[j].ProjectPath })
	for _, l := range running {
		own := in.ProjectPath != "" && l.ProjectPath == in.ProjectPath
		name := filepath.Base(l.ProjectPath)
		if own {
			name = LocalName
		}
		if name == "." || name == string(filepath.Separator) {
			continue
		}
		url := localURL(l)
		it := Instance{
			Name:         name,
			Kind:         KindLocal,
			Source:       SourceRunning,
			Where:        url,
			URL:          url,
			Project:      l.ProjectPath,
			AirflowMajor: l.AirflowMajor,
			Own:          own,
		}
		if !own {
			// The reserved name is this project's alone. A foreign project that
			// happens to be called `local` keeps its Airflow reachable through
			// --url rather than answering to a name that means something else.
			if name == LocalName {
				it.Problem = "`" + LocalName + "` names this project's own Airflow, so this one has no name here — reach it with --url " + it.URL
				shadowed = append(shadowed, it)
				continue
			}
			if held, taken := byName[name]; taken {
				it.Problem = shadowedBy(held, it.URL)
				shadowed = append(shadowed, it)
				continue
			}
		}
		byName[name] = it
	}

	items := make([]Instance, 0, len(byName))
	for name := range byName {
		items = append(items, byName[name])
	}
	sortByName(items)
	sortByName(shadowed)
	return Set{items: items, shadowed: shadowed, defaultLink: defaultLink}
}

// shadowedBy explains a name already taken, naming the thing that took it and
// the way to reach this one anyway.
func shadowedBy(held Instance, url string) string {
	what := "a deployment link of this name"
	if held.Source == SourceRunning {
		what = "the Airflow at " + held.Project
	}
	return "name already taken by " + what + " — reach this one with --url " + url
}

func sortByName(items []Instance) {
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
}

// All returns every instance this project can see, addressable or not, sorted
// by name. The shadowed ones carry the Problem that says why they have no name
// here.
func (s Set) All() []Instance {
	all := make([]Instance, 0, len(s.items)+len(s.shadowed))
	all = append(all, s.items...)
	all = append(all, s.shadowed...)
	sortByName(all)
	return all
}

// Names returns every addressable instance name, sorted.
func (s Set) Names() []string {
	names := make([]string, len(s.items))
	for i := range s.items {
		names[i] = s.items[i].Name
	}
	return names
}

// ownNames returns the names of this project's own instances — its links and
// its own local Airflow — sorted. These are the only candidates the default
// fall-through considers and the only ones a prompt offers.
func (s Set) ownNames() []string {
	var names []string
	for i := range s.items {
		if s.items[i].Own {
			names = append(names, s.items[i].Name)
		}
	}
	return names
}

// Lookup finds an instance by name.
func (s Set) Lookup(name string) (Instance, bool) {
	for i := range s.items {
		if s.items[i].Name == name {
			return s.items[i], true
		}
	}
	return Instance{}, false
}

// localURL is a local Airflow's always-reachable base URL: its backend port on
// localhost. The proxy hostname needs the proxy daemon up, so it is a display
// convenience rather than something to talk to.
func localURL(l Local) string {
	if l.Port == 0 {
		return ""
	}
	return "http://localhost:" + strconv.Itoa(l.Port)
}

// linkWhere summarizes where a link points, in the manifest's own words.
func linkWhere(d manifest.Link) string {
	switch d.Kind() {
	case manifest.KindAstro:
		return "deployment " + d.Deployment
	case manifest.KindMWAA, manifest.KindComposer:
		return "environment " + d.Environment
	case manifest.KindEndpoint:
		return d.URL
	}
	return ""
}
