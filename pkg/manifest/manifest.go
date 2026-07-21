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
package manifest

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"sort"

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
	// Deployments is the committed deployment inventory,
	// [tool.astro.deployments.<name>].
	Deployments map[string]Deployment
	// Targets is [tool.astro.target.<name>], decoded but untyped: target
	// config is backend-specific, so each backend types its own section.
	Targets map[string]map[string]any
	// Env is the decoded [tool.astro.env] section, untyped: its schema
	// belongs to pkg/envschema, which this package must not import.
	Env map[string]any
}

// Deployment is one committed deployment link. Control-plane coordinates
// only — no URL, no credential; those are resolved at request time.
type Deployment struct {
	Target     string
	Workspace  string
	Deployment string
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

	m := &Manifest{
		Astro: Astro{
			AirflowVersion: f.Tool.Astro.Airflow,
			Targets:        f.Tool.Astro.Target,
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
	if len(f.Tool.Astro.Deployments) > 0 {
		m.Astro.Deployments = make(map[string]Deployment, len(f.Tool.Astro.Deployments))
		for name, d := range f.Tool.Astro.Deployments {
			m.Astro.Deployments[name] = Deployment(d)
		}
	}

	if problems := validate(m); len(problems) > 0 {
		return nil, &ValidationError{Problems: problems}
	}
	return m, nil
}

// projectNameRe is PEP 508's name grammar, which PEP 621 requires of
// [project].name.
var projectNameRe = regexp.MustCompile(`^(?i:[a-z0-9]|[a-z0-9][a-z0-9._-]*[a-z0-9])$`)

// airflowVersionRe accepts a full or partial version: "3", "3.1", "3.1.2".
var airflowVersionRe = regexp.MustCompile(`^[0-9]+(\.[0-9]+){0,2}$`)

func validate(m *Manifest) []Problem {
	var ps []Problem
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

	for name, d := range m.Astro.Deployments {
		key := "tool.astro.deployments." + name
		if d.Target == "" {
			add(key+".target", "required")
		}
		if d.Workspace == "" {
			add(key+".workspace", "required")
		}
		if d.Deployment == "" {
			add(key+".deployment", "required")
		}
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
	Airflow     string                    `toml:"airflow"`
	Target      map[string]map[string]any `toml:"target"`
	Deployments map[string]wireDeployment `toml:"deployments"`
	Env         map[string]any            `toml:"env"`
}

type wireDeployment struct {
	Target     string `toml:"target"`
	Workspace  string `toml:"workspace"`
	Deployment string `toml:"deployment"`
}
