package cmd

import (
	"io"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/user"
	"github.com/spf13/cobra"
)


func newUserCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage astronomer user",
		Long:  "Users represents a human who has authenticated with the Astronomer platform",
	}
	cmd.AddCommand(
		newUserCreateCmd(client, out),
	)
	return cmd
}

func newUserCreateCmd(client *houston.Client, out io.Writer) *cobra.Command {
	var (
		userEmail string
		userPassword string
	)
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create a user in the astronomer platform",
		Long:    "Create a user in the astronomer platform, user will receive an invite at the email address provided",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return user.Create(userEmail, userPassword, client, out)
		},
	}
	cmd.Flags().StringVarP(&userEmail, "email", "e", "", "Supply user email at runtime")
	cmd.Flags().StringVarP(&userPassword, "password", "p", "", "Supply user password at runtime")
	return cmd
}
