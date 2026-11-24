package astro

import (
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// newDeploymentBundleRootCmd keeps astro deployment bundle, which Astro CLI
// 1.x has and which was the path of the bundle commands before astro bundle,
// working: the same commands and flags, and the same --output json shapes,
// hidden, each warning once per run to use astro bundle. They are astro
// bundle's own commands (newBundleCreateCmd and the others), so the two cannot
// drift apart, with what 1.x's astro deployment bundle also had on top:
// --deployment-id, --workspace-id and --force's refusal.
func newDeploymentBundleRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "bundle",
		Aliases: []string{"bundles"},
		Hidden:  true,
		Short:   "Manage the Dag and non-Dag bundles on an Astro Deployment",
		Long:    "Manage the bundles registered on an Astro Deployment. Dag bundles carry Dags, which astro deploy --dag-bundle ships to one. Non-Dag bundles mount other content (e.g. dbt projects) at a path, and 'astro bundle deploy' deploys them. This is the old spelling of astro bundle.",
	}
	cmd.SetOut(out)
	cliout.AddOutputFlag(cmd, &bundleOutput)
	for _, sub := range []*cobra.Command{newBundleCreateCmd(out), newBundleListCmd(out), newBundleUpdateCmd(out), newBundleDeleteCmd(out)} {
		sub.Example = strings.ReplaceAll(sub.Example, "astro bundle", "astro deployment bundle")
		cmd.AddCommand(cliout.DeprecateCommand(sub, func(c *cobra.Command) string { return "astro bundle " + c.Name() }, ""))
	}
	addDeploymentFlag(cmd.PersistentFlags(), bundleDeploymentUsage, deploymentTargets{id: &deploymentID}, oneXDeploymentID)
	addDeploymentWorkspaceFlag(cmd.PersistentFlags())
	return cmd
}
