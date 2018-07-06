package cmd

import (
	"github.com/astronomerio/astro-cli/workspace"
	"github.com/spf13/cobra"
)

var (
	createDesc string

	workspaceRootCmd = &cobra.Command{
		Use:     "workspace",
		Aliases: []string{"wo"},
		Short:   "Manage Astronomer workspaces",
		Long:    "Workspaces contain a group of Airflow Cluster Deployments. The creator of the workspace can invite other users into it",
	}

	workspaceListCmd = &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List astronomer workspaces",
		Long:    "List astronomer workspaces",
		RunE:    workspaceList,
	}

	workspaceCreateCmd = &cobra.Command{
		Use:     "create WORKSPACE",
		Aliases: []string{"cr"},
		Short:   "Create an astronomer workspaces",
		Long:    "Create an astronomer workspaces",
		Args:    cobra.ExactArgs(1),
		RunE:    workspaceCreate,
	}

	workspaceDeleteCmd = &cobra.Command{
		Use:     "delete WORKSPACE",
		Aliases: []string{"de"},
		Short:   "Delete an astronomer workspace",
		Long:    "Delete an astronomer workspace",
		Args:    cobra.ExactArgs(1),
		RunE:    workspaceDelete,
	}

	workspaceUpdateCmd = &cobra.Command{
		Use:     "update",
		Aliases: []string{"up"},
		Short:   "Update an Astronomer workspace",
		Long:    "Update a workspace name, as well as users and roles assigned to a workspace",
		RunE:    workspaceUpdate,
	}

	workspaceUserRootCmd = &cobra.Command{
		Use:   "user",
		Short: "Manage workspace user resources",
		Long:  "Users can be added or removed from workspaces",
	}

	workspaceUserAddCmd = &cobra.Command{
		Use:   "add EMAIL",
		Short: "Add a user to a workspace",
		Long:  "Add a user to a workspace",
		RunE:  workspaceUserAdd,
	}

	workspaceUserRmCmd = &cobra.Command{
		Use:     "remove EMAIL",
		Aliases: []string{"rm"},
		Short:   "Add a user to a workspace",
		Long:    "Add a user to a workspace",
		RunE:    workspaceUserRm,
	}
)

func init() {
	// workspace root
	RootCmd.AddCommand(workspaceRootCmd)

	// workspace list
	workspaceRootCmd.AddCommand(workspaceListCmd)

	// workspace create
	workspaceRootCmd.AddCommand(workspaceCreateCmd)
	workspaceCreateCmd.Flags().StringVarP(&createDesc, "desc", "d", "", "description for your new workspace")

	// workspace delete
	workspaceRootCmd.AddCommand(workspaceDeleteCmd)

	// workspace update
	workspaceRootCmd.AddCommand(workspaceUpdateCmd)

	// workspace user root
	workspaceRootCmd.AddCommand(workspaceUserRootCmd)

	// workspace user add
	workspaceUserRootCmd.AddCommand(workspaceUserAddCmd)
	workspaceUserAddCmd.PersistentFlags().StringVar(&workspaceId, "workspace-id", "", "workspace assigned to deployment")

	// workspace user remove
	workspaceUserRootCmd.AddCommand(workspaceUserRmCmd)
	workspaceUserRmCmd.PersistentFlags().StringVar(&workspaceId, "workspace-id", "", "workspace assigned to deployment")
}

func workspaceCreate(cmd *cobra.Command, args []string) error {
	if len(createDesc) == 0 {
		createDesc = "N/A"
	}
	return workspace.Create(args[0], createDesc)
}

func workspaceList(cmd *cobra.Command, args []string) error {
	return workspace.List()

}

func workspaceDelete(cmd *cobra.Command, args []string) error {
	return workspace.Delete(args[0])
}

// TODO
func workspaceUpdate(cmd *cobra.Command, args []string) error {
	return nil
}

func workspaceUserAdd(cmd *cobra.Command, args []string) error {
	ws := workspaceValidator()
	return workspace.Add(ws, args[0])
}

func workspaceUserRm(cmd *cobra.Command, args []string) error {
	ws := workspaceValidator()
	return workspace.Remove(ws, args[0])
}
