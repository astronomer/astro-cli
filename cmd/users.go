package cmd

import (
	"fmt"
	"os"

	"github.com/astronomerio/astro-cli/users"
	"github.com/spf13/cobra"
)

var (
	skipVerify bool
	userEmail  string

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
		RunE:  usersCreate,
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
	usersCreateCmd.Flags().BoolVar(&skipVerify, "skip-verify", false, "Skips password verification on create")
	usersCreateCmd.Flags().StringVar(&userEmail, "email", "", "Supply user email at runtime")

	// Users delete
	usersRootCmd.AddCommand(usersDeleteCmd)
}

func usersList(cmd *cobra.Command, args []string) {
}

func usersCreate(cmd *cobra.Command, args []string) error {
	err := users.CreateUser(skipVerify, userEmail)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	return nil
}

func usersDelete(cmd *cobra.Command, args []string) {
}
