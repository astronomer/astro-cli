// Package manifest is the typed view of a project's pyproject.toml:
// [project] plus [tool.astro]. Load reads and validates a manifest;
// the tomledit subpackage edits one while preserving comments and layout.
//
// This is a shared sub-module: Astro Desktop is expected to adopt the
// manifest as its project definition, so the leaf rules apply. Two sections
// are deliberately exposed raw rather than typed here:
//
//   - [tool.astro.env] is typed and validated by pkg/envschema, and
//     sub-modules do not import each other, so Astro.Env carries the decoded
//     section as plain data and each consumer composes the two packages one
//     layer up.
//   - [tool.astro.targets.*] is backend-specific by design — a target section
//     is meaningless to other targets — so Astro.Targets stays plain data and
//     each backend types its own section.
//
// [tool.astro] is decoded by hand rather than through struct tags, because it
// is authored config: an unknown key, a value of the wrong shape and a
// contradictory link are all things its author should be told about, each
// addressed by its dotted TOML key, and all of them at once.
//
// Links come out resolved. [tool.astro] may set a default workspace and a
// default target that every link inherits, and an absent target means "astro".
// Parse folds those defaults in, so each Link in Astro.Deployments already
// carries its own Workspace, Target and auth method — a consumer reads
// link.Workspace and never re-runs the fallback. The top-level defaults stay
// visible on Astro.Workspace and Astro.Target for a consumer that wants to
// show them.
package manifest

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

// Manifest is the parsed pyproject.toml, the parts astro reads.
type Manifest struct {
	Project Project
	Astro   Astro
}

// Project is the standard [project] table, the fields astro cares about.
type Project struct {
	Name           string
	RequiresPython string
	Dependencies   []string
}

// Astro is the [tool.astro] table.
type Astro struct {
	// AirflowVersion pins the Airflow the project runs and locks against.
	// It may be partial ("3", "3.1"): resolution to a concrete version
	// happens downstream, not here.
	AirflowVersion string
	// Packages is [tool.astro] packages, the OS (apt) packages the project
	// needs at the system level — v1's packages.txt. Docker mode installs
	// them into the runtime image; standalone mode cannot and warns.
	Packages []string
	// Workspace is [tool.astro] workspace, the default workspace every astro
	// link inherits when the link sets none. Empty if unset. It is already
	// folded into each Link.Workspace; kept here for display.
	Workspace string
	// Target is [tool.astro] target, the default target every link inherits
	// when the link sets none. Empty if unset (links then fall back to
	// "astro"). It is already folded into each Link.Target; kept here for
	// display.
	Target string
	// Deployments is the committed inventory of Airflows the project talks to,
	// [tool.astro.deployments.<name>], with every link's Workspace, Target and
	// auth method already resolved from the two levels.
	Deployments map[string]Link
	// Targets is [tool.astro.targets.<name>], decoded but untyped: target
	// config is backend-specific, so each backend types its own section.
	Targets map[string]map[string]any
	// Env is the decoded [tool.astro.env] section, untyped: its schema
	// belongs to pkg/envschema, which this package must not import.
	Env map[string]any
}

// Link is one committed deployment link: an Airflow the project talks to,
// named. A link carries coordinates for something the CLI can look up
// (Deployment for astro, Environment for mwaa and composer) or a URL for an
// Airflow with no control plane to ask — never both. No credential is ever
// stored here; Auth names how to prove yourself and the values themselves come
// from the environment at request time.
//
// Target and Workspace are already resolved: a link's own value if it set one,
// else the [tool.astro] default, and for Target "astro" when neither level
// sets it.
type Link struct {
	Target string
	// Workspace is the Astro workspace the Deployment lives in, and is set on
	// astro links only — the other kinds are not in a workspace.
	Workspace string
	// Deployment is the Astro Deployment id on an astro link.
	Deployment string
	// Environment is the platform's own environment name on an mwaa or
	// composer link — the coordinate that stands in for Deployment there.
	Environment string
	// URL is an endpoint link's Airflow base URL, pasted because there is
	// nothing to discover it from: an OSS instance on a VM, a platform team's
	// shared Airflow.
	URL string
	// Auth is how the CLI proves itself to this link's Airflow, with Method
	// already resolved from the link's own auth table or its kind's default.
	Auth Auth
	// Default marks the link a command falls through to when nothing else
	// picked one: the last layer of the query commands' resolution rule, and
	// the entry `astro deploy` highlights in its prompt. It does not decide a
	// deploy — deploy always asks, and this only moves the cursor
	// (docs/v2-instances.md decision 2). At most one link in a manifest may set
	// it.
	Default bool
}

// LinkKind is what a link points at. It is derived from the link's fields and
// never written in the manifest, so a consumer switches on Kind() rather than
// re-deriving the rule.
type LinkKind string

// The mwaa and composer kinds are spelled the same as the targets that select
// them, deliberately: one vocabulary for where a link deploys and what it
// points at.
const (
	KindAstro    LinkKind = "astro"
	KindMWAA     LinkKind = "mwaa"
	KindComposer LinkKind = "composer"
	KindEndpoint LinkKind = "endpoint"
)

// linkTargets is the closed set of targets a link may carry. The package
// targets are a wider set — `astro package oss` builds an artifact for an
// Airflow there is nothing to link to — so a link keeps its own list.
var linkTargets = []LinkKind{KindAstro, KindMWAA, KindComposer}

// Kind reports what the link points at: a URL makes it an endpoint, the mwaa
// and composer targets make it a coordinate link on that platform, and
// anything else is an Astro link.
//
//nolint:gocritic // hugeParam: the value receiver is the point — Kind() must be callable on a map element, which is not addressable
func (l Link) Kind() LinkKind {
	switch {
	case l.URL != "":
		return KindEndpoint
	case l.Target == string(KindMWAA):
		return KindMWAA
	case l.Target == string(KindComposer):
		return KindComposer
	default:
		return KindAstro
	}
}

// DefaultLink returns the link a command acts on when none is named: the one
// marked default = true — validation allows at most one — or, the interim
// rule, a project's only link, so a one-link project behaves the same whether
// that link is marked.
//
// It lives here because it is a fact about the manifest, and because both the
// deploy path and instance resolution ask it. Two copies of this rule would
// drift, and the day they did, `astro deploy` and `astro dags list` would
// disagree about where a project points.
func DefaultLink(links map[string]Link) (string, Link, bool) {
	// Ranged by key: a link is a wide struct, and copying one per iteration to
	// read a single field is waste.
	for name := range links {
		if links[name].Default {
			return name, links[name], true
		}
	}
	if len(links) == 1 {
		for name := range links {
			return name, links[name], true
		}
	}
	return "", Link{}, false
}

// ErrNoAstroSection reports a pyproject.toml without a [tool.astro] table: a
// Python project, but not an astro project. Callers branch on it with
// errors.Is, typically to offer init or import.
var ErrNoAstroSection = errors.New("no [tool.astro] section")

// ErrNotFound reports that no pyproject.toml exists at the path given to
// Load. It wraps the underlying error, so errors.Is(err, fs.ErrNotExist)
// still holds.
var ErrNotFound = errors.New("pyproject.toml not found")

// ParseError reports a pyproject.toml that does not decode as TOML. Err is
// the go-toml error and carries the position.
type ParseError struct {
	Path string
	Err  error
}

func (e *ParseError) Error() string {
	if e.Path == "" {
		return fmt.Sprintf("parse pyproject.toml: %v", e.Err)
	}
	return fmt.Sprintf("parse %s: %v", e.Path, e.Err)
}

func (e *ParseError) Unwrap() error { return e.Err }

// Problem is one validation finding, addressed by the dotted TOML key it
// concerns.
type Problem struct {
	Key    string
	Reason string
}

// ValidationError reports a manifest that decodes but does not validate.
// Problems are sorted by key and cover every finding, not just the first.
type ValidationError struct {
	Path     string
	Problems []Problem
}

// Error lists every problem, one keyed line each: a caller that prints the
// error shows the whole list, which is the promise the docs make about fixing
// a manifest in one pass.
func (e *ValidationError) Error() string {
	where := e.Path
	if where == "" {
		where = "pyproject.toml"
	}
	if len(e.Problems) == 1 {
		return fmt.Sprintf("invalid %s: %s: %s", where, e.Problems[0].Key, e.Problems[0].Reason)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "invalid %s: %d problems", where, len(e.Problems))
	for _, p := range e.Problems {
		fmt.Fprintf(&b, "\n  %s: %s", p.Key, p.Reason)
	}
	return b.String()
}

// Marker is the manifest's filename, and the file whose presence makes a
// directory a project root.
//
// It lives here rather than in the package that walks the tree looking for it,
// because it is a fact about the manifest rather than about discovery, and
// because every consumer that needs it is already holding this package: a
// caller joining a directory to a filename before calling Load should not have
// to import a project-discovery package to learn what that filename is. It was
// in internal/project, which put it out of reach of anything outside this
// repo — Astro Desktop spells it out a second time for exactly that reason.
const Marker = "pyproject.toml"

// Load reads and validates the manifest at path (a pyproject.toml). A
// missing file surfaces as ErrNotFound (which also satisfies
// errors.Is(err, fs.ErrNotExist)); other failure shapes are
// ErrNoAstroSection, ParseError, and ValidationError.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %w", ErrNotFound, err)
		}
		return nil, err
	}
	m, err := Parse(data)
	if err != nil {
		switch e := err.(type) {
		case *ParseError:
			e.Path = path
		case *ValidationError:
			e.Path = path
		}
		return nil, err
	}
	return m, nil
}

// Parse decodes and validates a pyproject.toml held in memory. Load is
// Parse plus the file read; consumers with the bytes already in hand (an
// editor buffer, a test) call Parse directly.
func Parse(data []byte) (*Manifest, error) {
	var f wireFile
	if err := toml.Unmarshal(data, &f); err != nil {
		return nil, &ParseError{Err: err}
	}
	if f.Tool.Astro == nil {
		return nil, ErrNoAstroSection
	}

	p := &parser{}
	m := &Manifest{Astro: p.astro(*f.Tool.Astro)}
	if f.Project != nil {
		m.Project = Project{
			Name:           f.Project.Name,
			RequiresPython: f.Project.RequiresPython,
			Dependencies:   f.Project.Dependencies,
		}
	}
	p.validate(m)
	if len(p.problems) > 0 {
		// Map iteration made the order random; error text must be stable.
		sort.Slice(p.problems, func(i, j int) bool { return p.problems[i].Key < p.problems[j].Key })
		return nil, &ValidationError{Problems: p.problems}
	}
	return m, nil
}

// astroRoot prefixes every key under the section this package owns.
const astroRoot = "tool.astro"

// The keys each table defines. Anything else is a problem: this is authored
// config, and a key that decodes to nothing — a misspelled `default` on the
// link meant to be the default — would otherwise send a deploy somewhere else
// in silence.
var (
	astroKeys = []string{"airflow", "deployments", "env", "packages", "target", "targets", "workspace"}
	linkKeys  = []string{"auth", "default", "deployment", "environment", "target", "url", "workspace"}
)

// ReservedLinkName is the word that means the Airflow running on this machine
// — the whole `astro local` surface — so a link may not take it. Nothing
// resolves it: a deployment called `local` would simply read as the machine to
// everyone who saw it, which is a name to refuse rather than a collision to
// arbitrate. It is exported so the resolver names the same string this package
// refuses, rather than the two agreeing by coincidence.
const ReservedLinkName = "local"

// parser accumulates the findings of one decode. Every helper takes the dotted
// key it is decoding, records a Problem when the value is the wrong shape, and
// returns the zero value, so the decode carries on and the caller reports
// every finding at once.
type parser struct {
	problems []Problem
}

func (p *parser) add(key, reason string) {
	p.problems = append(p.problems, Problem{Key: key, Reason: reason})
}

// astro decodes [tool.astro]. Links come last: they resolve against the
// defaults the section sets above them.
func (p *parser) astro(raw map[string]any) Astro {
	p.unknownKeys(astroRoot, raw, astroKeys)
	a := Astro{
		AirflowVersion: p.reqStr(astroRoot+".airflow", raw["airflow"]),
		Packages:       p.packages(raw["packages"]),
		Workspace:      p.str(astroRoot+".workspace", raw["workspace"]),
		Target:         p.defaultTarget(raw["target"]),
		Targets:        p.targets(raw["targets"]),
		Env:            p.table(astroRoot+".env", raw["env"]),
	}
	a.Deployments = p.links(raw["deployments"], &a)
	return a
}

// defaultTarget reads [tool.astro] target, the default every link inherits. It
// is a target name; a table here is the old spelling of the backend-config
// section, which is [tool.astro.targets.<name>] now — one TOML key cannot be
// both a string and a table, which is why the config side moved.
func (p *parser) defaultTarget(v any) string {
	const key = astroRoot + ".target"
	if _, isTable := v.(map[string]any); isTable {
		p.add(key, "is a target name; backend config lives in [tool.astro.targets.<name>]")
		return ""
	}
	return p.str(key, v)
}

// targets decodes [tool.astro.targets], one plain-data section per backend.
func (p *parser) targets(v any) map[string]map[string]any {
	const key = astroRoot + ".targets"
	table := p.table(key, v)
	if len(table) == 0 {
		return nil
	}
	out := make(map[string]map[string]any, len(table))
	for name, cfg := range table {
		if section := p.table(key+"."+name, cfg); section != nil {
			out[name] = section
		}
	}
	return out
}

// links decodes [tool.astro.deployments], the named inventory.
func (p *parser) links(v any, a *Astro) map[string]Link {
	const key = astroRoot + ".deployments"
	table := p.table(key, v)
	if len(table) == 0 {
		return nil
	}
	out := make(map[string]Link, len(table))
	for name, raw := range table {
		switch name {
		case "":
			p.add(key, "a link needs a name")
			continue
		case ReservedLinkName:
			p.add(key+"."+name, "reserved: local always means the Airflow running on this machine — name the link something else")
			continue
		}
		linkKey := key + "." + name
		fields := p.table(linkKey, raw)
		if fields == nil {
			continue
		}
		out[name] = p.link(linkKey, fields, a)
	}
	return out
}

// link decodes and checks one link. The order matters: an unusable target is
// reported on its own, because every rule below it is a rule about a kind the
// target no longer names.
func (p *parser) link(key string, table map[string]any, a *Astro) Link {
	p.unknownKeys(key, table, linkKeys)
	own := p.str(key+".target", table["target"])
	link := Link{
		Target:      firstNonEmpty(own, a.Target, string(KindAstro)),
		Deployment:  p.str(key+".deployment", table["deployment"]),
		Environment: p.str(key+".environment", table["environment"]),
		URL:         p.str(key+".url", table["url"]),
		Default:     p.boolean(key+".default", table["default"]),
	}
	if !slices.Contains(linkTargets, LinkKind(link.Target)) {
		p.badTarget(key, link.Target, own != "")
		return link
	}
	kind := link.Kind()
	p.coordinates(key, kind, &link)
	link.Workspace = p.workspace(key, kind, table["workspace"], a.Workspace)
	link.Auth = p.auth(key+".auth", table["auth"], kind)
	return link
}

// badTarget says where the target came from, because the fix differs: the
// link's own key, or the [tool.astro] default it inherited.
func (p *parser) badTarget(key, target string, own bool) {
	const supported = " — supported: astro, mwaa, composer, in lower case"
	if own {
		p.add(key+".target", fmt.Sprintf("%q is not a target a link can use", target)+supported)
		return
	}
	p.add(key+".target", fmt.Sprintf("inherits target = %q from [tool.astro], which is not a target a link can use", target)+supported)
}

// coordinates checks that the link names the one coordinate its kind uses, and
// only that one.
func (p *parser) coordinates(key string, kind LinkKind, link *Link) {
	if link.URL != "" && link.Target != string(KindAstro) {
		p.add(key, fmt.Sprintf("target = %q names an environment, not a url: set environment = '<%s environment name>', or drop target for a plain url link", link.Target, link.Target))
		return
	}
	switch kind {
	case KindEndpoint:
		if link.Deployment != "" || link.Environment != "" {
			p.add(key, "sets both a url and coordinates: a link names either a url or deployment/environment coordinates, never both")
			return
		}
		p.url(key+".url", link.URL)
	case KindMWAA, KindComposer:
		if link.Deployment != "" {
			p.add(key+".deployment", fmt.Sprintf("a %s link has no Astro deployment id: name the environment with environment = '<%s environment name>'", kind, kind))
		}
		if link.Environment == "" {
			p.add(key+".environment", fmt.Sprintf("required: the name of the %s environment", kind))
		}
	case KindAstro:
		if link.Environment != "" {
			p.add(key+".environment", "only an mwaa or composer link sets environment; this is an astro link")
		}
		if link.Deployment == "" {
			p.add(key+".deployment", "required on an astro link: the Deployment id — or set target = 'mwaa' or target = 'composer' with an environment, or a url")
		}
	}
}

// url checks an endpoint link's address: it must be one the CLI can dial, and
// it must not smuggle a credential into a committed file.
func (p *parser) url(key, raw string) {
	u, err := url.Parse(raw)
	switch {
	case err != nil:
		p.add(key, fmt.Sprintf("%q is not a URL", raw))
	case u.Scheme == "":
		p.add(key, fmt.Sprintf("%q has no scheme: write the address in full, like https://%s", raw, raw))
	case u.Scheme != "http" && u.Scheme != "https":
		p.add(key, fmt.Sprintf("%q is not an http(s) URL", raw))
	case u.Host == "":
		p.add(key, fmt.Sprintf("%q names no host", raw))
	case u.User != nil:
		p.add(key, "a url must not carry a username or password: name env vars instead, with auth = { method = 'basic', username-env = 'AIRFLOW_USER', password-env = 'AIRFLOW_PASSWORD' }")
	}
}

// workspace resolves the workspace, which only an astro link has: the other
// kinds are not in an Astro workspace, so the [tool.astro] default does not
// reach them and setting one on them is a mistake.
func (p *parser) workspace(key string, kind LinkKind, raw any, fallback string) string {
	own := p.str(key+".workspace", raw)
	if kind != KindAstro {
		if own != "" {
			p.add(key+".workspace", fmt.Sprintf("only an astro link has a workspace; this is a %s link", kind))
		}
		return ""
	}
	ws := firstNonEmpty(own, fallback)
	if ws == "" {
		p.add(key+".workspace", "no workspace: set workspace on the link or a default with [tool.astro] workspace")
	}
	return ws
}

// unknownKeys reports every key the table does not define.
func (p *parser) unknownKeys(key string, table map[string]any, known []string) {
	for name := range table {
		if !slices.Contains(known, name) {
			p.add(key+"."+name, "unknown key")
		}
	}
}

// str decodes an optional string. An absent key is the empty string and no
// problem; a key that is present must carry a non-empty string, since an empty
// value in authored config means someone meant something and got it wrong.
func (p *parser) str(key string, v any) string {
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		p.add(key, "expected a string")
		return ""
	}
	if s == "" {
		p.add(key, "must not be empty")
		return ""
	}
	return s
}

// reqStr decodes a string the section cannot do without.
func (p *parser) reqStr(key string, v any) string {
	if v == nil {
		p.add(key, "required")
		return ""
	}
	return p.str(key, v)
}

func (p *parser) boolean(key string, v any) bool {
	if v == nil {
		return false
	}
	b, ok := v.(bool)
	if !ok {
		p.add(key, "expected true or false")
	}
	return b
}

func (p *parser) table(key string, v any) map[string]any {
	if v == nil {
		return nil
	}
	t, ok := v.(map[string]any)
	if !ok {
		p.add(key, "expected a table")
		return nil
	}
	return t
}

// packages decodes [tool.astro] packages, an array of OS package names. Each
// entry is checked in place so a problem carries the index of the entry that
// caused it.
func (p *parser) packages(v any) []string {
	const key = astroRoot + ".packages"
	if v == nil {
		return nil
	}
	items, ok := v.([]any)
	if !ok {
		p.add(key, "expected an array of strings")
		return nil
	}
	out := make([]string, 0, len(items))
	for i, item := range items {
		s, ok := item.(string)
		if !ok || strings.TrimSpace(s) == "" {
			p.add(fmt.Sprintf("%s[%d]", key, i), "must be a non-empty string")
			continue
		}
		out = append(out, s)
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// projectNameRe is PEP 508's name grammar, which PEP 621 requires of
// [project].name.
var projectNameRe = regexp.MustCompile(`^(?i:[a-z0-9]|[a-z0-9][a-z0-9._-]*[a-z0-9])$`)

// airflowVersionRe accepts a full or partial version: "3", "3.1", "3.1.2".
var airflowVersionRe = regexp.MustCompile(`^[0-9]+(\.[0-9]+){0,2}$`)

// validate checks what the decode could not: the standard [project] table,
// which is typed, and the rules that span more than one key.
func (p *parser) validate(m *Manifest) {
	switch {
	case m.Project.Name == "":
		p.add("project.name", "required")
	case !projectNameRe.MatchString(m.Project.Name):
		p.add("project.name", "not a valid project name (letters, digits, -._; must start and end with a letter or digit)")
	}

	if v := m.Astro.AirflowVersion; v != "" && !airflowVersionRe.MatchString(v) {
		p.add(astroRoot+".airflow", fmt.Sprintf("%q is not a version like 3, 3.1, or 3.1.2", v))
	}

	var defaults []string
	for name := range m.Astro.Deployments {
		if m.Astro.Deployments[name].Default {
			defaults = append(defaults, name)
		}
	}
	if len(defaults) > 1 {
		sort.Strings(defaults)
		p.add(astroRoot+".deployments", fmt.Sprintf("more than one link sets default = true (%s): at most one may be the default", strings.Join(defaults, ", ")))
	}
}

// wire types mirror the TOML spelling. [project] is standard packaging and
// decodes by tag; [tool.astro] arrives as plain data and parser types it, so
// that this package's own section can report a wrong shape as a keyed problem
// rather than a decode failure.
type wireFile struct {
	Project *wireProject `toml:"project"`
	Tool    wireTool     `toml:"tool"`
}

// Astro is a pointer so that a present-but-empty [tool.astro] is told apart
// from an absent one: go-toml leaves a plain map nil for both, and the
// difference decides between "not an astro project" and "a manifest to fix".
type wireTool struct {
	Astro *map[string]any `toml:"astro"`
}

type wireProject struct {
	Name           string   `toml:"name"`
	RequiresPython string   `toml:"requires-python"`
	Dependencies   []string `toml:"dependencies"`
}
