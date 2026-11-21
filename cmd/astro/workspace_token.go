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

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/internal/platform/astro/organization"
	workspacetoken "github.com/astronomer/astro-cli/internal/platform/astro/workspace-token"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/input"
)

// workspaceTokenOutput is the --output of the whole `workspace token` family,
// registered once on its group.
var workspaceTokenOutput cliout.Format

// offeredWorkspaceTokens is the scopes a delete or a remove looks among: the
// one it can act on when it was named no token and has to ask, so the picker
// offers no row it would refuse, and none (every scope) when the token is
// named, so a wrong-scope one is refused with what to use instead.
func offeredWorkspaceTokens(scope workspacetoken.TokenType) []workspacetoken.TokenType {
	if tokenID == "" && name == "" {
		return []workspacetoken.TokenType{scope}
	}
	return nil
}

// addTokenNameFlag registers --name (-n) on cmd, read into name, with oneX,
// the spelling 1.x had for it on that command ("" when it had none), as its
// hidden alias.
func addTokenNameFlag(cmd *cobra.Command, usage, oneX string) {
	cmd.Flags().StringVarP(&name, "name", "n", "", usage)
	if oneX != "" {
		utils.AddAlias(cmd.Flags(), oneX, "name")
	}
}

// tokenArg lowercases a token ID given as the command's argument into id.
func tokenArg(args []string, id *string) {
	if len(args) > 0 {
		*id = strings.ToLower(args[0])
	}
}

func listWorkspaceToken(out io.Writer) error {
	return runWorkspaceTokenList(out, nil)
}

// listOrganizationTokensInWorkspace lists the Organization tokens with a role
// on the Workspace.
func listOrganizationTokensInWorkspace(out io.Writer) error {
	return runWorkspaceTokenList(out, []workspacetoken.TokenType{workspacetoken.TokenTypeORGANIZATION})
}

func runWorkspaceTokenList(out io.Writer, tokenTypes []workspacetoken.TokenType) error {
	tokens, err := workspacetoken.ListTokens(astroV1Client, workspaceID, tokenTypes)
	if err != nil {
		return err
	}
	return renderTokenList(workspaceTokenOutput, out, tokens, workspaceRoleHeader)
}

func createWorkspaceToken(cmd *cobra.Command, out io.Writer) error {
	ws, err := requiredWorkspace(cmd, workspaceID)
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
		fmt.Fprintln(os.Stderr, "Select a Workspace Role for the new API token:")
		// no role was provided so ask the user for it
		var err error
		tokenRole, err = selectWorkspaceRole()
		if err != nil {
			return err
		}
	}

	created, err := workspacetoken.CreateToken(tokenName, tokenDescription, tokenRole, ws, tokenExpiration, astroV1Client)
	if err != nil {
		return err
	}
	return renderTokenSecret(workspaceTokenOutput, out, cmd.ErrOrStderr(), &created, "Workspace", "Created", tokenName)
}

func updateWorkspaceToken(args []string, out io.Writer) error {
	tokenArg(args, &tokenID)
	res, err := workspacetoken.UpdateToken(tokenID, name, tokenName, tokenDescription, tokenRole, workspaceID, nil,
		workspaceTokenPicker("Select the API token to update:"), astroV1Client)
	if err != nil {
		return err
	}
	line := fmt.Sprintf("Updated Workspace API token %s", res.PreviousName)
	if res.Token.Scope != string(workspacetoken.TokenTypeWORKSPACE) {
		line = fmt.Sprintf("Updated Organization API token %s in the Workspace", res.PreviousName)
	}
	return renderTokenLine(workspaceTokenOutput, out, res.Token, line)
}

func rotateWorkspaceToken(args []string, out, errOut io.Writer) error {
	tokenArg(args, &tokenID)
	target, org, err := workspacetoken.Scope(workspaceID)
	if err != nil {
		return err
	}
	ws := target.Workspace
	tokenTypes := []workspacetoken.TokenType{workspacetoken.TokenTypeWORKSPACE}
	pick := workspaceTokenPicker("Select the Workspace API token to rotate:")
	token, err := workspacetoken.FindTokenIn(tokenID, name, ws, org, tokenTypes, pick, astroV1Client)
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
	rotated, err := workspacetoken.RotateToken(token, ws, astroV1Client)
	if err != nil {
		return err
	}
	// Named as found, which a rotate by ID knows only after the lookup.
	return renderTokenSecret(workspaceTokenOutput, out, errOut, &rotated, "Workspace", "Rotated", token.Name)
}

func deleteWorkspaceToken(cmd *cobra.Command, args []string, out io.Writer) error {
	tokenArg(args, &tokenID)
	target, org, err := workspacetoken.Scope(workspaceID)
	if err != nil {
		return err
	}
	ws := target.Workspace
	// A token named is found among every token with a role in the Workspace,
	// so that one of a broader scope is refused with what to use instead, not
	// reported as missing; a token picked is offered only among the ones this
	// can delete.
	pick := workspaceTokenPicker("Select the API token to delete from the Workspace:")
	token, err := workspacetoken.FindTokenIn(tokenID, name, ws, org, offeredWorkspaceTokens(workspacetoken.TokenTypeWORKSPACE), pick, astroV1Client)
	if err != nil {
		return err
	}
	if string(token.Scope) != string(workspacetoken.TokenTypeWORKSPACE) {
		return refuseDeleteOfBroader(cmd, &token, "Workspace")
	}
	if !forceDelete {
		if err := confirmTokenChange("API token deletion cannot be undone.",
			fmt.Sprintf("\nDelete API token %s?", ansi.Bold(token.Name))); err != nil {
			return err
		}
	}
	removal, err := workspacetoken.DeleteToken(token, ws, astroV1Client)
	if err != nil {
		return err
	}
	return renderTokenLine(workspaceTokenOutput, out, removal, fmt.Sprintf("Deleted Workspace API token %s", removal.Name))
}

// removeWorkspaceToken runs `workspace token remove`: it takes the Workspace
// role of a token of a broader scope, an Organization token, away. The
// Workspace's own tokens have no role to take, and are refused with delete.
func removeWorkspaceToken(cmd *cobra.Command, args []string, out io.Writer) error {
	tokenArg(args, &tokenID)
	target, org, err := workspacetoken.Scope(workspaceID)
	if err != nil {
		return err
	}
	ws := target.Workspace
	pick := workspaceTokenPicker("Select the Organization API token to remove from the Workspace:")
	token, err := workspacetoken.FindTokenIn(tokenID, name, ws, org, offeredWorkspaceTokens(workspacetoken.TokenTypeORGANIZATION), pick, astroV1Client)
	if err != nil {
		return err
	}
	if string(token.Scope) == string(workspacetoken.TokenTypeWORKSPACE) {
		return refuseRemoveOfOwn(cmd, &token, "Workspace")
	}
	// Taking the role off needs the Workspace, and the question below is not
	// asked on the way to saying it has none.
	if err := target.Require(); err != nil {
		return err
	}
	if !forceDelete {
		if err := confirmTokenChange("", fmt.Sprintf("\nRemove API token %s from the Workspace?", ansi.Bold(token.Name))); err != nil {
			return err
		}
	}
	removal, err := workspacetoken.RemoveToken(token, ws, astroV1Client)
	if err != nil {
		return err
	}
	return renderTokenLine(workspaceTokenOutput, out, removal, fmt.Sprintf("Removed Organization API token %s from the Workspace", removal.Name))
}

// addOrgTokenToWorkspace runs `workspace token add`, the older spelling of
// `workspace token organization-token add`, which picks the role from a table
// rather than asking for its name.
func addOrgTokenToWorkspace(cmd *cobra.Command, args []string, out io.Writer) error {
	ws, err := requiredWorkspace(cmd, workspaceID)
	if err != nil {
		return err
	}
	tokenArg(args, &orgTokenID)
	if tokenRole == "" {
		if err := mayPickRole(); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Select a Workspace Role for the Organization API token:")
		// no role was provided so ask the user for it
		var err error
		tokenRole, err = selectWorkspaceRole()
		if err != nil {
			return err
		}
	}
	pick := organizationTokenPicker("--name", "Select the Organization API token to add to the Workspace:")
	token, err := organization.AddOrgTokenToWorkspace(orgTokenID, name, tokenRole, ws, pick, astroV1Client)
	if err != nil {
		return err
	}
	return renderTokenLine(workspaceTokenOutput, out, token, fmt.Sprintf("Added Organization API token %s to the Workspace", token.Name))
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
	ws, err := requiredWorkspace(cmd, workspaceID)
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

	pick := workspaceTokenPicker("Select the Organization API token to update in the Workspace:")
	if operation == tokenRoleAdd {
		pick = organizationTokenPicker("--name", "Select the Organization API token to add to the Workspace:")
	}
	token, err := workspacetoken.UpsertOrgTokenWorkspaceRole(orgTokenID, name, tokenRole, ws, operation, pick, astroV1Client)
	if err != nil {
		return err
	}
	return renderTokenLine(workspaceTokenOutput, out, token, fmt.Sprintf("Set the Workspace role for Organization API token %s", token.Name))
}

func removeOrganizationTokenWorkspaceRole(args []string, out io.Writer) error {
	tokenArg(args, &orgTokenID)
	pick := workspaceTokenPicker("Select the Organization API token to remove from the Workspace:")
	removal, err := workspacetoken.RemoveOrgTokenWorkspaceRole(orgTokenID, name, workspaceID, pick, astroV1Client)
	if err != nil {
		return err
	}
	return renderTokenLine(workspaceTokenOutput, out, removal, fmt.Sprintf("Removed Organization API token %s from the Workspace", removal.Name))
}
