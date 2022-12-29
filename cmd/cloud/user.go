package cloud

import (
	"io"

	"github.com/astronomer/astro-cli/pkg/input"

	"github.com/astronomer/astro-cli/cloud/user"
	"github.com/spf13/cobra"
)

var (
	role string
	limit int
	limitDefault = 100
)

func newUserCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "user",
		Aliases: []string{"us"},
		Short:   "Invite a user to your Astro Organization",
		Long:    "Invite a user to your Astro Organization.",
	}
	cmd.SetOut(out)
	cmd.AddCommand(
		newUserInviteCmd(out),
		newUserListCmd(out),
		newUserUpdateCmd(out),
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

func newUserListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"inv"},
		Short:   "List all the users in your Astro Organization",
		Long: "List all the users in your Astro Organization",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listUsers(cmd, out)
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "l", limitDefault, "Maximum number of organization users listed")
	return cmd
}

func newUserUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update [email]",
		Aliases: []string{"inv"},
		Short:   "Update a the role of a user your in Astro Organization",
		Long: "Update the role of a user in your Astro Organization\n$astro user update [email] --role [ORGANIZATION_MEMBER, " +
			"ORGANIZATION_BILLING_ADMIN, ORGANIZATION_OWNER].",
		RunE: func(cmd *cobra.Command, args []string) error {
			return userUpdate(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&role, "role", "r", "ORGANIZATION_MEMBER", "The new role for the "+
		"user. Possible values are ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN and ORGANIZATION_OWNER ")
	return cmd
}

func userInvite(cmd *cobra.Command, args []string, out io.Writer) error {
	var email string

	// if an email was provided in the args we use it
	if len(args) > 0 {
		email = args[0]
	} else {
		// no email was provided so ask the user for it
		email = input.Text("enter email address to invite a user: ")
	}

	cmd.SilenceUsage = true
	return user.CreateInvite(email, role, out, astroCoreClient)
}

func listUsers(cmd *cobra.Command, out io.Writer) error {
	cmd.SilenceUsage = true
	return user.ListOrgUsers(out, astroCoreClient, limit)
}

func userUpdate(cmd *cobra.Command, args []string, out io.Writer) error {
	var email string

	// if an email was provided in the args we use it
	if len(args) > 0 {
		email = args[0]
	}

	cmd.SilenceUsage = true
	return user.UpdateUserRole(email, role, out, astroCoreClient)
}
