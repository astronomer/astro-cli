//nolint:dupl // Cobra wiring per env-object type is intentionally parallel.
package astro

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/internal/platform/astro/env"
	"github.com/astronomer/astro-cli/pkg/airflowenv"
	"github.com/astronomer/astro-cli/pkg/connmodel"
)

const envConnExamples = `
  # set from a URI, creating it if it does not exist
  astro env connection set db_main --workspace-id <ws> \
    --value 'postgres://admin@db.example.com:5432/warehouse' --password "$PW"

  # or field by field
  astro env connection set db_main --workspace-id <ws> \
    --type postgres --host db.example.com --login admin --port 5432

  # list and delete
  astro env connection list --workspace-id <ws>
  astro env connection delete db_main --workspace-id <ws> --yes`

func newEnvConnRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "connection",
		Aliases:                    []string{"conn", "connections"},
		Short:                      "Manage connections",
		Long:                       "Manage connections on Astro, scoped to a workspace or a deployment.",
		Example:                    envConnExamples,
		Args:                       cobra.ArbitraryArgs,
		RunE:                       helpOrUnknownSubcommand,
		SuggestionsMinimumDistance: 2,
	}
	cmd.SetOut(out)
	addScopePersistentFlags(cmd)
	cmd.AddCommand(
		newEnvConnListCmd(out),
		newEnvConnGetCmd(out),
		newEnvConnSetCmd(out),
		newRemovedVerbCmd("create", "connection"),
		newRemovedVerbCmd("update", "connection"),
		newEnvConnDeleteCmd(out),
		newEnvLinkRootCmd(out, &connLinkNoun),
	)
	return cmd
}

func newEnvConnListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List connections",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEnvConnList(cmd, out)
		},
	}
	cmd.Flags().StringVar(&envFormat, "format", string(env.FormatTable), "Output format: table|json|yaml")
	cmd.Flags().StringVar(&envOutputPath, "output", "-", "Write output to FILE (use '-' for stdout)")
	return cmd
}

func newEnvConnGetCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <id-or-key>",
		Short: "Show a connection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvConnGet(cmd, out, args[0])
		},
	}
	cmd.Flags().StringVar(&envFormat, "format", string(env.FormatTable), "Output format: table|json|yaml")
	return cmd
}

func newEnvConnSetCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <id-or-key>",
		Short: "Set a connection",
		Long: "Set a connection, creating it if it does not exist. Pass --no-create to fail\n" +
			"instead. Give it whole with --value, as a URI or JSON, or field by field with\n" +
			"--type, --host and the rest.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvConnSet(cmd, out, args[0])
		},
	}
	connFlags(cmd)
	cmd.Flags().BoolVar(&envConnNoCreate, "no-create", false, "Fail if the connection does not exist, instead of creating it")
	addAutoLinkFlag(cmd)
	// --type cannot be marked required: with --value the type comes out of the
	// URI or the JSON. The either/or is enforced in buildConnInput instead.
	// --value excludes the field flags: both describe the whole connection.
	// --password is the exception, and deliberately so. A URI is the one shape
	// that has nowhere safe to put a secret — embedding it means argv, which
	// is shell history and `ps` — so --password (or a pipe) supplies it
	// alongside, and without that the only working form was the unsafe one.
	for _, f := range connFieldFlagNames {
		if f == "password" {
			continue
		}
		cmd.MarkFlagsMutuallyExclusive("value", f)
	}
	return cmd
}

// connFieldFlagNames is the field-by-field way to describe a connection, which
// is the alternative to handing over the whole thing with --value.
var connFieldFlagNames = []string{"type", "host", "login", "password", "schema", "port", "extra"}

func newEnvConnDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <id-or-key>",
		Aliases: []string{"rm"},
		Short:   "Delete a connection",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvConnDelete(cmd, out, args[0])
		},
	}
	cmd.Flags().BoolVarP(&envYes, "yes", "y", false, "Skip confirmation prompt")
	return cmd
}

func connFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&envConnValue, "value", "v", "", "The whole connection as a URI or JSON. Replaces it, where the field flags patch; a stored port or password is kept unless given")
	cmd.Flags().StringVarP(&envConnType, "type", "t", "", "Connection type (e.g. postgres, http)")
	cmd.Flags().StringVar(&envConnHost, "host", "", "Connection host")
	cmd.Flags().StringVarP(&envConnLogin, "login", "l", "", "Connection login or username")
	cmd.Flags().StringVarP(&envConnPassword, "password", "p", "", "Connection password; prefer piping it, since a flag lands in shell history. Empty keeps the stored one: the platform cannot clear a password")
	cmd.Flags().StringVar(&envConnSchema, "schema", "", "Connection schema")
	cmd.Flags().IntVar(&envConnPort, "port", 0, "Connection port")
	cmd.Flags().StringVar(&envConnExtra, "extra", "", "Extra configuration as a JSON object string")
}

func runEnvConnList(cmd *cobra.Command, out io.Writer) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	f, err := env.ParseFormat(envFormat)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	objs, err := env.ListConns(scope, envResolveLinked, envIncludeSecrets, astroV1Client)
	if err != nil {
		return err
	}
	if envIncludeSecrets {
		fmt.Fprintln(os.Stderr, includeSecretsWarning)
	}
	w, closer, err := openOutput(out)
	if err != nil {
		return err
	}
	defer closer()
	return env.WriteConnList(objs, f, w)
}

func runEnvConnGet(cmd *cobra.Command, out io.Writer, idOrKey string) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	f, err := env.ParseFormat(envFormat)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	obj, err := env.GetConn(idOrKey, scope, envIncludeSecrets, astroV1Client)
	if err != nil {
		return err
	}
	return env.WriteConn(obj, f, out)
}

// runEnvConnSet upserts. Update is tried first and a not-found falls through
// to create, which is what `astro env variable` already did; the two verbs
// differed here only because connection never grew the fallthrough.
//
// # Why there is no completeness check here, unlike metrics-export
//
// The create arm of an upsert is worth guarding when creating needs something
// updating does not, because then a patch that falls through builds an object
// the platform will reject — or worse, accept half-formed. That is the case
// for metrics-export: CreateMetricsExport demands an endpoint and an exporter
// type (env/metrics.go), UpdateMetricsExport demands neither, so runEnvMetricsSet
// wraps the create error to say which flag is suddenly needed and why.
//
// Connection has no such gap. CreateConn requires exactly Type (env/conn.go)
// and UpdateConn requires exactly Type as well — it rejects an empty one
// before it even looks the object up. So every field-flag invocation already
// carries a type, and anything that satisfies an update satisfies a create.
//
// A stricter floor was considered and rejected: there is no field set that
// separates a half-formed connection from a deliberately minimal one. An fs
// connection is a type and a path in extra, an http one is a type and a host,
// a GCP one is a type and a keyfile in extra. Requiring --host, or any other
// single field, would refuse connections that are perfectly valid.
//
// What remains is the mistyped-key hazard — meaning to patch db_main, typing
// db_mian, and getting a new connection instead of a failure. That is real,
// but it is not specific to connections: `astro env variable set API_TOKN`
// has done the same since the tree was written. It is what --no-create is
// for, and this is the first release where every noun has that flag.
func runEnvConnSet(cmd *cobra.Command, out io.Writer, idOrKey string) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	in, err := buildConnInput(cmd, idOrKey)
	if err != nil {
		return err
	}
	obj, err := env.UpdateConn(idOrKey, scope, in, astroV1Client)
	if err != nil {
		if errors.Is(err, env.ErrNotFound) && !envConnNoCreate {
			if cerr := refuseCreateByID("connection", idOrKey); cerr != nil {
				return cerr
			}
			obj, err = env.CreateConn(scope, idOrKey, in, astroV1Client)
			if err != nil {
				return err
			}
			printCreated(out, obj)
			return nil
		}
		if errors.Is(err, env.ErrNotFound) && envConnNoCreate {
			return setNotFound("connection", idOrKey, err)
		}
		return err
	}
	fmt.Fprintf(out, "Updated %s\n", obj.ObjectKey)
	return nil
}

func runEnvConnDelete(cmd *cobra.Command, out io.Writer, idOrKey string) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	if !envYes {
		ok, err := confirmTTY(fmt.Sprintf("Delete connection %q?", idOrKey))
		if err != nil {
			return err
		}
		if !ok {
			return errAbortedDelete
		}
	}
	if err := env.DeleteConn(idOrKey, scope, astroV1Client); err != nil {
		return err
	}
	fmt.Fprintf(out, "Deleted %s\n", idOrKey)
	return nil
}

// connInputFromValue turns a whole connection — a URI or connection JSON —
// into the field-wise input the platform API wants.
//
// It parses through airflowenv, the same codec `astro local env connection
// set` writes with, rather than a second parser here. That is the point of
// accepting the shape at all: a connection string that works on one side has
// to mean the same object on the other, and two parsers is how that stops
// being true.
//
// Every field is set, not just the non-empty ones, because --value describes
// the whole connection: a URI without a login means the connection has no
// login, not that the existing one should be kept.
func connInputFromValue(cmd *cobra.Command, idOrKey, raw string) (env.ConnInput, error) {
	conn, err := parseWholeConn(idOrKey, raw)
	if err != nil {
		return env.ConnInput{}, err
	}
	extra := conn.ConnExtra
	if extra == nil {
		extra = map[string]any{}
	}
	in := env.ConnInput{
		Type:                conn.ConnType,
		Host:                &conn.ConnHost,
		Login:               &conn.ConnLogin,
		Password:            &conn.ConnPassword,
		Schema:              &conn.ConnSchema,
		Extra:               &extra,
		AutoLinkDeployments: autoLinkPtr(cmd),
	}
	// A password given alongside wins over whatever the URI carried, which is
	// how a credential reaches this path without going through argv. A piped
	// one does the same; an empty read means "not given", so a URI with no
	// password run non-interactively does not clear the stored one.
	if pw, ok, err := suppliedPassword(cmd); err != nil {
		return env.ConnInput{}, err
	} else if ok {
		in.Password = &pw
	} else if conn.ConnPassword == "" {
		in.Password = nil
	}
	// A port only when there is one. Taking the address unconditionally sent
	// port 0 for every URI that omitted it — a connection Airflow then dials
	// on port 0 — where the field-flag path leaves it unset.
	if conn.ConnPort != 0 {
		in.Port = &conn.ConnPort
	}
	return in, nil
}

// parseWholeConn reads a connection URI or connection JSON into the shared
// model, accepting exactly what `astro local env connection set` accepts.
//
// It calls airflowenv's two parsers rather than NormalizeConn, which looks
// like the obvious single entry point and is the wrong one here: Normalize
// finishes by encoding to the AIRFLOW_CONN_<ID> env var, so it inherits that
// form's rule that the id be a legal environment-variable name. Locally that
// is true by construction. On the platform it is not — a connection key may
// be hyphenated or dotted — and routing through Normalize made --value refuse
// keys the field flags accept, with an error ("could not encode value") that
// named neither the cause nor a fix.
func parseWholeConn(idOrKey, raw string) (connmodel.Connection, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return connmodel.Connection{}, fmt.Errorf("connection %q: --value is empty", idOrKey)
	}
	if strings.HasPrefix(trimmed, "{") {
		return airflowenv.DecodeConnValue(idOrKey, trimmed)
	}
	// url.Parse is happy to read `db.example.com:5432/warehouse` as scheme
	// "db.example.com" with an opaque rest, so ConnFromURI would return a
	// connection whose *type* is a hostname and whose host is empty — and
	// CreateConn accepts it, because the type is non-empty. Requiring the
	// separator rejects the paste instead of storing nonsense.
	if !strings.Contains(trimmed, "://") {
		return connmodel.Connection{}, fmt.Errorf(
			"connection %q: --value must be a connection URI (conn_type://host/...) or connection JSON; %q has no scheme",
			idOrKey, trimmed)
	}
	return airflowenv.ConnFromURI(idOrKey, trimmed)
}

// suppliedPassword reports a password given out of band — by flag, or piped —
// and whether one was given at all. An empty piped read is "not given": stdin
// is not a terminal in CI, so reading it as an empty password is how a stored
// credential gets cleared by a command that never mentioned one.
func suppliedPassword(cmd *cobra.Command) (password string, given bool, err error) {
	if cmd.Flags().Changed("password") {
		return envConnPassword, true, nil
	}
	if !hasPipedStdin() {
		return "", false, nil
	}
	pw, err := readSecretValue("", "Connection password")
	if err != nil {
		return "", false, err
	}
	return pw, pw != "", nil
}

func buildConnInput(cmd *cobra.Command, idOrKey string) (env.ConnInput, error) {
	if cmd.Flags().Changed("value") {
		return connInputFromValue(cmd, idOrKey, envConnValue)
	}
	if envConnType == "" {
		return env.ConnInput{}, errors.New("a connection needs a type: pass --type, or --value with a URI or connection JSON that carries one")
	}
	// Optional fields are sent only when the user explicitly set the flag, so
	// passing --host="" (etc.) is preserved as "clear this field" rather than
	// being silently skipped.
	in := env.ConnInput{Type: envConnType, AutoLinkDeployments: autoLinkPtr(cmd)}
	if cmd.Flags().Changed("host") {
		in.Host = &envConnHost
	}
	if cmd.Flags().Changed("login") {
		in.Login = &envConnLogin
	}
	if cmd.Flags().Changed("schema") {
		in.Schema = &envConnSchema
	}
	if cmd.Flags().Changed("port") {
		in.Port = &envConnPort
	}
	if cmd.Flags().Changed("extra") {
		// One parser for both shapes and both trees: DecodeExtra keeps numbers
		// exact (an account id above 2^53 is otherwise re-marshaled eleven
		// off) and distinguishes "not an object" from a syntax error. The
		// --value path reaches the same decoder through DecodeConnValue, so
		// --extra and --value now agree on the extras they produce.
		extra, err := airflowenv.DecodeExtra(envConnExtra)
		if err != nil {
			return env.ConnInput{}, fmt.Errorf("--%w", err)
		}
		in.Extra = &extra
	}
	// Password is opt-in: only resolve a value when the flag was set or stdin is
	// piped. Many connection types (HTTP, SSH-via-key, etc.) are passwordless;
	// prompting on TTY by default would block the common case.
	// --password given explicitly is sent as given. Empty does not clear a
	// stored password, though: the platform treats an empty password on update
	// as "keep the stored one", and has no way to remove it.
	//
	// Otherwise a piped password is still read — that is the only way to supply
	// one without putting it in shell history — but an EMPTY read is treated as
	// "no password given" rather than "the password is the empty string". That
	// distinction is the fix for a credential-destroying bug: hasPipedStdin is
	// just "stdin is not a terminal", which is true of every CI run, so
	// `astro env connection set db --type postgres --host new` used to read
	// zero bytes and send an explicit empty password, silently clearing the
	// stored one while reporting success.
	switch {
	case cmd.Flags().Changed("password"):
		in.Password = &envConnPassword
	case hasPipedStdin():
		pw, err := readSecretValue("", "Connection password")
		if err != nil {
			return env.ConnInput{}, err
		}
		if pw != "" {
			in.Password = &pw
		}
	}
	return in, nil
}
