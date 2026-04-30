//nolint:dupl // Cobra wiring per env-object type is intentionally parallel.
package cloud

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cloud/env"
)

const envMetricsExamples = `
  # List metrics exports in a workspace
  astro env metrics-export list --workspace-id <ws-id>

  # Create a Prometheus metrics export with basic auth
  astro env metrics-export create --workspace-id <ws-id> \
    --key prom_main \
    --endpoint https://prom.example.com/api/v1/write \
    --exporter-type PROMETHEUS \
    --auth-type BASIC --username scraper --password ...

  # Update labels
  astro env metrics-export update prom_main --workspace-id <ws-id> \
    --label env=prod --label team=data
`

func newEnvMetricsExportRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "metrics-export",
		Aliases: []string{"metrics", "metrics-exports"},
		Short:   "Manage environment-manager metrics exports",
		Long:    "List, create, update, or delete metrics exports managed through the platform's environment manager. Metrics exports can be scoped to a workspace or a deployment.",
		Example: envMetricsExamples,
	}
	cmd.SetOut(out)
	addScopePersistentFlags(cmd)
	cmd.AddCommand(
		newEnvMetricsListCmd(out),
		newEnvMetricsGetCmd(out),
		newEnvMetricsCreateCmd(out),
		newEnvMetricsUpdateCmd(out),
		newEnvMetricsDeleteCmd(out),
	)
	return cmd
}

func newEnvMetricsListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List metrics exports",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEnvMetricsList(cmd, out)
		},
	}
	cmd.Flags().StringVar(&envFormat, "format", string(env.FormatTable), "Output format: table|json|yaml")
	cmd.Flags().StringVar(&envOutputPath, "output", "-", "Write output to FILE (use '-' for stdout)")
	return cmd
}

func newEnvMetricsGetCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <id-or-key>",
		Short: "Get a single metrics export by ID or key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvMetricsGet(cmd, out, args[0])
		},
	}
	cmd.Flags().StringVar(&envFormat, "format", string(env.FormatTable), "Output format: table|json|yaml")
	return cmd
}

func newEnvMetricsCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create a metrics export",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEnvMetricsCreate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&envMetricsKey, "key", "k", "", "Metrics export key (required)")
	metricsCommonFlags(cmd)
	_ = cmd.MarkFlagRequired("key")
	_ = cmd.MarkFlagRequired("endpoint")
	_ = cmd.MarkFlagRequired("exporter-type")
	return cmd
}

func newEnvMetricsUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update <id-or-key>",
		Aliases: []string{"up"},
		Short:   "Update a metrics export",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvMetricsUpdate(cmd, out, args[0])
		},
	}
	metricsCommonFlags(cmd)
	return cmd
}

func newEnvMetricsDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <id-or-key>",
		Aliases: []string{"rm"},
		Short:   "Delete a metrics export",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvMetricsDelete(cmd, out, args[0])
		},
	}
	cmd.Flags().BoolVarP(&envYes, "yes", "y", false, "Skip confirmation prompt")
	return cmd
}

func metricsCommonFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&envMetricsEndpoint, "endpoint", "", "Remote endpoint to push metrics to")
	cmd.Flags().StringVar(&envMetricsExporterType, "exporter-type", "", "Exporter type (e.g. PROMETHEUS)")
	cmd.Flags().StringVar(&envMetricsAuthType, "auth-type", "", "Auth type: BASIC, AUTH_TOKEN, or SIGV4")
	cmd.Flags().StringVar(&envMetricsBasicToken, "basic-token", "", "Bearer/auth token (for AUTH_TOKEN auth)")
	cmd.Flags().StringVar(&envMetricsUsername, "username", "", "Username (for BASIC auth)")
	cmd.Flags().StringVar(&envMetricsPassword, "password", "", "Password (for BASIC auth). If omitted with --auth-type BASIC, read from stdin (piped) or prompted (TTY) with echo disabled.")
	cmd.Flags().StringVar(&envMetricsSigV4AssumeArn, "sigv4-assume-arn", "", "AWS IAM role to assume (for SIGV4 auth)")
	cmd.Flags().StringVar(&envMetricsSigV4StsRegion, "sigv4-sts-region", "", "AWS STS region (for SIGV4 auth)")
	cmd.Flags().StringSliceVar(&envMetricsHeaders, "header", nil, "Request header in KEY=VALUE form. Repeatable.")
	cmd.Flags().StringSliceVar(&envMetricsLabels, "label", nil, "Metric label in KEY=VALUE form. Repeatable.")
}

func runEnvMetricsList(cmd *cobra.Command, out io.Writer) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	f, err := env.ParseFormat(envFormat)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	objs, err := env.ListMetricsExports(scope, envResolveLinked, envIncludeSecrets, astroCoreClient)
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
	return env.WriteMetricsExportList(objs, f, w)
}

func runEnvMetricsGet(cmd *cobra.Command, out io.Writer, idOrKey string) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	f, err := env.ParseFormat(envFormat)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	obj, err := env.GetMetricsExport(idOrKey, scope, envIncludeSecrets, astroCoreClient)
	if err != nil {
		return err
	}
	return env.WriteMetricsExport(obj, f, out)
}

func runEnvMetricsCreate(cmd *cobra.Command, out io.Writer) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	in, err := buildMetricsInput()
	if err != nil {
		return err
	}
	obj, err := env.CreateMetricsExport(scope, envMetricsKey, in, astroCoreClient)
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

func runEnvMetricsUpdate(cmd *cobra.Command, out io.Writer, idOrKey string) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	in, err := buildMetricsInput()
	if err != nil {
		return err
	}
	obj, err := env.UpdateMetricsExport(idOrKey, scope, in, astroCoreClient)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Updated %s\n", obj.ObjectKey)
	return nil
}

func runEnvMetricsDelete(cmd *cobra.Command, out io.Writer, idOrKey string) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	if !envYes && !confirmTTY(fmt.Sprintf("Delete metrics export %q?", idOrKey)) {
		return errors.New("aborted: pass --yes (or confirm interactively) to delete")
	}
	if err := env.DeleteMetricsExport(idOrKey, scope, astroCoreClient); err != nil {
		return err
	}
	fmt.Fprintf(out, "Deleted %s\n", idOrKey)
	return nil
}

func buildMetricsInput() (*env.MetricsInput, error) {
	in := &env.MetricsInput{
		Endpoint:     envMetricsEndpoint,
		ExporterType: envMetricsExporterType,
		AuthType:     envMetricsAuthType,
	}
	if envMetricsBasicToken != "" {
		in.BasicToken = &envMetricsBasicToken
	}
	if envMetricsUsername != "" {
		in.Username = &envMetricsUsername
	}
	if envMetricsSigV4AssumeArn != "" {
		in.SigV4AssumeArn = &envMetricsSigV4AssumeArn
	}
	if envMetricsSigV4StsRegion != "" {
		in.SigV4StsRegion = &envMetricsSigV4StsRegion
	}
	if envMetricsAuthType == "BASIC" {
		// Read password from flag, stdin, or TTY prompt with echo off.
		pw, err := readSecretValue(envMetricsPassword, "Password")
		if err != nil {
			return nil, err
		}
		if pw != "" {
			in.Password = &pw
		}
	} else if envMetricsPassword != "" {
		in.Password = &envMetricsPassword
	}
	if len(envMetricsHeaders) > 0 {
		m, err := parseKVList(envMetricsHeaders, "--header")
		if err != nil {
			return nil, err
		}
		in.Headers = &m
	}
	if len(envMetricsLabels) > 0 {
		m, err := parseKVList(envMetricsLabels, "--label")
		if err != nil {
			return nil, err
		}
		in.Labels = &m
	}
	return in, nil
}

func parseKVList(raw []string, flagName string) (map[string]string, error) {
	out := make(map[string]string, len(raw))
	for _, kv := range raw {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 || parts[0] == "" {
			return nil, fmt.Errorf("invalid %s value %q (expected KEY=VALUE)", flagName, kv)
		}
		out[parts[0]] = parts[1]
	}
	return out, nil
}
