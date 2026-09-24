// Package manifest is the typed view of a project's pyproject.toml:
// [project] plus [tool.astro]. Load reads and validates a manifest;
// the tomledit subpackage edits one while preserving comments and layout.
//
// This is a shared sub-module: Astro Desktop is expected to adopt the
// manifest as its project definition, so the leaf rules apply. Two sections
// are deliberately exposed raw rather than typed here:
//
//   - [tool.astro.env] is parsed and validated by pkg/envschema, which this
//     module does not import, so Astro.Env carries the decoded section as
//     plain data and each consumer composes the two packages one layer up.
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
	"path/filepath"
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
	UV      UV
}

// UV is the [tool.uv] table, the fields astro reads. uv applies them to the
// project's own environment itself; astro reads them for the scratch
// environments it builds outside the project, which uv cannot see the project
// from, so a check there resolves the way the project does.
type UV struct {
	// ConstraintDependencies is [tool.uv] constraint-dependencies: version
	// limits on packages the project may pull in, without requiring them.
	ConstraintDependencies []string
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
	// Domain is [tool.astro] domain, the Astro host Workspace lives on
	// (astronomer.io, astronomer-dev.io, ...). Empty if unset; read it through
	// WorkspaceDomain, which applies the default. It names the login a
	// `source = "workspace"` value is read with — see docs/v2-workspace-link.md.
	Domain string
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
	// Dockerfile is [tool.astro] dockerfile, a slash-separated project-relative
	// path (shape-validated only — see validate) to the
	// project's own Dockerfile — "tier 3" in the project design, the escape
	// hatch for a multi-stage build or anything else a manifest cannot
	// express. Empty is the common case and means the image is generated from
	// AirflowVersion, Packages and [project] dependencies.
	//
	// DECLARED, rather than inferred from the file being on disk, and that is
	// the whole reason this key exists. Presence answers "is there a
	// Dockerfile", which is not the same question as "is it the build": a
	// conversion KEEPS a Dockerfile whose instructions it could not carry, and
	// with only presence to go on the desktop built from that file while
	// `astro local` generated an image over the runtime base and ignored it.
	// One project, two tools, two different images — the thing sharing a disk
	// contract is supposed to prevent.
	//
	// Docker mode runs this file as the build, and AirflowVersion, Packages and
	// the dependency list stop describing the image. Standalone mode has no
	// image and ignores it.
	Dockerfile string
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
// drift, and the day they did, `astro deploy` and `astro af dags list` would
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

// ProblemCode names the rule a problem came from.
//
// The Reason beside it is a sentence for whoever wrote the manifest, and a
// sentence for a person is not a thing to branch on: it gets reworded, it
// will one day be translated, and a caller matching its text breaks both
// times. The code is what stays put, so a caller decides on the code and the
// prose improves freely.
//
// Shape and rule are both here, and the difference is which one repeats. A
// shape code — CodeRequired, CodeExpectedString — is one rule applied to
// whichever key is wrong, so many keys share it and the Key says which.
// A rule code belongs to one refusal and appears once.
//
// Values are written out rather than derived from the constant names,
// because they are the stable part: renaming a Go identifier must not change
// what a caller sees.
type ProblemCode string

const (
	// Shape: the value is not the kind of thing the key takes. One rule,
	// many keys.
	CodeRequired            ProblemCode = "required"
	CodeExpectedString      ProblemCode = "expected_string"
	CodeExpectedBool        ProblemCode = "expected_bool"
	CodeExpectedTable       ProblemCode = "expected_table"
	CodeExpectedStringArray ProblemCode = "expected_string_array"
	CodeEmptyString         ProblemCode = "empty_string"
	CodeUnknownKey          ProblemCode = "unknown_key"

	// The project and the [tool.astro] block.
	CodeProjectNameInvalid       ProblemCode = "project_name_invalid"
	CodeAirflowVersionInvalid    ProblemCode = "airflow_version_invalid"
	CodeDockerfileSeparators     ProblemCode = "dockerfile_separators"
	CodeDockerfileOutsideProject ProblemCode = "dockerfile_outside_project"
	// CodeTargetNotAName is [tool.astro] target given a table: the old
	// spelling of the backend-config section, which is its own block now.
	CodeTargetNotAName ProblemCode = "target_not_a_name"
	// CodeDomainWithoutWorkspace is [tool.astro] domain with no workspace: a
	// host for nothing, most likely a workspace line deleted and its domain
	// left behind.
	CodeDomainWithoutWorkspace ProblemCode = "domain_without_workspace"

	// Links: what a deployment link may be called and what it must name.
	CodeLinkNeedsName               ProblemCode = "link_needs_name"
	CodeLinkNameReserved            ProblemCode = "link_name_reserved"
	CodeTargetUnusable              ProblemCode = "target_unusable"
	CodeInheritedTargetUnusable     ProblemCode = "inherited_target_unusable"
	CodeTargetNeedsEnvironment      ProblemCode = "target_needs_environment"
	CodeURLAndCoordinates           ProblemCode = "url_and_coordinates"
	CodeDeploymentOnEnvironmentLink ProblemCode = "deployment_on_environment_link"
	CodeEnvironmentRequired         ProblemCode = "environment_required"
	CodeEnvironmentOnAstroLink      ProblemCode = "environment_on_astro_link"
	CodeDeploymentRequired          ProblemCode = "deployment_required"
	CodeWorkspaceOnNonAstroLink     ProblemCode = "workspace_on_non_astro_link"
	CodeWorkspaceRequired           ProblemCode = "workspace_required"
	CodeMultipleDefaults            ProblemCode = "multiple_defaults"

	// A link's url.
	CodeURLInvalid  ProblemCode = "url_invalid"
	CodeURLNoScheme ProblemCode = "url_no_scheme"
	CodeURLNotHTTP  ProblemCode = "url_not_http"
	CodeURLNoHost   ProblemCode = "url_no_host"
	//nolint:gosec // G101 reads the name, not the value: this identifies a rule about credentials, it does not hold one
	CodeURLHasCredentials ProblemCode = "url_has_credentials"

	// A link's auth table.
	CodeAuthRequired          ProblemCode = "auth_required"
	CodeAuthMethodRequired    ProblemCode = "auth_method_required"
	CodeAuthMethodUnknown     ProblemCode = "auth_method_unknown"
	CodeAuthFieldNotForMethod ProblemCode = "auth_field_not_for_method"
	CodeAuthFieldRequired     ProblemCode = "auth_field_required"
	CodeAuthEnvNameInvalid    ProblemCode = "auth_env_name_invalid"
	CodeAuthCommandEmpty      ProblemCode = "auth_command_empty"
	CodeAuthPairIncomplete    ProblemCode = "auth_pair_incomplete"
	//nolint:gosec // G101: as above, a rule identifier
	CodeAuthNeedsCredentials ProblemCode = "auth_needs_credentials"
	CodeAuthTooManyPairs     ProblemCode = "auth_too_many_pairs"
)

// problemCodes is every code above, in declaration order — the closed set, in
// the package rather than in a test, so the tests that check the set (unique
// values, identifier spelling, each one reachable from some manifest) read it
// instead of keeping a second copy that a new code could be left out of.
var problemCodes = []ProblemCode{
	CodeRequired, CodeExpectedString, CodeExpectedBool, CodeExpectedTable,
	CodeExpectedStringArray, CodeEmptyString, CodeUnknownKey,

	CodeProjectNameInvalid, CodeAirflowVersionInvalid,
	CodeDockerfileSeparators, CodeDockerfileOutsideProject, CodeTargetNotAName,
	CodeDomainWithoutWorkspace,

	CodeLinkNeedsName, CodeLinkNameReserved, CodeTargetUnusable,
	CodeInheritedTargetUnusable, CodeTargetNeedsEnvironment,
	CodeURLAndCoordinates, CodeDeploymentOnEnvironmentLink,
	CodeEnvironmentRequired, CodeEnvironmentOnAstroLink, CodeDeploymentRequired,
	CodeWorkspaceOnNonAstroLink, CodeWorkspaceRequired, CodeMultipleDefaults,

	CodeURLInvalid, CodeURLNoScheme, CodeURLNotHTTP, CodeURLNoHost,
	CodeURLHasCredentials,

	CodeAuthRequired, CodeAuthMethodRequired, CodeAuthMethodUnknown,
	CodeAuthFieldNotForMethod, CodeAuthFieldRequired, CodeAuthEnvNameInvalid,
	CodeAuthCommandEmpty, CodeAuthPairIncomplete, CodeAuthNeedsCredentials,
	CodeAuthTooManyPairs,
}

// Problem is one validation finding, addressed by the dotted TOML key it
// concerns.
type Problem struct {
	// Code names the rule, and is the part a caller may rely on. Reason is
	// the part a person reads; see ProblemCode for why they are separate.
	Code   ProblemCode
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
	m := &Manifest{Astro: p.astro(*f.Tool.Astro), UV: uvTable(f.Tool.UV)}
	if f.Project != nil {
		m.Project = Project{
			Name:           f.Project.Name,
			RequiresPython: f.Project.RequiresPython,
			Dependencies:   f.Project.Dependencies,
		}
	}
	p.validate(m)
	if len(p.problems) > 0 {
		p.sortProblems()
		return nil, &ValidationError{Problems: p.problems}
	}
	return m, nil
}

// uvTable reads the [tool.uv] fields astro uses, skipping any value that is not
// the shape uv documents rather than reporting it: the table is uv's.
func uvTable(t map[string]any) UV {
	var out UV
	list, _ := t["constraint-dependencies"].([]any)
	for _, v := range list {
		if s, ok := v.(string); ok {
			out.ConstraintDependencies = append(out.ConstraintDependencies, s)
		}
	}
	return out
}

// astroRoot prefixes every key under the section this package owns.
const astroRoot = "tool.astro"

// The keys each table defines. Anything else is a problem: this is authored
// config, and a key that decodes to nothing — a misspelled `default` on the
// link meant to be the default — would otherwise send a deploy somewhere else
// in silence.
var (
	astroKeys = []string{"airflow", "deployments", "dockerfile", "domain", "env", "packages", "target", "targets", "workspace"}
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

// sortProblems puts the findings in a fixed order. Links are decoded out of a
// map, so the order they were found in is random and the error text would
// differ run to run.
//
// Key first, because that is what a reader scans. Key alone would very nearly
// do — no two rules currently address the same key — but nothing enforces
// that and sort.Slice is not stable, so a second rule on some key would start
// randomizing the pair with no other sign. Code settles it, and settles it
// totally, since codes are unique.
func (p *parser) sortProblems() {
	sort.Slice(p.problems, func(i, j int) bool {
		if p.problems[i].Key != p.problems[j].Key {
			return p.problems[i].Key < p.problems[j].Key
		}
		return p.problems[i].Code < p.problems[j].Code
	})
}

func (p *parser) add(code ProblemCode, key, reason string) {
	p.problems = append(p.problems, Problem{Code: code, Key: key, Reason: reason})
}

// astro decodes [tool.astro]. Links come last: they resolve against the
// defaults the section sets above them.
func (p *parser) astro(raw map[string]any) Astro {
	p.unknownKeys(astroRoot, raw, astroKeys)
	a := Astro{
		AirflowVersion: p.reqStr(astroRoot+".airflow", raw["airflow"]),
		Packages:       p.packages(raw["packages"]),
		Workspace:      p.str(astroRoot+".workspace", raw["workspace"]),
		Domain:         strings.TrimSpace(p.str(astroRoot+".domain", raw["domain"])),
		Target:         p.defaultTarget(raw["target"]),
		Targets:        p.targets(raw["targets"]),
		Env:            p.table(astroRoot+".env", raw["env"]),
		// Trimmed at DECODE, not just in validate, because the stored value is
		// what consumers branch on. `dockerfile = " "` is non-empty to a
		// `declared != ""` test and names no file, so leaving it untrimmed here
		// would have let a whitespace declaration suppress the desktop's
		// presence fallback and take the project's real Dockerfile away.
		Dockerfile: strings.TrimSpace(p.str(astroRoot+".dockerfile", raw["dockerfile"])),
	}
	if a.Domain != "" && a.Workspace == "" {
		p.add(CodeDomainWithoutWorkspace, astroRoot+".domain", "names the host of a workspace, and [tool.astro] sets no workspace")
	}
	a.Deployments = p.links(raw["deployments"], &a)
	return a
}

// DefaultWorkspaceDomain is the host a workspace link means when the manifest
// names none and ASTRO_DOMAIN does not either. Every manifest written before
// [tool.astro] domain existed links a production workspace, and a default that
// followed the current login would ask a dev host for a production id.
const DefaultWorkspaceDomain = "astronomer.io"

// WorkspaceDomain is the Astro host the linked workspace lives on, normalized
// the way `astro login` stores a login's domain so the two meet: [tool.astro]
// domain; else ASTRO_DOMAIN, the explicit override CI sets beside
// ASTRO_API_TOKEN and Astro Desktop reads as its own host; else
// DefaultWorkspaceDomain.
func (a *Astro) WorkspaceDomain() string {
	d := a.Domain
	if d == "" {
		d = os.Getenv("ASTRO_DOMAIN")
	}
	if d = NormalizeDomain(d); d != "" {
		return d
	}
	return DefaultWorkspaceDomain
}

// NormalizeDomain reduces an Astro host as someone might write it — a copied
// URL, the cloud UI's host, mixed case — to the form `astro login` stores its
// login under: "https://cloud.astronomer-dev.io/" is "astronomer-dev.io". A
// domain written any other way names no stored login, and the fix the error
// suggests would store the next login under a different key again.
func NormalizeDomain(d string) string {
	d = strings.ToLower(strings.TrimSpace(d))
	d = strings.TrimPrefix(d, "https://")
	d = strings.TrimPrefix(d, "http://")
	d = strings.TrimRight(d, "/")
	return strings.TrimPrefix(d, "cloud.")
}

// defaultTarget reads [tool.astro] target, the default every link inherits. It
// is a target name; a table here is the old spelling of the backend-config
// section, which is [tool.astro.targets.<name>] now — one TOML key cannot be
// both a string and a table, which is why the config side moved.
func (p *parser) defaultTarget(v any) string {
	const key = astroRoot + ".target"
	if _, isTable := v.(map[string]any); isTable {
		p.add(CodeTargetNotAName, key, "is a target name; backend config lives in [tool.astro.targets.<name>]")
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
			p.add(CodeLinkNeedsName, key, "a link needs a name")
			continue
		case ReservedLinkName:
			p.add(CodeLinkNameReserved, key+"."+name, "reserved: local always means the Airflow running on this machine — name the link something else")
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
		p.add(CodeTargetUnusable, key+".target", fmt.Sprintf("%q is not a target a link can use", target)+supported)
		return
	}
	p.add(CodeInheritedTargetUnusable, key+".target", fmt.Sprintf("inherits target = %q from [tool.astro], which is not a target a link can use", target)+supported)
}

// coordinates checks that the link names the one coordinate its kind uses, and
// only that one.
func (p *parser) coordinates(key string, kind LinkKind, link *Link) {
	if link.URL != "" && link.Target != string(KindAstro) {
		p.add(CodeTargetNeedsEnvironment, key, fmt.Sprintf("target = %q names an environment, not a url: set environment = '<%s environment name>', or drop target for a plain url link", link.Target, link.Target))
		return
	}
	switch kind {
	case KindEndpoint:
		if link.Deployment != "" || link.Environment != "" {
			p.add(CodeURLAndCoordinates, key, "sets both a url and coordinates: a link names either a url or deployment/environment coordinates, never both")
			return
		}
		p.url(key+".url", link.URL)
	case KindMWAA, KindComposer:
		if link.Deployment != "" {
			p.add(CodeDeploymentOnEnvironmentLink, key+".deployment", fmt.Sprintf("a %s link has no Astro deployment id: name the environment with environment = '<%s environment name>'", kind, kind))
		}
		if link.Environment == "" {
			p.add(CodeEnvironmentRequired, key+".environment", fmt.Sprintf("required: the name of the %s environment", kind))
		}
	case KindAstro:
		if link.Environment != "" {
			p.add(CodeEnvironmentOnAstroLink, key+".environment", "only an mwaa or composer link sets environment; this is an astro link")
		}
		if link.Deployment == "" {
			p.add(CodeDeploymentRequired, key+".deployment", "required on an astro link: the Deployment id — or set target = 'mwaa' or target = 'composer' with an environment, or a url")
		}
	}
}

// url checks an endpoint link's address: it must be one the CLI can dial, and
// it must not smuggle a credential into a committed file.
func (p *parser) url(key, raw string) {
	u, err := url.Parse(raw)
	switch {
	case err != nil:
		p.add(CodeURLInvalid, key, fmt.Sprintf("%q is not a URL", raw))
	case u.Scheme == "":
		p.add(CodeURLNoScheme, key, fmt.Sprintf("%q has no scheme: write the address in full, like https://%s", raw, raw))
	case u.Scheme != "http" && u.Scheme != "https":
		p.add(CodeURLNotHTTP, key, fmt.Sprintf("%q is not an http(s) URL", raw))
	case u.Host == "":
		p.add(CodeURLNoHost, key, fmt.Sprintf("%q names no host", raw))
	case u.User != nil:
		p.add(CodeURLHasCredentials, key, "a url must not carry a username or password: name env vars instead, with auth = { method = 'basic', username-env = 'AIRFLOW_USER', password-env = 'AIRFLOW_PASSWORD' }")
	}
}

// workspace resolves the workspace, which only an astro link has: the other
// kinds are not in an Astro workspace, so the [tool.astro] default does not
// reach them and setting one on them is a mistake.
func (p *parser) workspace(key string, kind LinkKind, raw any, fallback string) string {
	own := p.str(key+".workspace", raw)
	if kind != KindAstro {
		if own != "" {
			p.add(CodeWorkspaceOnNonAstroLink, key+".workspace", fmt.Sprintf("only an astro link has a workspace; this is a %s link", kind))
		}
		return ""
	}
	ws := firstNonEmpty(own, fallback)
	if ws == "" {
		p.add(CodeWorkspaceRequired, key+".workspace", "no workspace: set workspace on the link or a default with [tool.astro] workspace")
	}
	return ws
}

// unknownKeys reports every key the table does not define.
func (p *parser) unknownKeys(key string, table map[string]any, known []string) {
	for name := range table {
		if !slices.Contains(known, name) {
			p.add(CodeUnknownKey, key+"."+name, "unknown key")
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
		p.add(CodeExpectedString, key, "expected a string")
		return ""
	}
	if s == "" {
		p.add(CodeEmptyString, key, "must not be empty")
		return ""
	}
	return s
}

// reqStr decodes a string the section cannot do without.
func (p *parser) reqStr(key string, v any) string {
	if v == nil {
		p.add(CodeRequired, key, "required")
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
		p.add(CodeExpectedBool, key, "expected true or false")
	}
	return b
}

func (p *parser) table(key string, v any) map[string]any {
	if v == nil {
		return nil
	}
	t, ok := v.(map[string]any)
	if !ok {
		p.add(CodeExpectedTable, key, "expected a table")
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
		p.add(CodeExpectedStringArray, key, "expected an array of strings")
		return nil
	}
	out := make([]string, 0, len(items))
	for i, item := range items {
		s, ok := item.(string)
		if !ok {
			p.add(CodeExpectedString, fmt.Sprintf("%s[%d]", key, i), "expected a string")
			continue
		}
		if strings.TrimSpace(s) == "" {
			p.add(CodeEmptyString, fmt.Sprintf("%s[%d]", key, i), "must be a non-empty string")
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
var airflowVersionRe = regexp.MustCompile(`^\d+(\.\d+){0,2}$`)

// ValidAirflowVersion reports whether v is a [tool.astro] airflow value Parse
// accepts: a full or partial version, "3", "3.1" or "3.1.2". A writer checks a
// pin with it before writing, so a bad one is refused with its own error.
func ValidAirflowVersion(v string) bool {
	return airflowVersionRe.MatchString(v)
}

// validate checks what the decode could not: the standard [project] table,
// which is typed, and the rules that span more than one key.
func (p *parser) validate(m *Manifest) {
	switch {
	case m.Project.Name == "":
		p.add(CodeRequired, "project.name", "required")
	case !projectNameRe.MatchString(m.Project.Name):
		p.add(CodeProjectNameInvalid, "project.name", "not a valid project name (letters, digits, -._; must start and end with a letter or digit)")
	}

	if v := m.Astro.AirflowVersion; v != "" && !ValidAirflowVersion(v) {
		p.add(CodeAirflowVersionInvalid, astroRoot+".airflow", fmt.Sprintf("%q is not a version like 3, 3.1, or 3.1.2", v))
	}

	// The path has to stay inside the project, because the consumer joins it to
	// the project directory and hands the result to a docker build. An absolute
	// path or one climbing out with .. would make the build read a file the
	// project does not contain, which a manifest has no business asking for
	// even when its author is the one who wrote it.
	//
	// filepath.IsLocal is a LEXICAL check, and shape is all this validation
	// claims. It rejects absolute paths and every .. escape, including ones
	// laundered through a subdirectory (a/../../Dockerfile), and it says yes to
	// "." — so a consumer joining this to the project directory still has to
	// check it named a FILE. It also cannot see symlinks, so a link inside the
	// project pointing out of it passes here and resolves outside. That is not a
	// boundary this field can enforce anyway: anyone who can write the manifest
	// can write a Dockerfile with the same effect. The point is a well-defined
	// field, not a sandbox.
	//
	// It is stricter on Windows, where it also refuses the reserved device
	// names, so a manifest can in principle validate on one OS and not another
	// — the safe direction for a difference to run in, and `dockerfile = "NUL"`
	// is not a thing to keep portable.
	//
	// TrimSpace, because `dockerfile = " "` is non-empty to a consumer and names
	// no file. p.str does not trim and every reader tests against "", so a
	// whitespace-only declaration passed validation, then satisfied
	// `declared != ""` in the desktop and took the project's real Dockerfile
	// away — the exact outcome the presence fallback exists to prevent.
	// p.packages already treats a blank entry as a problem; this is that rule
	// for a scalar.
	if v := m.Astro.Dockerfile; v != "" {
		// Backslashes are refused on every platform, including the one where
		// they work. This manifest is committed and read on macOS, Linux and
		// Windows, and a backslash is a path separator on exactly one of them —
		// so `dockerfile = 'docker\Dockerfile'` resolves for the Windows user
		// who wrote it and is one filename containing a backslash to everyone
		// who pulls it, failing with a missing file they did not write. Refusing
		// it puts the error on the machine that can fix it. Forward slashes work
		// on Windows too, so nothing is lost.
		switch {
		case strings.Contains(v, "\\"):
			p.add(CodeDockerfileSeparators, astroRoot+".dockerfile", fmt.Sprintf("%q has to use forward slashes, which work on every platform", v))
		case !filepath.IsLocal(v):
			// Lexical, and it says yes to "." — a consumer that joins this to
			// the project directory has to check it is a file, not just that
			// something is there. See pkg/localrt.
			p.add(CodeDockerfileOutsideProject, astroRoot+".dockerfile", fmt.Sprintf("%q has to be a path inside the project", v))
		}
	}

	var defaults []string
	for name := range m.Astro.Deployments {
		if m.Astro.Deployments[name].Default {
			defaults = append(defaults, name)
		}
	}
	if len(defaults) > 1 {
		sort.Strings(defaults)
		p.add(CodeMultipleDefaults, astroRoot+".deployments", fmt.Sprintf("more than one link sets default = true (%s): at most one may be the default", strings.Join(defaults, ", ")))
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
//
// UV is left as plain data because astro does not own [tool.uv]: a shape uv
// would reject is uv's to report, and must not stop astro loading the project.
type wireTool struct {
	Astro *map[string]any `toml:"astro"`
	UV    map[string]any  `toml:"uv"`
}

type wireProject struct {
	Name           string   `toml:"name"`
	RequiresPython string   `toml:"requires-python"`
	Dependencies   []string `toml:"dependencies"`
}
