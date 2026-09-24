package scaffold

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/fsatomic"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
)

// The [tool.astro.env] writer. Every declaration edit, from the CLI and from
// Astro Desktop, goes through EditEnvDeclaration or one of the operations built
// on it, and every one of those goes through EditManifest, so the write rules
// there hold for all of them.
//
// An edit is surgical at the level of one annotation. The declaration's
// current annotations are read, the caller changes them, and only the keys
// whose value changed are set or deleted. The rest of the section, the rest of
// that declaration's table, and the comments around both are left as they
// were. A declaration a person wrote under its own [header] keeps the header.
//
// A name is looked up as given first, then as the grammar reads it
// (envschema.DeclarationName), which is also the name a new declaration is
// written under. So an env-form variable or connection key is declared by its
// plain name, yet a variable a person did write as AIRFLOW_VAR_ZONE is still
// the one AIRFLOW_VAR_ZONE addresses, never a zone declared beside it. A
// variable or connection declared under a different case is found rather than
// declared a second time, since both would encode to the same env var.

// ErrWorkspaceSourceWithDefault reports an edit that would make a declaration
// with a default resolve from the workspace. A workspace-sourced name never
// falls back to its default, so the edit would silently drop the value the
// default commits to. Nothing is written.
var ErrWorkspaceSourceWithDefault = errors.New("a name declared source = 'workspace' never falls back to a default")

// EnvDeclarationEdit changes one declaration. spec holds its annotations as
// they are, or the zero declaration when declared is false, and the edit
// changes them in place. Returning an error abandons the edit and writes
// nothing.
type EnvDeclarationEdit func(spec *envschema.ValueSpec, declared bool) error

// EditEnvDeclaration applies edit to the declaration of name in section and
// writes the result into the pyproject.toml in dir, through EditManifest. wrap
// is EditManifest's wrapper, run around the whole read-modify-write.
//
// name is looked up as given, then as envschema.DeclarationName reads it, so
// AIRFLOW_VAR_REGION under airflow_variables edits the variable region unless
// AIRFLOW_VAR_REGION itself is declared there. A declaration that does
// not load is refused as it stands, with the parser's reason: its annotations
// cannot be read to edit. So is an edit that gives a declaration with a default
// source = 'workspace', with ErrWorkspaceSourceWithDefault.
func EditEnvDeclaration(dir string, wrap func(run func() error) error, section envschema.Section, name string, edit EnvDeclarationEdit) error {
	folded, err := envschema.DeclarationName(section, name)
	if err != nil {
		return err
	}
	return EditManifest(dir, wrap, func(_ *manifest.Manifest, ed tomledit.Editor) error {
		return editEnvDeclaration(ed, section, []string{name, folded}, edit)
	})
}

// AddEnvDeclaration declares name in section with spec's annotations, or with
// none for a nil spec. A name already declared is left exactly as it is, so the
// call is idempotent and never overwrites what a person wrote.
//
// spec.ConnType belongs to a connection only; on another section the result
// does not load and is refused.
func AddEnvDeclaration(dir string, wrap func(run func() error) error, section envschema.Section, name string, spec *envschema.ValueSpec) error {
	return EditEnvDeclaration(dir, wrap, section, name, func(s *envschema.ValueSpec, declared bool) error {
		if declared || spec == nil {
			return nil
		}
		*s = cloneSpec(spec)
		return nil
	})
}

// SetEnvDeclaration declares name in section with exactly spec's annotations,
// or with none for a nil spec, replacing the ones it had. An annotation that
// keeps its value is not rewritten, and the table's other lines and comments
// stay where they are.
func SetEnvDeclaration(dir string, wrap func(run func() error) error, section envschema.Section, name string, spec *envschema.ValueSpec) error {
	return EditEnvDeclaration(dir, wrap, section, name, func(s *envschema.ValueSpec, _ bool) error {
		if spec == nil {
			*s = envschema.ValueSpec{}
			return nil
		}
		*s = cloneSpec(spec)
		return nil
	})
}

// DeclareEnvFromWorkspace makes name resolve from the workspace's Environment
// Manager objects: it gives the declaration source = 'workspace', declaring it
// first if it is not declared. An existing declaration keeps every other
// annotation. connType is written for a new connection only and ignored for
// the other sections, so a caller can pass what it has.
//
// A declaration with a default is refused with ErrWorkspaceSourceWithDefault,
// the committed-default string form included, rather than rewritten to drop
// the default.
func DeclareEnvFromWorkspace(dir string, wrap func(run func() error) error, section envschema.Section, name, connType string) error {
	return EditEnvDeclaration(dir, wrap, section, name, func(s *envschema.ValueSpec, declared bool) error {
		if !declared && section == envschema.SectionConnection {
			s.ConnType = connType
		}
		s.Source = envschema.SourceWorkspace
		return nil
	})
}

// RemoveEnvDeclaration removes the declaration of name from section. A name
// that is not declared is not an error, so the call is idempotent.
//
// The name is folded (envschema.FoldName) but not checked, so a declaration
// under a name the grammar refuses can still be removed: that is how a section
// that no longer loads gets fixed. A section with several problems is fixed one
// removal at a time, since the result is refused only for a problem the file
// did not already have. The one exception is an env var named
// connections or airflow_variables: those keys are the sub-sections, not
// declarations, and removing one would drop every declaration in it.
func RemoveEnvDeclaration(dir string, wrap func(run func() error) error, section envschema.Section, name string) error {
	folded, err := envschema.FoldName(section, name)
	if err != nil {
		return err
	}
	if isReservedEnvKey(section, folded) {
		return fmt.Errorf("%q is a sub-section of [tool.astro.env], not an env var declaration, so it was not removed", folded)
	}
	return editManifestJudged(dir, wrap, func(_ *manifest.Manifest, ed tomledit.Editor) error {
		key, _, declared := findEnvDeclaration(ed, section, name, folded)
		if declared {
			ed.Delete(append(envSectionPath(section), key))
		}
		return nil
	}, noNewEnvProblems)
}

// EnvDeclarationKey reports the key name is declared under in section of the
// pyproject.toml in dir, found the way EditEnvDeclaration and
// RemoveEnvDeclaration find it: name as given, then case-insensitively for a
// variable key or connection id, then the same for its folded form. A name
// that is not declared reports the key a new declaration would be written
// under, with declared false.
//
// It reads the file and does not judge it, so a caller can name a declaration
// in a section that does not load. It is for reporting: an edit made after it
// looks the name up again under its own wrapper.
func EnvDeclarationKey(dir string, section envschema.Section, name string) (key string, declared bool, err error) {
	folded, err := envschema.FoldName(section, name)
	if err != nil {
		return "", false, err
	}
	path := filepath.Join(dir, manifest.Marker)
	src, err := fsatomic.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", false, fmt.Errorf("%w: %w", manifest.ErrNotFound, err)
		}
		return "", false, fmt.Errorf("reading %s: %w", path, err)
	}
	ed, err := tomledit.NewSurgical(src)
	if err != nil {
		return "", false, fmt.Errorf("reading %s: %w", path, err)
	}
	key, _, declared = findEnvDeclaration(ed, section, name, folded)
	return key, declared, nil
}

func editEnvDeclaration(ed tomledit.Editor, section envschema.Section, names []string, edit EnvDeclarationEdit) error {
	key, raw, declared := findEnvDeclaration(ed, section, names...)
	path := append(envSectionPath(section), key)

	old := envschema.ValueSpec{Sensitive: section == envschema.SectionConnection}
	if declared {
		var err error
		if old, err = readEnvDeclaration(section, key, raw); err != nil {
			return err
		}
	}
	next := cloneSpec(&old)
	if err := edit(&next, declared); err != nil {
		return err
	}
	if next.Source == envschema.SourceWorkspace && next.HasDefault &&
		(old.Source != envschema.SourceWorkspace || !old.HasDefault) {
		return fmt.Errorf("%s has a default, so it was not declared from the workspace: %w. "+
			"Remove the default in %s to declare it", key, ErrWorkspaceSourceWithDefault, manifest.Marker)
	}

	table := envschema.DeclarationTable(&next, section)
	if !declared {
		return ed.Set(path, table)
	}
	before := envschema.DeclarationTable(&old, section)
	if _, isTable := raw.(map[string]any); isTable {
		if err := setChangedAnnotations(ed, path, before, table); err != nil {
			return err
		}
		// A declaration written as dotted keys (X.type = 'url') is nothing but
		// those keys, so deleting the last one undeclares the name. It stays
		// declared, with no annotations.
		if _, still := ed.Get(path); !still {
			return ed.Set(path, map[string]any{})
		}
		return nil
	}
	// The string shorthand, a committed default. It stays a string while the
	// default is all it says, and becomes a table once it says more, since the
	// shorthand has nowhere to put anything else. Either way it is set in
	// place, so it keeps its line and the comments around it.
	switch {
	case reflect.DeepEqual(before, table):
		return nil
	case len(table) == 1 && next.HasDefault:
		return ed.Set(path, next.Default)
	default:
		return ed.Set(path, table)
	}
}

// setChangedAnnotations sets each annotation of after that differs from
// before and deletes each one after no longer has, leaving the rest of the
// table untouched. Both are renderings of the declaration's spec, so an
// annotation a person spelled differently but that means the same thing
// (default = 8080 against '8080') compares equal and is not rewritten.
func setChangedAnnotations(ed tomledit.Editor, path []string, before, after map[string]any) error {
	fields := make([]string, 0, len(before)+len(after))
	for f := range before {
		fields = append(fields, f)
	}
	for f := range after {
		if _, ok := before[f]; !ok {
			fields = append(fields, f)
		}
	}
	sort.Strings(fields)
	for _, f := range fields {
		want, keep := after[f]
		switch {
		case !keep:
			ed.Delete(append(slices.Clone(path), f))
		case !reflect.DeepEqual(before[f], want):
			if err := ed.Set(append(slices.Clone(path), f), want); err != nil {
				return err
			}
		}
	}
	return nil
}

// findEnvDeclaration looks each of names up in section, in order, returning
// the key the first one found is declared under and its raw value. Each name
// is tried exactly and then, for a variable key or connection id,
// case-insensitively, since REGION and region encode to the same env var; an
// env var name matches exactly. When nothing matched, key is the last name,
// which is the one a new declaration is written under. The two sub-section
// keys are never an env var's declaration.
func findEnvDeclaration(ed tomledit.Editor, section envschema.Section, names ...string) (key string, raw any, declared bool) {
	for _, name := range names {
		if key, raw, ok := lookupEnvDeclaration(ed, section, name); ok {
			return key, raw, true
		}
	}
	return names[len(names)-1], nil, false
}

func lookupEnvDeclaration(ed tomledit.Editor, section envschema.Section, name string) (key string, raw any, declared bool) {
	if isReservedEnvKey(section, name) {
		return "", nil, false
	}
	if raw, ok := ed.Get(append(envSectionPath(section), name)); ok {
		return name, raw, true
	}
	if section == envschema.SectionEnvVar {
		return "", nil, false
	}
	table, ok := ed.Get(envSectionPath(section))
	if !ok {
		return "", nil, false
	}
	decls, ok := table.(map[string]any)
	if !ok {
		return "", nil, false
	}
	keys := make([]string, 0, len(decls))
	for k := range decls {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if strings.EqualFold(k, name) {
			return k, decls[k], true
		}
	}
	return "", nil, false
}

// readEnvDeclaration parses one declaration through ParseSchema, on its own,
// so the rest of the section cannot make it unreadable.
func readEnvDeclaration(section envschema.Section, key string, raw any) (envschema.ValueSpec, error) {
	env := map[string]any{key: raw}
	if sub := envSubsection(section); sub != "" {
		env = map[string]any{sub: env}
	}
	schema, err := envschema.ParseSchema(env)
	if err != nil {
		return envschema.ValueSpec{}, fmt.Errorf("%s is declared in a form that does not load, so it was not changed: %w", key, err)
	}
	switch section {
	case envschema.SectionAirflowVariable:
		return schema.AirflowVariables[key], nil
	case envschema.SectionConnection:
		return schema.Connections[key], nil
	case envschema.SectionEnvVar:
	}
	return schema.EnvVars[key], nil
}

// envSectionPath is the table section's declarations sit in.
func envSectionPath(section envschema.Section) []string {
	path := []string{"tool", "astro", "env"}
	if sub := envSubsection(section); sub != "" {
		path = append(path, sub)
	}
	return path
}

// isReservedEnvKey reports whether name, as an env var, would address one of
// the sub-sections inside [tool.astro.env] rather than a declaration.
func isReservedEnvKey(section envschema.Section, name string) bool {
	return section == envschema.SectionEnvVar &&
		(name == envSubsection(envschema.SectionConnection) || name == envSubsection(envschema.SectionAirflowVariable))
}

// envSubsection is the key of section's table inside [tool.astro.env], or ""
// for env vars, which sit in it directly.
func envSubsection(section envschema.Section) string {
	switch section {
	case envschema.SectionAirflowVariable:
		return "airflow_variables"
	case envschema.SectionConnection:
		return "connections"
	case envschema.SectionEnvVar:
	}
	return ""
}

// cloneSpec copies spec, Enum included, so an edit appending to its copy's
// Enum cannot reach the original's backing array.
func cloneSpec(spec *envschema.ValueSpec) envschema.ValueSpec {
	out := *spec
	out.Enum = slices.Clone(spec.Enum)
	return out
}
