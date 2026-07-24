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
//   - [tool.astro.target.*] is backend-specific by design — a target section
//     is meaningless to other targets — so Astro.Targets stays plain data and
//     each backend types its own section.
//
// Deployment links come out resolved. [tool.astro] may set a default
// workspace and a default target that every link inherits, and an absent
// target means "astro". Parse folds those defaults in, so each Deployment in
// Astro.Deployments already carries its own Workspace and Target — a consumer
// reads link.Workspace and link.Target and never re-runs the fallback. The
// top-level defaults stay visible on Astro.Workspace and Astro.Target for a
// consumer that wants to show them.
package manifest

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
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
	// Workspace is [tool.astro] workspace, the default workspace every
	// deployment link inherits when the link sets none. Empty if unset. It is
	// already folded into each Deployment.Workspace; kept here for display.
	Workspace string
	// Target is [tool.astro] target, the default target every deployment link
	// inherits when the link sets none. Empty if unset (links then fall back
	// to "astro"). It is already folded into each Deployment.Target; kept here
	// for display.
	Target string
	// Deployments is the committed deployment inventory,
	// [tool.astro.deployments.<name>], with every link's Workspace and Target
	// already resolved from the two levels.
	Deployments map[string]Deployment
	// Targets is [tool.astro.target.<name>], decoded but untyped: target
	// config is backend-specific, so each backend types its own section.
	Targets map[string]map[string]any
	// Env is the decoded [tool.astro.env] section, untyped: its schema
	// belongs to pkg/envschema, which this package must not import.
	Env map[string]any
}

// Deployment is one committed deployment link. Control-plane coordinates
// only — no URL, no credential; those are resolved at request time. Target and
// Workspace are already resolved: a link's own value if it set one, else the
// [tool.astro] default, and for Target "astro" when neither level sets it.
type Deployment struct {
	Target     string
	Workspace  string
	Deployment string
	// Default marks the link `astro deploy` ships to when the command names no
	// link. At most one link in a manifest may set it.
	Default bool
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

func (e *ValidationError) Error() string {
	where := e.Path
	if where == "" {
		where = "pyproject.toml"
	}
	if len(e.Problems) == 1 {
		return fmt.Sprintf("invalid %s: %s: %s", where, e.Problems[0].Key, e.Problems[0].Reason)
	}
	return fmt.Sprintf("invalid %s: %d problems, first is %s: %s", where, len(e.Problems), e.Problems[0].Key, e.Problems[0].Reason)
}

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

	// [tool.astro] target is either a string (the default target for every
	// link) or the [tool.astro.target.<name>] table of backend config. TOML
	// forbids both spellings of one key in a file, so at most one arrives.
	defaultTarget, targets, targetProblem := resolveTargetSection(f.Tool.Astro.Target)

	m := &Manifest{
		Astro: Astro{
			AirflowVersion: f.Tool.Astro.Airflow,
			Packages:       f.Tool.Astro.Packages,
			Workspace:      f.Tool.Astro.Workspace,
			Target:         defaultTarget,
			Targets:        targets,
			Env:            f.Tool.Astro.Env,
		},
	}
	if f.Project != nil {
		m.Project = Project{
			Name:           f.Project.Name,
			RequiresPython: f.Project.RequiresPython,
			Dependencies:   f.Project.Dependencies,
		}
	}
	// Problems the decode turns up, before the typed validation pass: the
	// overloaded target key, and any per-link target set to an empty string —
	// told apart from an absent key only here, at the wire pointer. validate
	// works on the typed Manifest and cannot see either, so they come in as a
	// seed.
	var decodeProblems []Problem
	if targetProblem != nil {
		decodeProblems = append(decodeProblems, *targetProblem)
	}
	if len(f.Tool.Astro.Deployments) > 0 {
		m.Astro.Deployments = make(map[string]Deployment, len(f.Tool.Astro.Deployments))
		for name, d := range f.Tool.Astro.Deployments {
			// Target is no longer required — it defaults to [tool.astro] target
			// and then to "astro" — but a link that sets it to an empty string
			// meant something and got it wrong.
			if d.Target != nil && *d.Target == "" {
				decodeProblems = append(decodeProblems, Problem{Key: "tool.astro.deployments." + name + ".target", Reason: "must not be empty"})
			}
			m.Astro.Deployments[name] = Deployment{
				Deployment: d.Deployment,
				Workspace:  firstNonEmpty(d.Workspace, m.Astro.Workspace),
				Target:     firstNonEmpty(derefString(d.Target), defaultTarget, defaultTargetName),
				Default:    d.Default,
			}
		}
	}

	if problems := validate(m, decodeProblems); len(problems) > 0 {
		return nil, &ValidationError{Problems: problems}
	}
	return m, nil
}

// defaultTargetName is the target a link falls back to when neither the link
// nor [tool.astro] names one.
const defaultTargetName = "astro"

// resolveTargetSection reads the [tool.astro] target key, which is overloaded:
// a string is the default target name, a table is the [tool.astro.target.<name>]
// backend config. It returns whichever is present, plus a Problem for a value
// that is neither (an empty-string default, or a wrong type). TOML forbids one
// key from being both a string and a table, so a file that sets a string
// default cannot also carry target-config tables — fine while the Astro target
// needs no table, a real limit to know before another target does.
func resolveTargetSection(v any) (defaultTarget string, targets map[string]map[string]any, problem *Problem) {
	const key = "tool.astro.target"
	switch t := v.(type) {
	case nil:
		return "", nil, nil
	case string:
		if t == "" {
			return "", nil, &Problem{Key: key, Reason: "must not be empty"}
		}
		return t, nil, nil
	case map[string]any:
		targets = make(map[string]map[string]any, len(t))
		for name, cfg := range t {
			section, ok := cfg.(map[string]any)
			if !ok {
				return "", nil, &Problem{Key: key + "." + name, Reason: "must be a table of target config"}
			}
			targets[name] = section
		}
		return "", targets, nil
	default:
		return "", nil, &Problem{Key: key, Reason: "must be a target name or a [tool.astro.target.<name>] table"}
	}
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
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

// validate checks the typed manifest. seed carries the problems the decode
// already found (see Parse), which validate sorts in with its own so the error
// still lists every finding in a stable order.
func validate(m *Manifest, seed []Problem) []Problem {
	ps := seed
	add := func(key, reason string) { ps = append(ps, Problem{Key: key, Reason: reason}) }

	if m.Project.Name == "" {
		add("project.name", "required")
	} else if !projectNameRe.MatchString(m.Project.Name) {
		add("project.name", "not a valid project name (letters, digits, -._; must start and end with a letter or digit)")
	}

	switch {
	case m.Astro.AirflowVersion == "":
		add("tool.astro.airflow", "required")
	case !airflowVersionRe.MatchString(m.Astro.AirflowVersion):
		add("tool.astro.airflow", fmt.Sprintf("%q is not a version like 3, 3.1, or 3.1.2", m.Astro.AirflowVersion))
	}

	for i, p := range m.Astro.Packages {
		if strings.TrimSpace(p) == "" {
			add(fmt.Sprintf("tool.astro.packages[%d]", i), "must be a non-empty string")
		}
	}

	var defaults []string
	for name, d := range m.Astro.Deployments {
		key := "tool.astro.deployments." + name
		// Workspace must resolve from one of the two levels.
		if d.Workspace == "" {
			add(key+".workspace", "no workspace: set workspace on the link or a default with [tool.astro] workspace")
		}
		if d.Deployment == "" {
			add(key+".deployment", "required")
		}
		if d.Default {
			defaults = append(defaults, name)
		}
	}
	if len(defaults) > 1 {
		sort.Strings(defaults)
		add("tool.astro.deployments", fmt.Sprintf("more than one link sets default = true (%s): at most one may be the default", strings.Join(defaults, ", ")))
	}

	// Map iteration made the order random; error text must be stable.
	sort.Slice(ps, func(i, j int) bool { return ps[i].Key < ps[j].Key })
	return ps
}

// wire types mirror the TOML spelling; the exported types keep Go names.
// wireDeployment duplicates Deployment so the tags pin the file format:
// untagged decoding matches Go names case-insensitively, and a field
// rename could then change the format without anyone noticing.
type wireFile struct {
	Project *wireProject `toml:"project"`
	Tool    wireTool     `toml:"tool"`
}

type wireTool struct {
	Astro *wireAstro `toml:"astro"`
}

type wireProject struct {
	Name           string   `toml:"name"`
	RequiresPython string   `toml:"requires-python"`
	Dependencies   []string `toml:"dependencies"`
}

type wireAstro struct {
	Airflow   string   `toml:"airflow"`
	Packages  []string `toml:"packages"`
	Workspace string   `toml:"workspace"`
	// Target is overloaded: a string default target name, or the
	// [tool.astro.target.<name>] table of backend config. resolveTargetSection
	// splits the two. It stays any because TOML decodes each spelling to a
	// different Go type and a file carries only one.
	Target      any                       `toml:"target"`
	Deployments map[string]wireDeployment `toml:"deployments"`
	Env         map[string]any            `toml:"env"`
}

// wireDeployment mirrors the TOML spelling. Target is a pointer so an absent
// key (nil, take the default) is told apart from target = "" (a rejected
// empty string). Workspace stays a plain string: workspace = "" is treated as
// absent — it falls back to the [tool.astro] default rather than erroring on
// its own — so it needs no such distinction.
type wireDeployment struct {
	Target     *string `toml:"target"`
	Workspace  string  `toml:"workspace"`
	Deployment string  `toml:"deployment"`
	Default    bool    `toml:"default"`
}
