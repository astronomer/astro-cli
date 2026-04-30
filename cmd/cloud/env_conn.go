//nolint:dupl // Cobra wiring per env-object type is intentionally parallel.
package cloud

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cloud/env"
)

const envConnExamples = `
  # List connections in a workspace
  astro env conn list --workspace-id <ws-id>

  # Create a Postgres connection
  astro env conn create --workspace-id <ws-id> --key db_main --type postgres --host db.example.com --login admin --port 5432

  # Update only the host
  astro env conn update db_main --workspace-id <ws-id> --type postgres --host db-new.example.com

  # Delete
  astro env conn delete db_main --workspace-id <ws-id> --yes
`

func newEnvConnRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "conn",
		Aliases: []string{"connection", "connections"},
		Short:   "Manage environment-manager connections",
		Long:    "List, create, update, or delete connections managed through the platform's environment manager. Connections can be scoped to a workspace or a deployment.",
		Example: envConnExamples,
	}
	cmd.SetOut(out)
	addScopePersistentFlags(cmd)
	cmd.AddCommand(
		newEnvConnListCmd(out),
		newEnvConnGetCmd(out),
		newEnvConnCreateCmd(out),
		newEnvConnUpdateCmd(out),
		newEnvConnDeleteCmd(out),
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
		Short: "Get a single connection by ID or key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvConnGet(cmd, out, args[0])
		},
	}
	cmd.Flags().StringVar(&envFormat, "format", string(env.FormatTable), "Output format: table|json|yaml")
	return cmd
}

func newEnvConnCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create a connection",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEnvConnCreate(cmd, out)
		},
	}
	connFlags(cmd)
	_ = cmd.MarkFlagRequired("key")
	_ = cmd.MarkFlagRequired("type")
	return cmd
}

func newEnvConnUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update <id-or-key>",
		Aliases: []string{"up"},
		Short:   "Update a connection",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvConnUpdate(cmd, out, args[0])
		},
	}
	connUpdateFlags(cmd)
	_ = cmd.MarkFlagRequired("type")
	return cmd
}

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
	cmd.Flags().StringVarP(&envConnKey, "key", "k", "", "Connection key (required)")
	connUpdateFlags(cmd)
}

func connUpdateFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&envConnType, "type", "t", "", "Connection type (e.g. postgres, http)")
	cmd.Flags().StringVar(&envConnHost, "host", "", "Connection host")
	cmd.Flags().StringVarP(&envConnLogin, "login", "l", "", "Connection login or username")
	cmd.Flags().StringVarP(&envConnPassword, "password", "p", "", "Connection password. If omitted with --type, read from stdin (piped) or prompted (TTY) with echo disabled.")
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

	objs, err := env.ListConns(scope, envResolveLinked, envIncludeSecrets, astroCoreClient)
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

	obj, err := env.GetConn(idOrKey, scope, envIncludeSecrets, astroCoreClient)
	if err != nil {
		return err
	}
	return env.WriteConn(obj, f, out)
}

func runEnvConnCreate(cmd *cobra.Command, out io.Writer) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	in, err := buildConnInput()
	if err != nil {
		return err
	}
	obj, err := env.CreateConn(scope, envConnKey, in, astroCoreClient)
	if err != nil {
		return err
	}
	id := ""
	if obj.Id != nil {
		id = *obj.Id
	}
	fmt.Fprintf(out, "Created %s (id: %s)\n", obj.ObjectKey, id)
	return nil
}

func runEnvConnUpdate(cmd *cobra.Command, out io.Writer, idOrKey string) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	in, err := buildConnInput()
	if err != nil {
		return err
	}
	obj, err := env.UpdateConn(idOrKey, scope, in, astroCoreClient)
	if err != nil {
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

	if !envYes && !confirmTTY(fmt.Sprintf("Delete connection %q?", idOrKey)) {
		return errors.New("aborted: pass --yes (or confirm interactively) to delete")
	}
	if err := env.DeleteConn(idOrKey, scope, astroCoreClient); err != nil {
		return err
	}
	fmt.Fprintf(out, "Deleted %s\n", idOrKey)
	return nil
}

func buildConnInput() (env.ConnInput, error) {
	in := env.ConnInput{Type: envConnType}
	if envConnHost != "" {
		in.Host = &envConnHost
	}
	if envConnLogin != "" {
		in.Login = &envConnLogin
	}
	if envConnSchema != "" {
		in.Schema = &envConnSchema
	}
	if envConnPort != 0 {
		in.Port = &envConnPort
	}
	if envConnExtra != "" {
		var extra map[string]any
		if err := json.Unmarshal([]byte(envConnExtra), &extra); err != nil {
			return env.ConnInput{}, fmt.Errorf("--extra is not valid JSON: %w", err)
		}
		in.Extra = &extra
	}
	// Password reads from stdin when piped or prompts on TTY (matches env var entry).
	pw, err := readSecretValue(envConnPassword, "Connection password")
	if err != nil {
		return env.ConnInput{}, err
	}
	if pw != "" {
		in.Password = &pw
	}
	return in, nil
}

const includeSecretsWarning = "Warning: --include-secrets returns secret values in the response. Treat the output as sensitive: do not commit, paste into shared channels, or leave on disk longer than necessary." //nolint:gosec // user-facing warning text, not a credential
