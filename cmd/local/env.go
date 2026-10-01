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
const vaultFlagHelp = "Use the encrypted vault instead of a plain file"

// scopeFlags carries the shared --project/--global choice and the --secret
// store choice. It is filled by cobra before any RunE runs, so every env leaf
// reads the same struct.
//
// The two are separate axes on purpose: --project/--global pick WHICH SCOPE a
// value belongs to, --secret picks WHICH STORE holds it. They compose, so
// `--global --secret` is a machine-wide secret and `--global` alone is a
// machine-wide plaintext default.
//
// secretGiven records whether --secret was on the command line at all, which
// the value of secret cannot say: `--secret=false` and no flag both leave it
// false, and only the first is a request for a plain file. set refuses that
// request for a name the manifest declares sensitive, and the way to keep an
// undeclared connection or Airflow variable out of the vault; see routeSet.
type scopeFlags struct {
	project     bool
	global      bool
	secret      bool
	secretGiven bool
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
	// Has reports whether the store holds a value for (kind, name) without
	// decrypting one, so a read can pick between two stores without opening
	// the keyring for the one it does not use.
	Has(kind localenv.Kind, name string) (bool, error)
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

func (f fileStore) Has(kind localenv.Kind, name string) (bool, error) {
	_, ok, err := f.Get(kind, name)
	return ok, err
}

// setInput carries the shared value-source flags for `set`.
type setInput struct {
	stdin bool
	value string
	// everywhere creates a new global vault entry reaching every project
	// instead of none.
	everywhere bool
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
		Short: "Manage environment values for local Airflow",
		// Wrapped by hand at the same width as `astro env --help`, which cobra
		// does not do for us. What used to follow here moved to where it is
		// needed: the keyring requirement for --secret is on `set --help`, and
		// the files' 0600 mode is in docs/v2-secrets.md.
		Long: "Manage environment values for local Airflow: environment variables,\n" +
			"connections, and Airflow variables.\n\n" +
			"Values live in the project's .env, or ~/.astro/env with --global. Pass --secret\n" +
			"to use the encrypted vault instead.\n\n" +
			"At start, each value comes from the first of: shell env, project .env, project\n" +
			"vault, global vault, ~/.astro/env, the linked workspace's Environment Manager,\n" +
			"a declaration's default. Every source reaches Airflow whole, declared or not.\n" +
			"A workspace that cannot be read is skipped with a note.",
		Args:                       cobra.ArbitraryArgs,
		RunE:                       helpOrUnknownSubcommand,
		SuggestionsMinimumDistance: 2,
	}
	cmd.PersistentFlags().BoolVar(&scope.project, "project", false, "Act on the project's .env (the default inside a project)")
	cmd.PersistentFlags().BoolVar(&scope.global, "global", false, "Act on the global ~/.astro/env (the default outside a project)")
	for _, k := range envKinds() {
		cmd.AddCommand(newEnvKindCmd(c, scope, k))
	}
	cmd.AddCommand(newEnvListCmd(c, scope, "", "List every declared value and where it resolves from"))
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
	example string
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
			example: `
  # set a variable (prompts for the value, echo off)
  astro local env variable set API_TOKEN

  # read the value from a pipe, or keep it in the encrypted vault
  echo "$TOKEN" | astro local env variable set API_TOKEN --stdin
  astro local env variable set API_TOKEN --secret

  # show, list and delete
  astro local env variable get API_TOKEN
  astro local env variable list
  astro local env variable delete API_TOKEN

  # declare it in pyproject.toml, so a start requires it, and keep it in the vault
  astro local env variable declare API_TOKEN --sensitive --description 'Token for the API'`,
		},
		{
			aliases: []string{"conn", "connections"},
			kind:    localenv.KindConn,
			arg:     "<id>",
			article: "a",
			label:   "connection",
			example: `
  # set from a URI (or connection JSON)
  astro local env connection set db_main --value 'postgres://admin@db.example.com:5432/warehouse'

  # or field by field, with the password piped
  echo "$PW" | astro local env connection set db_main \
    --type postgres --host db.example.com --login admin --port 5432

  # show, list and delete
  astro local env connection get db_main
  astro local env connection list
  astro local env connection delete db_main

  # declare it in pyproject.toml, resolved from the workspace when no file sets it
  astro local env connection declare db_main --type postgres --source workspace`,
		},
		{
			aliases: []string{"airflow-var", "airflow-vars", "airflow-variables"},
			kind:    localenv.KindVar,
			arg:     "<key>",
			article: "an",
			label:   "Airflow variable",
			example: `
  # set a variable
  astro local env airflow-variable set region --value us-east-1

  # show, list and delete
  astro local env airflow-variable get region
  astro local env airflow-variable list
  astro local env airflow-variable delete region

  # declare it with a committed default, or remove the declaration
  astro local env airflow-variable declare region --default us-east-1
  astro local env airflow-variable undeclare region`,
		},
	}
}

// newEnvKindCmd builds one noun with its verbs. Every noun carries the same
// six, so what a user learns on one transfers to the others.
func newEnvKindCmd(c *cli, scope *scopeFlags, k envKind) *cobra.Command {
	cmd := &cobra.Command{
		// The subcommand word comes from localenv.Noun, not a second copy here.
		// Spelled twice, a rename leaves every hint composed from Noun naming a
		// command that no longer exists — which is the failure Noun exists to
		// prevent, so it cannot be the one thing that drifts from it.
		Use:                        localenv.Noun(k.kind),
		Aliases:                    k.aliases,
		SuggestionsMinimumDistance: 2,
		Short:                      "Manage " + k.label + "s",
		Long:                       "Manage " + k.label + "s for local Airflow.",
		Example:                    k.example,
		Args:                       cobra.ArbitraryArgs,
		RunE:                       helpOrUnknownSubcommand,
	}
	// Reads, then the write, then the destructive one — the same order
	// `astro env` uses for the same four verbs, so help reads the same
	// whichever tree you are in. The two declaration verbs, which the cloud
	// tree has no counterpart for, follow them, and then the two that decide
	// which projects a global vault entry reaches. TestEnvVerbOrderMatchesTheCloudTree
	// pins it.
	cmd.AddCommand(
		newEnvListCmd(c, scope, k.kind, "List "+k.label+"s and where each resolves from"),
		newEnvGetCmd(c, scope, k),
		newEnvSetCmd(c, scope, k),
		newEnvDeleteCmd(c, scope, k),
		newEnvDeclareCmd(c, scope, k),
		newEnvUndeclareCmd(c, scope, k),
		newEnvLinkCmd(c, scope, k),
		newEnvUnlinkCmd(c, scope, k),
	)
	return cmd
}

func newEnvSetCmd(c *cli, scope *scopeFlags, k envKind) *cobra.Command {
	in := &setInput{}
	fields := &connFields{}
	long := "Set " + k.article + " " + k.label + ", creating it if it does not exist.\n\n" +
		"The value comes from a prompt with echo off, --stdin, or --value; never a bare\n" +
		"argument, which would land in shell history. --secret stores it in the\n" +
		"encrypted vault, which needs an OS keyring.\n\n" +
		"Connections and Airflow variables go to the vault by default; --secret=false\n" +
		"keeps one in a plain file. An environment variable goes to the vault when\n" +
		"the project's pyproject.toml declares it sensitive. --secret=false is refused\n" +
		"for a declared-sensitive name.\n\n" +
		"A new global in the vault reaches no project until you link it with\n" +
		"`astro local env " + localenv.Noun(k.kind) + " link`, as in Astro Desktop. --everywhere creates it\n" +
		"reaching every project instead. Updating an existing global keeps its links.\n" +
		"A global goes to the Airflow of every project it reaches, whether or not the\n" +
		"project declares it."
	if k.kind == localenv.KindConn {
		long += "\n\nGive the connection whole, as a URI or JSON, or field by field with --type,\n" +
			"--host and the rest."
	}
	cmd := &cobra.Command{
		Use:   "set " + k.arg,
		Short: "Set " + k.article + " " + k.label,
		Long:  long,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			scope.secretGiven = cmd.Flags().Changed("secret")
			// The route is decided before the value is read, so a save the
			// manifest will refuse fails before prompting for a credential.
			route, err := c.routeSet(scope, k.kind, args[0])
			if err != nil {
				return err
			}
			value, err := c.readSetValue(cmd, in, fields, k.kind, args[0])
			if err != nil {
				return err
			}
			return c.runEnvSet(route, k.kind, args[0], value, in.everywhere)
		},
	}
	cmd.Flags().BoolVar(&scope.secret, "secret", false, vaultFlagHelp)
	cmd.Flags().BoolVar(&in.stdin, "stdin", false, "Read the value from stdin instead of prompting")
	cmd.Flags().BoolVar(&in.everywhere, "everywhere", false, "With --global, create a new vault entry reaching every project instead of none")
	valueHelp := "The value; omit it to be prompted with echo off, or use --stdin"
	if k.kind == localenv.KindConn {
		valueHelp = "The whole connection as a URI or JSON; omit it to be prompted, or use --stdin"
	}
	cmd.Flags().StringVar(&in.value, "value", "", valueHelp)
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
	cmd.Flags().StringVar(&f.password, "password", "", "Connection password; prefer piping it, since a flag lands in shell history")
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
		Short: "Show " + k.article + " " + k.label + " and where it resolves from",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			scope.secretGiven = cmd.Flags().Changed("secret")
			return c.runEnvGet(scope, k.kind, args[0])
		},
	}
	cmd.Flags().BoolVar(&scope.secret, "secret", false, vaultFlagHelp)
	return cmd
}

func newEnvDeleteCmd(c *cli, scope *scopeFlags, k envKind) *cobra.Command {
	var undeclare bool
	cmd := &cobra.Command{
		Use:     "delete " + k.arg,
		Aliases: []string{"rm"},
		Short:   "Delete " + k.article + " " + k.label,
		Long: "Delete " + k.article + " " + k.label + "'s value.\n\n" +
			"A declaration in the project's pyproject.toml is not a value, so delete leaves\n" +
			"it in place and says what the name resolves to now: another source, nothing\n" +
			"(list shows it as absent), or, for a required name, nothing the next start\n" +
			"will accept. --undeclare also removes the declaration, as undeclare does.\n" +
			"With --global it removes it only from the current project's pyproject.toml,\n" +
			"and refuses outside a project.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			scope.secretGiven = cmd.Flags().Changed("secret")
			return c.runEnvDelete(scope, k.kind, args[0], undeclare)
		},
	}
	cmd.Flags().BoolVar(&scope.secret, "secret", false, vaultFlagHelp)
	cmd.Flags().BoolVar(&undeclare, "undeclare", false, "Also remove the name's declaration from the current project's pyproject.toml")
	return cmd
}

// newEnvListCmd builds a list. only names the single kind to show, or is empty
// for the cross-kind view that sits beside the nouns, which `astro env list`
// mirrors on the cloud side. This one additionally reports where each value
// resolves from, and is what the start-time missing-value report is built
// from; the cloud has no resolution chain to report.
func newEnvListCmd(c *cli, scope *scopeFlags, only localenv.Kind, short string) *cobra.Command {
	var all bool
	// Only the cross-kind list gets a Long, matching `astro env list`; a
	// noun's own list says all it needs in its Short.
	long := ""
	if only == "" {
		long = "List every declared value and where it resolves from, and every undeclared\n" +
			"value a start passes to this project anyway. Whatever reaches a project goes\n" +
			"to its Airflow, declared or not: declaring a name makes it a requirement, and\n" +
			"`link` narrows which projects a global reaches. Values are not shown; use get\n" +
			"to see one."
	}
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   short,
		Long:    long,
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return c.runEnvList(scope, all, only)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "Also include the global file, every known project's .env, and vault globals not linked to this project")
	return cmd
}

// envResult is the set/delete status object, rendered the same in text and
// json.
type envResult struct {
	Kind   localenv.Kind  `json:"kind"`
	Name   string         `json:"name"`
	Scope  localenv.Scope `json:"scope"`
	Status string         `json:"status"`
	// The rest is a delete's alone, and only in a project that declares the
	// name: Remainder is what the declaration leaves (supplied, absent or
	// required), Source what supplies it when supplied, and the hints the
	// commands that settle it. Undeclared reports that --undeclare removed the
	// declaration from Manifest, in which case there is no remainder.
	Remainder     envschema.Remainder `json:"remainder,omitempty"`
	Source        string              `json:"source,omitempty"`
	SetHint       string              `json:"set_hint,omitempty"`
	UndeclareHint string              `json:"undeclare_hint,omitempty"`
	Undeclared    bool                `json:"undeclared,omitempty"`
	// Workspace is the linked workspace's id when nothing local supplies the
	// name: a delete reads no workspace, so the name may still come from it.
	Workspace string `json:"workspace,omitempty"`
	Manifest  string `json:"manifest,omitempty"`
}

// envValue is the get object: the one deliberate reveal.
type envValue struct {
	Kind   localenv.Kind `json:"kind"`
	Name   string        `json:"name"`
	Source string        `json:"source"`
	Value  string        `json:"value"`
	// Reach is where a global vault entry is linked; absent for every other
	// source, since only those have link state.
	Reach *reachJSON `json:"reach,omitempty"`
}

// envRoute is the store choice a set acts on once the defaults and the manifest
// have had their say: the flags as given, or the flags with the vault chosen.
type envRoute struct {
	scope *scopeFlags
	// vaultedBecause says why the vault was chosen without --secret, as a
	// clause for the messages ("is declared sensitive in <file>"), and is empty
	// when the flags alone chose the store.
	vaultedBecause string
	// declared is true when that reason is a declaration, which --secret=false
	// cannot override, rather than the default for the kind, which it can.
	declared bool
}

// vaultsByDefault reports whether a kind goes to the vault when set without
// --secret. Connections and Airflow variables do, whether or not the manifest
// declares them, as they do in Astro Desktop: both routinely carry credentials,
// and nothing outside Airflow reads them. A plain environment variable stays in
// the .env file unless it is declared sensitive, since most are not secrets and
// other tools read that file.
func vaultsByDefault(kind localenv.Kind) bool {
	return kind == localenv.KindConn || kind == localenv.KindVar
}

// routeSet decides the store for a set of one name:
//
//   - --secret: the vault, and the manifest is not read, since no declaration
//     makes the vault more secret.
//   - A connection or Airflow variable with no --secret flag: the vault, by
//     default. The manifest is not read for this either.
//   - A name the project's manifest declares sensitive: the vault, and
//     --secret=false is refused. That flag is an explicit request for the
//     plaintext file the declaration rules out. Declared connections count,
//     because envschema makes every one of them sensitive.
//   - Anything else, including an undeclared connection or Airflow variable
//     with --secret=false, the escape hatch where there is no keyring: the
//     plain file.
//
// Where the answer depends on the declarations and they do not read, the set is
// refused rather than guessing. Treating an unreadable manifest as "nothing is
// sensitive" decides the question from an answer that does not give it, and
// the result is a declared credential in a plaintext file. `astro local start`
// refuses the same project, so declining the save until it is fixed is the
// smaller surprise. Outside a project there is no manifest to consult.
func (c *cli) routeSet(scope *scopeFlags, kind localenv.Kind, name string) (envRoute, error) {
	route := envRoute{scope: scope}
	if scope.secret {
		return route, nil
	}
	if vaultsByDefault(kind) && !scope.secretGiven {
		return vaultRoute(scope, "is "+kindPhrase(kind)+", and those are stored in the vault by default", false), nil
	}
	projectDir, err := c.discoverProject()
	if err != nil {
		return route, nil //nolint:nilerr // outside a project there are no declarations; envStore reports a --project that needed one
	}
	manifestPath := filepath.Join(projectDir, project.Marker)
	_, schema, err := c.loadManifestSchema(projectDir)
	if err != nil {
		return route, fmt.Errorf("cannot tell whether %s declares %s sensitive: %w", manifestPath, name, err)
	}
	spec, ok := declaredSpec(schema, kind, name)
	if !ok || !spec.Sensitive {
		return route, nil
	}
	if scope.secretGiven {
		return route, plaintextRefusal(kind, name, manifestPath)
	}
	return vaultRoute(scope, "is declared sensitive in "+manifestPath, true), nil
}

func vaultRoute(scope *scopeFlags, because string, declared bool) envRoute {
	vaulted := *scope
	vaulted.secret = true
	return envRoute{scope: &vaulted, vaultedBecause: because, declared: declared}
}

// kindPhrase names a vault-by-default kind with its article, for a sentence
// about one name.
func kindPhrase(kind localenv.Kind) string {
	if kind == localenv.KindConn {
		return "a connection"
	}
	return "an Airflow variable"
}

// plaintextRefusal is the error for `set --secret=false` on a declared-sensitive
// name. A connection cannot be declared otherwise, so its message does not
// offer the edit that would allow it.
func plaintextRefusal(kind localenv.Kind, name, manifestPath string) error {
	if kind == localenv.KindConn {
		return fmt.Errorf("connection %s is declared in %s, and a declared connection is always sensitive, "+
			"so it can only be stored in the encrypted vault. Drop --secret=false to store it there",
			name, manifestPath)
	}
	return fmt.Errorf("%s %s is declared sensitive in %s, so it can only be stored in the encrypted vault. "+
		"Drop --secret=false to store it there, or remove `sensitive = true` from its declaration to keep it in a plain file",
		localenv.Noun(kind), name, manifestPath)
}

func (c *cli) runEnvSet(route envRoute, kind localenv.Kind, name, value string, everywhere bool) error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
	store, projectDir, err := c.envStore(route.scope)
	if err != nil {
		return err
	}
	if err := refusePinnedToPlain(store, kind, name); err != nil {
		return err
	}
	globalVault, isGlobalVault := store.(*vaultenv.Writer)
	isGlobalVault = isGlobalVault && store.ScopeName() == vaultenv.SourceGlobal
	if everywhere && !isGlobalVault {
		return errors.New("--everywhere applies to a global in the vault: pass --global, and drop --secret=false")
	}
	created := false
	if isGlobalVault {
		had, herr := store.Has(kind, name)
		if herr != nil {
			return herr
		}
		created = !had
		globalVault.NewEverywhere = everywhere
		// A value moving in from ~/.astro/env is not new: it already reached
		// every project, and the plain copy is about to be
		// removed, so seeding an empty row would take it away from all of
		// them. It moves in with no row.
		if created && plainGlobalHas(kind, name) {
			created = false
			globalVault.NewEverywhere = true
		}
	}
	if _, err := store.Set(kind, name, value); err != nil {
		switch {
		case route.declared:
			return fmt.Errorf("%s %s %s, so it is stored only in the vault: %w",
				localenv.Noun(kind), name, route.vaultedBecause, err)
		case route.vaultedBecause != "":
			return fmt.Errorf("%s %s %s: %w", localenv.Noun(kind), name, route.vaultedBecause, err)
		}
		return err
	}
	if route.declared {
		fmt.Fprintf(c.d.Stderr, "note: %s %s, so it was stored in the vault\n", name, route.vaultedBecause)
	}
	if err := c.removeOtherCopy(route.scope, kind, name); err != nil {
		return err
	}
	if route.scope.global {
		if !c.warnProjectShadows(route.scope, kind, name) {
			c.noteUndeclaredGlobal(kind, name)
		}
	}
	// Warn (on stderr, so json stdout stays clean) when a project .env would
	// be tracked by git — the failure mode that actually leaks secrets.
	// Only a plain file can be committed by accident; the vault holds nothing
	// inside the project, so it reports no dotenv path.
	if path := store.DotenvPath(); path != "" {
		c.warnUnignoredEnv(path, store.ScopeName(), projectDir)
	}
	switch {
	case created && !everywhere:
		fmt.Fprintf(c.d.Stderr, "note: %s reaches no project yet. Link it with `%s`, or re-run with --everywhere.\n",
			name, localenv.LinkHint(kind, name))
	case !created && everywhere:
		fmt.Fprintf(c.d.Stderr, "note: %s already existed, so its links were kept; to reach every project run: %s --everywhere\n",
			name, localenv.LinkHint(kind, name))
	}
	res := envResult{Kind: kind, Name: name, Scope: store.ScopeName(), Status: "set"}
	return r.Emit(res, func(w io.Writer) error {
		_, werr := fmt.Fprintf(w, "set %s %s in %s (%s)\n", localenv.Noun(kind), name, store.ScopeName(), store.Location())
		return werr
	})
}

// removeOtherCopy deletes the name from the store a set did not write, in the
// same scope, so each name has one home per scope. Two copies are worse than
// untidy: the project .env outranks the vault, so a plaintext copy left beside a
// vaulted value keeps winning at start and keeps the credential in a file, and a
// delete against one store leaves the other copy standing. Astro Desktop's
// saves keep the same rule.
//
// The vault side needs no keyring for this: removing a vault entry deletes its
// file, so a plain set on a machine with no keyring still succeeds. A vault that
// cannot even be opened holds nothing to remove.
func (c *cli) removeOtherCopy(scope *scopeFlags, kind localenv.Kind, name string) error {
	flipped := *scope
	flipped.secret = !scope.secret
	other, _, err := c.envStore(&flipped)
	if err != nil {
		return nil //nolint:nilerr // a store that cannot be opened holds no copy to remove
	}
	removed, err := c.deleteKeepingRow(other, kind, name)
	if err != nil {
		return fmt.Errorf("%s %s was set, but its other copy in %s could not be removed: %w",
			localenv.Noun(kind), name, other.Location(), err)
	}
	if removed {
		fmt.Fprintf(c.d.Stderr, "note: removed the other copy of %s from %s (%s)\n", name, other.ScopeName(), other.Location())
	}
	return nil
}

// warnProjectShadows warns, after a --global set inside a project, about a copy
// of the name in the project scope: the project .env and the project vault both
// outrank every global tier, so start keeps using that copy and the value just
// set does not reach this project. It names the command that removes each copy
// rather than removing it: a --global command does not delete project values.
// Best effort, like the gitignore advisory. It reports whether it warned.
func (c *cli) warnProjectShadows(scope *scopeFlags, kind localenv.Kind, name string) bool {
	if _, err := c.discoverProject(); err != nil {
		return false
	}
	warned := false
	for _, secret := range []bool{false, true} {
		projScope := *scope
		projScope.global, projScope.project, projScope.secret = false, true, secret
		st, _, err := c.envStore(&projScope)
		if err != nil {
			continue
		}
		if ok, err := st.Has(kind, name); err != nil || !ok {
			continue
		}
		flag := "--secret=false"
		if secret {
			flag = "--secret"
		}
		fmt.Fprintf(c.d.Stderr,
			"warning: %s is also set in %s (%s), which outranks the global value for this project, so start uses that copy. "+
				"Remove it with: astro local env %s delete %s --project %s\n",
			name, st.ScopeName(), st.Location(), localenv.Noun(kind), name, flag)
		warned = true
	}
	return warned
}

// noteUndeclaredGlobal tells a --global set inside a project that the project
// does not declare the name. A start passes every global that reaches the
// project, declared or not, so the note is not that the value is missing: it
// is that nothing makes the project require it, and a Deployment or a
// teammate's clone will not know to supply it. Silent outside a project
// and when the manifest does not read, where there is nothing to check, and
// after warnProjectShadows, whose project copy wins whether or not it is
// declared.
func (c *cli) noteUndeclaredGlobal(kind localenv.Kind, name string) {
	projectDir, err := c.discoverProject()
	if err != nil {
		return
	}
	_, schema, err := c.loadManifestSchema(projectDir)
	if err != nil {
		return
	}
	if _, ok := declaredSpec(schema, kind, name); ok {
		return
	}
	fmt.Fprintf(c.d.Stderr, "note: this project does not declare %s. It gets the global value wherever the value reaches, "+
		"but declare it to make it a requirement: %s\n",
		name, localenv.DeclareHint(kind, name))
}

func (c *cli) runEnvGet(scope *scopeFlags, kind localenv.Kind, name string) error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
	// A scope or store flag reads that scope; no flag resolves the whole chain
	// and reports the winning source.
	if scope.project || scope.global || scope.secretGiven {
		return c.getScoped(r, scope, kind, name)
	}
	return c.getResolved(r, kind, name)
}

// getScoped reads one scope. --secret reads only its vault and --secret=false
// only its plain file. With neither, it reads both, since set files a value in
// whichever one its routing picked and delete clears both: a get that looked in
// one store would miss what the same flags just set.
//
// set keeps one copy per scope, so at most one store should answer. If both
// do, the one the resolution chain would use wins, the order being the chain's
// within one scope: the project .env outranks the project vault, and the global
// vault outranks ~/.astro/env. A note on stderr names the other copy. The
// manifest is not read.
func (c *cli) getScoped(r Renderer, scope *scopeFlags, kind localenv.Kind, name string) error {
	store, _, err := c.envStore(scope)
	if err != nil {
		return err
	}
	stores := []valueStore{store}
	if !scope.secretGiven {
		vaultScope := *scope
		vaultScope.secret = true
		if vault, _, verr := c.envStore(&vaultScope); verr == nil {
			if string(vault.ScopeName()) == vaultenv.SourceGlobal {
				stores = []valueStore{vault, store}
			} else {
				stores = append(stores, vault)
			}
		}
	}
	var winner valueStore
	for _, st := range stores {
		ok, err := st.Has(kind, name)
		if err != nil {
			return err
		}
		switch {
		case !ok:
		case winner == nil:
			winner = st
		default:
			fmt.Fprintf(c.d.Stderr, "note: %s is also set in %s (%s); showing the %s copy, which is the one Airflow gets\n",
				name, st.ScopeName(), st.Location(), winner.ScopeName())
		}
	}
	if winner == nil {
		names := make([]string, len(stores))
		for i, st := range stores {
			names[i] = string(st.ScopeName())
		}
		return fmt.Errorf("%s %q is not set in %s", localenv.Noun(kind), name, strings.Join(names, " or "))
	}
	value, ok, err := winner.Get(kind, name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%s %q is not set in %s", localenv.Noun(kind), name, winner.ScopeName())
	}
	v := envValue{Kind: kind, Name: name, Source: string(winner.ScopeName()), Value: value}
	if w, isVault := winner.(*vaultenv.Writer); isVault {
		v.Reach = globalReach(w, kind, name)
	}
	return c.emitValue(r, v)
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
			ev := envValue{Kind: kind, Name: name, Source: p.Label(), Value: v}
			if p.Label() == vaultenv.SourceGlobal {
				if w, err := vaultenv.NewWriter(""); err == nil {
					ev.Reach = globalReach(w, kind, name)
				}
			}
			return c.emitValue(r, ev)
		}
	}
	// No local source held it. The linked workspace resolves it from
	// Environment Manager — the one place `get` reveals a cloud value, and only
	// for the single name asked.
	v, source, ok, err := c.getFromWorkspace(projectDir, kind, name, key)
	if err != nil {
		return err
	}
	if ok {
		return c.emitValue(r, envValue{Kind: kind, Name: name, Source: source, Value: v})
	}
	msg := fmt.Sprintf("%s %q is not set anywhere (shell env, project .env, the encrypted vault, or global ~/.astro/env)", localenv.Noun(kind), name)
	// A global the vault holds that does not reach this checkout is the one
	// absence worth explaining: the value exists, and the fix is a link.
	for _, p := range vaultenv.Load(projectDir).Providers() {
		if d, ok := p.(envresolve.Diagnoser); ok {
			if why := d.Diagnose(key); why != "" {
				msg += ": " + why
				break
			}
		}
	}
	return errors.New(msg)
}

// getFromWorkspace resolves a workspace-source name from Environment Manager
// workspaceProvider adapts the Astro client this command was wired with into
// the constructor plan.Options asks for, and returns nil when there is no
// client — which is what leaves workspace-source names unresolved offline.
//
// plan takes a constructor rather than the client because the workspace comes
// from the manifest, which plan is what reads. Building it here keeps the Astro
// client in cmd/, where reaching into a platform is allowed.
func (c *cli) workspaceProvider() func(workspace, domain string, reveal bool) envresolve.Provider {
	if c.d.WorkspaceClients == nil {
		return nil
	}
	return func(workspace, domain string, reveal bool) envresolve.Provider {
		return emenv.NewProvider(workspace, domain, c.d.WorkspaceClients, reveal)
	}
}

// for `get`: a workspace-source name, or any name when the manifest links a
// workspace, since that tier reaches every name. It returns ok=false (no
// error) when the workspace does not supply it and the name has no workspace
// source, so the caller reports the plain "not set anywhere". A
// workspace-source name that cannot be fetched is an error naming the cause.
func (c *cli) getFromWorkspace(projectDir string, kind localenv.Kind, name, key string) (value, source string, ok bool, err error) {
	if projectDir == "" || c.d.WorkspaceClients == nil {
		return "", "", false, nil
	}
	m, schema, err := c.loadManifestSchema(projectDir)
	if err != nil || schema == nil {
		return "", "", false, err
	}
	workspaceSource := declaredSource(schema, kind, name) == envschema.SourceWorkspace
	if !workspaceSource && m.Astro.Workspace == "" {
		return "", "", false, nil
	}
	// reveal = true: get is the one deliberate reveal of a value.
	wp := emenv.NewProvider(m.Astro.Workspace, m.Astro.WorkspaceDomain(), c.d.WorkspaceClients, true)
	if v, has := wp.Lookup(key); has {
		return v, localenv.WorkspaceSource(m.Astro.Workspace), true, nil
	}
	if !workspaceSource {
		// The linked workspace reaches every name, declared or not, but only a
		// workspace source is expected to be there: any other miss is the
		// plain "not set anywhere".
		return "", "", false, nil
	}
	return "", "", false, fmt.Errorf("%s %q resolves from the workspace but has no value: %s", localenv.Noun(kind), name, envresolve.Diagnose(wp, key))
}

// declaredSource returns a declared name's source, so `get` knows to consult
// Environment Manager for a workspace source.
func declaredSource(schema *envschema.Schema, kind localenv.Kind, name string) envschema.Source {
	spec, _ := declaredSpec(schema, kind, name)
	return spec.Source
}

// declaredSpec returns the declaration of (kind, name) and whether the schema
// has one. A nil schema declares nothing.
func declaredSpec(schema *envschema.Schema, kind localenv.Kind, name string) (envschema.ValueSpec, bool) {
	if schema == nil {
		return envschema.ValueSpec{}, false
	}
	var specs map[string]envschema.ValueSpec
	switch kind {
	case localenv.KindEnv:
		specs = schema.EnvVars
	case localenv.KindVar:
		specs = schema.AirflowVariables
	case localenv.KindConn:
		specs = schema.Connections
	}
	spec, ok := specs[name]
	return spec, ok
}

// emitValue prints a get. The text form's stdout is the value alone, so
// `$(astro local env ... get NAME)` keeps working; a global's reach goes to
// stderr beside it.
func (c *cli) emitValue(r Renderer, v envValue) error {
	return r.Emit(v, func(w io.Writer) error {
		if v.Reach != nil {
			fmt.Fprintf(c.d.Stderr, "Reach: %s\n", v.Reach.text())
		}
		_, werr := fmt.Fprintln(w, v.Value)
		return werr
	})
}

// runEnvDelete removes a name from the scope. With no --secret flag it clears
// both stores, the plain file and the vault, as Astro Desktop's delete does:
// set keeps one copy per scope, so "delete this name" means wherever it is, and
// a copy that got into the other store by hand or before the routing changed is
// the one a delete of a credential most needs to catch. --secret deletes only
// the vault copy, and --secret=false only the plaintext one, which is how a stale
// plaintext copy of a sensitive name is removed on its own.
//
// Removing a vault entry deletes its file and needs no keyring, so the no-flag
// form works where there is none.
//
// The value is all it removes: a declaration of the name stays, and the result
// says what it leaves (see deleteRemainder). undeclare also removes the
// declaration, through the path undeclare takes, and only ever from the
// current project's manifest, so it is refused outside a project before
// anything is deleted, --global or not.
func (c *cli) runEnvDelete(scope *scopeFlags, kind localenv.Kind, name string, undeclare bool) error {
	r, err := c.renderer()
	if err != nil {
		return err
	}
	projectDir, perr := c.discoverProject()
	if undeclare && perr != nil {
		return fmt.Errorf("--undeclare removes a declaration from the current project's %s, and there is no project here, so nothing was deleted. It never edits another project's", project.Marker)
	}
	if undeclare {
		if err := checkUndeclarable(projectDir, kind, name); err != nil {
			return err
		}
	}
	store, err := c.deleteValue(scope, kind, name)
	if err != nil {
		if undeclare && errors.Is(err, errValueNotSet) {
			return fmt.Errorf("%w, so nothing was deleted. To remove only its declaration: %s", err, localenv.UndeclareHint(kind, name))
		}
		return err
	}
	res := envResult{Kind: kind, Name: name, Scope: store.ScopeName(), Status: "deleted"}
	switch {
	case undeclare:
		decl, changed, err := c.undeclareIn(projectDir, kind, name)
		if err != nil {
			return fmt.Errorf("deleted %s %s from %s (%s), but its declaration was not removed: %w",
				localenv.Noun(kind), name, store.ScopeName(), store.Location(), err)
		}
		res.Undeclared, res.Manifest = changed, decl.Manifest
	case perr == nil:
		c.deleteRemainder(&res, projectDir)
	}
	return r.Emit(res, func(w io.Writer) error { return renderDeleted(w, &res, store) })
}

// errValueNotSet is deleteValue's error for a name no store it looked in holds.
var errValueNotSet = errors.New("not set")

// deleteValue removes the value of name from the stores the scope names, and
// returns the one to report: see runEnvDelete.
func (c *cli) deleteValue(scope *scopeFlags, kind localenv.Kind, name string) (valueStore, error) {
	store, _, err := c.envStore(scope)
	if err != nil {
		return nil, err
	}
	if scope.secretGiven {
		ok, err := c.deleteKeepingRow(store, kind, name)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("%s %q is %w in %s", localenv.Noun(kind), name, errValueNotSet, store.ScopeName())
		}
		return store, nil
	}
	// Both stores, the vault first. A vault removal that fails then leaves both
	// copies as they were, so the error is the whole story; a file removal that
	// fails after the vault copy went says that it went.
	vaultScope := *scope
	vaultScope.secret = true
	var vaulted valueStore
	removedVault := false
	if vault, _, verr := c.envStore(&vaultScope); verr == nil {
		removedVault, err = c.deleteKeepingRow(vault, kind, name)
		if err != nil {
			return nil, fmt.Errorf("could not remove %s %s from the vault, so nothing was deleted: %w", localenv.Noun(kind), name, err)
		}
		vaulted = vault
	}
	removedFile, err := store.Delete(kind, name)
	if err != nil {
		if removedVault {
			return nil, fmt.Errorf("deleted %s %s from %s (%s), but its plaintext copy in %s could not be removed: %w",
				localenv.Noun(kind), name, vaulted.ScopeName(), vaulted.Location(), store.Location(), err)
		}
		return nil, err
	}
	ok := removedFile || removedVault
	if removedVault {
		// The status names the store that held it; when both did, the vault
		// is the one worth naming.
		store = vaulted
	}
	if !ok {
		return nil, fmt.Errorf("%s %q is %w in %s or the vault", localenv.Noun(kind), name, errValueNotSet, store.ScopeName())
	}
	return store, nil
}

// deleteKeepingRow is store.Delete, with a global's link row that could not be
// removed after its value was reported as a warning rather than a failure: the
// value is gone, and the row left behind only narrows what a re-created entry
// of the same name reaches, which get shows.
func (c *cli) deleteKeepingRow(store valueStore, kind localenv.Kind, name string) (bool, error) {
	ok, err := store.Delete(kind, name)
	if errors.Is(err, vaultenv.ErrLinkRowKept) {
		fmt.Fprintf(c.d.Stderr, "warning: deleted %s %s, but %v. A new global of this name would reach only the projects that row names; once the index is usable, clear it with: %s --everywhere\n",
			localenv.Noun(kind), name, err, localenv.LinkHint(kind, name))
		return ok, nil
	}
	return ok, err
}

// renderDeleted is a delete's text: what was removed, then what the
// declaration leaves, or what --undeclare did to it.
func renderDeleted(w io.Writer, res *envResult, store valueStore) error {
	noun := localenv.Noun(res.Kind)
	if _, err := fmt.Fprintf(w, "deleted %s %s from %s (%s)\n", noun, res.Name, store.ScopeName(), store.Location()); err != nil {
		return err
	}
	var err error
	switch {
	case res.Undeclared:
		_, err = fmt.Fprintf(w, "undeclared %s %s in %s\n", noun, res.Name, res.Manifest)
	case res.Remainder == envschema.RemainderAbsent:
		_, err = fmt.Fprintf(w, "%s is still declared in %s, so `astro local env list` shows it as absent. Remove the declaration with `%s`.\n",
			res.Name, res.Manifest, res.UndeclareHint)
	case res.Remainder == envschema.RemainderRequired:
		_, err = fmt.Fprintf(w, "%s is still declared in %s, and required: the next `astro local start` refuses until a value is set with `%s`. Remove the declaration with `%s`.\n",
			res.Name, res.Manifest, res.SetHint, res.UndeclareHint)
	case res.Remainder == envschema.RemainderSupplied && res.Source == envresolve.SourceDefault:
		_, err = fmt.Fprintf(w, "%s is still declared in %s, and its declared default now applies.\n", res.Name, res.Manifest)
	case res.Remainder == envschema.RemainderSupplied:
		_, err = fmt.Fprintf(w, "%s is still declared in %s, and now resolves from %s.\n", res.Name, res.Manifest, res.Source)
	}
	if err == nil && res.Workspace != "" {
		_, err = fmt.Fprintf(w, "It may still come from workspace %s, which a delete does not read.\n", res.Workspace)
	}
	return err
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
	opts := c.listOptions(projectDir, m)
	opts.All = all
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
	// A workspace that could not be read lists no rows, so say so in one line,
	// on stderr so stdout stays the listing. The fetch is cached, so this
	// reads nothing more.
	if opts.Workspace != "" && opts.Scope == "" {
		if short, _ := envresolve.Outage(opts.WorkspaceProvider); short != "" {
			fmt.Fprintf(c.d.Stderr, "workspace %s not read (%s): its values are not listed\n", opts.Workspace, short)
		} else if note := envresolve.SkippedNote(opts.WorkspaceProvider, opts.Workspace); note != "" {
			fmt.Fprintln(c.d.Stderr, note)
		}
	}
	if only != "" {
		kept := make([]localenv.ListItem, 0, len(items))
		for i := range items {
			if items[i].Kind == only {
				kept = append(kept, items[i])
			}
		}
		items = kept
	}
	if r.Format == FormatJSON {
		for i := range items {
			if err := r.Emit(items[i], nil); err != nil {
				return err
			}
		}
		return nil
	}
	return r.Emit(items, func(w io.Writer) error { return renderEnvList(w, items, only) })
}

// listOptions is the full resolution chain a listing of projectDir reads: the
// files, both vault tiers, and the linked workspace. m is the project's
// manifest, nil outside a project.
func (c *cli) listOptions(projectDir string, m *manifest.Manifest) localenv.ListOptions {
	var opts localenv.ListOptions
	// The vault tiers, or a name held only there reports as "absent" while start
	// injects it and get returns it.
	vault := vaultenv.Load(projectDir)
	opts.VaultProviders = vault.Providers()
	// And what the vault holds undeclared, listed as orphans the way an
	// undeclared file entry is.
	opts.VaultTiers = vault.Tiers()
	// reveal = false: list reports where each name resolves, never a value, so
	// it reads Environment Manager for presence only and pulls no secret.
	if m != nil && c.d.WorkspaceClients != nil {
		opts.WorkspaceProvider = emenv.NewProvider(m.Astro.Workspace, m.Astro.WorkspaceDomain(), c.d.WorkspaceClients, false)
		opts.Workspace = m.Astro.Workspace
	}
	return opts
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
	// DESCRIPTION is a column only when some row has one, so a project that
	// writes no descriptions keeps the four-column table.
	described := false
	for i := range items {
		if items[i].Description != "" {
			described = true
			break
		}
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	// The KIND cell names the subcommand that manages the value, the same as
	// every other user-facing string in this tree. The JSON `kind` field keeps
	// env/conn/var, because that one is a wire contract; this one is a table a
	// person reads, and printing `var` for an Airflow Variable contradicts the
	// grammar, where `var` is an alias for a plain environment variable.
	header := "KIND\tNAME\tSOURCE\tNOTE"
	if described {
		header += "\tDESCRIPTION"
	}
	fmt.Fprintln(tw, header)
	for i := range items {
		it := &items[i]
		row := fmt.Sprintf("%s\t%s\t%s\t%s", localenv.Noun(it.Kind), it.Name, it.Source, listNote(it))
		if described {
			// Collapsed to one line: a TOML multi-line string would otherwise
			// break the row out of the table.
			row += "\t" + strings.Join(strings.Fields(it.Description), " ")
		}
		fmt.Fprintln(tw, row)
	}
	return tw.Flush()
}

// listNote is a list row's NOTE cell.
func listNote(it *localenv.ListItem) string {
	note := ""
	switch {
	case it.Invalid != "":
		note = it.Invalid
		if it.Project != "" {
			note += " (in " + it.Project + ")"
		} else if it.RemoveHint != "" {
			note += " (remove: " + it.RemoveHint + ")"
		}
	case it.NotLinkedHere && it.LinksDown != "":
		note = "not linked here: " + it.LinksDown
	case it.NotLinkedHere:
		note = "not linked here (link: " + it.LinkHint + ")"
	case it.SetHint != "" && it.UndeclareHint != "":
		note = "declared, no value; set: " + it.SetHint + "; or remove the declaration: " + it.UndeclareHint
	case it.Orphan && it.Applied != nil && *it.Applied && it.DeclareHint != "":
		// The value reaches Airflow already, so declaring it only makes it
		// a requirement the project states.
		note = "not declared (declare it to make it a requirement: " + it.DeclareHint + ")"
		if it.RemoveHint != "" {
			note += "; remove: " + it.RemoveHint
		}
	case it.Orphan:
		note = "undeclared"
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
		switch {
		case it.DeclareHint != "" && it.RemoveHint != "":
			note += "; declare: " + it.DeclareHint + " (or remove: " + it.RemoveHint + ")"
		case it.RemoveHint != "":
			note += "; remove: " + it.RemoveHint
		}
	}
	return note
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
