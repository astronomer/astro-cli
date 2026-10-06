package astro

// Running and rendering `astro deployment token`. The platform package returns
// what each command did; this file asks the questions (which token, are you
// sure) and decides how the answer looks, in text and in json.

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

// deploymentTokenOutput is the --output of the whole `deployment token`
// family, registered once on its group.
var deploymentTokenOutput string

// deploymentTokenUpdateRole is `deployment token update --role`. It is its own
// variable, not the tokenRole the other token commands share: each of them
// re-registers that one with its own default, so a default given here would
// be overwritten by whichever registered last. "" leaves the role alone.
var deploymentTokenUpdateRole string

var (
	errInvalidDeploymentTokenKey = errors.New("invalid Deployment API token selection")
	errCleanOutputWithJSON       = errors.New("--clean-output prints the bare token for a script, and --output json the whole token with it in a \"token\" field; use one")
)

// deploymentTokenList is what a token list publishes.
type deploymentTokenList struct {
	Tokens []deployment.TokenInfo `json:"tokens"`
}

// tokenFormat parses the family's --output. A command calls it before it asks
// anything, so a bad value fails as usage before a prompt. --clean-output
// (create and rotate) is a text format of its own, so it and json together
// are a usage error rather than one silently winning.
func tokenFormat() (cliout.Format, error) {
	format, err := cliout.ParseFormat(deploymentTokenOutput)
	if err != nil {
		return "", err
	}
	if format == cliout.FormatJSON && cleanTokenOutput {
		return "", cliout.Usage(errCleanOutputWithJSON)
	}
	return format, nil
}

func renderTokenList(format cliout.Format, out io.Writer, tokens []deployment.TokenInfo) error {
	return cliout.Renderer{Format: format, Out: out}.Emit(deploymentTokenList{Tokens: tokens}, func(w io.Writer) error {
		tab := &printutil.Table{
			DynamicPadding: true,
			Header:         []string{"ID", "NAME", "DESCRIPTION", "SCOPE", "DEPLOYMENT ROLE", "CREATED", "CREATED BY"},
		}
		for i := range tokens {
			t := &tokens[i]
			tab.AddRow([]string{t.ID, t.Name, t.Description, t.Scope, t.Role, deployment.TimeAgo(t.CreatedAt), t.CreatedBy}, false)
		}
		return tab.Print(w)
	})
}

// renderTokenSecret renders a token that carries its secret: the new one, or
// the rotated one. verb and name complete the text's first line; clean prints
// the bare secret instead, for a script.
func renderTokenSecret(format cliout.Format, out io.Writer, t *deployment.TokenInfo, verb, name string, clean bool) error {
	return cliout.Renderer{Format: format, Out: out}.Emit(t, func(w io.Writer) error {
		if clean {
			if t.Token != "" {
				fmt.Fprintln(w, t.Token)
			}
			return nil
		}
		fmt.Fprintf(w, "\nAstro Deployment API token %s was successfully %s\n", name, verb)
		fmt.Fprintln(w, "Copy and paste this API token for your records.")
		if t.Token != "" {
			fmt.Fprintln(w, "\n"+t.Token)
		}
		fmt.Fprintln(w, "\nYou will not be shown this API token value again.")
		return nil
	})
}

// renderTokenLine renders v as one line of text, or as itself in json.
func renderTokenLine(format cliout.Format, out io.Writer, v any, line string) error {
	return cliout.Renderer{Format: format, Out: out}.Emit(v, func(w io.Writer) error {
		_, err := fmt.Fprintln(w, line)
		return err
	})
}

// tokenPicker asks a person to choose a token from a numbered table. Under
// --output json it refuses before it writes anything.
func tokenPicker(out io.Writer) deployment.TokenPicker {
	return func(heading string, tokens []deployment.TokenInfo) (int, error) {
		about := input.About("an API token")
		answeredBy := input.AnsweredBy("the token ID or --name")
		if err := input.MayAsk("\n> ", about, answeredBy); err != nil {
			return 0, err
		}
		fmt.Fprintln(out, heading)
		tab := &printutil.Table{
			DynamicPadding: true,
			Header:         []string{"#", "ID", "NAME", "DESCRIPTION", "SCOPE", "DEPLOYMENT ROLE", "CREATED", "CREATED BY"},
		}
		for i := range tokens {
			t := &tokens[i]
			tab.AddRow([]string{strconv.Itoa(i + 1), t.ID, t.Name, t.Description, t.Scope, t.Role, deployment.TimeAgo(t.CreatedAt), t.CreatedBy}, false)
		}
		tab.Print(out) //nolint:errcheck // best-effort render to the terminal
		choice, err := input.Text("\n> ", about, answeredBy)
		if err != nil {
			return 0, err
		}
		n, err := strconv.Atoi(choice)
		if err != nil || n < 1 || n > len(tokens) || strconv.Itoa(n) != choice {
			return 0, errInvalidDeploymentTokenKey
		}
		return n - 1, nil
	}
}

// mayPickElsewhere refuses, under --output json, a command that names no token
// and would pick one through another family's picker (the workspace-token and
// organization packages'), which prints its heading before it checks whether
// it may ask. In text mode it does nothing.
func mayPickElsewhere(id, name, nameFlag string) error {
	if id != "" || name != "" {
		return nil
	}
	return input.MayAsk("\n> ", input.About("an API token"), input.AnsweredBy("the token ID or "+nameFlag))
}

// confirmTokenChange asks question, after warning when there is one. Under
// --output json it refuses, naming --yes, before it writes anything.
func confirmTokenChange(out io.Writer, warning, question string) (bool, error) {
	if err := input.MayAsk(question, input.AnsweredBy("--yes")); err != nil {
		return false, err
	}
	if warning != "" {
		fmt.Fprintln(out, warning)
	}
	return input.Confirm(question, input.AnsweredBy("--yes"))
}

func runDeploymentTokenList(format cliout.Format, out io.Writer, tokenTypes ...deployment.DeploymentTokenType) error {
	tokens, err := deployment.ListTokens(astroV1Client, deploymentID, tokenTypes)
	if err != nil {
		return err
	}
	return renderTokenList(format, out, tokens)
}

func runDeploymentTokenCreate(format cliout.Format, out io.Writer) error {
	created, err := deployment.CreateToken(tokenName, tokenDescription, tokenRole, deploymentID, tokenExpiration, astroV1Client)
	if err != nil {
		return err
	}
	return renderTokenSecret(format, out, &created, "created", tokenName, cleanTokenOutput)
}

func runDeploymentTokenUpdate(format cliout.Format, out io.Writer) error {
	res, err := deployment.UpdateToken(tokenID, name, tokenName, tokenDescription, deploymentTokenUpdateRole, deploymentID, tokenPicker(out), astroV1Client)
	if err != nil {
		return err
	}
	return renderTokenLine(format, out, res.Token, fmt.Sprintf("Astro Deployment API token %s was successfully updated", res.PreviousName))
}

func runDeploymentTokenRotate(format cliout.Format, out io.Writer) error {
	tokenTypes := []deployment.DeploymentTokenType{deployment.DeploymentTokenTypeDEPLOYMENT}
	token, err := deployment.FindToken(tokenID, name, deploymentID, tokenTypes, tokenPicker(out), astroV1Client)
	if err != nil {
		return err
	}
	if !forceRotate {
		ok, err := confirmTokenChange(out,
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
	return renderTokenSecret(format, out, &rotated, "rotated", token.Name, cleanTokenOutput)
}

func runDeploymentTokenDelete(format cliout.Format, out io.Writer) error {
	token, err := deployment.FindToken(tokenID, name, deploymentID, nil, tokenPicker(out), astroV1Client)
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
		ok, err := confirmTokenChange(out, warning, question)
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
	if removal.Action == deployment.TokenDeleted {
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
// or Organization token.
func runDeploymentTokenUpsert(format cliout.Format, out io.Writer, kind, operation string) error {
	var (
		token deployment.TokenInfo
		err   error
	)
	if operation == tokenRoleAdd {
		nameFlag, id := "--org-token-name", orgTokenID
		if kind == tokenKindWorkspace {
			nameFlag, id = "--workspace-token-name", workspaceTokenID
		}
		if err := mayPickElsewhere(id, orgTokenName, nameFlag); err != nil {
			return err
		}
	}
	if kind == tokenKindWorkspace {
		token, err = deployment.UpsertWorkspaceTokenDeploymentRole(workspaceTokenID, orgTokenName, tokenRole, workspaceID, deploymentID, operation, tokenPicker(out), astroV1Client)
	} else {
		token, err = deployment.UpsertOrgTokenDeploymentRole(orgTokenID, orgTokenName, tokenRole, deploymentID, operation, tokenPicker(out), astroV1Client)
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
		removal deployment.TokenRemoval
		err     error
	)
	if kind == tokenKindWorkspace {
		if err := mayPickElsewhere(workspaceTokenID, orgTokenName, "--workspace-token-name"); err != nil {
			return err
		}
		removal, err = deployment.RemoveWorkspaceTokenDeploymentRole(workspaceTokenID, orgTokenName, workspaceID, deploymentID, astroV1Client)
	} else {
		removal, err = deployment.RemoveOrgTokenDeploymentRole(orgTokenID, orgTokenName, deploymentID, tokenPicker(out), astroV1Client)
	}
	if err != nil {
		return err
	}
	return renderTokenLine(format, out, removal, fmt.Sprintf("Astro %s API token %s was successfully removed from the Deployment", kind, removal.Name))
}
