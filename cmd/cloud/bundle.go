package cloud

import (
	"io"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/pkg/output"
)

const defaultBundleVersionLimit = 20

var (
	bundleName                   string
	bundleMountPath              string
	bundleContentType            string
	bundleNonDagType             string
	bundleDescription            string
	bundleDagBundleIDs           []string
	bundleVersion                string
	bundleVersionLimit           int
	forceBundleDelete            bool
	bundleListOutputFlags        output.Flags
	bundleVersionListOutputFlags output.Flags
)

func newBundleRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "bundle",
		Aliases: []string{"bundles"},
		Short:   "Manage Dag and non-Dag bundles, their versions and their files",
		Long:    "Manage the bundles registered on an Astro Deployment, their versions, and the files in file bundles. Dag bundles carry Dags and are targeted by 'astro deploy --dag-bundle-name'; non-Dag bundles mount other content (e.g. dbt projects) at a path.",
	}
	cmd.SetOut(out)
	cmd.AddCommand(
		newBundleCreateCmd(out),
		newBundleListCmd(out),
		newBundleUpdateCmd(out),
		newBundleDeleteCmd(out),
		newBundleRollbackCmd(out),
		newBundleVersionsRootCmd(out),
	)
	cmd.PersistentFlags().StringVar(&deploymentID, "deployment-id", "", "The Deployment whose bundles you'd like to manage, unless a bundle path names one. Run 'astro deployment list' to find valid IDs")
	cmd.PersistentFlags().StringVar(&workspaceID, "workspace-id", "", "The Workspace of the Deployment. Defaults to the current Workspace")
	return cmd
}

func newBundleCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a bundle on an Astro Deployment",
		Long:  "Create a Dag bundle (with --name) or a non-Dag bundle (with --mount-path) on an Astro Deployment. A deploy bundle is written by 'astro deploy'; a file bundle (--type file) has its files written with 'astro bundle files' and must be a Dag bundle.",
		Example: `  # Create a named Dag bundle
  astro bundle create --deployment-id <id> --name my-dags

  # Create a file bundle
  astro bundle create --deployment-id <id> --name my-files --type file

  # Create a non-Dag bundle mounted at a path
  astro bundle create --deployment-id <id> --mount-path /usr/local/airflow/dbt --bundle-type dbt`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := coalesceWorkspace()
			if err != nil {
				return errors.Wrap(err, "failed to find a valid workspace")
			}
			cmd.SilenceUsage = true
			return deployment.CreateBundle(bundleName, bundleMountPath, bundleContentType, bundleNonDagType, bundleDescription, bundleDagBundleIDs, ws, deploymentID, out, astroV1Client, astroV1Alpha1Client)
		},
	}
	cmd.Flags().StringVar(&bundleName, "name", "", "Name of the Dag bundle to create. Mutually exclusive with --mount-path")
	cmd.Flags().StringVar(&bundleMountPath, "mount-path", "", "Mount path for a non-Dag bundle. Mutually exclusive with --name")
	cmd.Flags().StringVar(&bundleContentType, "type", "deploy", "How the bundle's content is written: deploy (by 'astro deploy') or file (by 'astro bundle files')")
	cmd.Flags().StringVar(&bundleNonDagType, "bundle-type", "", "Type of a non-Dag bundle (e.g. dbt). Only valid with --mount-path")
	cmd.Flags().StringVar(&bundleDescription, "description", "", "Description for the bundle")
	cmd.Flags().StringSliceVar(&bundleDagBundleIDs, "dag-bundle-ids", nil, "Dag bundle IDs a non-Dag bundle is served alongside. Only valid with --mount-path")
	return cmd
}

func newBundleListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List the bundles on an Astro Deployment",
		Long:    "List every Dag and non-Dag bundle registered on an Astro Deployment.",
		Example: `  astro bundle list --deployment-id <id>
  astro bundle list --deployment-id <id> --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := bundleListOutputFlags.Resolve()
			if err != nil {
				return err
			}
			ws, err := coalesceWorkspace()
			if err != nil {
				return errors.Wrap(err, "failed to find a valid workspace")
			}
			cmd.SilenceUsage = true
			return deployment.ListBundlesWithFormat(ws, deploymentID, format, bundleListOutputFlags.Template, out, astroV1Client, astroV1Alpha1Client)
		},
	}
	bundleListOutputFlags.AddFlags(cmd)
	return cmd
}

func newBundleUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [BUNDLE-ID]",
		Short: "Update a bundle on an Astro Deployment",
		Long:  "Update a bundle's description or, for a non-Dag bundle, the Dag bundles it is served alongside. Identify the bundle by its ID argument, its Dag bundle --name, or its non-Dag --mount-path.",
		Example: `  # Update a bundle's description, identified by ID
  astro bundle update <bundle-id> --deployment-id <id> --description "my bundle"

  # Identify a Dag bundle by name instead of ID
  astro bundle update --deployment-id <id> --name my-dags --description "my bundle"

  # Re-associate a non-Dag bundle (identified by mount path) with a different set of Dag bundles
  astro bundle update --deployment-id <id> --mount-path /usr/local/airflow/dbt --dag-bundle-ids <dag-bundle-id>`,
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
	cmd.Flags().StringVar(&bundleName, "name", "", "Identify the Dag bundle to update by name, instead of by ID")
	cmd.Flags().StringVar(&bundleMountPath, "mount-path", "", "Identify the non-Dag bundle to update by mount path, instead of by ID")
	cmd.Flags().StringVar(&bundleDescription, "description", "", "New description for the bundle")
	cmd.Flags().StringSliceVar(&bundleDagBundleIDs, "dag-bundle-ids", nil, "Dag bundle IDs a non-Dag bundle is served alongside. Replaces the existing set")
	return cmd
}

func newBundleDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [BUNDLE-ID]",
		Short: "Delete a bundle from an Astro Deployment",
		Long:  "Delete a Dag or non-Dag bundle from an Astro Deployment. Identify the bundle by its ID argument, its Dag bundle --name, or its non-Dag --mount-path.",
		Example: `  # Delete a bundle identified by ID
  astro bundle delete <bundle-id> --deployment-id <id>

  # Identify a Dag bundle by name
  astro bundle delete --deployment-id <id> --name my-dags

  # Identify a non-Dag bundle by mount path
  astro bundle delete --deployment-id <id> --mount-path /usr/local/airflow/dbt`,
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
	cmd.Flags().StringVar(&bundleName, "name", "", "Identify the Dag bundle to delete by name, instead of by ID")
	cmd.Flags().StringVar(&bundleMountPath, "mount-path", "", "Identify the non-Dag bundle to delete by mount path, instead of by ID")
	cmd.Flags().BoolVarP(&forceBundleDelete, "force", "f", false, "Delete the bundle without confirmation")
	return cmd
}

func newBundleRollbackCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rollback [BUNDLE-ID]",
		Short: "Roll a bundle back to an earlier version",
		Long:  "Return a bundle to one of the versions 'astro bundle versions list' shows. Identify the bundle by its ID argument, its Dag bundle --name, or its non-Dag --mount-path.",
		Example: `  astro bundle rollback --deployment-id <id> --name my-files --version <version>
  astro bundle rollback <bundle-id> --deployment-id <id> --version <version> --description "Undo the ETL change"`,
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
			return deployment.RollbackBundle(bundleID, bundleName, bundleMountPath, bundleVersion, bundleDescription, ws, deploymentID, out, astroV1Client, astroV1Alpha1Client)
		},
	}
	cmd.Flags().StringVar(&bundleName, "name", "", "Identify the Dag bundle to roll back by name, instead of by ID")
	cmd.Flags().StringVar(&bundleMountPath, "mount-path", "", "Identify the non-Dag bundle to roll back by mount path, instead of by ID")
	cmd.Flags().StringVar(&bundleVersion, "version", "", "The version to roll the bundle back to")
	cmd.Flags().StringVar(&bundleDescription, "description", "", "Description recorded on the rollback")
	_ = cmd.MarkFlagRequired("version")
	return cmd
}

func newBundleVersionsRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "versions",
		Aliases: []string{"version"},
		Short:   "List the versions of a bundle",
		Long:    "List the versions a bundle has moved to. 'astro bundle rollback' returns a bundle to one of them.",
	}
	cmd.AddCommand(newBundleVersionsListCmd(out))
	return cmd
}

func newBundleVersionsListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list [BUNDLE-ID]",
		Aliases: []string{"ls"},
		Short:   "List the versions of a bundle",
		Long:    "List the versions a bundle has moved to, newest first, up to --limit. Identify the bundle by its ID argument, its Dag bundle --name, or its non-Dag --mount-path.",
		Example: `  astro bundle versions list --deployment-id <id> --name my-files
  astro bundle versions list <bundle-id> --deployment-id <id> --limit 100 --json`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := bundleVersionListOutputFlags.Resolve()
			if err != nil {
				return err
			}
			ws, err := coalesceWorkspace()
			if err != nil {
				return errors.Wrap(err, "failed to find a valid workspace")
			}
			cmd.SilenceUsage = true
			var bundleID string
			if len(args) > 0 {
				bundleID = args[0]
			}
			return deployment.ListBundleVersionsWithFormat(bundleID, bundleName, bundleMountPath, bundleVersionLimit, ws, deploymentID, format, bundleVersionListOutputFlags.Template, out, astroV1Client, astroV1Alpha1Client)
		},
	}
	cmd.Flags().StringVar(&bundleName, "name", "", "Identify the Dag bundle by name, instead of by ID")
	cmd.Flags().StringVar(&bundleMountPath, "mount-path", "", "Identify the non-Dag bundle by mount path, instead of by ID")
	cmd.Flags().IntVar(&bundleVersionLimit, "limit", defaultBundleVersionLimit, "The maximum number of versions to list, newest first")
	bundleVersionListOutputFlags.AddFlags(cmd)
	return cmd
}
