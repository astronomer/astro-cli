package astro

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/env"
	"github.com/astronomer/astro-cli/pkg/airflowenv"
)

// linkNoun is one object kind's `link` group. The platform links every kind
// the same way, so the verbs are shared; what differs is how the object is
// named and what its override is made of.
//
// `astro env variable link` predates this and keeps its own command, with the
// same verbs and flags.
type linkNoun struct {
	kind env.LinkKind
	// noun is the command word, as in `astro env <noun> link`.
	noun string
	// what names the object in help text.
	what     string
	examples string
	// addOverrideFlags wires the flags describing a link's override onto
	// `set`; override reads them back, nil when none were given.
	addOverrideFlags func(cmd *cobra.Command, f *linkFlags)
	override         func(cmd *cobra.Command, f *linkFlags) (*env.LinkOverride, error)
}

// linkFlags holds one group's flag values. Each group gets its own, so two
// nouns never share state the way package-level flag variables would.
type linkFlags struct {
	objectID, objectKey string
	deploymentID        string
	exclude, noCreate   bool
	output              string
	value               string
	conn                struct {
		connType, host, login, password, schema, extra string
		port                                           int
	}
}

var connLinkNoun = linkNoun{
	kind: env.LinkConnection,
	noun: "connection",
	what: "connection",
	examples: `
  # link a workspace connection to a deployment, pointing it at another host
  astro env connection link set --connection-key db --workspace-id <ws> \
    --deployment-id <dep> --host db.prod.internal --port 5432

  # drop those overrides, keeping the link
  astro env connection link set --connection-key db --workspace-id <ws> --deployment-id <dep>

  # opt one deployment out of an auto-linked connection
  astro env connection link set --connection-key db --workspace-id <ws> --deployment-id <dep> --exclude

  # list links, and remove one
  astro env connection link list --connection-key db --workspace-id <ws>
  astro env connection link delete --connection-key db --workspace-id <ws> --deployment-id <dep>`,
	addOverrideFlags: func(cmd *cobra.Command, f *linkFlags) {
		cmd.Flags().StringVarP(&f.conn.connType, "type", "t", "", "The deployment's own connection type")
		cmd.Flags().StringVar(&f.conn.host, "host", "", "The deployment's own host")
		cmd.Flags().StringVarP(&f.conn.login, "login", "l", "", "The deployment's own login")
		cmd.Flags().StringVarP(&f.conn.password, "password", "p", "", "The deployment's own password; prefer piping it, since a flag lands in shell history")
		cmd.Flags().StringVar(&f.conn.schema, "schema", "", "The deployment's own schema")
		cmd.Flags().IntVar(&f.conn.port, "port", 0, "The deployment's own port")
		cmd.Flags().StringVar(&f.conn.extra, "extra", "", "The deployment's own extra fields, as a JSON object; keys left out inherit")
		for _, name := range []string{"type", "host", "login", "password", "schema", "port", "extra"} {
			cmd.MarkFlagsMutuallyExclusive(name, "exclude")
		}
	},
	override: connLinkOverride,
}

var airflowVarLinkNoun = linkNoun{
	kind: env.LinkAirflowVariable,
	noun: "airflow-variable",
	what: "Airflow variable",
	examples: `
  # link a workspace Airflow variable to a deployment, with a value only it sees
  astro env airflow-variable link set --airflow-variable-key region --workspace-id <ws> \
    --deployment-id <dep> --value eu-west-1

  # drop that value, keeping the link
  astro env airflow-variable link set --airflow-variable-key region --workspace-id <ws> --deployment-id <dep>

  # opt one deployment out of an auto-linked Airflow variable
  astro env airflow-variable link set --airflow-variable-key region --workspace-id <ws> --deployment-id <dep> --exclude

  # list links, and remove one
  astro env airflow-variable link list --airflow-variable-key region --workspace-id <ws>
  astro env airflow-variable link delete --airflow-variable-key region --workspace-id <ws> --deployment-id <dep>`,
	addOverrideFlags: func(cmd *cobra.Command, f *linkFlags) {
		cmd.Flags().StringVar(&f.value, "value", "", "The deployment's own value; omit it to clear any it had")
		cmd.MarkFlagsMutuallyExclusive("value", "exclude")
	},
	override: func(cmd *cobra.Command, f *linkFlags) (*env.LinkOverride, error) {
		if !cmd.Flags().Changed("value") {
			return nil, nil
		}
		return &env.LinkOverride{Value: &f.value}, nil
	},
}

// connLinkOverride reads the field flags into a connection override. Like
// `connection set`, only flags actually passed are sent, and a piped password
// is read so it need not sit in shell history; an empty pipe is no password,
// since every CI run has a non-terminal stdin.
func connLinkOverride(cmd *cobra.Command, f *linkFlags) (*env.LinkOverride, error) {
	var o env.ConnOverride
	changed := cmd.Flags().Changed
	if changed("type") {
		o.Type = &f.conn.connType
	}
	if changed("host") {
		o.Host = &f.conn.host
	}
	if changed("login") {
		o.Login = &f.conn.login
	}
	if changed("schema") {
		o.Schema = &f.conn.schema
	}
	if changed("port") {
		o.Port = &f.conn.port
	}
	if changed("extra") {
		extra, err := airflowenv.DecodeExtra(f.conn.extra)
		if err != nil {
			return nil, fmt.Errorf("--%w", err)
		}
		o.Extra = &extra
	}
	switch {
	case changed("password"):
		o.Password = &f.conn.password
	case hasPipedStdin():
		pw, err := readSecretValue("", "Connection password")
		if err != nil {
			return nil, err
		}
		if pw != "" {
			o.Password = &pw
		}
	}
	if o == (env.ConnOverride{}) {
		return nil, nil
	}
	return &env.LinkOverride{Connection: &o}, nil
}

func newEnvLinkRootCmd(out io.Writer, n *linkNoun) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "link",
		Aliases: []string{"links"},
		Short:   fmt.Sprintf("Manage which deployments a workspace %s reaches", n.what),
		Long: fmt.Sprintf("Control which deployments a workspace %s reaches, and give a deployment\n"+
			"its own values. Identify the %s with --%s-key or --%s-id.", n.what, n.what, n.noun, n.noun),
		Example:                    n.examples,
		Args:                       cobra.ArbitraryArgs,
		RunE:                       helpOrUnknownSubcommand,
		SuggestionsMinimumDistance: 2,
	}
	f := &linkFlags{}
	cmd.AddCommand(
		newEnvLinkSetCmd(out, n, f),
		newEnvLinkDeleteCmd(out, n, f),
		newEnvLinkListCmd(out, n, f),
	)
	return cmd
}

// addLinkObjectFlags wires the identifier flags for the linked object:
// exactly one of --<noun>-id / --<noun>-key is required.
func addLinkObjectFlags(cmd *cobra.Command, n *linkNoun, f *linkFlags) {
	cmd.Flags().StringVar(&f.objectID, n.noun+"-id", "", "ID of the workspace "+n.what)
	cmd.Flags().StringVar(&f.objectKey, n.noun+"-key", "", "Key of the workspace "+n.what)
	cmd.MarkFlagsMutuallyExclusive(n.noun+"-id", n.noun+"-key")
	cmd.MarkFlagsOneRequired(n.noun+"-id", n.noun+"-key")
}

// idOrKey returns whichever identifier flag was set. Errors when the set flag
// is empty (cobra's one-required group counts --x-id "" as set).
func (f *linkFlags) idOrKey(n *linkNoun) (string, error) {
	if f.objectID != "" {
		return f.objectID, nil
	}
	if f.objectKey != "" {
		return f.objectKey, nil
	}
	return "", fmt.Errorf("--%s-id or --%s-key cannot be empty", n.noun, n.noun)
}

func newEnvLinkSetCmd(out io.Writer, n *linkNoun, f *linkFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set",
		Short: fmt.Sprintf("Link %s to a deployment", article(n.what)),
		Long: fmt.Sprintf("Link a workspace %s to a deployment. The override flags give that deployment\n"+
			"its own values and describe the whole override: a value the link had that\n"+
			"they leave out is cleared. --exclude opts the deployment out of an auto-linked\n"+
			"%s instead. Pass --no-create to fail if it is not already linked.", n.what, n.what),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEnvLinkSet(cmd, out, n, f)
		},
	}
	addLinkObjectFlags(cmd, n, f)
	cmd.Flags().StringVar(&f.deploymentID, "deployment-id", "", "ID of the deployment to link (required)")
	cmd.Flags().BoolVar(&f.exclude, "exclude", false, "Opt the deployment out of an auto-linked "+n.what+" instead")
	cmd.Flags().BoolVar(&f.noCreate, "no-create", false, "Fail if the deployment is not already linked")
	// --exclude takes a different path entirely (the platform's exclude-linking
	// endpoint), which has no create/update distinction for --no-create to
	// govern. Accepting the pair would have silently ignored the guard.
	cmd.MarkFlagsMutuallyExclusive("exclude", "no-create")
	_ = cmd.MarkFlagRequired("deployment-id") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	n.addOverrideFlags(cmd, f)
	return cmd
}

func newEnvLinkDeleteCmd(out io.Writer, n *linkNoun, f *linkFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete",
		Aliases: []string{"rm"},
		Short:   "Remove a link or exclude",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEnvLinkDelete(cmd, out, n, f)
		},
	}
	addLinkObjectFlags(cmd, n, f)
	cmd.Flags().StringVar(&f.deploymentID, "deployment-id", "", "ID of the deployment to unlink (required)")
	cmd.Flags().BoolVar(&f.exclude, "exclude", false, "Remove an exclude instead of a link")
	_ = cmd.MarkFlagRequired("deployment-id") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	return cmd
}

func newEnvLinkListCmd(out io.Writer, n *linkNoun, f *linkFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   fmt.Sprintf("List %s's links and excludes", article(n.what)),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEnvLinkList(cmd, out, n, f)
		},
	}
	addLinkObjectFlags(cmd, n, f)
	cliout.AddOutputFlag(cmd, &f.output)
	return cmd
}

func runEnvLinkSet(cmd *cobra.Command, out io.Writer, n *linkNoun, f *linkFlags) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	idOrKey, err := f.idOrKey(n)
	if err != nil {
		return err
	}
	if f.exclude {
		if err := env.Exclude(n.kind, idOrKey, scope, f.deploymentID, astroV1Client); err != nil {
			return err
		}
		fmt.Fprintf(out, "Excluded %s from deployment %s\n", idOrKey, f.deploymentID)
		fmt.Fprintln(cmd.ErrOrStderr(), deploymentPickupNote)
		return nil
	}
	override, err := n.override(cmd, f)
	if err != nil {
		return err
	}
	if err := env.Link(n.kind, idOrKey, scope, f.deploymentID, override, f.noCreate, astroV1Client); err != nil {
		return err
	}
	if override != nil {
		fmt.Fprintf(out, "Linked %s to deployment %s (override applied)\n", idOrKey, f.deploymentID)
	} else {
		fmt.Fprintf(out, "Linked %s to deployment %s (no override)\n", idOrKey, f.deploymentID)
	}
	fmt.Fprintln(cmd.ErrOrStderr(), deploymentPickupNote)
	return nil
}

func runEnvLinkDelete(cmd *cobra.Command, out io.Writer, n *linkNoun, f *linkFlags) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	idOrKey, err := f.idOrKey(n)
	if err != nil {
		return err
	}
	if f.exclude {
		if err := env.Unexclude(n.kind, idOrKey, scope, f.deploymentID, astroV1Client); err != nil {
			return err
		}
		fmt.Fprintf(out, "Removed exclude on %s for deployment %s\n", idOrKey, f.deploymentID)
		fmt.Fprintln(cmd.ErrOrStderr(), deploymentPickupNote)
		return nil
	}
	if err := env.Unlink(n.kind, idOrKey, scope, f.deploymentID, astroV1Client); err != nil {
		return err
	}
	fmt.Fprintf(out, "Unlinked %s from deployment %s\n", idOrKey, f.deploymentID)
	fmt.Fprintln(cmd.ErrOrStderr(), deploymentPickupNote)
	return nil
}

func runEnvLinkList(cmd *cobra.Command, out io.Writer, n *linkNoun, f *linkFlags) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	format, err := cliout.ParseFormat(f.output)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	idOrKey, err := f.idOrKey(n)
	if err != nil {
		return err
	}
	report, err := env.ListLinks(n.kind, idOrKey, scope, envIncludeSecrets, astroV1Client)
	if err != nil {
		return err
	}
	return env.WriteLinks(report, env.Format(format), out)
}

// article prefixes a noun with "a" or "an".
func article(noun string) string {
	if noun == "" {
		return noun
	}
	switch noun[0] {
	case 'a', 'e', 'i', 'o', 'u', 'A', 'E', 'I', 'O', 'U':
		return "an " + noun
	}
	return "a " + noun
}
