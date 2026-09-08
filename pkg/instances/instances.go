// Package instances answers "which deployment am I acting on?" for every
// Airflow-facing command. It builds the project's named deployment set from
// the manifest's links, applies one precedence rule to pick a winner, and
// opens a pkg/airflowapi transport to it.
//
// The machine's own Airflow is not in that set. It is not a deployment and it
// never competes for a name: `astro local af dags list` acts on it, spelled that
// way so a top-level command can never silently hit localhost. LocalInstance
// builds it for the commands that do act on it, and for the inventory bare
// `astro use` prints.
//
// The two halves are deliberately separate. Building the set and picking a
// winner touch nothing but the inputs handed in, so `astro use` renders with
// no network call at all; only Instance.Transport reaches out, and only for
// the one instance a command acts on. Nothing here prints, exits, or reads
// config: the session, env, and coordinate lookups arrive through Deps, and
// every failure travels as an error naming its cause and its fix. The AWS and
// Google credential chains are the exception, resolved through their own SDKs
// — see Deps.
package instances

import (
	"sort"
	"strconv"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// LocalName is the reserved word for the Airflow running on this machine: the
// name the inventory shows it under, the name resolution refuses at every
// layer, and the link name the manifest rejects (manifest.ReservedLinkName).
// It is that same constant, so the three cannot drift apart.
const LocalName = manifest.ReservedLinkName

// EnvVar is the ephemeral layer of the resolution rule. It is an env var
// rather than CLI-managed session state because the shell already owns session
// lifetime — one exported value makes this terminal the prod terminal.
const EnvVar = "ASTRO_DEPLOYMENT"

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

// Instance is one Airflow a command can act on: a deployment the manifest
// links, the Airflow running on this machine, or a bare --url.
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
	// table. Zero for the machine's Airflow.
	Link manifest.Link
	// TargetConfig is the link's [tool.astro.targets.<target>] section, plain
	// data as the manifest carries it, copied rather than shared. It travels with the instance because a
	// link names only its environment: which region that MWAA environment is
	// in, and which project and location that Composer environment is in, are
	// facts about the backend rather than the link, and the resolver needs
	// both halves to reach the Airflow. Read it through TargetString. Nil for a
	// link whose target declares no section.
	//
	// A copy, not the manifest's own map. It used to be the map itself, with a
	// doc comment asking callers not to write to it — a convention this repo's
	// review could hold when the package was internal. Published, the consumer
	// is another repo: one write would reach every other reader of a manifest
	// that a long-lived process parses once and shares, and every instance
	// built from the same target.
	TargetConfig map[string]any
	// Project is the project root of a local Airflow, empty for a manifest
	// link.
	Project string
	// AirflowMajor is the generation a local Airflow was started for, from its
	// runtime record.
	AirflowMajor string
	// Mode is the engine running a local Airflow ("standalone" or "docker", as
	// pkg/localrt spells it), from its runtime record. The two engines
	// provision the Airflow 2 admin account differently, so which password to
	// send depends on it.
	Mode string
}

// Local is one running local Airflow, as the caller discovered it. It mirrors
// the fields of a runtime state record (pkg/localrt) that this package
// needs; the caller passes only the ones whose runtime is actually alive,
// because a leftover record is not an Airflow to talk to. ProjectPath must
// already be canonical — the caller resolves it, so this package touches no
// filesystem.
type Local struct {
	ProjectPath string
	Port        int
	// AirflowMajor is the generation this runtime was started for, from its
	// record. Empty on a record written before the field existed.
	AirflowMajor string
	// Mode is the engine running it, from its record.
	Mode string
}

// LocalInstance is a running local Airflow as an instance: what every
// `astro local` query command acts on, and how the inventory lists it. The
// project in front of the user passes LocalName; another project's Airflow is
// listed under a name of the caller's choosing, since only the caller knows
// which project it is looking at.
//
// It is built rather than resolved. The machine is reached by spelling the
// command `astro local …`, never by winning a precedence rule, so nothing here
// can make a top-level command hit localhost.
func LocalInstance(l Local, name string) Instance {
	url := localURL(l)
	return Instance{
		Name:         name,
		Kind:         KindLocal,
		Source:       SourceRunning,
		Where:        url,
		URL:          url,
		Project:      l.ProjectPath,
		AirflowMajor: l.AirflowMajor,
		Mode:         l.Mode,
	}
}

// Set is the deployments a project can act on, sorted by name.
type Set struct {
	items []Instance
	// defaultLink is the manifest's default link by name, "" when the manifest
	// names none. It comes from manifest.DefaultLink, the same call
	// `astro deploy` makes, so the two can never disagree about a project's
	// default.
	defaultLink string
}

// Build assembles the set: every link the manifest declares, and nothing else.
// A nil manifest — no project in front of the user — is an empty set.
func Build(m *manifest.Manifest) Set {
	if m == nil {
		return Set{}
	}
	defaultLink := ""
	if name, _, ok := manifest.DefaultLink(m.Astro.Deployments); ok {
		defaultLink = name
	}
	items := make([]Instance, 0, len(m.Astro.Deployments))
	for name := range m.Astro.Deployments {
		link := m.Astro.Deployments[name]
		items = append(items, Instance{
			Name:         name,
			Kind:         Kind(link.Kind()),
			Source:       SourceManifest,
			Where:        linkWhere(link),
			URL:          link.URL,
			Link:         link,
			TargetConfig: copyTarget(m.Astro.Targets[link.Target]),
		})
	}
	// Sorted, because a map's order is not one, and every rendering below is
	// expected to be stable between runs.
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return Set{items: items, defaultLink: defaultLink}
}

// All returns every deployment in the set, sorted by name.
func (s Set) All() []Instance {
	return append([]Instance(nil), s.items...)
}

// Names returns every deployment name, sorted.
func (s Set) Names() []string {
	names := make([]string, len(s.items))
	for i := range s.items {
		names[i] = s.items[i].Name
	}
	return names
}

// Lookup finds a deployment by name.
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

// copyTarget copies a target section so an instance cannot reach back into the
// parsed manifest. Nil in, nil out: a target declaring no section has none.
func copyTarget(section map[string]any) map[string]any {
	if section == nil {
		return nil
	}
	out := make(map[string]any, len(section))
	for k, v := range section {
		out[k] = v
	}
	return out
}
