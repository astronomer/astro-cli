package astro

// Running and rendering `astro deployment token`. The platform package returns
// what each command did; this file asks the questions (which token, are you
// sure) and decides how the answer looks, in text and in json, with the
// helpers the three token families share (api_token_render.go).

import (
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

func runDeploymentTokenList(format cliout.Format, out io.Writer, tokenTypes ...deployment.DeploymentTokenType) error {
	tokens, err := deployment.ListTokens(astroV1Client, deploymentID, tokenTypes)
	if err != nil {
		return err
	}
	return renderTokenList(format, out, tokens, deploymentRoleHeader)
}

func runDeploymentTokenCreate(format cliout.Format, out io.Writer) error {
	created, err := deployment.CreateToken(tokenName, tokenDescription, tokenRole, deploymentID, tokenExpiration, astroV1Client)
	if err != nil {
		return err
	}
	return renderTokenSecret(format, out, &created, "Deployment", "created", tokenName, cleanTokenOutput)
}

func runDeploymentTokenUpdate(format cliout.Format, out io.Writer) error {
	res, err := deployment.UpdateToken(tokenID, name, tokenName, tokenDescription, deploymentTokenUpdateRole, deploymentID, nil, tokenPicker(), astroV1Client)
	if err != nil {
		return err
	}
	line := fmt.Sprintf("Astro Deployment API token %s was successfully updated", res.PreviousName)
	if res.Token.Scope != string(deployment.DeploymentTokenTypeDEPLOYMENT) {
		line = fmt.Sprintf("Astro %s API token %s was successfully updated in the Deployment", scopeNoun(astrov1.ApiTokenScope(res.Token.Scope)), res.PreviousName)
	}
	return renderTokenLine(format, out, res.Token, line)
}

func runDeploymentTokenRotate(format cliout.Format, out io.Writer) error {
	tokenTypes := []deployment.DeploymentTokenType{deployment.DeploymentTokenTypeDEPLOYMENT}
	token, err := deployment.FindToken(tokenID, name, deploymentID, tokenTypes, tokenPicker(), astroV1Client)
	if err != nil {
		return err
	}
	if !forceRotate {
		if err := confirmTokenChange(
			"API Token rotation will invalidate the current token and cannot be undone.",
			fmt.Sprintf("\nAre you sure you want to rotate the %s API token?", ansi.Bold(token.Name))); err != nil {
			return err
		}
	}
	rotated, err := deployment.RotateToken(token, deploymentID, astroV1Client)
	if err != nil {
		return err
	}
	return renderTokenSecret(format, out, &rotated, "Deployment", "rotated", token.Name, cleanTokenOutput)
}

func runDeploymentTokenDelete(cmd *cobra.Command, format cliout.Format, out io.Writer) error {
	// Found among every token with a role on the Deployment, so that a token
	// of a broader scope is refused with what to use instead, not reported as
	// missing.
	token, err := deployment.FindToken(tokenID, name, deploymentID, nil, tokenPicker(), astroV1Client)
	if err != nil {
		return err
	}
	if string(token.Scope) != string(deployment.DeploymentTokenTypeDEPLOYMENT) {
		return refuseDeleteOfBroader(cmd, &token, "Deployment")
	}
	if !forceDelete {
		if err := confirmTokenChange("API token deletion cannot be undone.",
			fmt.Sprintf("\nAre you sure you want to delete the %s API token?", ansi.Bold(token.Name))); err != nil {
			return err
		}
	}
	removal, err := deployment.DeleteToken(token, deploymentID, astroV1Client)
	if err != nil {
		return err
	}
	return renderTokenLine(format, out, removal, fmt.Sprintf("Astro Deployment API token %s was successfully deleted", removal.Name))
}

// runDeploymentTokenRemoveRole runs `deployment token remove`: it takes the
// Deployment role of a token of a broader scope, a Workspace or an
// Organization token, away. The Deployment's own tokens have no role to take,
// and are refused with delete.
func runDeploymentTokenRemoveRole(cmd *cobra.Command, format cliout.Format, out io.Writer) error {
	token, err := deployment.FindToken(tokenID, name, deploymentID, nil, tokenPicker(), astroV1Client)
	if err != nil {
		return err
	}
	if string(token.Scope) == string(deployment.DeploymentTokenTypeDEPLOYMENT) {
		return refuseRemoveOfOwn(cmd, &token, "Deployment")
	}
	if !forceDelete {
		if err := confirmTokenChange("", fmt.Sprintf("\nAre you sure you want to remove the %s API token from the Deployment?", ansi.Bold(token.Name))); err != nil {
			return err
		}
	}
	removal, err := deployment.RemoveToken(token, deploymentID, astroV1Client)
	if err != nil {
		return err
	}
	return renderTokenLine(format, out, removal, fmt.Sprintf("Astro %s API token %s was successfully removed from the Deployment", scopeNoun(astrov1.ApiTokenScope(removal.Scope)), removal.Name))
}

// runDeploymentTokenAdd runs `deployment token add`: it gives a Workspace or
// an Organization token a role on the Deployment, which of the two it is being
// found out from the token (deployment.AddToken).
func runDeploymentTokenAdd(cmd *cobra.Command, args []string, out io.Writer) error {
	if err := refuseCleanOutputWithJSON(deploymentTokenOutput); err != nil {
		return err
	}
	id := ""
	if len(args) > 0 {
		id = strings.ToLower(args[0])
	}
	// A token given by name is looked for among the Organization's and the
	// Workspace's. A run with no Workspace to look in (an Organization
	// token's) looks among the Organization's alone, and says so when that
	// finds nothing.
	ws, wsErr := "", error(nil)
	if id == "" {
		ws, wsErr = workspaceTokenWorkspace(cmd, id)
	}
	if tokenRole == "" {
		answer, err := input.Text("Enter a role for the API token (Possible values are DEPLOYMENT_ADMIN or a custom role name): ", input.AnsweredBy("--role"))
		if err != nil {
			return err
		}
		tokenRole = answer
	}
	cmd.SilenceUsage = true
	pick := newTokenPicker(tokenHeader(deploymentRoleHeader), tokenRow, errInvalidDeploymentTokenKey, "--name", "Please select the Workspace or Organization API token you would like to add to the Deployment:")
	token, err := deployment.AddToken(id, name, tokenRole, ws, deploymentID, nil, pick, astroV1Client)
	if err != nil {
		if wsErr != nil && errors.Is(err, deployment.ErrTokenNotFound) {
			return fmt.Errorf("%w; there was no Workspace to look for a Workspace token in (%w): pass --workspace, or the token's ID", err, wsErr)
		}
		return err
	}
	return renderTokenLine(deploymentTokenOutput, out, token, fmt.Sprintf("Astro %s API token %s was successfully added to the Deployment", scopeNoun(astrov1.ApiTokenScope(token.Scope)), token.Name))
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
func setTokenDeploymentRole(cmd *cobra.Command, args []string, out io.Writer, kind, operation string) error {
	if deploymentID == "" {
		return errRequiredFlag("deployment", "astro deployment list")
	}
	if err := refuseCleanOutputWithJSON(deploymentTokenOutput); err != nil {
		return err
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
		if ws, err = workspaceTokenWorkspace(cmd, workspaceTokenID); err != nil {
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
	cmd.SilenceUsage = true
	return runDeploymentTokenUpsert(deploymentTokenOutput, out, kind, operation, ws)
}

// runDeploymentTokenUpsert adds or updates the Deployment role of a Workspace
// or Organization token. An add picks among the Workspace's or the
// Organization's tokens, with that family's picker; an update among the
// Deployment's.
func runDeploymentTokenUpsert(format cliout.Format, out io.Writer, kind, operation, ws string) error {
	var (
		token apitoken.Token
		err   error
	)
	if kind == tokenKindWorkspace {
		// An update finds the token among the Deployment's, so only an add
		// reads a workspace.
		pick := deploymentTokenPicker()
		if operation == tokenRoleAdd {
			pick = workspaceTokenPicker("Please select the Workspace API token you would like to add to the Deployment:")
		}
		token, err = deployment.UpsertWorkspaceTokenDeploymentRole(workspaceTokenID, name, tokenRole, ws, deploymentID, operation, pick, astroV1Client)
	} else {
		pick := deploymentTokenPicker()
		if operation == tokenRoleAdd {
			pick = organizationTokenPicker("--name", "Please select the Organization API token you would like to add to the Deployment:")
		}
		token, err = deployment.UpsertOrgTokenDeploymentRole(orgTokenID, name, tokenRole, deploymentID, operation, pick, astroV1Client)
	}
	if err != nil {
		return err
	}
	return renderTokenLine(format, out, token, fmt.Sprintf("Astro %s API token %s was successfully added/updated to the Deployment", kind, token.Name))
}

// workspaceTokenWorkspace is the workspace an add or remove finds its
// Workspace token in: --workspace, else the project's, else the current one
// (coalesceWorkspace). A token given by id (the argument, id) is read
// wherever it is, so then none is needed, and a login with no workspace, such
// as an Organization token's, can still name one.
func workspaceTokenWorkspace(cmd *cobra.Command, id string) (string, error) {
	ws, err := coalesceWorkspaceOr(cmd, astrosession.ByID)
	if err != nil && id == "" {
		if errors.Is(err, astrosession.ErrOrgTokenNoWorkspace) {
			return "", err
		}
		return "", fmt.Errorf("failed to find a valid workspace to find the Workspace token in; pass --workspace, or the token's ID: %w", err)
	}
	return ws, nil
}

// runDeploymentTokenRemove removes the Deployment role of a Workspace or
// Organization token.
func runDeploymentTokenRemove(cmd *cobra.Command, format cliout.Format, out io.Writer, kind string) error {
	var (
		removal apitoken.DeploymentRemoval
		err     error
	)
	if kind == tokenKindWorkspace {
		pick := workspaceTokenPicker("Please select the Workspace API token you would like to remove from the Deployment:")
		ws, wsErr := workspaceTokenWorkspace(cmd, workspaceTokenID)
		if wsErr != nil {
			return wsErr
		}
		removal, err = deployment.RemoveWorkspaceTokenDeploymentRole(workspaceTokenID, name, ws, deploymentID, pick, astroV1Client)
	} else {
		pick := deploymentTokenPicker()
		removal, err = deployment.RemoveOrgTokenDeploymentRole(orgTokenID, name, deploymentID, pick, astroV1Client)
	}
	if err != nil {
		return err
	}
	return renderTokenLine(format, out, removal, fmt.Sprintf("Astro %s API token %s was successfully removed from the Deployment", kind, removal.Name))
}
