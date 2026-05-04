package cloud

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/astronomer/astro-cli/cloud/env"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

var (
	envLinkDeploymentID string
	envLinkValue        string
	envLinkExclude      bool
)

func newEnvVarLinkCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "link <id-or-key>",
		Short: "Link a workspace variable to a deployment, optionally with an override",
		Long: `Attach a workspace-scoped environment variable to a specific deployment.
If --value is provided, that value overrides the workspace default for the linked deployment only.
Pass --exclude to add the deployment to the excludeLinks list instead (used with --auto-link to opt specific deployments out).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvVarLink(cmd, out, args[0])
		},
	}
	cmd.Flags().StringVar(&envLinkDeploymentID, "deployment-id", "", "ID of the deployment to link (required)")
	cmd.Flags().StringVar(&envLinkValue, "value", "", "Override value to use for the linked deployment (only the linked deployment sees this value)")
	cmd.Flags().BoolVar(&envLinkExclude, "exclude", false, "Add to excludeLinks instead of links (auto-link only)")
	_ = cmd.MarkFlagRequired("deployment-id")
	cmd.MarkFlagsMutuallyExclusive("value", "exclude")
	return cmd
}

func newEnvVarUnlinkCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unlink <id-or-key>",
		Short: "Remove an explicit link or exclude between a workspace variable and a deployment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvVarUnlink(cmd, out, args[0])
		},
	}
	cmd.Flags().StringVar(&envLinkDeploymentID, "deployment-id", "", "ID of the deployment to unlink (required)")
	cmd.Flags().BoolVar(&envLinkExclude, "exclude", false, "Remove from excludeLinks instead of links")
	_ = cmd.MarkFlagRequired("deployment-id")
	return cmd
}

func newEnvVarLinksCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "links <id-or-key>",
		Short: "Show every deployment a workspace variable is linked to or excluded from",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvVarLinks(cmd, out, args[0])
		},
	}
	cmd.Flags().StringVar(&envFormat, "format", string(env.FormatTable), "Output format: table|json|yaml")
	return cmd
}

func runEnvVarLink(cmd *cobra.Command, out io.Writer, idOrKey string) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	if envLinkExclude {
		if err := env.ExcludeVar(idOrKey, scope, envLinkDeploymentID, astroCoreClient); err != nil {
			return err
		}
		fmt.Fprintf(out, "Excluded %s from deployment %s\n", idOrKey, envLinkDeploymentID)
		return nil
	}
	var override *string
	if cmd.Flags().Changed("value") {
		override = &envLinkValue
	}
	if err := env.LinkVar(idOrKey, scope, envLinkDeploymentID, override, astroCoreClient); err != nil {
		return err
	}
	if override != nil {
		fmt.Fprintf(out, "Linked %s to deployment %s (override value applied)\n", idOrKey, envLinkDeploymentID)
	} else {
		fmt.Fprintf(out, "Linked %s to deployment %s\n", idOrKey, envLinkDeploymentID)
	}
	return nil
}

func runEnvVarUnlink(cmd *cobra.Command, out io.Writer, idOrKey string) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	if envLinkExclude {
		if err := env.UnexcludeVar(idOrKey, scope, envLinkDeploymentID, astroCoreClient); err != nil {
			return err
		}
		fmt.Fprintf(out, "Removed exclude on %s for deployment %s\n", idOrKey, envLinkDeploymentID)
		return nil
	}
	if err := env.UnlinkVar(idOrKey, scope, envLinkDeploymentID, astroCoreClient); err != nil {
		return err
	}
	fmt.Fprintf(out, "Unlinked %s from deployment %s\n", idOrKey, envLinkDeploymentID)
	return nil
}

func runEnvVarLinks(cmd *cobra.Command, out io.Writer, idOrKey string) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	f, err := env.ParseFormat(envFormat)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	report, err := env.ListVarLinks(idOrKey, scope, envIncludeSecrets, astroCoreClient)
	if err != nil {
		return err
	}
	switch f {
	case env.FormatJSON:
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	case env.FormatYAML:
		enc := yaml.NewEncoder(out)
		defer enc.Close()
		return enc.Encode(report)
	case env.FormatDotenv:
		return errors.New("dotenv format is not supported for links")
	case env.FormatTable, "":
		writeLinksTable(report, out)
		return nil
	}
	return fmt.Errorf("invalid format %q", f)
}

// overrideDisplay renders an override value for the table view, with a
// special "(hidden)" marker for secret vars when --include-secrets is off
// (in which case the platform redacts the override and we can't distinguish
// "no override" from "redacted override" — flag the ambiguity instead of
// quietly showing "-").
func overrideDisplay(override *string, isSecret, includeSecrets bool) string {
	if isSecret && !includeSecrets {
		return "(hidden, use --include-secrets)"
	}
	if override == nil {
		return "-"
	}
	return *override
}

func writeLinksTable(report *env.VarLinksReport, out io.Writer) {
	value := report.WorkspaceValue
	if report.IsSecret {
		value = "**** (secret)"
	}
	fmt.Fprintf(out, "KEY:                %s\n", report.ObjectKey)
	fmt.Fprintf(out, "ID:                 %s\n", report.ObjectID)
	fmt.Fprintf(out, "WORKSPACE VALUE:    %s\n", value)
	fmt.Fprintf(out, "AUTO-LINK:          %t\n", report.AutoLinkDeployments)
	fmt.Fprintln(out)

	if len(report.Links) == 0 {
		fmt.Fprintln(out, "LINKS:              (none)")
	} else {
		linkTable := &printutil.Table{DynamicPadding: true, Header: []string{"#", "DEPLOYMENT", "OVERRIDE"}}
		for i, l := range report.Links {
			override := overrideDisplay(l.OverrideValue, report.IsSecret, envIncludeSecrets)
			linkTable.AddRow([]string{strconv.Itoa(i + 1), l.DeploymentID, override}, false)
		}
		fmt.Fprintln(out, "LINKS:")
		linkTable.Print(out)
	}

	fmt.Fprintln(out)
	if len(report.ExcludeLinks) == 0 {
		fmt.Fprintln(out, "EXCLUDES:           (none)")
	} else {
		fmt.Fprintln(out, "EXCLUDES:")
		for i, depID := range report.ExcludeLinks {
			fmt.Fprintf(out, "  %d. %s\n", i+1, depID)
		}
	}
}
