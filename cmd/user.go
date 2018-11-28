package cmd

import (
	"github.com/astronomer/astro-cli/user"
	"github.com/spf13/cobra"
)

var (
	userEmail string

	userRootCmd = &cobra.Command{
		Use:   "user",
		Short: "Manage astronomer user",
		Long:  "Users represents a human who has authenticated with the Astronomer platform",
	}

	// userListCmd = &cobra.Command{
	// 	Use:     "list",
	// 	Aliases: []string{"ls"},
	// 	Short:   "List astronomer users",
	// 	Long:    "List astronomer users",
	// 	RunE:    userList,
	// }

	userCreateCmd = &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create a user in the astronomer platform",
		Long:    "Create a user in the astronomer platform, user will receive an invite at the email address provided",
		RunE:    userCreate,
	}

	// userDeleteCmd = &cobra.Command{
	// 	Use:     "delete",
	// 	Aliases: []string{"de"},
	// 	Short:   "Delete an astronomer user",
	// 	Long:    "Delete an astronomer user",
	// 	Run:     userDelete,
	// }
)

func init() {
	// User root
	RootCmd.AddCommand(userRootCmd)

	// User list
	// userRootCmd.AddCommand(userListCmd)

	// User create
	userRootCmd.AddCommand(userCreateCmd)
	userCreateCmd.Flags().StringVar(&userEmail, "email", "", "Supply user email at runtime")

	// User delete
	// userRootCmd.AddCommand(userDeleteCmd)
}

// func userList(cmd *cobra.Command, args []string) error {
// 	return nil
// }

func userCreate(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return user.Create(userEmail)
}

// func userDelete(cmd *cobra.Command, args []string) {
// }
