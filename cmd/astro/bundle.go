package astro

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
)

// defaultBundleVersionLimit is how many versions astro bundle versions list
// shows when --limit is not given: the newest ones, since a bundle written
// to often has a long history.
const defaultBundleVersionLimit = 20

var (
	bundleName          string
	bundleMountPath     string
	bundleContentType   string
	bundleNonDagType    string
	bundleDescription   string
	bundleDagBundleIDs  []string
	bundleVersion       string
	forceBundleDelete   bool
	forceBundleRollback bool
	// bundleOutput is the bundle group's --output, which every subcommand takes.
	bundleOutput cliout.Format
)

// bundleDeploymentUsage is --deployment on the bundle commands.
const bundleDeploymentUsage = "Deployment whose bundles you'd like to manage: a Deployment id, or a link name from pyproject.toml. Run 'astro deployment list' to find valid IDs"

func newBundleRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "bundle",
		Aliases: []string{"bundles"},
		Short:   "Manage Dag and non-Dag bundles and their versions",
		Long:    "Manage the bundles registered on an Astro Deployment and their versions. Dag bundles carry Dags; Astro CLI v2 does not deploy to a named Dag bundle yet. Non-Dag bundles mount other content (e.g. dbt projects) at a path, and 'astro deploy bundle' deploys them.",
	}
	cmd.PersistentPreRunE = followProjectPreRun(cmd)
	cmd.SetOut(out)
	cliout.AddOutputFlag(cmd, &bundleOutput)
	cmd.AddCommand(
		newBundleCreateCmd(out),
		newBundleListCmd(out),
		newBundleUpdateCmd(out),
		newBundleDeleteCmd(out),
		newBundleRollbackCmd(out),
		newBundleVersionsRootCmd(out),
		newBundleFilesRootCmd(out),
	)
	// astro bundle is new in v2, so it has no 1.x spellings to keep: --deployment
	// and --workspace only.
	addDeploymentFlag(cmd.PersistentFlags(), bundleDeploymentUsage, deploymentTargets{id: &deploymentID})
	addWorkspaceFlag(cmd.PersistentFlags(), &workspaceID, "w", deploymentWorkspaceUsage)
	applyPreferredFlagsIn(cmd)
	return cmd
}

func newBundleCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a bundle on an Astro Deployment",
		Long:  "Create a Dag bundle (with --name) or a non-Dag bundle (with --mount-path) on an Astro Deployment.",
		Example: `  # Create a named Dag bundle
  astro bundle create --deployment <id> --name my-dags

  # Create a non-Dag bundle mounted at a path
  astro bundle create --deployment <id> --mount-path /usr/local/airflow/dbt \
    --bundle-type dbt

  # Print the bundle created, as bundle list shows each one
  astro bundle create --deployment <id> --name my-dags -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			defer strayStdoutToStderr(bundleOutput)()
			ws, client, err := deploymentWorkspace(cmd, deploymentID, "could not find a valid Workspace")
			if err != nil {
				return err
			}
			return deployment.CreateBundle(bundleName, bundleMountPath, bundleContentType, bundleNonDagType, bundleDescription, bundleDagBundleIDs, ws, deploymentID, cliout.Renderer{Format: bundleOutput, Out: out, Err: cmd.ErrOrStderr()}, cmd.ErrOrStderr(), client, astroV1Alpha1Client)
		},
	}
	cmd.Flags().StringVarP(&bundleName, "name", "n", "", "Name of the Dag bundle to create. Mutually exclusive with --mount-path")
	cmd.Flags().StringVar(&bundleMountPath, "mount-path", "", "Mount path for a non-Dag bundle. Mutually exclusive with --name")
	cmd.Flags().StringVar(&bundleContentType, "type", "deploy", "How the bundle's content is written: deploy (by astro deploy) or file (by astro bundle files)")
	cmd.Flags().StringVar(&bundleNonDagType, "bundle-type", "", "Type of a non-Dag bundle (e.g. dbt). Only valid with --mount-path")
	cmd.Flags().StringVar(&bundleDescription, "description", "", "Description for the bundle")
	cmd.Flags().StringSliceVar(&bundleDagBundleIDs, "dag-bundle-ids", nil, "Dag bundle IDs a non-Dag bundle is served alongside. Only valid with --mount-path")
	// File bundles are not announced yet, as on 1.x main.
	_ = cmd.Flags().MarkHidden("type")
	return cmd
}

func newBundleListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List the bundles on an Astro Deployment",
		Long:    "List every Dag and non-Dag bundle registered on an Astro Deployment.",
		Example: `  astro bundle list --deployment <id>
  astro bundle list --deployment <id> -o json`,
	}
	addListLimit(cmd, "bundles", "Bundles are listed in the order the API returns them", func(cmd *cobra.Command, limit int) error {
		defer strayStdoutToStderr(bundleOutput)()
		ws, client, err := deploymentWorkspace(cmd, deploymentID, "could not find a valid Workspace")
		if err != nil {
			return err
		}
		return deployment.ListBundlesWithFormat(cmd.Context(), ws, deploymentID, limit, cliout.Renderer{Format: bundleOutput, Err: cmd.ErrOrStderr(), Out: out}, client, astroV1Alpha1Client)
	})
	return cmd
}

func newBundleUpdateCmd(out io.Writer) *cobra.Command {
	cmd := cliout.Args(&cobra.Command{
		Use:   "update",
		Short: "Update a bundle on an Astro Deployment",
		Long:  "Update a bundle's description or, for a non-Dag bundle, the Dag bundles it is served alongside. Identify the bundle by its ID argument, its Dag bundle --name, or its non-Dag --mount-path.",
		Example: `  # Update a bundle's description, identified by ID
  astro bundle update <bundle-id> --deployment <id> --description "my bundle"

  # Identify a Dag bundle by name instead of ID
  astro bundle update --deployment <id> --name my-dags --description "my bundle"

  # Re-associate a non-Dag bundle (identified by mount path) with a different set of Dag bundles
  astro bundle update --deployment <id> --mount-path /usr/local/airflow/dbt \
    --dag-bundle-ids <dag-bundle-id>

  # Print the bundle as the update left it
  astro bundle update <bundle-id> --deployment <id> --description "my bundle" -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			defer strayStdoutToStderr(bundleOutput)()
			ws, client, err := deploymentWorkspace(cmd, deploymentID, "could not find a valid Workspace")
			if err != nil {
				return err
			}
			return deployment.UpdateBundle(optionalArg(args), bundleName, bundleMountPath, bundleDescription, bundleDagBundleIDs, ws, deploymentID, cliout.Renderer{Format: bundleOutput, Out: out, Err: cmd.ErrOrStderr()}, cmd.ErrOrStderr(), client, astroV1Alpha1Client)
		},
	}, cliout.Optional("BUNDLE_ID"))
	cmd.Flags().StringVarP(&bundleName, "name", "n", "", "Identify the Dag bundle to update by name, instead of by ID")
	cmd.Flags().StringVar(&bundleMountPath, "mount-path", "", "Identify the non-Dag bundle to update by mount path, instead of by ID")
	cmd.Flags().StringVar(&bundleDescription, "description", "", "New description for the bundle")
	cmd.Flags().StringSliceVar(&bundleDagBundleIDs, "dag-bundle-ids", nil, "Dag bundle IDs a non-Dag bundle is served alongside. Replaces the existing set")
	return cmd
}

func newBundleDeleteCmd(out io.Writer) *cobra.Command {
	cmd := cliout.Args(&cobra.Command{
		Use:     "delete",
		Aliases: []string{"rm"},
		Short:   "Delete a bundle from an Astro Deployment",
		Long:    "Delete a Dag or non-Dag bundle from an Astro Deployment. Identify the bundle by its ID argument, its Dag bundle --name, or its non-Dag --mount-path.",
		Example: `  # Delete a bundle identified by ID
  astro bundle delete <bundle-id> --deployment <id>

  # Identify a Dag bundle by name
  astro bundle delete --deployment <id> --name my-dags

  # Identify a non-Dag bundle by mount path
  astro bundle delete --deployment <id> --mount-path /usr/local/airflow/dbt

  # Print what was deleted; under -o json there is no question, so pass --yes
  astro bundle delete <bundle-id> --deployment <id> --yes -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			defer strayStdoutToStderr(bundleOutput)()
			ws, client, err := deploymentWorkspace(cmd, deploymentID, "could not find a valid Workspace")
			if err != nil {
				return err
			}
			return deployment.DeleteBundle(optionalArg(args), bundleName, bundleMountPath, ws, deploymentID, forceBundleDelete, cliout.Renderer{Format: bundleOutput, Out: out, Err: cmd.ErrOrStderr()}, client, astroV1Alpha1Client)
		},
	}, cliout.Optional("BUNDLE_ID"))
	cmd.Flags().StringVarP(&bundleName, "name", "n", "", "Identify the Dag bundle to delete by name, instead of by ID")
	cmd.Flags().StringVar(&bundleMountPath, "mount-path", "", "Identify the non-Dag bundle to delete by mount path, instead of by ID")
	cmd.Flags().BoolVarP(&forceBundleDelete, "yes", "y", false, "Don't ask for confirmation before deleting the bundle")
	return cmd
}

func newBundleRollbackCmd(out io.Writer) *cobra.Command {
	cmd := cliout.Args(&cobra.Command{
		Use:   "rollback",
		Short: "Roll a bundle back to an earlier version",
		Long:  "Return a bundle to one of the versions 'astro bundle versions list' shows. Identify the bundle by its ID argument, its Dag bundle --name, or its non-Dag --mount-path. It asks for confirmation unless --yes.",
		Example: `  astro bundle rollback --deployment <id> --name my-files --version <version>
  astro bundle rollback <bundle-id> --deployment <id> --version <version> \
    --description "Undo the ETL change"

  # Under -o json there is no question, so pass --yes
  astro bundle rollback <bundle-id> --deployment <id> --version <version> --yes -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			defer strayStdoutToStderr(bundleOutput)()
			ws, client, err := deploymentWorkspace(cmd, deploymentID, "could not find a valid Workspace")
			if err != nil {
				return err
			}
			return deployment.RollbackBundle(cmd.Context(), optionalArg(args), bundleName, bundleMountPath, bundleVersion, bundleDescription, ws, deploymentID, forceBundleRollback, cliout.Renderer{Format: bundleOutput, Out: out, Err: cmd.ErrOrStderr()}, client, astroV1Alpha1Client)
		},
	}, cliout.Optional("BUNDLE_ID"))
	cmd.Flags().StringVarP(&bundleName, "name", "n", "", "Identify the Dag bundle to roll back by name, instead of by ID")
	cmd.Flags().StringVar(&bundleMountPath, "mount-path", "", "Identify the non-Dag bundle to roll back by mount path, instead of by ID")
	cmd.Flags().StringVar(&bundleVersion, "version", "", "The version to roll the bundle back to")
	cmd.Flags().StringVar(&bundleDescription, "description", "", "Description recorded on the rollback")
	cmd.Flags().BoolVarP(&forceBundleRollback, "yes", "y", false, "Don't ask for confirmation before rolling the bundle back")
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
	var limit int
	cmd := cliout.Args(&cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List the versions of a bundle",
		Long:    "List the versions a bundle has moved to, newest first, up to --limit. Identify the bundle by its ID argument, its Dag bundle --name, or its non-Dag --mount-path.",
		Example: `  astro bundle versions list --deployment <id> --name my-files
  astro bundle versions list <bundle-id> --deployment <id> --limit 100 -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkListLimit(cmd, "limit", limit); err != nil {
				return err
			}
			defer strayStdoutToStderr(bundleOutput)()
			ws, client, err := deploymentWorkspace(cmd, deploymentID, "could not find a valid Workspace")
			if err != nil {
				return err
			}
			return deployment.ListBundleVersionsWithFormat(cmd.Context(), optionalArg(args), bundleName, bundleMountPath, limit, ws, deploymentID, cliout.Renderer{Format: bundleOutput, Out: out, Err: cmd.ErrOrStderr()}, client, astroV1Alpha1Client)
		},
	}, cliout.Optional("BUNDLE_ID"))
	cmd.Flags().StringVarP(&bundleName, "name", "n", "", "Identify the Dag bundle by name, instead of by ID")
	cmd.Flags().StringVar(&bundleMountPath, "mount-path", "", "Identify the non-Dag bundle by mount path, instead of by ID")
	cmd.Flags().IntVar(&limit, "limit", defaultBundleVersionLimit, "Show at most this many versions, newest first (at least 1)")
	return cmd
}

// optionalArg is the first argument, or "" when there is none.
func optionalArg(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}
