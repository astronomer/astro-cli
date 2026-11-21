package astro

// Running and rendering `astro deployment token`. The platform package returns
// what each command did; this file asks the questions (which token, are you
// sure) and decides how the answer looks, in text and in json, with the
// helpers the three token families share (api_token_render.go).

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/astrosession"
	"github.com/astronomer/astro-cli/internal/platform/astro/apitoken"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/internal/platform/astro/pagination"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/input"
)

// deploymentTokenOutput is the --output of the whole `deployment token`
// family, registered once on its group.
var deploymentTokenOutput cliout.Format

// deploymentTokenUpdateRole is `deployment token update --role`. It is its own
// variable, not the tokenRole the other token commands share: each of them
// re-registers that one with its own default, so a default given here would
// be overwritten by whichever registered last. "" leaves the role alone.
var deploymentTokenUpdateRole string

// tokenPicker picks among the Deployment's tokens, a choice the token ID or
// --name answers.
func tokenPicker() apitoken.Picker {
	return deploymentTokenPicker()
}

func runDeploymentTokenList(ctx context.Context, limit int, format cliout.Format, out, errOut io.Writer, tokenTypes ...deployment.DeploymentTokenType) error {
	tokens, err := deployment.ListTokens(ctx, astroV1Client, deploymentID, tokenTypes, limit)
	if err != nil {
		return err
	}
	return renderTokenList(format, out, errOut, tokens, deploymentRoleHeader)
}

func runDeploymentTokenCreate(format cliout.Format, out, errOut io.Writer) error {
	created, err := deployment.CreateToken(tokenName, tokenDescription, tokenRole, deploymentID, tokenExpiration, astroV1Client)
	if err != nil {
		return err
	}
	return renderTokenSecret(format, out, errOut, &created, "Deployment", "Created", tokenName)
}

func runDeploymentTokenUpdate(ctx context.Context, format cliout.Format, out, errOut io.Writer) error {
	res, err := deployment.UpdateToken(ctx, tokenID, name, tokenName, tokenDescription, deploymentTokenUpdateRole, deploymentID, nil, tokenPicker(), astroV1Client)
	if err != nil {
		return err
	}
	line := fmt.Sprintf("Updated Deployment API token %s", res.PreviousName)
	if res.Token.Scope != string(deployment.DeploymentTokenTypeDEPLOYMENT) {
		line = fmt.Sprintf("Updated %s API token %s in the Deployment", scopeNoun(astrov1.ApiTokenScope(res.Token.Scope)), res.PreviousName)
	}
	return renderTokenLine(format, out, errOut, res.Token, line)
}

func runDeploymentTokenRotate(ctx context.Context, format cliout.Format, out, errOut io.Writer) error {
	tokenTypes := []deployment.DeploymentTokenType{deployment.DeploymentTokenTypeDEPLOYMENT}
	token, err := deployment.FindToken(ctx, tokenID, name, deploymentID, tokenTypes, tokenPicker(), astroV1Client)
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
	if err := pagination.Interrupted(ctx); err != nil {
		return err
	}
	rotated, err := deployment.RotateToken(token, deploymentID, astroV1Client)
	if err != nil {
		return err
	}
	return renderTokenSecret(format, out, errOut, &rotated, "Deployment", "Rotated", token.Name)
}

// offeredDeploymentTokens is the scopes a delete or a remove looks among: the
// ones it can act on when it was named no token and has to ask, so the picker
// offers no row it would refuse, and none (every scope) when the token is
// named, so a wrong-scope one is refused with what to use instead.
func offeredDeploymentTokens(scopes ...deployment.DeploymentTokenType) []deployment.DeploymentTokenType {
	if tokenID == "" && name == "" {
		return scopes
	}
	return nil
}

func runDeploymentTokenDelete(cmd *cobra.Command, format cliout.Format, out, errOut io.Writer) error {
	// A token named is found among every token with a role on the Deployment,
	// so that one of a broader scope is refused with what to use instead, not
	// reported as missing; a token picked is offered only among the ones this
	// can delete.
	token, err := deployment.FindToken(cmd.Context(), tokenID, name, deploymentID, offeredDeploymentTokens(deployment.DeploymentTokenTypeDEPLOYMENT), tokenPicker(), astroV1Client)
	if err != nil {
		return err
	}
	if string(token.Scope) != string(deployment.DeploymentTokenTypeDEPLOYMENT) {
		return refuseDeleteOfBroader(cmd, &token, "Deployment")
	}
	if !forceDelete {
		if err := confirmTokenChange("API token deletion cannot be undone.",
			fmt.Sprintf("\nDelete API token %s?", ansi.Bold(token.Name))); err != nil {
			return err
		}
	}
	if err := pagination.Interrupted(cmd.Context()); err != nil {
		return err
	}
	removal, err := deployment.DeleteToken(token, deploymentID, astroV1Client)
	if err != nil {
		return err
	}
	return renderTokenLine(format, out, errOut, removal, fmt.Sprintf("Deleted Deployment API token %s", removal.Name))
}

// runDeploymentTokenRemoveRole runs `deployment token remove`: it takes the
// Deployment role of a token of a broader scope, a Workspace or an
// Organization token, away. The Deployment's own tokens have no role to take,
// and are refused with delete.
func runDeploymentTokenRemoveRole(cmd *cobra.Command, format cliout.Format, out, errOut io.Writer) error {
	token, err := deployment.FindToken(cmd.Context(), tokenID, name, deploymentID,
		offeredDeploymentTokens(deployment.DeploymentTokenTypeWORKSPACE, deployment.DeploymentTokenTypeORGANIZATION), tokenPicker(), astroV1Client)
	if err != nil {
		return err
	}
	if string(token.Scope) == string(deployment.DeploymentTokenTypeDEPLOYMENT) {
		return refuseRemoveOfOwn(cmd, &token, "Deployment")
	}
	if !forceDelete {
		if err := confirmTokenChange("", fmt.Sprintf("\nRemove API token %s from the Deployment?", ansi.Bold(token.Name))); err != nil {
			return err
		}
	}
	if err := pagination.Interrupted(cmd.Context()); err != nil {
		return err
	}
	removal, err := deployment.RemoveToken(token, deploymentID, astroV1Client)
	if err != nil {
		return err
	}
	return renderTokenLine(format, out, errOut, removal, fmt.Sprintf("Removed %s API token %s from the Deployment", scopeNoun(astrov1.ApiTokenScope(removal.Scope)), removal.Name))
}

// runDeploymentTokenAdd runs `deployment token add`: it gives a Workspace or
// an Organization token a role on the Deployment, which of the two it is being
// found out from the token (deployment.AddToken).
func runDeploymentTokenAdd(ctx context.Context, args []string, out, errOut io.Writer) error {
	id := ""
	if len(args) > 0 {
		id = strings.ToLower(args[0])
	}
	// A token given by name is looked for among the Organization's and the
	// Deployment's own Workspace's (tokenSearchWorkspace). A Deployment that
	// could not be read has no Workspace to look in, so the search is among
	// the Organization's alone, and says so when that finds nothing.
	ws, wsErr := "", error(nil)
	if id == "" {
		ws, wsErr = tokenSearchWorkspace(ctx, id)
		// A Workspace the run named itself is not quietly left out.
		if wsErr != nil && workspaceID != "" {
			return wsErr
		}
	}
	if tokenRole == "" {
		answer, err := input.Text("Enter a role for the API token (Possible values are DEPLOYMENT_ADMIN or a custom role name): ", input.AnsweredBy("--role"))
		if err != nil {
			return err
		}
		tokenRole = answer
	}
	pick := newTokenPicker(tokenHeader(deploymentRoleHeader), tokenRow, errInvalidDeploymentTokenKey, "--name", "Select the Workspace or Organization API token to add to the Deployment:")
	token, err := deployment.AddToken(ctx, id, name, tokenRole, ws, deploymentID, nil, pick, astroV1Client)
	if err != nil {
		if wsErr != nil && errors.Is(err, deployment.ErrTokenNotFound) {
			return fmt.Errorf("%w; no Workspace token was looked for: %w", err, wsErr)
		}
		return err
	}
	return renderTokenLine(deploymentTokenOutput, out, errOut, token, fmt.Sprintf("Added %s API token %s to the Deployment", scopeNoun(astrov1.ApiTokenScope(token.Scope)), token.Name))
}

// The kinds of token whose role on a Deployment the workspace-token and
// organization-token subcommands manage, as their text names them, and the two
// things they do to it, as the platform package names them.
const (
	tokenKindWorkspace    = "Workspace"
	tokenKindOrganization = "Organization"
	tokenRoleAdd          = "create"
	tokenRoleUpdate       = "update"
)

// setTokenDeploymentRole runs `deployment token {workspace,organization}-token
// {add,update}`: it gives a token of kind a role on the Deployment, asking
// for the role when --role named none.
func setTokenDeploymentRole(ctx context.Context, args []string, out, errOut io.Writer, kind, operation string) error {
	if deploymentID == "" {
		return errRequiredFlag("deployment", "astro deployment list")
	}
	// if an id was provided in the args we use it, lowercased
	if len(args) > 0 {
		if kind == tokenKindWorkspace {
			workspaceTokenID = strings.ToLower(args[0])
		} else {
			orgTokenID = strings.ToLower(args[0])
		}
	}
	// An add reads the Workspace its token is found in, so a run with none
	// fails before it asks for a role.
	ws := ""
	if kind == tokenKindWorkspace && operation == tokenRoleAdd {
		var err error
		if ws, err = tokenSearchWorkspace(ctx, workspaceTokenID); err != nil {
			return err
		}
	}
	if tokenRole == "" {
		prompt := "Enter a role for the API token (Possible values are DEPLOYMENT_ADMIN or a custom role name): "
		if operation == tokenRoleUpdate {
			prompt = "Enter a role for the new Deployment API token (Possible values are DEPLOYMENT_ADMIN or a custom role name): "
		}
		answer, err := input.Text(prompt, input.AnsweredBy("--role"))
		if err != nil {
			return err
		}
		tokenRole = answer
	}
	return runDeploymentTokenUpsert(ctx, deploymentTokenOutput, out, errOut, kind, operation, ws)
}

// runDeploymentTokenUpsert adds or updates the Deployment role of a Workspace
// or Organization token. An add picks among the Workspace's or the
// Organization's tokens, with that family's picker; an update among the
// Deployment's.
func runDeploymentTokenUpsert(ctx context.Context, format cliout.Format, out, errOut io.Writer, kind, operation, ws string) error {
	var (
		token apitoken.Token
		err   error
	)
	if kind == tokenKindWorkspace {
		// An update finds the token among the Deployment's, so only an add
		// reads a workspace.
		pick := deploymentTokenPicker()
		if operation == tokenRoleAdd {
			pick = workspaceTokenPicker("Select the Workspace API token to add to the Deployment:")
		}
		token, err = deployment.UpsertWorkspaceTokenDeploymentRole(ctx, workspaceTokenID, name, tokenRole, ws, deploymentID, operation, pick, astroV1Client)
	} else {
		pick := deploymentTokenPicker()
		if operation == tokenRoleAdd {
			pick = organizationTokenPicker("--name", "Select the Organization API token to add to the Deployment:")
		}
		token, err = deployment.UpsertOrgTokenDeploymentRole(ctx, orgTokenID, name, tokenRole, deploymentID, operation, pick, astroV1Client)
	}
	if err != nil {
		return err
	}
	return renderTokenLine(format, out, errOut, token, fmt.Sprintf("Set the Deployment role for %s API token %s", kind, token.Name))
}

// tokenSearchWorkspace is the Workspace an add looks for a Workspace token
// named by --name, or picked, in: the Deployment's own. A Workspace token can
// hold a role only on Deployments of its own Workspace, so no other Workspace
// has a token the add could use; the caller's current or the project's
// Workspace, which this once searched, are not the Deployment's. It is read
// from the Deployment by its ID.
//
// --workspace is the person's explicit override and wins, as it does
// everywhere; under an ASTRO_API_TOKEN a Workspace the token is not for is
// refused (astrosession.WorkspaceDisagreeError), like any other. A token given
// by ID (tokenID) is read wherever it is, so no Workspace is looked for and
// the Deployment is not read.
func tokenSearchWorkspace(ctx context.Context, tokenID string) (string, error) {
	if workspaceID != "" {
		if err := astrosession.CheckWorkspace(workspaceID, astrosession.SourceFlag); err != nil {
			return "", err
		}
		return workspaceID, nil
	}
	if tokenID != "" {
		return "", nil
	}
	d, err := deployment.GetDeploymentByID(ctx, "", deploymentID, astroV1Client)
	if err != nil {
		if pagination.IsInterrupt(err) {
			return "", err
		}
		return "", fmt.Errorf("could not read Deployment %s to find the Workspace its Workspace tokens are in; pass --workspace: %w", deploymentID, err)
	}
	return d.WorkspaceId, nil
}

// workspaceTokenWorkspace is the workspace an add or remove finds its
// Workspace token in: --workspace, else the project's, else the current one
// (coalesceWorkspace). A token given by id (the argument, id) is read
// wherever it is, so then none is needed, and a login with no workspace, such
// as an Organization token's, can still name one.
func workspaceTokenWorkspace(cmd *cobra.Command, id string) (string, error) {
	ws, err := coalesceWorkspaceOr(cmd, astrosession.ByID)
	if isWorkspaceRefusal(err) {
		// A Workspace named against the token is no better for a token given by id.
		return "", err
	}
	if err != nil && id == "" {
		if errors.Is(err, astrosession.ErrOrgTokenNoWorkspace) {
			return "", err
		}
		return "", fmt.Errorf("could not find a valid Workspace to look up the token in; pass --workspace or the token's ID: %w", err)
	}
	return ws, nil
}

// runDeploymentTokenRemove removes the Deployment role of a Workspace or
// Organization token.
func runDeploymentTokenRemove(cmd *cobra.Command, format cliout.Format, out, errOut io.Writer, kind string) error {
	var (
		removal apitoken.DeploymentRemoval
		err     error
	)
	if kind == tokenKindWorkspace {
		pick := workspaceTokenPicker("Select the Workspace API token to remove from the Deployment:")
		ws, wsErr := workspaceTokenWorkspace(cmd, workspaceTokenID)
		if wsErr != nil {
			return wsErr
		}
		removal, err = deployment.RemoveWorkspaceTokenDeploymentRole(cmd.Context(), workspaceTokenID, name, ws, deploymentID, pick, astroV1Client)
	} else {
		pick := deploymentTokenPicker()
		removal, err = deployment.RemoveOrgTokenDeploymentRole(cmd.Context(), orgTokenID, name, deploymentID, pick, astroV1Client)
	}
	if err != nil {
		return err
	}
	return renderTokenLine(format, out, errOut, removal, fmt.Sprintf("Removed %s API token %s from the Deployment", kind, removal.Name))
}
