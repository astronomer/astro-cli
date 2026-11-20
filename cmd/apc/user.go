package apc

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/apc/user"
)

const (
	createUserExample = `  # Create a user, prompting for the password
  astro user create --email user@company.com`
)

func newUserCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage APC user",
		Long:  "A user represents a human who has authenticated with the APC platform",
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
		Short:   "Create a user in the APC platform",
		Long:    "Create a user in the APC platform. The user receives an invite at the email address.",
		Example: createUserExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			r := cliout.Renderer{Format: accessOutput, Out: out}
			created, err := user.Create(userEmail, userPassword, houstonClient)
			if err != nil {
				return err
			}
			return renderUserCreated(r, &created)
		},
	}
	cmd.Flags().StringVar(&userEmail, "email", "", "Email of the user to create. Prompted for if not set")
	cmd.Flags().StringVar(&userPassword, "password", "", "Password for the new user. Prompted for if not set")
	addAccessOutputFlag(cmd)
	return cmd
}
