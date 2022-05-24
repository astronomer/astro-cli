package software

import (
	"io"

	"github.com/astronomer/astro-cli/software/user"

	"github.com/spf13/cobra"
)

const (
	createUserExample = `astro user create --email=<user-email-address>`
)

func newUserCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage Astronomer user",
		Long:  "Users represents a human who has authenticated with the Astronomer platform",
	}
	cmd.AddCommand(
		newUserCreateCmd(out),
	)
	return cmd
}

func newUserCreateCmd(out io.Writer) *cobra.Command {
	var (
		userEmail    string
		userPassword string
	)
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create a user in the astronomer platform",
		Long:    "Create a user in the astronomer platform, user will receive an invite at the email address provided",
		Example: createUserExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return user.Create(userEmail, userPassword, houstonClient, out)
		},
	}
	cmd.Flags().StringVarP(&userEmail, "email", "e", "", "Email of the user to create on the platform. If not specified, you will be prompted to enter this value")
	cmd.Flags().StringVarP(&userPassword, "password", "p", "", "Password to be set for the new user. If not specified, you will be prompted to enter this value")
	return cmd
}
