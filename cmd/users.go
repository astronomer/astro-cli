package cmd

import "github.com/spf13/cobra"

var (
	usersRootCmd = &cobra.Command{
		Use:   "users",
		Short: "Manage astronomer users",
		Long:  "Manage astronomer users",
	}

	usersListCmd = &cobra.Command{
		Use:   "list",
		Short: "List astronomer users",
		Long:  "List astronomer users",
		Run:   usersList,
	}

	usersCreateCmd = &cobra.Command{
		Use:   "create",
		Short: "Add an astronomer user",
		Long:  "Add an astronomer user",
		Run:   usersCreate,
	}

	usersDeleteCmd = &cobra.Command{
		Use:   "delete",
		Short: "Delete an astronomer user",
		Long:  "Delete an astronomer user",
		Run:   usersDelete,
	}
)

func init() {
	// Users root
	RootCmd.AddCommand(usersRootCmd)

	// Users list
	usersRootCmd.AddCommand(usersListCmd)

	// Users create
	usersRootCmd.AddCommand(usersCreateCmd)

	// Users delete
	usersRootCmd.AddCommand(usersDeleteCmd)
}

func usersList(cmd *cobra.Command, args []string) {
}

func usersCreate(cmd *cobra.Command, args []string) {
}

func usersDelete(cmd *cobra.Command, args []string) {
}
