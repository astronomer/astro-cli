package astro

// Running and rendering `astro organization token`. The platform package
// returns what each command did; this file asks the questions (which token,
// which role, are you sure) and decides how the answer looks, in text and in
// json.

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/apitoken"
	"github.com/astronomer/astro-cli/internal/platform/astro/organization"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/input"
)

// organizationTokenOutput is the --output of the whole `organization token`
// family, registered once on its group.
var organizationTokenOutput string

func listOrganizationToken(cmd *cobra.Command, out io.Writer) error {
	format, err := tokenFormatOf(organizationTokenOutput)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true
	tokens, err := organization.ListTokens(astroV1Client)
	if err != nil {
		return err
	}
	return renderTokenList(format, out, tokens, organizationRoleHeader)
}

func listOrganizationTokenRoles(cmd *cobra.Command, args []string, out io.Writer) error {
	format, err := tokenFormatOf(organizationTokenOutput)
	if err != nil {
		return err
	}
	tokenArg(args, &tokenID)
	cmd.SilenceUsage = true
	// roles takes no --name: only the token ID answers its picker.
	roles, err := organization.ListTokenRoles(tokenID, organizationTokenPicker("", "Please select the Organization API token whose roles you would like to list:"), astroV1Client)
	if err != nil {
		return err
	}
	tab := &cliout.Table{Header: []string{"ENTITY_TYPE", "ENTITY_ID", "ROLE"}}
	for _, r := range roles {
		tab.AddRow(r.EntityType, r.EntityID, r.Role)
	}
	return cliout.Renderer{Format: format, Out: out}.Emit(apitoken.RoleList{Roles: roles}, cliout.Text(tab.Render))
}

func createOrganizationToken(cmd *cobra.Command, out io.Writer) error {
	format, err := tokenFormatOf(organizationTokenOutput)
	if err != nil {
		return err
	}
	if tokenName == "" {
		// no role was provided so ask the user for it
		answer, err := input.Text("Enter a name for the new Organization API token: ", input.AnsweredBy("--name"))
		if err != nil {
			return err
		}
		tokenName = answer
	}
	if tokenRole == "" {
		if err := mayPickRole(); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "select a Organization Role for the new API token:")
		// no role was provided so ask the user for it
		tokenRole, err = selectOrganizationRole()
		if err != nil {
			return err
		}
	}
	cmd.SilenceUsage = true

	created, err := organization.CreateToken(tokenName, tokenDescription, tokenRole, tokenExpiration, astroV1Client)
	if err != nil {
		return err
	}
	return renderTokenSecret(format, out, &created, "Organization", "created", tokenName, cleanTokenOutput)
}

func updateOrganizationToken(cmd *cobra.Command, args []string, out io.Writer) error {
	format, err := tokenFormatOf(organizationTokenOutput)
	if err != nil {
		return err
	}
	tokenArg(args, &tokenID)
	cmd.SilenceUsage = true
	res, err := organization.UpdateToken(tokenID, name, tokenName, tokenDescription, tokenRole,
		organizationTokenPicker("--name", "Please select the Organization API token you would like to update:"), astroV1Client)
	if err != nil {
		return err
	}
	return renderTokenLine(format, out, res.Token, fmt.Sprintf("Astro Organization API token %s was successfully updated", res.PreviousName))
}

func rotateOrganizationToken(cmd *cobra.Command, args []string, out io.Writer) error {
	format, err := tokenFormatOf(organizationTokenOutput)
	if err != nil {
		return err
	}
	tokenArg(args, &tokenID)
	cmd.SilenceUsage = true
	pick := organizationTokenPicker("--name", "Please select the Organization API token you would like to rotate:")
	token, err := organization.FindCurrentToken(tokenID, name, pick, astroV1Client)
	if err != nil {
		return err
	}
	if !forceRotate {
		ok, err := confirmTokenChange(
			"WARNING: API Token rotation will invalidate the current token and cannot be undone.",
			fmt.Sprintf("\nAre you sure you want to rotate the %s API token?", ansi.Bold(token.Name)))
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(out, "Canceling token rotation")
			return nil
		}
	}
	rotated, err := organization.RotateToken(token, astroV1Client)
	if err != nil {
		return err
	}
	// Named as found, which a rotate by ID knows only after the lookup.
	return renderTokenSecret(format, out, &rotated, "Organization", "rotated", token.Name, cleanTokenOutput)
}

func deleteOrganizationToken(cmd *cobra.Command, args []string, out io.Writer) error {
	format, err := tokenFormatOf(organizationTokenOutput)
	if err != nil {
		return err
	}
	tokenArg(args, &tokenID)
	cmd.SilenceUsage = true
	pick := organizationTokenPicker("--name", "Please select the Organization API token you would like to delete:")
	token, err := organization.FindCurrentToken(tokenID, name, pick, astroV1Client)
	if err != nil {
		return err
	}
	if !forceDelete {
		ok, err := confirmTokenChange("WARNING: API token deletion cannot be undone.",
			fmt.Sprintf("\nAre you sure you want to delete the %s API token?", ansi.Bold(token.Name)))
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(out, "Canceling API Token deletion")
			return nil
		}
	}
	removal, err := organization.DeleteToken(token, astroV1Client)
	if err != nil {
		return err
	}
	return renderTokenLine(format, out, removal, fmt.Sprintf("Astro Organization API token %s was successfully deleted", removal.Name))
}
