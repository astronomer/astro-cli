package cloud

import (
	"io"

	"github.com/spf13/cobra"
)

func newUserCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "user",
		Aliases: []string{"us"},
		Short:   "Manage users in your Astro Organization",
		Long:    "Manage users in your Astro Organization.",
	}
	cmd.SetOut(out)
	cmd.AddCommand(
		newUserInviteCmd(out),
	)
	return cmd
}

func newUserInviteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "invite [email]",
		Aliases: []string{"inv"},
		Short:   "Invite a user to your Astro Organization",
		Long: "Invite a user to your Astro Organization\n$astro user invite [email] --role [ORGANIZATION_MEMBER, " +
			"ORGANIZATION_BILLING_ADMIN, ORGANIZATION_OWNER].",
		RunE: func(cmd *cobra.Command, args []string) error {
			return userInvite(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&role, "role", "r", "ORGANIZATION_MEMBER", "The role for the "+
		"user. Possible values are ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN and ORGANIZATION_OWNER ")
	return cmd
}
