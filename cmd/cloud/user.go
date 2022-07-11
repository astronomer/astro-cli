package cloud

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

var role string

func newUserCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "user",
		Aliases: []string{"us"},
		Short:   "Invite Users to your Astro Organization",
		Long:    "Invite Users to your Astro Organization.",
	}
	cmd.SetOut(out)
	cmd.AddCommand(
		newUserInviteCmd(out),
	)
	return cmd
}

func newUserInviteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "invite",
		Aliases: []string{"inv"},
		Short:   "Invite a user to your Astro Organization",
		Long:    "astro user invite [email] --role [ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN, ORGANIZATION_OWNER].",
		RunE: func(cmd *cobra.Command, args []string) error {
			return userInvite(cmd, args, out)
		},
	}
	// TODO Check if ORGANIZATION_MEMBER is the correct default?
	// TODO Check if the 3 roles below are the only ones allowed?
	cmd.Flags().StringVarP(&role, "role", "r", "ORGANIZATION_MEMBER", "The invited user's role. It can be one of ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN or ORGANIZATION_OWNER ")
	return cmd
}

func userInvite(cmd *cobra.Command, args []string, out io.Writer) error {
	var email, outMsg string
	cmd.SilenceUsage = false
	// TODO is this the right spot for validating args?
	if len(args) < 1 {
		_, err := out.Write([]byte("valid email is required to create an invite"))
		if err != nil {
			return err
		}
	}

	// TODO should we check if args[0] is a valid email address?
	// TODO make the API request here and return success/error
	email = args[0]
	outMsg = fmt.Sprintf("invite for %s with role %s created", email, role)
	_, err := out.Write([]byte(outMsg))
	if err != nil {
		return err
	}
	return nil
}
