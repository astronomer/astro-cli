package local

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/astronomer/astro-cli/internal/emenv"
	"github.com/astronomer/astro-cli/internal/envresolve"
	"github.com/astronomer/astro-cli/internal/localenv"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// scopeFlags carries the shared --project/--global choice. It is filled by
// cobra before any RunE runs, so every env leaf reads the same struct.
type scopeFlags struct {
	project bool
	global  bool
}

// setInput carries the shared value-source flags for `set`.
type setInput struct {
	stdin bool
	value string
}

// newEnvCmd builds the `astro local env` tree: set/get/list/delete over
// plain env vars (a bare NAME), connections (conn <id>), and Airflow
// Variables (var <key>). Values live in the project's .env or the global
// ~/.astro/env; see docs/v2-secrets.md.
func newEnvCmd(c *cli) *cobra.Command {
	scope := &scopeFlags{}
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Set, read, and list local Airflow env values for this project",
		Long: "Manage the environment values local Airflow runs with: plain env vars, connections, and Airflow Variables.\n\n" +
			"Values are stored in plain files — the project's .env (default inside a project) or the global ~/.astro/env " +
			"(--global) — created readable only by you. Resolution order at start is shell env > project .env > " +
			"global ~/.astro/env > the workspace's Environment Manager. A name resolves from Environment Manager only " +
			"when the schema declares source = \"workspace\" and you are logged in; a local value always wins. This is " +
			"the local sibling of `astro env`, which manages values on the platform.",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
		},
	}
	cmd.PersistentFlags().BoolVar(&scope.project, "project", false, "Act on the project's .env (the default inside a project)")
	cmd.PersistentFlags().BoolVar(&scope.global, "global", false, "Act on the global ~/.astro/env (the default outside a project)")
	cmd.AddCommand(
		newEnvSetCmd(c, scope),
		newEnvGetCmd(c, scope),
		newEnvListCmd(c, scope),
		newEnvDeleteCmd(c, scope),
	)
	return cmd
}

// kindLeaf builds one conn or var subcommand shared by get/delete. The parent
// handles the bare-NAME (env) case; these name a connection or a Variable
// instead.
func kindLeaf(use, short string, kind localenv.Kind, run func(kind localenv.Kind, name string) error) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE:  func(_ *cobra.Command, args []string) error { return run(kind, args[0]) },
	}
}

func newEnvSetCmd(c *cli, scope *scopeFlags) *cobra.Command {
	in := &setInput{}
	run := func(cmd *cobra.Command, kind localenv.Kind, name string) error {
		value, err := c.readSetValue(cmd, in, kind, name)
		if err != nil {
			return err
		}
		return c.runEnvSet(scope, kind, name, value)
	}
	cmd := &cobra.Command{
		Use:   "set <NAME>",
		Short: "Set an env var (or a connection / variable via the subcommands)",
		Long: "Set a value in a .env file. The value never comes from a bare argument — it would land in shell " +
			"history and `ps`. By default `set` prompts with echo off; pass --stdin to read it from a pipe, or " +
			"--value to pass it inline (which is visible in shell history).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd, localenv.KindEnv, args[0])
		},
	}
	cmd.PersistentFlags().BoolVar(&in.stdin, "stdin", false, "Read the value from stdin instead of prompting")
	cmd.PersistentFlags().StringVar(&in.value, "value", "", "Pass the value inline (visible in shell history; prefer a prompt or --stdin)")
	cmd.AddCommand(
		&cobra.Command{
			Use:   "conn <id>",
			Short: "Set a connection from a URI or JSON",
			Args:  cobra.ExactArgs(1),
			RunE:  func(cmd *cobra.Command, args []string) error { return run(cmd, localenv.KindConn, args[0]) },
		},
		&cobra.Command{
			Use:   "var <key>",
			Short: "Set an Airflow Variable",
			Args:  cobra.ExactArgs(1),
			RunE:  func(cmd *cobra.Command, args []string) error { return run(cmd, localenv.KindVar, args[0]) },
		},
	)
	return cmd
}

func newEnvGetCmd(c *cli, scope *scopeFlags) *cobra.Command {
	run := func(kind localenv.Kind, name string) error { return c.runEnvGet(scope, kind, name) }
	cmd := &cobra.Command{
		Use:   "get <NAME>",
		Short: "Print a value and the source it resolves from",
		Args:  cobra.ExactArgs(1),
		RunE:  func(_ *cobra.Command, args []string) error { return run(localenv.KindEnv, args[0]) },
	}
	cmd.AddCommand(
		kindLeaf("conn <id>", "Get a connection", localenv.KindConn, run),
		kindLeaf("var <key>", "Get an Airflow Variable", localenv.KindVar, run),
	)
	return cmd
}

func newEnvDeleteCmd(c *cli, scope *scopeFlags) *cobra.Command {
	run := func(kind localenv.Kind, name string) error { return c.runEnvDelete(scope, kind, name) }
	cmd := &cobra.Command{
		Use:     "delete <NAME>",
		Aliases: []string{"rm"},
		Short:   "Remove a value from a .env file",
		Args:    cobra.ExactArgs(1),
		RunE:    func(_ *cobra.Command, args []string) error { return run(localenv.KindEnv, args[0]) },
	}
	cmd.AddCommand(
		kindLeaf("conn <id>", "Remove a connection", localenv.KindConn, run),
		kindLeaf("var <key>", "Remove an Airflow Variable", localenv.KindVar, run),
	)
	return cmd
}

func newEnvListCmd(c *cli, scope *scopeFlags) *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List declared values with the source each resolves from",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return c.runEnvList(scope, all)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "Widen to the global file plus every project's .env the CLI knows")
	return cmd
}

// envResult is the set/delete status object, rendered the same in text and
// json.
type envResult struct {
	Kind   localenv.Kind  `json:"kind"`
	Name   string         `json:"name"`
	Scope  localenv.Scope `json:"scope"`
	Status string         `json:"status"`
}

// envValue is the get object: the one deliberate reveal.
type envValue struct {
	Kind   localenv.Kind `json:"kind"`
	Name   string        `json:"name"`
	Source string        `json:"source"`
	Value  string        `json:"value"`
}

func (c *cli) runEnvSet(scope *scopeFlags, kind localenv.Kind, name, value string) error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
	store, projectDir, err := c.envStore(scope)
	if err != nil {
		return err
	}
	if _, err := store.Set(kind, name, value); err != nil {
		return err
	}
	// Warn (on stderr, so json stdout stays clean) when a project .env would
	// be tracked by git — the failure mode that actually leaks secrets.
	c.warnUnignoredEnv(store, projectDir)
	res := envResult{Kind: kind, Name: name, Scope: store.Scope, Status: "set"}
	return r.Emit(res, func(w io.Writer) error {
		_, werr := fmt.Fprintf(w, "set %s %s in %s (%s)\n", kind, name, store.Scope, store.Path)
		return werr
	})
}

func (c *cli) runEnvGet(scope *scopeFlags, kind localenv.Kind, name string) error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
	// A scope flag reads that one file; no flag resolves the whole chain and
	// reports the winning source.
	if scope.project || scope.global {
		store, _, err := c.envStore(scope)
		if err != nil {
			return err
		}
		value, ok, err := store.Get(kind, name)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%s %q is not set in %s", kind, name, store.Scope)
		}
		return emitValue(r, envValue{Kind: kind, Name: name, Source: string(store.Scope), Value: value})
	}
	return c.getResolved(r, kind, name)
}

// getResolved reads a value through the full chain and reports the winning
// source.
func (c *cli) getResolved(r Renderer, kind localenv.Kind, name string) error {
	projectDir, _ := c.discoverProject() //nolint:errcheck // no project just means the chain skips the project file
	src, err := localenv.LoadSources(os.Environ(), projectDir)
	if err != nil {
		return err
	}
	key, ok := localenv.EnvKeyFor(kind, name)
	if !ok {
		return fmt.Errorf("%q is not a valid %s name", name, kind)
	}
	for _, p := range src.Providers() {
		if v, has := p.Lookup(key); has {
			return emitValue(r, envValue{Kind: kind, Name: name, Source: p.Label(), Value: v})
		}
	}
	// No local source held it. A name declared source = "workspace" resolves
	// from Environment Manager — the one place `get` reveals a cloud value, and
	// only for the single name asked.
	v, source, ok, err := c.getFromWorkspace(projectDir, kind, name, key)
	if err != nil {
		return err
	}
	if ok {
		return emitValue(r, envValue{Kind: kind, Name: name, Source: source, Value: v})
	}
	return fmt.Errorf("%s %q is not set anywhere (shell env, project .env, or global ~/.astro/env)", kind, name)
}

// getFromWorkspace resolves a workspace-source name from Environment Manager
// for `get`. It returns ok=false (no error) when the name has no workspace
// source, so the caller reports the plain "not set anywhere". A workspace-source
// name that cannot be fetched is an error naming the cause.
func (c *cli) getFromWorkspace(projectDir string, kind localenv.Kind, name, key string) (value, source string, ok bool, err error) {
	if projectDir == "" || c.d.AstroV1Client == nil {
		return "", "", false, nil
	}
	m, schema, err := c.loadManifestSchema(projectDir)
	if err != nil || schema == nil {
		return "", "", false, err
	}
	if declaredSource(schema, kind, name) != envschema.SourceWorkspace {
		return "", "", false, nil
	}
	// reveal = true: get is the one deliberate reveal of a value.
	wp := emenv.NewProvider(m.Astro.Workspace, c.d.AstroV1Client, true)
	if v, has := wp.Lookup(key); has {
		return v, wp.Label(), true, nil
	}
	return "", "", false, fmt.Errorf("%s %q resolves from the workspace but has no value: %s", kind, name, envresolve.Diagnose(wp, key))
}

// declaredSource returns a declared name's source, so `get` knows to consult
// Environment Manager for a workspace source.
func declaredSource(schema *envschema.Schema, kind localenv.Kind, name string) envschema.Source {
	switch kind {
	case localenv.KindEnv:
		return schema.EnvVars[name].Source
	case localenv.KindVar:
		return schema.AirflowVariables[name].Source
	case localenv.KindConn:
		return schema.Connections[name].Source
	}
	return ""
}

func emitValue(r Renderer, v envValue) error {
	return r.Emit(v, func(w io.Writer) error {
		_, werr := fmt.Fprintln(w, v.Value)
		return werr
	})
}

func (c *cli) runEnvDelete(scope *scopeFlags, kind localenv.Kind, name string) error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
	store, _, err := c.envStore(scope)
	if err != nil {
		return err
	}
	ok, err := store.Delete(kind, name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%s %q is not set in %s", kind, name, store.Scope)
	}
	res := envResult{Kind: kind, Name: name, Scope: store.Scope, Status: "deleted"}
	return r.Emit(res, func(w io.Writer) error {
		_, werr := fmt.Fprintf(w, "deleted %s %s from %s (%s)\n", kind, name, store.Scope, store.Path)
		return werr
	})
}

func (c *cli) runEnvList(scope *scopeFlags, all bool) error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
	projectDir, _ := c.discoverProject() //nolint:errcheck // outside a project, list still shows the global file
	m, schema, err := c.loadManifestSchema(projectDir)
	if err != nil {
		return err
	}
	opts := localenv.ListOptions{All: all}
	// reveal = false: list reports where each name resolves, never a value, so
	// it reads Environment Manager for presence only and pulls no secret.
	if m != nil && c.d.AstroV1Client != nil {
		opts.WorkspaceProvider = emenv.NewProvider(m.Astro.Workspace, c.d.AstroV1Client, false)
	}
	switch {
	case scope.project && scope.global:
		return errors.New("--project and --global are mutually exclusive")
	case scope.project:
		opts.Scope = localenv.ScopeProject
	case scope.global:
		opts.Scope = localenv.ScopeGlobal
	}
	items, err := localenv.List(os.Environ(), projectDir, schema, opts)
	if err != nil {
		return err
	}
	if r.Format == FormatJSON {
		for _, it := range items {
			if err := r.Emit(it, nil); err != nil {
				return err
			}
		}
		return nil
	}
	return r.Emit(items, func(w io.Writer) error { return renderEnvList(w, items) })
}

func renderEnvList(w io.Writer, items []localenv.ListItem) error {
	if len(items) == 0 {
		_, err := fmt.Fprintln(w, "No declared env values and no entries in any .env.")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "KIND\tNAME\tSOURCE\tNOTE")
	for _, it := range items {
		note := ""
		if it.Orphan {
			note = "orphan; remove: " + it.RemoveHint
			if it.Project != "" {
				note = "orphan in " + it.Project + "; remove: " + it.RemoveHint
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", it.Kind, it.Name, it.Source, note)
	}
	return tw.Flush()
}

// envStore resolves the scope flags to the one file a set/get/delete edits,
// returning the store and the project dir (empty for global scope with no
// project). --project outside a project is an error; the default is project
// inside one, global otherwise.
func (c *cli) envStore(scope *scopeFlags) (*localenv.Store, string, error) {
	if scope.project && scope.global {
		return nil, "", errors.New("--project and --global are mutually exclusive")
	}
	projectDir, perr := c.discoverProject()
	switch {
	case scope.global:
		store, err := localenv.GlobalStore()
		return store, projectDir, err
	case scope.project:
		if perr != nil {
			return nil, "", perr
		}
		return localenv.ProjectStore(projectDir), projectDir, nil
	default:
		if perr == nil {
			return localenv.ProjectStore(projectDir), projectDir, nil
		}
		store, err := localenv.GlobalStore()
		return store, "", err
	}
}

// discoverProject returns the current project's root, or an error when the
// cwd is outside any project.
func (c *cli) discoverProject() (string, error) {
	wd, err := c.d.WorkingDir()
	if err != nil {
		return "", err
	}
	proj, err := project.Discover(wd)
	if err != nil {
		return "", err
	}
	return proj.Dir, nil
}

// loadManifestSchema loads the project's manifest and types its
// [tool.astro.env] section. Outside a project both are nil, so list/get show
// only file entries. The manifest is returned too, for the top-level workspace
// a workspace-source name resolves through.
func (c *cli) loadManifestSchema(projectDir string) (*manifest.Manifest, *envschema.Schema, error) {
	if projectDir == "" {
		return nil, nil, nil
	}
	m, err := manifest.Load(filepath.Join(projectDir, project.Marker))
	if err != nil {
		return nil, nil, err
	}
	schema, err := envresolve.ParseSchema(m.Astro.Env)
	if err != nil {
		return nil, nil, err
	}
	return m, schema, nil
}

// readSetValue resolves the value for a `set`: --value inline, else --stdin or
// a piped stdin, else a no-echo prompt. A bare positional value is never
// accepted.
func (c *cli) readSetValue(cmd *cobra.Command, in *setInput, kind localenv.Kind, name string) (string, error) {
	if cmd.Flags().Changed("value") {
		return in.value, nil
	}
	f, isFile := c.d.Stdin.(*os.File)
	piped := !isFile || !term.IsTerminal(int(f.Fd()))
	if in.stdin || piped {
		b, err := io.ReadAll(c.d.Stdin)
		if err != nil {
			return "", fmt.Errorf("reading stdin: %w", err)
		}
		return strings.TrimRight(string(b), "\r\n"), nil
	}
	fmt.Fprintf(c.d.Stderr, "Value for %s %s: ", kind, name)
	b, err := term.ReadPassword(int(f.Fd()))
	fmt.Fprintln(c.d.Stderr)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// warnUnignoredEnv prints a stderr warning when a project .env is not covered
// by .gitignore, with the fix. Best-effort: a stat error is not worth failing
// a successful set.
func (c *cli) warnUnignoredEnv(store *localenv.Store, projectDir string) {
	if store.Scope != localenv.ScopeProject || projectDir == "" {
		return
	}
	ignored, err := localenv.EnvIgnored(projectDir)
	if err != nil || ignored {
		return
	}
	fmt.Fprintf(c.d.Stderr, "warning: %s is not covered by .gitignore; add a line `.env` to it so the file is never committed\n", store.Path)
}
