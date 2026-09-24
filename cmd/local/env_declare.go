package local

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/internal/localenv"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// declare and undeclare edit the project's [tool.astro.env], the declarations
// set, get and list read. Every write goes through pkg/scaffold's env
// declaration writer, which Astro Desktop's edits use too, so the rules that
// keep the section loadable live in one place.
//
// A declaration holds no value, so nothing here reads or prints one. The one
// annotation that carries a value, `default`, is written from its flag and
// never echoed back.

// The flag names declare registers. Constants because several are also words
// the rest of this package uses as literals.
const (
	flagDeclType        = "type"
	flagDeclEnum        = "enum"
	flagDeclDescription = "description"
	flagDeclOptional    = "optional"
	flagDeclSensitive   = "sensitive"
	flagDeclSource      = "source"
	flagDeclDefault     = "default"
	flagDeclNoDefault   = "no-default"
)

// The --source values. workspace is envschema's own; local is the absence of
// one, which the manifest spells by leaving the key out.
const sourceLocal = "local"

// The status values of an envDeclarationResult.
const (
	declStatusDeclared   = "declared"
	declStatusUndeclared = "undeclared"
	declStatusUnchanged  = "unchanged"
)

// envDeclarationResult is the declare/undeclare status object, rendered the
// same in text and json. Status is declared or undeclared when the manifest
// changed, and unchanged when it already said what was asked. Name is the name
// the declaration is under, which for an env-form key (AIRFLOW_VAR_REGION) is
// its plain name.
type envDeclarationResult struct {
	Kind     localenv.Kind `json:"kind"`
	Name     string        `json:"name"`
	Status   string        `json:"status"`
	Manifest string        `json:"manifest"`
}

// declareInput holds declare's flags. typ is the value type for an env var or
// an Airflow variable, and the connection type for a connection, the way
// `connection set --type` names it.
type declareInput struct {
	typ          string
	enum         []string
	description  string
	source       string
	defaultValue string
	noDefault    bool
	optional     bool
	sensitive    bool
}

// sectionFor maps a noun's kind to the part of [tool.astro.env] it declares in.
func sectionFor(kind localenv.Kind) envschema.Section {
	switch kind {
	case localenv.KindConn:
		return envschema.SectionConnection
	case localenv.KindVar:
		return envschema.SectionAirflowVariable
	case localenv.KindEnv:
	}
	return envschema.SectionEnvVar
}

func newEnvDeclareCmd(c *cli, scope *scopeFlags, k envKind) *cobra.Command {
	in := &declareInput{}
	long := "Declare " + k.article + " " + k.label + " in the project's pyproject.toml, or change how it is\n" +
		"declared. A declared name is required at start unless it is optional.\n\n" +
		"Only the annotations you pass change; the rest of the declaration, and any\n" +
		"comments around it, stay as they are. No value is stored: use set for that."
	if k.kind == localenv.KindConn {
		long += "\n\nA connection is always sensitive and takes no default, type or enum; --type\n" +
			"is the connection type, as it is for set."
	} else {
		long += "\n\n--sensitive keeps the value out of plain files: set stores it only in the\n" +
			"vault. A sensitive name cannot have a default."
	}
	long += "\n\n--source workspace resolves the name from the workspace's Environment Manager\n" +
		"when no local file sets it. A name with a default cannot resolve from the\n" +
		"workspace."
	cmd := &cobra.Command{
		Use:   "declare " + k.arg,
		Short: "Declare " + k.article + " " + k.label + " in pyproject.toml",
		Long:  long,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runEnvDeclare(cmd, scope, in, k.kind, args[0])
		},
	}
	f := cmd.Flags()
	f.StringVar(&in.description, flagDeclDescription, "", "Prose for whoever has to supply the value; an empty string removes it")
	f.BoolVar(&in.optional, flagDeclOptional, false, "Do not require it at start; --optional=false requires it again")
	f.StringVar(&in.source, flagDeclSource, "", "Where it resolves from when no local file sets it: workspace or local")
	if k.kind == localenv.KindConn {
		f.StringVar(&in.typ, flagDeclType, "", "The expected connection type (e.g. postgres, http); an empty string removes it")
		return cmd
	}
	f.StringVar(&in.typ, flagDeclType, "", "The value's type: string, int, number, bool, enum, url, port or json")
	f.StringSliceVar(&in.enum, flagDeclEnum, nil, "The allowed values, comma-separated; implies --type enum")
	f.BoolVar(&in.sensitive, flagDeclSensitive, false, "Store its value only in the vault; --sensitive=false clears it")
	f.StringVar(&in.defaultValue, flagDeclDefault, "", "A default committed to pyproject.toml, used when nothing else sets it")
	f.BoolVar(&in.noDefault, flagDeclNoDefault, false, "Remove the default")
	cmd.MarkFlagsMutuallyExclusive(flagDeclDefault, flagDeclNoDefault)
	return cmd
}

func newEnvUndeclareCmd(c *cli, scope *scopeFlags, k envKind) *cobra.Command {
	return &cobra.Command{
		Use:   "undeclare " + k.arg,
		Short: "Remove " + k.article + " " + k.label + "'s declaration from pyproject.toml",
		Long: "Remove " + k.article + " " + k.label + "'s declaration from the project's pyproject.toml.\n\n" +
			"Values already set for it stay where they are: use delete to remove one.",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return c.runEnvUndeclare(scope, k.kind, args[0])
		},
	}
}

// annotationFlags are declare's flags other than --source. Any of them turns a
// --source workspace declare into a general edit; see runEnvDeclare.
var annotationFlags = []string{
	flagDeclType, flagDeclEnum, flagDeclDescription, flagDeclOptional,
	flagDeclSensitive, flagDeclDefault, flagDeclNoDefault,
}

func (c *cli) runEnvDeclare(cmd *cobra.Command, scope *scopeFlags, in *declareInput, kind localenv.Kind, name string) error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
	dir, err := c.declarationProject(scope)
	if err != nil {
		return err
	}
	section := sectionFor(kind)
	source, err := parseDeclSource(cmd, in.source)
	if err != nil {
		return err
	}
	annotated := false
	for _, f := range annotationFlags {
		if cmd.Flags().Changed(f) {
			annotated = true
		}
	}

	changed, err := watchManifest(dir, func(wrap func(run func() error) error) error {
		switch {
		case source == envschema.SourceWorkspace && !annotated:
			return scaffold.DeclareEnvFromWorkspace(dir, wrap, section, name, "")
		case !annotated && !cmd.Flags().Changed(flagDeclSource):
			return scaffold.AddEnvDeclaration(dir, wrap, section, name, nil)
		default:
			return scaffold.EditEnvDeclaration(dir, wrap, section, name, in.edit(cmd, kind, source))
		}
	})
	if err != nil {
		if errors.Is(err, scaffold.ErrWorkspaceSourceWithDefault) && kind != localenv.KindConn {
			if cmd.Flags().Changed(flagDeclDefault) {
				return fmt.Errorf("%w. --%s and --source workspace cannot be combined", err, flagDeclDefault)
			}
			return fmt.Errorf("%w. Passing --%s with --source workspace removes it in the same edit", err, flagDeclNoDefault)
		}
		return err
	}
	res, err := declarationResult(dir, kind, name)
	if err != nil {
		return err
	}
	res.Status = declStatusUnchanged
	if changed {
		res.Status = declStatusDeclared
	}
	if source == envschema.SourceWorkspace {
		c.noteNoWorkspace(dir, res.Name)
	}
	if cmd.Flags().Changed(flagDeclSensitive) && in.sensitive {
		c.notePlaintextCopy(dir, kind, res.Name)
	}
	return r.Emit(res, func(w io.Writer) error {
		var werr error
		if changed {
			_, werr = fmt.Fprintf(w, "declared %s %s in %s\n", localenv.Noun(kind), res.Name, res.Manifest)
		} else {
			_, werr = fmt.Fprintf(w, "%s %s is already declared that way in %s\n", localenv.Noun(kind), res.Name, res.Manifest)
		}
		return werr
	})
}

func (c *cli) runEnvUndeclare(scope *scopeFlags, kind localenv.Kind, name string) error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
	dir, err := c.declarationProject(scope)
	if err != nil {
		return err
	}
	// Named before the removal, since afterwards there is nothing to find.
	res, err := declarationResult(dir, kind, name)
	if err != nil {
		return err
	}
	changed, err := watchManifest(dir, func(wrap func(run func() error) error) error {
		return scaffold.RemoveEnvDeclaration(dir, wrap, sectionFor(kind), name)
	})
	if err != nil {
		return err
	}
	res.Status = declStatusUnchanged
	if changed {
		res.Status = declStatusUndeclared
	}
	return r.Emit(res, func(w io.Writer) error {
		var werr error
		if changed {
			_, werr = fmt.Fprintf(w, "undeclared %s %s in %s; any value set for it is kept\n", localenv.Noun(kind), res.Name, res.Manifest)
		} else {
			_, werr = fmt.Fprintf(w, "%s %s is not declared in %s\n", localenv.Noun(kind), res.Name, res.Manifest)
		}
		return werr
	})
}

// edit is the change declare's flags ask for: each annotation whose flag was
// passed, and nothing else, so an existing declaration keeps what the command
// line did not mention.
func (in *declareInput) edit(cmd *cobra.Command, kind localenv.Kind, source envschema.Source) scaffold.EnvDeclarationEdit {
	given := cmd.Flags().Changed
	return func(s *envschema.ValueSpec, _ bool) error {
		if given(flagDeclSource) {
			s.Source = source
		}
		if given(flagDeclDescription) {
			s.Description = in.description
		}
		if given(flagDeclOptional) {
			s.Optional = in.optional
		}
		if kind == localenv.KindConn {
			if given(flagDeclType) {
				s.ConnType = in.typ
			}
			return nil
		}
		if given(flagDeclType) {
			s.Type = envschema.ValueType(in.typ)
			// An enum belongs to type enum alone, so moving to another type
			// drops the list rather than leaving one the parser refuses.
			if s.Type != envschema.TypeEnum && !given(flagDeclEnum) {
				s.Enum = nil
			}
		}
		if given(flagDeclEnum) {
			s.Enum = in.enum
			if !given(flagDeclType) {
				s.Type = envschema.TypeEnum
			}
		}
		if given(flagDeclSensitive) {
			s.Sensitive = in.sensitive
		}
		switch {
		case given(flagDeclDefault):
			s.Default, s.HasDefault = in.defaultValue, true
		case in.noDefault:
			s.Default, s.HasDefault = "", false
		}
		return nil
	}
}

// parseDeclSource reads --source: workspace, or local for none.
func parseDeclSource(cmd *cobra.Command, value string) (envschema.Source, error) {
	if !cmd.Flags().Changed(flagDeclSource) {
		return "", nil
	}
	switch value {
	case string(envschema.SourceWorkspace):
		return envschema.SourceWorkspace, nil
	case sourceLocal:
		return "", nil
	}
	return "", fmt.Errorf("--%s must be %s or %s, not %q", flagDeclSource, envschema.SourceWorkspace, sourceLocal, value)
}

// declarationProject is the project whose manifest a declare or undeclare
// edits. A declaration belongs to the project, so --global is refused rather
// than ignored, and outside a project there is nothing to edit.
func (c *cli) declarationProject(scope *scopeFlags) (string, error) {
	if scope.global {
		return "", fmt.Errorf("--global does not apply here: a declaration lives in the project's %s", project.Marker)
	}
	return c.discoverProject()
}

// watchManifest runs a write through the scaffold writer and reports whether it
// changed the manifest. The writer writes nothing when the result is the file
// as it was, so comparing the bytes it leaves with the bytes it found, inside
// the wrapper that spans its read and write, is exact.
func watchManifest(dir string, write func(wrap func(run func() error) error) error) (changed bool, err error) {
	path := filepath.Join(dir, project.Marker)
	err = write(func(run func() error) error {
		before, rerr := os.ReadFile(path)
		if err := run(); err != nil {
			return err
		}
		after, aerr := os.ReadFile(path)
		changed = rerr != nil || aerr != nil || !bytes.Equal(before, after)
		return nil
	})
	return changed, err
}

// declarationResult names the declaration an edit of name addresses, found
// the way the writer finds it (scaffold.EnvDeclarationKey), so the name
// reported is the key the edit changed: REGION when that is how the manifest
// declares region. A name not declared reports the key a new declaration is
// written under, which for an env-form key (AIRFLOW_VAR_REGION) is its plain
// name. A manifest that cannot be read leaves the folded name, and the writer
// then reports why.
func declarationResult(dir string, kind localenv.Kind, name string) (envDeclarationResult, error) {
	section := sectionFor(kind)
	folded, err := envschema.FoldName(section, name)
	if err != nil {
		return envDeclarationResult{}, err
	}
	res := envDeclarationResult{Kind: kind, Name: folded, Manifest: filepath.Join(dir, project.Marker)}
	if key, _, kerr := scaffold.EnvDeclarationKey(dir, section, name); kerr == nil {
		res.Name = key
	}
	return res, nil
}

// noteNoWorkspace says, on stderr, that a workspace-sourced name has no
// workspace to resolve from yet. Not an error: until a workspace is linked the
// name is simply missing, and a local value still satisfies it.
func (c *cli) noteNoWorkspace(dir, name string) {
	m, err := manifest.Load(filepath.Join(dir, project.Marker))
	if err != nil || m.Astro.Workspace != "" {
		return
	}
	fmt.Fprintf(c.d.Stderr, "note: [tool.astro] sets no workspace, so %s resolves only from local values until one is linked\n", name)
}

// notePlaintextCopy says, on stderr, that a name just declared sensitive still
// has a copy in the project's plain .env, which outranks the vault at start.
// The declaration changes where set stores the value next, not where it is
// now, and moving it takes the value, which declare never asks for.
func (c *cli) notePlaintextCopy(dir string, kind localenv.Kind, name string) {
	store := localenv.ProjectStore(dir)
	if _, ok, err := store.Get(kind, name); err != nil || !ok {
		return
	}
	fmt.Fprintf(c.d.Stderr, "note: %s is still in %s, a plain file. Run `astro local env %s set %s` to move it into the vault\n",
		name, store.Path, localenv.Noun(kind), name)
}
