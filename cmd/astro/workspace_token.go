package astro

// Running and rendering `astro workspace token`. The platform package returns
// what each command did; this file asks the questions (which token, which
// role, are you sure) and decides how the answer looks, in text and in json.

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/internal/platform/astro/apitoken"
	"github.com/astronomer/astro-cli/internal/platform/astro/organization"
	workspacetoken "github.com/astronomer/astro-cli/internal/platform/astro/workspace-token"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/input"
)

// workspaceTokenOutput is the --output of the whole `workspace token` family,
// registered once on its group.
var workspaceTokenOutput string

// tokenArg lowercases a token ID given as the command's argument into id.
func tokenArg(args []string, id *string) {
	if len(args) > 0 {
		*id = strings.ToLower(args[0])
	}
}

func listWorkspaceToken(cmd *cobra.Command, out io.Writer) error {
	return runWorkspaceTokenList(cmd, out, nil)
}

// listOrganizationTokensInWorkspace lists the Organization tokens with a role
// on the Workspace.
func listOrganizationTokensInWorkspace(cmd *cobra.Command, out io.Writer) error {
	return runWorkspaceTokenList(cmd, out, []workspacetoken.TokenType{workspacetoken.TokenTypeORGANIZATION})
}

func runWorkspaceTokenList(cmd *cobra.Command, out io.Writer, tokenTypes []workspacetoken.TokenType) error {
	format, err := tokenFormatOf(workspaceTokenOutput)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true
	tokens, err := workspacetoken.ListTokens(astroV1Client, workspaceID, tokenTypes)
	if err != nil {
		return err
	}
	return renderTokenList(format, out, tokens, workspaceRoleHeader)
}

func createWorkspaceToken(cmd *cobra.Command, out io.Writer) error {
	format, err := tokenFormatOf(workspaceTokenOutput)
	if err != nil {
		return err
	}
	if tokenName == "" {
		// no role was provided so ask the user for it
		answer, err := input.Text("Enter a name for the new Workspace API token: ", input.AnsweredBy("--name"))
		if err != nil {
			return err
		}
		tokenName = answer
	}
	if tokenRole == "" {
		if err := mayPickRole(); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "select a Workspace Role for the new API token:")
		// no role was provided so ask the user for it
		tokenRole, err = selectWorkspaceRole()
		if err != nil {
			return err
		}
	}
	cmd.SilenceUsage = true

	created, err := workspacetoken.CreateToken(tokenName, tokenDescription, tokenRole, workspaceID, tokenExpiration, astroV1Client)
	if err != nil {
		return err
	}
	return renderTokenSecret(format, out, &created, "Workspace", "created", tokenName, cleanTokenOutput)
}

func updateWorkspaceToken(cmd *cobra.Command, args []string, out io.Writer) error {
	format, err := tokenFormatOf(workspaceTokenOutput)
	if err != nil {
		return err
	}
	tokenArg(args, &tokenID)
	cmd.SilenceUsage = true
	res, err := workspacetoken.UpdateToken(tokenID, name, tokenName, tokenDescription, tokenRole, workspaceID,
		workspaceTokenPicker("--name", "Please select the Workspace API token you would like to update:"), astroV1Client)
	if err != nil {
		return err
	}
	return renderTokenLine(format, out, res.Token, fmt.Sprintf("Astro Workspace API token %s was successfully updated", res.PreviousName))
}

func rotateWorkspaceToken(cmd *cobra.Command, args []string, out io.Writer) error {
	format, err := tokenFormatOf(workspaceTokenOutput)
	if err != nil {
		return err
	}
	tokenArg(args, &tokenID)
	cmd.SilenceUsage = true
	ws, org, err := workspacetoken.Target(workspaceID)
	if err != nil {
		return err
	}
	tokenTypes := []workspacetoken.TokenType{workspacetoken.TokenTypeWORKSPACE}
	pick := workspaceTokenPicker("--name", "Please select the Workspace API token you would like to rotate:")
	token, err := workspacetoken.FindToken(tokenID, name, ws, org, tokenTypes, pick, astroV1Client)
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
	rotated, err := workspacetoken.RotateToken(token, ws, astroV1Client)
	if err != nil {
		return err
	}
	// Named as found, which a rotate by ID knows only after the lookup.
	return renderTokenSecret(format, out, &rotated, "Workspace", "rotated", token.Name, cleanTokenOutput)
}

func deleteWorkspaceToken(cmd *cobra.Command, args []string, out io.Writer) error {
	format, err := tokenFormatOf(workspaceTokenOutput)
	if err != nil {
		return err
	}
	tokenArg(args, &tokenID)
	cmd.SilenceUsage = true
	ws, org, err := workspacetoken.Target(workspaceID)
	if err != nil {
		return err
	}
	pick := workspaceTokenPicker("--name", "Please select the API token you would like to delete from the Workspace:")
	token, err := workspacetoken.FindToken(tokenID, name, ws, org, nil, pick, astroV1Client)
	if err != nil {
		return err
	}
	isWS := string(token.Scope) == string(workspacetoken.TokenTypeWORKSPACE)
	if !forceDelete {
		warning, question, canceled := "", fmt.Sprintf("\nAre you sure you want to remove the %s API token from the Workspace?", ansi.Bold(token.Name)), "Canceling API Token removal"
		if isWS {
			warning = "WARNING: API token deletion cannot be undone."
			question = fmt.Sprintf("\nAre you sure you want to delete the %s API token?", ansi.Bold(token.Name))
			canceled = "Canceling API Token deletion"
		}
		ok, err := confirmTokenChange(warning, question)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(out, canceled)
			return nil
		}
	}
	removal, err := workspacetoken.DeleteToken(token, ws, astroV1Client)
	if err != nil {
		return err
	}
	line := fmt.Sprintf("Astro Organization API token %s was successfully removed from the Workspace", removal.Name)
	if removal.Action == apitoken.Deleted {
		line = fmt.Sprintf("Astro Workspace API token %s was successfully deleted", removal.Name)
	}
	return renderTokenLine(format, out, removal, line)
}

// addOrgTokenToWorkspace runs `workspace token add`, the older spelling of
// `workspace token organization-token add`, which picks the role from a table
// rather than asking for its name.
func addOrgTokenToWorkspace(cmd *cobra.Command, args []string, out io.Writer) error {
	format, err := tokenFormatOf(workspaceTokenOutput)
	if err != nil {
		return err
	}
	tokenArg(args, &orgTokenID)
	if tokenRole == "" {
		if err := mayPickRole(); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "select a Workspace Role for the Organization Token:")
		// no role was provided so ask the user for it
		tokenRole, err = selectWorkspaceRole()
		if err != nil {
			return err
		}
	}
	cmd.SilenceUsage = true
	pick := organizationTokenPicker("--org-token-name", "Please select the Organization API token you would like to add to the Workspace:")
	token, err := organization.AddOrgTokenToWorkspace(orgTokenID, orgTokenName, tokenRole, workspaceID, pick, astroV1Client)
	if err != nil {
		return err
	}
	return renderTokenLine(format, out, token, fmt.Sprintf("Astro Organization API token %s was successfully added to the Workspace", token.Name))
}

func addOrgTokenWorkspaceRole(cmd *cobra.Command, args []string, out io.Writer) error {
	return upsertOrgTokenWorkspaceRole(cmd, args, out, tokenRoleAdd,
		"Enter a role for the API token. Possible values are "+allowedWorkspaceRoleNamesProse+": ")
}

func updateOrgTokenWorkspaceRole(cmd *cobra.Command, args []string, out io.Writer) error {
	return upsertOrgTokenWorkspaceRole(cmd, args, out, tokenRoleUpdate,
		"Enter a role for the new Workspace API token. Possible values are "+allowedWorkspaceRoleNamesProse+": ")
}

// upsertOrgTokenWorkspaceRole runs `workspace token organization-token add`
// and `update`: it gives an Organization token a role on the Workspace,
// asking for the role with prompt when --role named none. An add picks among
// the Organization's tokens, an update among the Workspace's.
func upsertOrgTokenWorkspaceRole(cmd *cobra.Command, args []string, out io.Writer, operation, prompt string) error {
	format, err := tokenFormatOf(workspaceTokenOutput)
	if err != nil {
		return err
	}
	tokenArg(args, &orgTokenID)
	if tokenRole == "" {
		// no role was provided so ask the user for it
		answer, err := input.Text(prompt, input.AnsweredBy("--role"))
		if err != nil {
			return err
		}
		tokenRole = answer
	}
	cmd.SilenceUsage = true

	pick := workspaceTokenPicker("--org-token-name", "Please select the Organization API token whose Workspace role you would like to update:")
	if operation == tokenRoleAdd {
		pick = organizationTokenPicker("--org-token-name", "Please select the Organization API token you would like to add to the Workspace:")
	}
	token, err := workspacetoken.UpsertOrgTokenWorkspaceRole(orgTokenID, orgTokenName, tokenRole, workspaceID, operation, pick, astroV1Client)
	if err != nil {
		return err
	}
	return renderTokenLine(format, out, token, fmt.Sprintf("Astro Organization API token %s was successfully added/updated to the Workspace", token.Name))
}

func removeOrganizationTokenWorkspaceRole(cmd *cobra.Command, args []string, out io.Writer) error {
	format, err := tokenFormatOf(workspaceTokenOutput)
	if err != nil {
		return err
	}
	tokenArg(args, &orgTokenID)
	cmd.SilenceUsage = true
	pick := workspaceTokenPicker("--org-token-name", "Please select the Organization API token you would like to remove from the Workspace:")
	removal, err := workspacetoken.RemoveOrgTokenWorkspaceRole(orgTokenID, orgTokenName, workspaceID, pick, astroV1Client)
	if err != nil {
		return err
	}
	return renderTokenLine(format, out, removal, fmt.Sprintf("Astro Organization API token %s was successfully removed from the Workspace", removal.Name))
}
