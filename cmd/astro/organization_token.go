package astro

// Running and rendering `astro organization token`. The platform package
// returns what each command did; this file asks the questions (which token,
// which role, are you sure) and decides how the answer looks, in text and in
// json.

import (
	"fmt"
	"io"
	"os"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/apitoken"
	"github.com/astronomer/astro-cli/internal/platform/astro/organization"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/input"
)

// organizationTokenOutput is the --output of the whole `organization token`
// family, registered once on its group.
var organizationTokenOutput cliout.Format

func listOrganizationToken(out io.Writer) error {
	if err := refuseCleanOutputWithJSON(organizationTokenOutput); err != nil {
		return err
	}
	tokens, err := organization.ListTokens(astroV1Client)
	if err != nil {
		return err
	}
	return renderTokenList(organizationTokenOutput, out, tokens, organizationRoleHeader)
}

func listOrganizationTokenRoles(args []string, out io.Writer) error {
	if err := refuseCleanOutputWithJSON(organizationTokenOutput); err != nil {
		return err
	}
	tokenArg(args, &tokenID)
	// roles takes no --name: only the token ID answers its picker.
	roles, err := organization.ListTokenRoles(tokenID, organizationTokenPicker("", "Select the Organization API token to list roles for:"), astroV1Client)
	if err != nil {
		return err
	}
	tab := &cliout.Table{Header: []string{"ENTITY_TYPE", "ENTITY_ID", "ROLE"}}
	for _, r := range roles {
		tab.AddRow(r.EntityType, r.EntityID, r.Role)
	}
	return cliout.Renderer{Format: organizationTokenOutput, Out: out}.Emit(apitoken.RoleList{Roles: roles}, cliout.Text(tab.Render))
}

func createOrganizationToken(out io.Writer) error {
	if err := refuseCleanOutputWithJSON(organizationTokenOutput); err != nil {
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
		fmt.Fprintln(os.Stderr, "Select an Organization Role for the new API token:")
		// no role was provided so ask the user for it
		var err error
		tokenRole, err = selectOrganizationRole()
		if err != nil {
			return err
		}
	}

	created, err := organization.CreateToken(tokenName, tokenDescription, tokenRole, tokenExpiration, astroV1Client)
	if err != nil {
		return err
	}
	return renderTokenSecret(organizationTokenOutput, out, &created, "Organization", "Created", tokenName, cleanTokenOutput)
}

func updateOrganizationToken(args []string, out io.Writer) error {
	if err := refuseCleanOutputWithJSON(organizationTokenOutput); err != nil {
		return err
	}
	tokenArg(args, &tokenID)
	res, err := organization.UpdateToken(tokenID, name, tokenName, tokenDescription, tokenRole,
		organizationTokenPicker("--name", "Select the Organization API token to update:"), astroV1Client)
	if err != nil {
		return err
	}
	return renderTokenLine(organizationTokenOutput, out, res.Token, fmt.Sprintf("Updated Organization API token %s", res.PreviousName))
}

func rotateOrganizationToken(args []string, out io.Writer) error {
	if err := refuseCleanOutputWithJSON(organizationTokenOutput); err != nil {
		return err
	}
	tokenArg(args, &tokenID)
	pick := organizationTokenPicker("--name", "Select the Organization API token to rotate:")
	token, err := organization.FindCurrentToken(tokenID, name, pick, astroV1Client)
	if err != nil {
		return err
	}
	if !forceRotate {
		if err := confirmTokenChange(
			"API Token rotation will invalidate the current token and cannot be undone.",
			fmt.Sprintf("\nRotate API token %s?", ansi.Bold(token.Name))); err != nil {
			return err
		}
	}
	rotated, err := organization.RotateToken(token, astroV1Client)
	if err != nil {
		return err
	}
	// Named as found, which a rotate by ID knows only after the lookup.
	return renderTokenSecret(organizationTokenOutput, out, &rotated, "Organization", "Rotated", token.Name, cleanTokenOutput)
}

func deleteOrganizationToken(args []string, out io.Writer) error {
	if err := refuseCleanOutputWithJSON(organizationTokenOutput); err != nil {
		return err
	}
	tokenArg(args, &tokenID)
	pick := organizationTokenPicker("--name", "Select the Organization API token to delete:")
	token, err := organization.FindCurrentToken(tokenID, name, pick, astroV1Client)
	if err != nil {
		return err
	}
	if !forceDelete {
		if err := confirmTokenChange("API token deletion cannot be undone.",
			fmt.Sprintf("\nDelete API token %s?", ansi.Bold(token.Name))); err != nil {
			return err
		}
	}
	removal, err := organization.DeleteToken(token, astroV1Client)
	if err != nil {
		return err
	}
	return renderTokenLine(organizationTokenOutput, out, removal, fmt.Sprintf("Deleted Organization API token %s", removal.Name))
}
