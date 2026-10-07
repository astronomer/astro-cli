package astro

import (
	"io"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
)

var (
	bundleName         string
	bundleMountPath    string
	bundleNonDagType   string
	bundleDescription  string
	bundleDagBundleIDs []string
	forceBundleDelete  bool
	bundleListOutput   string
)

func newDeploymentBundleRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "bundle",
		Aliases: []string{"bundles"},
		Short:   "Manage the DAG and non-DAG bundles on an Astro Deployment",
		Long:    "Manage the bundles registered on an Astro Deployment. DAG bundles carry DAGs and are targeted by 'astro deploy --dag-bundle-name'; non-DAG bundles mount other content (e.g. dbt projects) at a path.",
		Hidden:  true,
	}
	cmd.SetOut(out)
	cmd.AddCommand(
		newDeploymentBundleCreateCmd(out),
		newDeploymentBundleListCmd(out),
		newDeploymentBundleUpdateCmd(out),
		newDeploymentBundleDeleteCmd(out),
	)
	cmd.PersistentFlags().StringVar(&deploymentID, "deployment-id", "", "The Deployment whose bundles you'd like to manage. Run 'astro deployment list' to find valid IDs")
	addDeploymentFlag(cmd.PersistentFlags(), "Deployment whose bundles you'd like to manage: a Deployment id, or a link name from pyproject.toml. Run 'astro deployment list' to find valid IDs")
	return cmd
}

func newDeploymentBundleCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a bundle on an Astro Deployment",
		Long:  "Create a DAG bundle (with --name) or a non-DAG bundle (with --mount-path) on an Astro Deployment.",
		Example: `  # Create a named DAG bundle
  astro deployment bundle create --deployment <id> --name my-dags

  # Create a non-DAG bundle mounted at a path
  astro deployment bundle create --deployment <id> --mount-path /usr/local/airflow/dbt \
    --bundle-type dbt`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := coalesceWorkspace()
			if err != nil {
				return errors.Wrap(err, "failed to find a valid workspace")
			}
			cmd.SilenceUsage = true
			return deployment.CreateBundle(bundleName, bundleMountPath, bundleNonDagType, bundleDescription, bundleDagBundleIDs, ws, deploymentID, out, astroV1Client, astroV1Alpha1Client)
		},
	}
	cmd.Flags().StringVar(&bundleName, "name", "", "Name of the DAG bundle to create. Mutually exclusive with --mount-path")
	cmd.Flags().StringVar(&bundleMountPath, "mount-path", "", "Mount path for a non-DAG bundle. Mutually exclusive with --name")
	cmd.Flags().StringVar(&bundleNonDagType, "bundle-type", "", "Type of a non-DAG bundle (e.g. dbt). Only valid with --mount-path")
	cmd.Flags().StringVar(&bundleDescription, "description", "", "Description for the bundle")
	cmd.Flags().StringSliceVar(&bundleDagBundleIDs, "dag-bundle-ids", nil, "DAG bundle IDs a non-DAG bundle is served alongside. Only valid with --mount-path")
	return cmd
}

func newDeploymentBundleListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List the bundles on an Astro Deployment",
		Long:    "List every DAG and non-DAG bundle registered on an Astro Deployment.",
		Example: `  astro deployment bundle list --deployment <id>
  astro deployment bundle list --deployment <id> -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := cliout.ParseFormat(bundleListOutput)
			if err != nil {
				return err
			}
			ws, err := coalesceWorkspace()
			if err != nil {
				return errors.Wrap(err, "failed to find a valid workspace")
			}
			cmd.SilenceUsage = true
			return deployment.ListBundlesWithFormat(ws, deploymentID, cliout.Renderer{Format: format, Out: out}, astroV1Client, astroV1Alpha1Client)
		},
	}
	cliout.AddOutputFlag(cmd, &bundleListOutput)
	return cmd
}

func newDeploymentBundleUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [BUNDLE_ID]",
		Short: "Update a bundle on an Astro Deployment",
		Long:  "Update a bundle's description or, for a non-DAG bundle, the DAG bundles it is served alongside. Identify the bundle by its ID argument, its DAG bundle --name, or its non-DAG --mount-path.",
		Example: `  # Update a bundle's description, identified by ID
  astro deployment bundle update <bundle-id> --deployment <id> --description "my bundle"

  # Identify a DAG bundle by name instead of ID
  astro deployment bundle update --deployment <id> --name my-dags --description "my bundle"

  # Re-associate a non-DAG bundle (identified by mount path) with a different set of DAG bundles
  astro deployment bundle update --deployment <id> --mount-path /usr/local/airflow/dbt \
    --dag-bundle-ids <dag-bundle-id>`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := coalesceWorkspace()
			if err != nil {
				return errors.Wrap(err, "failed to find a valid workspace")
			}
			cmd.SilenceUsage = true
			var bundleID string
			if len(args) > 0 {
				bundleID = args[0]
			}
			return deployment.UpdateBundle(bundleID, bundleName, bundleMountPath, bundleDescription, bundleDagBundleIDs, ws, deploymentID, out, astroV1Client, astroV1Alpha1Client)
		},
	}
	cmd.Flags().StringVar(&bundleName, "name", "", "Identify the DAG bundle to update by name, instead of by ID")
	cmd.Flags().StringVar(&bundleMountPath, "mount-path", "", "Identify the non-DAG bundle to update by mount path, instead of by ID")
	cmd.Flags().StringVar(&bundleDescription, "description", "", "New description for the bundle")
	cmd.Flags().StringSliceVar(&bundleDagBundleIDs, "dag-bundle-ids", nil, "DAG bundle IDs a non-DAG bundle is served alongside. Replaces the existing set")
	return cmd
}

func newDeploymentBundleDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete [BUNDLE_ID]",
		Aliases: []string{"rm"},
		Short:   "Delete a bundle from an Astro Deployment",
		Long:    "Delete a DAG or non-DAG bundle from an Astro Deployment. Identify the bundle by its ID argument, its DAG bundle --name, or its non-DAG --mount-path.",
		Example: `  # Delete a bundle identified by ID
  astro deployment bundle delete <bundle-id> --deployment <id>

  # Identify a DAG bundle by name
  astro deployment bundle delete --deployment <id> --name my-dags

  # Identify a non-DAG bundle by mount path
  astro deployment bundle delete --deployment <id> --mount-path /usr/local/airflow/dbt`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := coalesceWorkspace()
			if err != nil {
				return errors.Wrap(err, "failed to find a valid workspace")
			}
			cmd.SilenceUsage = true
			var bundleID string
			if len(args) > 0 {
				bundleID = args[0]
			}
			return deployment.DeleteBundle(bundleID, bundleName, bundleMountPath, ws, deploymentID, forceBundleDelete, out, astroV1Client, astroV1Alpha1Client)
		},
	}
	cmd.Flags().StringVar(&bundleName, "name", "", "Identify the DAG bundle to delete by name, instead of by ID")
	cmd.Flags().StringVar(&bundleMountPath, "mount-path", "", "Identify the non-DAG bundle to delete by mount path, instead of by ID")
	cmd.Flags().BoolVarP(&forceBundleDelete, "yes", "y", false, "Don't ask for confirmation before deleting the bundle")
	return cmd
}
