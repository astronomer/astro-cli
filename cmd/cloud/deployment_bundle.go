package cloud

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// newDeploymentBundleRootCmd keeps 'astro deployment bundle', the path of the
// bundle commands before 'astro bundle', working as a hidden alias.
func newDeploymentBundleRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:        "bundle",
		Aliases:    []string{"bundles"},
		Short:      "Manage the Dag and non-Dag bundles on an Astro Deployment",
		Hidden:     true,
		Deprecated: "use 'astro bundle' instead",
	}
	deleteCmd := newBundleDeleteCmd(out)
	deleteCmd.Aliases = []string{"rm"}
	for _, sub := range []*cobra.Command{newBundleCreateCmd(out), newBundleListCmd(out), newBundleUpdateCmd(out), deleteCmd} {
		sub.Deprecated = fmt.Sprintf("use 'astro bundle %s' instead", sub.Name())
		cmd.AddCommand(sub)
	}
	cmd.PersistentFlags().StringVar(&deploymentID, "deployment-id", "", "The Deployment whose bundles you'd like to manage. Run 'astro deployment list' to find valid IDs")
	return cmd
}
