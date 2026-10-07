package astro

// Running and rendering `astro deployment token`. The platform package returns
// what each command did; this file asks the questions (which token, are you
// sure) and decides how the answer looks, in text and in json, with the
// helpers the three token families share (api_token_render.go).

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/apitoken"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/input"
)

// deploymentTokenOutput is the --output of the whole `deployment token`
// family, registered once on its group.
var deploymentTokenOutput string

// deploymentTokenUpdateRole is `deployment token update --role`. It is its own
// variable, not the tokenRole the other token commands share: each of them
// re-registers that one with its own default, so a default given here would
// be overwritten by whichever registered last. "" leaves the role alone.
var deploymentTokenUpdateRole string

// tokenFormat parses the deployment token family's --output.
func tokenFormat() (cliout.Format, error) {
	return tokenFormatOf(deploymentTokenOutput)
}

// tokenPicker picks among the Deployment's tokens, a choice the token ID or
// --name answers.
func tokenPicker() apitoken.Picker {
	return deploymentTokenPicker("--name")
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
	res, err := deployment.UpdateToken(tokenID, name, tokenName, tokenDescription, deploymentTokenUpdateRole, deploymentID, tokenPicker(), astroV1Client)
	if err != nil {
		return err
	}
	return renderTokenLine(format, out, res.Token, fmt.Sprintf("Astro Deployment API token %s was successfully updated", res.PreviousName))
}

func runDeploymentTokenRotate(format cliout.Format, out io.Writer) error {
	tokenTypes := []deployment.DeploymentTokenType{deployment.DeploymentTokenTypeDEPLOYMENT}
	token, err := deployment.FindToken(tokenID, name, deploymentID, tokenTypes, tokenPicker(), astroV1Client)
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
	rotated, err := deployment.RotateToken(token, deploymentID, astroV1Client)
	if err != nil {
		return err
	}
	return renderTokenSecret(format, out, &rotated, "Deployment", "rotated", token.Name, cleanTokenOutput)
}

func runDeploymentTokenDelete(format cliout.Format, out io.Writer) error {
	token, err := deployment.FindToken(tokenID, name, deploymentID, nil, tokenPicker(), astroV1Client)
	if err != nil {
		return err
	}
	isDep := string(token.Scope) == string(deployment.DeploymentTokenTypeDEPLOYMENT)
	if !forceDelete {
		warning, question, canceled := "", fmt.Sprintf("\nAre you sure you want to remove the %s API token from the Deployment?", ansi.Bold(token.Name)), "Canceling API Token removal"
		if isDep {
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
	removal, err := deployment.DeleteToken(token, deploymentID, astroV1Client)
	if err != nil {
		return err
	}
	line := fmt.Sprintf("Astro API token %s was successfully removed from the Deployment", removal.Name)
	if removal.Action == apitoken.Deleted {
		line = fmt.Sprintf("Astro Deployment API token %s was successfully deleted", removal.Name)
	}
	return renderTokenLine(format, out, removal, line)
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
	format, err := tokenFormat()
	if err != nil {
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
	return runDeploymentTokenUpsert(format, out, kind, operation)
}

// runDeploymentTokenUpsert adds or updates the Deployment role of a Workspace
// or Organization token. An add picks among the Workspace's or the
// Organization's tokens, with that family's picker; an update among the
// Deployment's.
func runDeploymentTokenUpsert(format cliout.Format, out io.Writer, kind, operation string) error {
	var (
		token apitoken.Token
		err   error
	)
	if kind == tokenKindWorkspace {
		pick := deploymentTokenPicker("--workspace-token-name")
		if operation == tokenRoleAdd {
			pick = workspaceTokenPicker("--workspace-token-name", "Please select the Workspace API token you would like to add to the Deployment:")
		}
		token, err = deployment.UpsertWorkspaceTokenDeploymentRole(workspaceTokenID, orgTokenName, tokenRole, workspaceID, deploymentID, operation, pick, astroV1Client)
	} else {
		pick := deploymentTokenPicker("--org-token-name")
		if operation == tokenRoleAdd {
			pick = organizationTokenPicker("--org-token-name", "Please select the Organization API token you would like to add to the Deployment:")
		}
		token, err = deployment.UpsertOrgTokenDeploymentRole(orgTokenID, orgTokenName, tokenRole, deploymentID, operation, pick, astroV1Client)
	}
	if err != nil {
		return err
	}
	return renderTokenLine(format, out, token, fmt.Sprintf("Astro %s API token %s was successfully added/updated to the Deployment", kind, token.Name))
}

// runDeploymentTokenRemove removes the Deployment role of a Workspace or
// Organization token.
func runDeploymentTokenRemove(format cliout.Format, out io.Writer, kind string) error {
	var (
		removal apitoken.DeploymentRemoval
		err     error
	)
	if kind == tokenKindWorkspace {
		pick := workspaceTokenPicker("--workspace-token-name", "Please select the Workspace API token you would like to remove from the Deployment:")
		removal, err = deployment.RemoveWorkspaceTokenDeploymentRole(workspaceTokenID, orgTokenName, workspaceID, deploymentID, pick, astroV1Client)
	} else {
		pick := deploymentTokenPicker("--org-token-name")
		removal, err = deployment.RemoveOrgTokenDeploymentRole(orgTokenID, orgTokenName, deploymentID, pick, astroV1Client)
	}
	if err != nil {
		return err
	}
	return renderTokenLine(format, out, removal, fmt.Sprintf("Astro %s API token %s was successfully removed from the Deployment", kind, removal.Name))
}
