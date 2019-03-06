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

	userCreateCmd = &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create a user in the astronomer platform",
		Long:    "Create a user in the astronomer platform, user will receive an invite at the email address provided",
		RunE:    userCreate,
	}
)

func init() {
	// User root
	rootCmd.AddCommand(userRootCmd)

	// User create
	userRootCmd.AddCommand(userCreateCmd)
	userCreateCmd.Flags().StringVar(&userEmail, "email", "", "Supply user email at runtime")
}

func userCreate(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return user.Create(userEmail)
}

