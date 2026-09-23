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
	"github.com/astronomer/astro-cli/internal/vaultenv"
	"github.com/astronomer/astro-cli/pkg/airflowenv"
	"github.com/astronomer/astro-cli/pkg/connmodel"
	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// vaultFlagHelp documents --secret, in one place because set, get and delete
// each register the flag and must describe it identically. Named for the vault
// rather than the flag so gosec's hardcoded-credential heuristic does not read
// a help string as a password.
const vaultFlagHelp = "Use the encrypted vault shared with Astro Desktop instead of a plain file"

// scopeFlags carries the shared --project/--global choice and the --secret
// store choice. It is filled by cobra before any RunE runs, so every env leaf
// reads the same struct.
//
// The two are separate axes on purpose: --project/--global pick WHICH SCOPE a
// value belongs to, --secret picks WHICH STORE holds it. They compose, so
// `--global --secret` is a machine-wide secret and `--global` alone is a
// machine-wide plaintext default.
type scopeFlags struct {
	project bool
	global  bool
	secret  bool
}

// valueStore is the one store a set/get/delete acts on: a plain dotenv file or
// the vault shared with Astro Desktop. --secret picks which.
//
// The two already had the same three operations with the same signatures, which
// is not a coincidence — the vault writer was built to mirror the file store,
// so that `--secret` changes where a value goes and nothing else about how the
// command behaves or what it reports.
type valueStore interface {
	Set(kind localenv.Kind, name, value string) (string, error)
	Get(kind localenv.Kind, name string) (string, bool, error)
	Delete(kind localenv.Kind, name string) (bool, error)
	// ScopeName is the tier, for the confirmation message.
	ScopeName() localenv.Scope
	// Location is where the value went, for the same message.
	Location() string
	// DotenvPath is the plaintext file a write landed in, or empty when the
	// store keeps nothing in the project. It exists so the gitignore advisory
	// can be decided by the store that knows the answer: a type switch here
	// meant every future store — an exec hook, a cloud provider — silently
	// inherited whichever branch it happened to miss.
	DotenvPath() string
}

// fileStore adapts localenv.Store, whose scope and path are fields, to the
// interface the vault writer implements with methods.
type fileStore struct{ *localenv.Store }

func (f fileStore) ScopeName() localenv.Scope { return f.Scope }
func (f fileStore) Location() string          { return f.Path }
func (f fileStore) DotenvPath() string        { return f.Path }

// setInput carries the shared value-source flags for `set`.
type setInput struct {
	stdin bool
	value string
}

// newEnvCmd builds the `astro local env` tree. The three kinds — environment
// variables, connections and Airflow Variables — are peer nouns, each carrying
// the same verbs, and none of them is a default: there is no bare-NAME form,
// so no name is unreachable because it collides with a subcommand.
//
// The nouns and their aliases are `astro env`'s, word for word, so a token
// names the same object on both sides of the CLI. See docs/v2-secrets.md.
func newEnvCmd(c *cli) *cobra.Command {
	scope := &scopeFlags{}
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Set, read, and list local Airflow env values for this project",
		Long: "Manage the environment values local Airflow runs with: plain env vars, connections, and Airflow Variables.\n\n" +
			"Each kind is a noun with the same verbs under it — `variable`, `connection` and `airflow-variable`, the " +
			"words `astro env` uses for the same objects on the cloud side.\n\n" +
			"Values are stored in plain files — the project's .env (default inside a project) or the global ~/.astro/env " +
			"(--global) — created readable only by you. With --secret a value goes instead to the encrypted vault this " +
			"machine shares with Astro Desktop, so a value set in either tool is readable in the other; that needs an OS " +
			"keyring, and the command refuses where there is none rather than quietly writing a credential to a plain " +
			"file.\n\n" +
			"Resolution order at start is shell env > project .env > project vault > global vault > global ~/.astro/env > " +
			"the workspace's Environment Manager.",
		Args:                       cobra.ArbitraryArgs,
		RunE:                       helpOrUnknownSubcommand,
		SuggestionsMinimumDistance: 2,
	}
	cmd.PersistentFlags().BoolVar(&scope.project, "project", false, "Act on the project's .env (the default inside a project)")
	cmd.PersistentFlags().BoolVar(&scope.global, "global", false, "Act on the global ~/.astro/env (the default outside a project)")
	for _, k := range envKinds() {
		cmd.AddCommand(newEnvKindCmd(c, scope, k))
	}
	cmd.AddCommand(newEnvListCmd(c, scope, "", "List declared values of every kind with the source each resolves from"))
	return cmd
}

// helpOrUnknownSubcommand is the RunE for a group that only holds
// subcommands: help when called bare, an error naming what was typed
// otherwise.
//
// A group needs this at all because cobra, given no Run, prints help and
// exits 0 for an unknown subcommand — so `astro local env connection seet x`
// would look like success to a script. Returning the error restores the
// failure; SuggestionsFor restores the "Did you mean this?" that cobra's own
// legacyArgs path would have appended and a hand-rolled error drops.
func helpOrUnknownSubcommand(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}
	if hint := removedVerbHint(cmd, args[0]); hint != "" {
		return errors.New(hint)
	}
	msg := fmt.Sprintf("unknown command %q for %q", args[0], cmd.CommandPath())
	// SuggestionsFor, unlike cobra's internal findSuggestions, does not apply
	// the default minimum distance; the groups set it declaratively so both
	// this and cobra's own path agree. (At zero, prefix matches still fire —
	// only the edit-distance ones are lost.)
	if suggestions := cmd.SuggestionsFor(args[0]); len(suggestions) > 0 {
		msg += "\n\nDid you mean this?\n"
		for _, s := range suggestions {
			msg += "\t" + s + "\n"
		}
	}
	return errors.New(msg)
}

// removedVerbHint names the replacement for a verb-first form this tree used
// to have, or "" when the argument is not one.
//
// `astro local env set API_TOKEN` was the shape in docs, in the demo scripts,
// in the start-time missing-value hint and in pkg/instances' error text. Those
// were all updated, but a user's shell history and a teammate's notes were
// not, and the bare "unknown command" they now get does not point anywhere:
// SuggestionsFor matches none of variable/connection/airflow-variable for
// "set" — too far by edit distance, no shared prefix — so the "Did you mean"
// block never fires for exactly the words most likely to be typed.
//
// cmd/local/tree_test.go carries the same rule for `astro dev` and `astro
// airflow`: a removed spelling names its replacement rather than dead-ending.
func removedVerbHint(cmd *cobra.Command, arg string) string {
	if cmd.Name() != nameEnv {
		return ""
	}
	switch arg {
	case "set", "get", "delete", "rm":
		verb := arg
		if verb == "rm" {
			verb = "delete"
		}
		return fmt.Sprintf(
			"`astro local env %s <NAME>` was removed in v2: each kind is its own noun now.\n"+
				"  env var:           astro local env variable %s <NAME>\n"+
				"  connection:        astro local env connection %s <id>\n"+
				"  Airflow Variable:  astro local env airflow-variable %s <key>",
			verb, verb, verb, verb)
	default:
		return ""
	}
}

// envKind is one noun of the env tree: the word, its aliases, the localenv
// kind its verbs act on, and how its argument and its name read in help.
//
// article is carried rather than derived because the three labels do not
// agree — "a connection" but "an environment variable" — and a Short built by
// concatenation gets it wrong exactly often enough to notice.
type envKind struct {
	aliases []string
	kind    localenv.Kind
	arg     string
	article string
	label   string
}

// envKinds is the noun list, named and aliased as `astro env` names the same
// four objects it manages on the cloud side. The overlap is the point: `conn`
// is a connection in both trees, and `var` is a plain environment variable in
// both, rather than an Airflow Variable in one and not the other.
func envKinds() []envKind {
	return []envKind{
		{
			aliases: []string{"var", "variables", "vars"},
			kind:    localenv.KindEnv,
			arg:     "<NAME>",
			article: "an",
			label:   "environment variable",
		},
		{
			aliases: []string{"conn", "connections"},
			kind:    localenv.KindConn,
			arg:     "<id>",
			article: "a",
			label:   "connection",
		},
		{
			aliases: []string{"airflow-var", "airflow-vars", "airflow-variables"},
			kind:    localenv.KindVar,
			arg:     "<key>",
			article: "an",
			label:   "Airflow variable",
		},
	}
}

// newEnvKindCmd builds one noun with its verbs. Every noun carries the same
// four, so what a user learns on one transfers to the others.
func newEnvKindCmd(c *cli, scope *scopeFlags, k envKind) *cobra.Command {
	cmd := &cobra.Command{
		// The subcommand word comes from localenv.Noun, not a second copy here.
		// Spelled twice, a rename leaves every hint composed from Noun naming a
		// command that no longer exists — which is the failure Noun exists to
		// prevent, so it cannot be the one thing that drifts from it.
		Use:                        localenv.Noun(k.kind),
		Aliases:                    k.aliases,
		SuggestionsMinimumDistance: 2,
		Short:                      "Manage local " + k.label + "s",
		Args:                       cobra.ArbitraryArgs,
		RunE:                       helpOrUnknownSubcommand,
	}
	// Reads, then the write, then the destructive one — the same order
	// `astro env` uses for the same four verbs, so help reads the same
	// whichever tree you are in. TestEnvVerbOrderMatchesTheCloudTree pins it.
	cmd.AddCommand(
		newEnvListCmd(c, scope, k.kind, "List "+k.label+"s with the source each resolves from"),
		newEnvGetCmd(c, scope, k),
		newEnvSetCmd(c, scope, k),
		newEnvDeleteCmd(c, scope, k),
	)
	return cmd
}

func newEnvSetCmd(c *cli, scope *scopeFlags, k envKind) *cobra.Command {
	in := &setInput{}
	fields := &connFields{}
	long := "Set a value in a .env file, or in the encrypted vault with --secret. The value never comes from a " +
		"bare argument — it would land in shell history and `ps`. By default `set` prompts with echo off; pass " +
		"--stdin to read it from a pipe, or --value to pass it inline (which is visible in shell history).\n\n" +
		"--secret stores the value in the vault this machine shares with Astro Desktop, so a value set in " +
		"either tool is readable in the other. It needs an OS keyring: on a headless machine or in CI there " +
		"is none, and the command refuses rather than quietly writing a credential to a plain file. " +
		"--project/--global choose the scope either way."
	if k.kind == localenv.KindConn {
		long += "\n\nA connection can be given whole — a URI or its JSON, through the prompt, --stdin or --value — " +
			"or field by field with --type and friends, which are the flags `astro env connection set` takes for the " +
			"same object. The two ways are exclusive: the field flags describe a whole connection too, not a patch."
	}
	cmd := &cobra.Command{
		Use:   "set " + k.arg,
		Short: "Set " + k.article + " " + k.label,
		Long:  long,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			value, err := c.readSetValue(cmd, in, fields, k.kind, args[0])
			if err != nil {
				return err
			}
			return c.runEnvSet(scope, k.kind, args[0], value)
		},
	}
	cmd.Flags().BoolVar(&scope.secret, "secret", false, vaultFlagHelp)
	cmd.Flags().BoolVar(&in.stdin, "stdin", false, "Read the value from stdin instead of prompting")
	cmd.Flags().StringVar(&in.value, "value", "", "Pass the value inline (visible in shell history; prefer a prompt or --stdin)")
	if k.kind == localenv.KindConn {
		addConnFieldFlags(cmd, fields)
	}
	return cmd
}

// connFields is the field-by-field way to describe a connection, mirroring the
// flags `astro env connection set` takes on the cloud side.
type connFields struct {
	connType string
	host     string
	login    string
	password string
	schema   string
	port     int
	extra    string
}

// connFieldFlagNames is the set that makes a set "field-wise" rather than
// whole-value. --value excludes these; --stdin deliberately does not, because
// in field mode it is how the password arrives without going through argv.
var connFieldFlagNames = []string{"type", "host", "login", "password", "schema", "port", "extra"}

// addConnFieldFlags registers the per-field flags on a connection's `set`.
//
// The long names are the cloud sibling's, so the same invocation describes the
// same connection in both trees. The single-letter forms are not: cloud has
// -t/-l/-p free, but here -p next to --project reads as the scope flag, and a
// shorthand that invites the wrong guess is worse than no shorthand.
func addConnFieldFlags(cmd *cobra.Command, f *connFields) {
	cmd.Flags().StringVar(&f.connType, "type", "", "Connection type (e.g. postgres, http)")
	cmd.Flags().StringVar(&f.host, "host", "", "Connection host")
	cmd.Flags().StringVar(&f.login, "login", "", "Connection login or username")
	cmd.Flags().StringVar(&f.password, "password", "", "Connection password. Visible in shell history, so prefer piping it (or --stdin), which reads the password from stdin.")
	cmd.Flags().StringVar(&f.schema, "schema", "", "Connection schema")
	cmd.Flags().IntVar(&f.port, "port", 0, "Connection port")
	cmd.Flags().StringVar(&f.extra, "extra", "", "Extra configuration as a JSON object string")
	// --value is exclusive with the fields: both describe a whole connection,
	// so taking them together has no meaning. --stdin is NOT, because in field
	// mode it is how the password arrives without going through argv.
	for _, n := range connFieldFlagNames {
		cmd.MarkFlagsMutuallyExclusive("value", n)
	}
	// --stdin names where a secret comes from, so pairing it with a flag that
	// also supplies one means half the input is silently thrown away. Refusing
	// the combination says so instead.
	cmd.MarkFlagsMutuallyExclusive("stdin", "password")
}

// connFieldsGiven reports whether the user described the connection field-wise
// rather than handing over a whole value.
func connFieldsGiven(cmd *cobra.Command) bool {
	for _, n := range connFieldFlagNames {
		if cmd.Flags().Changed(n) {
			return true
		}
	}
	return false
}

// connValueFromFields assembles the fields into the stored connection value.
//
// It encodes through airflowenv, the same codec a whole-value set normalizes
// into, so a connection built field by field is byte-identical to the same
// connection given as a URI. Assembling the JSON here instead would be a
// second definition of the stored shape.
// The connection id is deliberately not a parameter: EncodeConnValue writes
// the value only — there is no conn_id field in it — and Store.Set derives the
// id from the key it was given. Taking one here would imply it tags the record.
func connValueFromFields(f *connFields, password string) (string, error) {
	if f.connType == "" {
		return "", errors.New("a connection needs a type: pass --type, or give the whole connection as a URI or JSON")
	}
	conn := connmodel.Connection{
		ConnType:     f.connType,
		ConnHost:     f.host,
		ConnLogin:    f.login,
		ConnPassword: password,
		ConnSchema:   f.schema,
		ConnPort:     f.port,
	}
	if f.extra != "" {
		extra, err := airflowenv.DecodeExtra(f.extra)
		if err != nil {
			return "", fmt.Errorf("--%w", err)
		}
		conn.ConnExtra = extra
	}
	return airflowenv.EncodeConnValue(conn)
}

func newEnvGetCmd(c *cli, scope *scopeFlags, k envKind) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get " + k.arg,
		Short: "Print " + k.article + " " + k.label + " and the source it resolves from",
		Args:  cobra.ExactArgs(1),
		RunE:  func(_ *cobra.Command, args []string) error { return c.runEnvGet(scope, k.kind, args[0]) },
	}
	cmd.Flags().BoolVar(&scope.secret, "secret", false, vaultFlagHelp)
	return cmd
}

func newEnvDeleteCmd(c *cli, scope *scopeFlags, k envKind) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete " + k.arg,
		Aliases: []string{"rm"},
		Short:   "Remove " + k.article + " " + k.label,
		Args:    cobra.ExactArgs(1),
		RunE:    func(_ *cobra.Command, args []string) error { return c.runEnvDelete(scope, k.kind, args[0]) },
	}
	cmd.Flags().BoolVar(&scope.secret, "secret", false, vaultFlagHelp)
	return cmd
}

// newEnvListCmd builds a list. only names the single kind to show, or is empty
// for the cross-kind view that sits beside the nouns, which `astro env list`
// mirrors on the cloud side. This one additionally reports where each value
// resolves from, and is what the start-time missing-value report is built
// from; the cloud has no resolution chain to report.
func newEnvListCmd(c *cli, scope *scopeFlags, only localenv.Kind, short string) *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   short,
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return c.runEnvList(scope, all, only)
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
	// Only a plain file can be committed by accident; the vault holds nothing
	// inside the project, so it reports no dotenv path.
	if path := store.DotenvPath(); path != "" {
		c.warnUnignoredEnv(path, store.ScopeName(), projectDir)
	}
	res := envResult{Kind: kind, Name: name, Scope: store.ScopeName(), Status: "set"}
	return r.Emit(res, func(w io.Writer) error {
		_, werr := fmt.Fprintf(w, "set %s %s in %s (%s)\n", localenv.Noun(kind), name, store.ScopeName(), store.Location())
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
	if scope.project || scope.global || scope.secret {
		store, _, err := c.envStore(scope)
		if err != nil {
			return err
		}
		value, ok, err := store.Get(kind, name)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%s %q is not set in %s", localenv.Noun(kind), name, store.ScopeName())
		}
		return emitValue(r, envValue{Kind: kind, Name: name, Source: string(store.ScopeName()), Value: value})
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
		return fmt.Errorf("%q is not a valid %s name", name, localenv.Noun(kind))
	}
	for _, p := range src.Providers(vaultenv.Load(projectDir).Providers()) {
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
	return fmt.Errorf("%s %q is not set anywhere (shell env, project .env, the shared vault, or global ~/.astro/env)", localenv.Noun(kind), name)
}

// getFromWorkspace resolves a workspace-source name from Environment Manager
// workspaceProvider adapts the Astro client this command was wired with into
// the constructor plan.Options asks for, and returns nil when there is no
// client — which is what leaves workspace-source names unresolved offline.
//
// plan takes a constructor rather than the client because the workspace comes
// from the manifest, which plan is what reads. Building it here keeps the Astro
// client in cmd/, where reaching into a platform is allowed.
func (c *cli) workspaceProvider() func(string, bool) envresolve.Provider {
	if c.d.AstroV1Client == nil {
		return nil
	}
	return func(workspace string, reveal bool) envresolve.Provider {
		return emenv.NewProvider(workspace, c.d.AstroV1Client, reveal)
	}
}

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
	return "", "", false, fmt.Errorf("%s %q resolves from the workspace but has no value: %s", localenv.Noun(kind), name, envresolve.Diagnose(wp, key))
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
		return fmt.Errorf("%s %q is not set in %s", localenv.Noun(kind), name, store.ScopeName())
	}
	res := envResult{Kind: kind, Name: name, Scope: store.ScopeName(), Status: "deleted"}
	return r.Emit(res, func(w io.Writer) error {
		_, werr := fmt.Fprintf(w, "deleted %s %s from %s (%s)\n", localenv.Noun(kind), name, store.ScopeName(), store.Location())
		return werr
	})
}

// runEnvList renders the list. only names a single kind to keep, or is empty
// for the cross-kind view; it filters the rendered rows rather than the query,
// so a narrowed list reports exactly what the full one would have for that
// kind, resolution order and all.
func (c *cli) runEnvList(scope *scopeFlags, all bool, only localenv.Kind) error {
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
	// The vault tiers, or a name held only there reports as "absent" while start
	// injects it and get returns it.
	opts.VaultProviders = vaultenv.Load(projectDir).Providers()
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
	if only != "" {
		kept := make([]localenv.ListItem, 0, len(items))
		for _, it := range items {
			if it.Kind == only {
				kept = append(kept, it)
			}
		}
		items = kept
	}
	if r.Format == FormatJSON {
		for _, it := range items {
			if err := r.Emit(it, nil); err != nil {
				return err
			}
		}
		return nil
	}
	return r.Emit(items, func(w io.Writer) error { return renderEnvList(w, items, only) })
}

// renderEnvList prints the table. only names the kind the rows were narrowed
// to, or is empty for the cross-kind view; it exists for the empty case, where
// the cross-kind wording ("no entries in any .env") is simply false when the
// project holds values of the other kinds.
func renderEnvList(w io.Writer, items []localenv.ListItem, only localenv.Kind) error {
	if len(items) == 0 {
		if only != "" {
			_, err := fmt.Fprintf(w, "No %ss declared, and none in any .env.\n", localenv.Noun(only))
			return err
		}
		_, err := fmt.Fprintln(w, "No declared env values and no entries in any .env.")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	// The KIND cell names the subcommand that manages the value, the same as
	// every other user-facing string in this tree. The JSON `kind` field keeps
	// env/conn/var, because that one is a wire contract; this one is a table a
	// person reads, and printing `var` for an Airflow Variable contradicts the
	// grammar, where `var` is an alias for a plain environment variable.
	fmt.Fprintln(tw, "KIND\tNAME\tSOURCE\tNOTE")
	for _, it := range items {
		note := ""
		if it.Orphan {
			note = "orphan"
			// An orphan found under --all lives in another project's file, and
			// says which. There is no command to offer for one: delete acts on
			// the working directory's project, so a hint would name a command
			// that edits the wrong file, which is why ListItem leaves
			// RemoveHint empty for these. The label follows the hint rather
			// than being printed regardless, or the row reads "remove: " with
			// nothing after it.
			if it.Project != "" {
				note += " in " + it.Project
			}
			if it.RemoveHint != "" {
				note += "; remove: " + it.RemoveHint
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", localenv.Noun(it.Kind), it.Name, it.Source, note)
	}
	return tw.Flush()
}

// envStore resolves the flags to the one store a set/get/delete acts on,
// returning it and the project dir (empty for global scope with no project).
//
// Two independent choices. The scope: --project outside a project is an error;
// the default is project inside one, global otherwise. And the store: --secret
// selects the shared vault, anything else a plain dotenv file. The scope is
// decided first and identically for both, so --secret never changes which tier
// a value belongs to.
func (c *cli) envStore(scope *scopeFlags) (valueStore, string, error) {
	if scope.project && scope.global {
		return nil, "", errors.New("--project and --global are mutually exclusive")
	}
	projectDir, perr := c.discoverProject()
	global := scope.global || (!scope.project && perr != nil)
	if scope.project && perr != nil {
		return nil, "", perr
	}
	if scope.secret {
		// The vault's project tier is keyed by the project path, so a global
		// write must not pass one.
		dir := projectDir
		if global {
			dir = ""
		}
		w, err := vaultenv.NewWriter(dir)
		if err != nil {
			return nil, "", err
		}
		return w, dir, nil
	}
	if global {
		store, err := localenv.GlobalStore()
		// projectDir is still reported for a --global write inside a project:
		// the gitignore warning is about the project, not about the file.
		if scope.global {
			return fileStore{store}, projectDir, err
		}
		return fileStore{store}, "", err
	}
	return fileStore{localenv.ProjectStore(projectDir)}, projectDir, nil
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
	schema, err := envschema.ParseSchema(m.Astro.Env)
	if err != nil {
		return nil, nil, err
	}
	return m, schema, nil
}

// readSetValue resolves the value for a `set`: --value inline, else --stdin or
// a piped stdin, else a no-echo prompt. A bare positional value is never
// accepted.
func (c *cli) readSetValue(cmd *cobra.Command, in *setInput, fields *connFields, kind localenv.Kind, name string) (string, error) {
	// Field flags describe the whole connection, so they answer the value
	// question outright. This is checked before the piped-stdin branch below,
	// which would otherwise win in CI — stdin is not a terminal there, so a
	// connection the user spelled out in flags would be overwritten by an
	// empty read.
	if kind == localenv.KindConn && connFieldsGiven(cmd) {
		password, err := c.connFieldPassword(cmd, in, fields, name)
		if err != nil {
			return "", err
		}
		return connValueFromFields(fields, password)
	}
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
	fmt.Fprintf(c.d.Stderr, "Value for %s %s: ", localenv.Noun(kind), name)
	b, err := term.ReadPassword(int(f.Fd()))
	fmt.Fprintln(c.d.Stderr)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// connFieldPassword resolves the password for a field-wise connection set.
//
// Field mode cannot use the whole-value paths — those describe a different
// input — so without this there is no way to supply a password except on
// argv, which is the leak this command's help tells you to avoid. Worse, a
// piped password was read by nobody and the set reported success, writing a
// connection that cannot authenticate.
//
// Order matches the cloud sibling: the flag if given, else stdin when it is
// piped or --stdin was passed. There is deliberately no TTY prompt — many
// connection types are passwordless (http, fs, a role-based aws), and
// prompting by default would block the common case.
//
// An empty read means "no password", not "the password is the empty string".
// Field mode builds a whole connection, so an absent password is simply an
// absent field, and this is what keeps a stray pipe from blanking one.
func (c *cli) connFieldPassword(cmd *cobra.Command, in *setInput, f *connFields, name string) (string, error) {
	if cmd.Flags().Changed("password") {
		return f.password, nil
	}

	var password string
	file, isFile := c.d.Stdin.(*os.File)
	piped := !isFile || !term.IsTerminal(int(file.Fd()))
	if in.stdin || piped {
		b, err := io.ReadAll(c.d.Stdin)
		if err != nil {
			return "", fmt.Errorf("reading the connection password from stdin: %w", err)
		}
		password = strings.TrimRight(string(b), "\r\n")
	}

	// Passwordless is legitimate — http, fs, an aws connection on an instance
	// role — so ending with none is neither prompted for nor an error. But a
	// login with no password is almost always the mistake it looks like, and
	// the set otherwise reports plain success for a connection that cannot
	// authenticate.
	//
	// The check is on the result rather than on which branch ran, because the
	// two ways of arriving at no password both matter: a terminal with nothing
	// piped, and — the one that bites in CI — a pipe that turned out empty.
	if password == "" && f.login != "" {
		fmt.Fprintf(c.d.Stderr,
			"warning: connection %s has a login but no password; pipe one, or pass --password, if it needs one\n",
			name)
	}
	return password, nil
}

// warnUnignoredEnv prints a stderr warning when a project .env is not covered
// by .gitignore, with the fix. Best-effort: a stat error is not worth failing
// a successful set.
func (c *cli) warnUnignoredEnv(path string, scope localenv.Scope, projectDir string) {
	if scope != localenv.ScopeProject || projectDir == "" {
		return
	}
	ignored, err := scaffold.EnvIgnored(projectDir)
	if err != nil || ignored {
		return
	}
	fmt.Fprintf(c.d.Stderr, "warning: %s is not covered by .gitignore; add a line `.env` to it so the file is never committed\n", path)
}
