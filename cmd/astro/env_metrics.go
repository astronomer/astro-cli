package astro

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/env"
	"github.com/astronomer/astro-cli/pkg/input"
)

const envMetricsExamples = `
  # set a Prometheus export, creating it if it does not exist
  astro env metrics-export set prom_main --workspace <ws> \
    --endpoint https://prom.example.com/api/v1/write --exporter-type PROMETHEUS \
    --auth-type BASIC --username scraper --password "$PW"

  # change labels on one that must already exist
  astro env metrics-export set prom_main --workspace <ws> --label env=prod --no-create

  # list and delete
  astro env metrics-export list --workspace <ws>
  astro env metrics-export delete prom_main --workspace <ws> --yes`

func newEnvMetricsExportRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "metrics-export",
		Aliases:                    []string{"metrics", "metrics-exports"},
		Short:                      "Manage metrics exports",
		Long:                       "Manage metrics exports on Astro, scoped to a workspace or a deployment.",
		Example:                    envMetricsExamples,
		Args:                       cobra.ArbitraryArgs,
		RunE:                       helpOrUnknownSubcommand,
		SuggestionsMinimumDistance: 2,
	}
	cmd.SetOut(out)
	addScopePersistentFlags(cmd)
	cmd.AddCommand(
		newEnvMetricsListCmd(out),
		newEnvMetricsGetCmd(out),
		newEnvMetricsSetCmd(out),
		newRemovedVerbCmd("create", "metrics-export"),
		newRemovedVerbCmd("update", "metrics-export"),
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
		Example: `  # List the metrics exports in the workspace
  astro env metrics-export list`,
	}
	cliout.AddOutputFlag(cmd, &envOutput)
	return cmd
}

func newEnvMetricsGetCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <ID_OR_KEY>",
		Short: "Show a metrics export",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvMetricsGet(cmd, out, args[0])
		},
		Example: `  # Show a metrics export by its key
  astro env metrics-export get prom_main`,
	}
	cliout.AddOutputFlag(cmd, &envOutput)
	return cmd
}

func newEnvMetricsSetCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <ID_OR_KEY>",
		Short: "Set a metrics export",
		Long: "Set a metrics export, creating it if it does not exist. Pass --no-create to fail\n" +
			"instead. Creating one needs --endpoint and --exporter-type.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvMetricsSet(cmd, out, args[0])
		},
		Example: `  # Create a Prometheus export
  astro env metrics-export set prom_main --endpoint https://prom.example.com/api/v1/write --exporter-type PROMETHEUS

  # Replace the labels on one that must already exist
  astro env metrics-export set prom_main --label env=prod --label team=data --no-create`,
	}
	metricsCommonFlags(cmd)
	cmd.Flags().BoolVar(&envMetricsNoCreate, "no-create", false, "Fail if the metrics export does not exist, instead of creating it")
	addAutoLinkFlag(cmd)
	return cmd
}

func newEnvMetricsDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <ID_OR_KEY>",
		Aliases: []string{"rm"},
		Short:   "Delete a metrics export",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvDelete(cmd, out, "metrics export", args[0], env.DeleteMetricsExport)
		},
		Example: `  # Delete a metrics export without the confirmation prompt
  astro env metrics-export delete prom_main --yes`,
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
	cmd.Flags().StringVar(&envMetricsPassword, "password", "", "Password for BASIC auth; prefer piping it, since a flag lands in shell history. Empty keeps the stored one: the platform cannot clear a password")
	cmd.Flags().StringVar(&envMetricsSigV4AssumeArn, "sigv4-assume-arn", "", "AWS IAM role to assume (for SIGV4 auth)")
	cmd.Flags().StringVar(&envMetricsSigV4StsRegion, "sigv4-sts-region", "", "AWS STS region (for SIGV4 auth)")
	cmd.Flags().StringToStringVar(&envMetricsHeaders, "header", nil, "Request header in KEY=VALUE form; repeatable")
	cmd.Flags().StringToStringVar(&envMetricsLabels, "label", nil, "Metric label in KEY=VALUE form; repeatable")
}

func runEnvMetricsList(cmd *cobra.Command, out io.Writer) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	f, err := cliout.ParseFormat(envOutput)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	objs, err := env.ListMetricsExports(scope, envResolveLinked, envIncludeSecrets, astroV1Client)
	if err != nil {
		return err
	}
	if envIncludeSecrets {
		fmt.Fprintln(os.Stderr, includeSecretsWarning)
	}
	return env.WriteMetricsExportList(objs, envIncludeSecrets, cliout.Renderer{Format: f, Out: out})
}

func runEnvMetricsGet(cmd *cobra.Command, out io.Writer, idOrKey string) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	f, err := cliout.ParseFormat(envOutput)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	obj, err := env.GetMetricsExport(idOrKey, scope, envIncludeSecrets, astroV1Client)
	if err != nil {
		return err
	}
	return env.WriteMetricsExport(obj, envIncludeSecrets, cliout.Renderer{Format: f, Out: out})
}

// runEnvMetricsSet upserts, with one asymmetry the other nouns do not have:
// the platform requires an endpoint and an exporter type to CREATE an export,
// but neither to update one. So the flags cannot simply be marked required —
// that would break every partial update — and they cannot simply be omitted
// either, or a set that falls through to create sends an object the API will
// reject for a reason the user has to infer.
//
// They are therefore checked on the create path only, and the error names both
// the missing flags and the reason they are suddenly needed.
func runEnvMetricsSet(cmd *cobra.Command, out io.Writer, idOrKey string) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	r, err := envRenderer(out)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	in, err := buildMetricsInput(cmd)
	if err != nil {
		return err
	}
	obj, err := env.UpdateMetricsExport(idOrKey, scope, in, astroV1Client)
	if err != nil {
		if errors.Is(err, env.ErrNotFound) && !envMetricsNoCreate {
			if cerr := refuseCreateByID("metrics export", idOrKey); cerr != nil {
				return cerr
			}
			obj, err = env.CreateMetricsExport(scope, idOrKey, in, astroV1Client)
			if err != nil {
				// CreateMetricsExport is the one definition of what creating
				// needs and it names the missing flag itself, so this wraps
				// its error rather than re-checking: a field the platform
				// later adds or drops changes that function and this message
				// follows.
				//
				// Deliberately no --no-create hint. This arm means the create
				// was genuine and a flag is missing, so pointing at the flag
				// that suppresses creating tells the user to abandon what they
				// were doing. That hint belongs on a bare not-found, the typo
				// reading, which setNotFound handles below.
				return fmt.Errorf("metrics export %q does not exist and could not be created: %w", idOrKey, err)
			}
			held := createdAsHeld(cmd.ErrOrStderr(), r, obj, scope, env.GetMetricsExport)
			return renderEnvSet(r, held, true)
		}
		if errors.Is(err, env.ErrNotFound) && envMetricsNoCreate {
			return setNotFound("metrics export", idOrKey, err)
		}
		return err
	}
	return renderEnvSet(r, obj, false)
}

func buildMetricsInput(cmd *cobra.Command) (*env.MetricsInput, error) {
	// Optional fields are sent only when the user explicitly set the flag, so
	// passing --basic-token="" (etc.) is preserved as "clear this field"
	// rather than being silently skipped.
	in := &env.MetricsInput{
		Endpoint:            envMetricsEndpoint,
		ExporterType:        envMetricsExporterType,
		AuthType:            envMetricsAuthType,
		AutoLinkDeployments: autoLinkPtr(cmd),
	}
	if cmd.Flags().Changed("basic-token") {
		in.BasicToken = &envMetricsBasicToken
	}
	if cmd.Flags().Changed("username") {
		in.Username = &envMetricsUsername
	}
	if cmd.Flags().Changed("sigv4-assume-arn") {
		in.SigV4AssumeArn = &envMetricsSigV4AssumeArn
	}
	if cmd.Flags().Changed("sigv4-sts-region") {
		in.SigV4StsRegion = &envMetricsSigV4StsRegion
	}
	// Same rule as a connection's password: an explicit --password is sent as
	// given, though empty does not clear a stored one — the platform keeps the
	// stored password when an update's is empty. Under BASIC auth a piped password is still read, but
	// an EMPTY read means "not given" rather than "the password is empty".
	//
	// Without that last distinction, `set prom_main --auth-type BASIC
	// --username newuser` in CI — stdin is not a terminal, so the read returns
	// nothing — sent an explicit empty password and silently cleared the
	// stored one while reporting success. Narrower than the connection version
	// of this bug, which fired on any non-interactive run, but the same fault.
	switch {
	case cmd.Flags().Changed("password"):
		in.Password = &envMetricsPassword
	case envMetricsAuthType == string(astrov1.CreateEnvironmentObjectMetricsExportRequestAuthTypeBASIC):
		pw, err := readSecretValue("", "Password", input.AnsweredBy("--password, or pipe it on stdin"))
		if err != nil {
			return nil, err
		}
		if pw != "" {
			in.Password = &pw
		}
	}
	if cmd.Flags().Changed("header") {
		m := envMetricsHeaders
		in.Headers = &m
	}
	if cmd.Flags().Changed("label") {
		m := envMetricsLabels
		in.Labels = &m
	}
	return in, nil
}
