package astro

// How the three API token families (`deployment token`, `workspace token`,
// `organization token`) look, in text and in json, and how they ask which
// token and whether to go on. They publish the same apitoken shapes; what
// differs between them is the object a token's role is on, which names a
// column, and the word in their messages.

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/apitoken"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/picker"
)

// The role column of each family's tables.
const (
	deploymentRoleHeader   = "DEPLOYMENT ROLE"
	workspaceRoleHeader    = "WORKSPACE ROLE"
	organizationRoleHeader = "ORGANIZATION ROLE"
)

// renderTokenList renders a token list, its role column headed roleHeader.
func renderTokenList(format cliout.Format, out, errOut io.Writer, tokens []apitoken.Token, roleHeader string) error {
	tab := &cliout.Table{Header: tokenHeader(roleHeader)}
	// A person reads "3 days ago"; a script gets the moment, as the other
	// tables print one.
	plain := !cliout.IsTerminal(out)
	for i := range tokens {
		created := apitoken.TimeAgo(tokens[i].CreatedAt)
		if plain {
			created = createdAtText(tokens[i].CreatedAt)
		}
		tab.AddRow(tokenRowCreated(&tokens[i], created)...)
	}
	return cliout.Renderer{Format: format, Out: out, Err: errOut}.Emit(apitoken.List{Tokens: tokens}, cliout.TableText(tab))
}

// tokenHeader and tokenRow lay a token out, for a list and for a picker.
// KIND is STANDARD or DIRECT_ACCESS, as the API names it: "type" would read
// as the scope. It is the last column, added after the others, so a script
// that reads the table by position finds every earlier column where it was.
func tokenHeader(roleHeader string) []string {
	return []string{"ID", "NAME", "DESCRIPTION", "SCOPE", roleHeader, "CREATED", "CREATED BY", "KIND"}
}

// createdAtText is a creation time as a script reads it: RFC 3339, in UTC.
func createdAtText(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func tokenRow(t *apitoken.Token) []string {
	return tokenRowCreated(t, apitoken.TimeAgo(t.CreatedAt))
}

// tokenRowCreated is tokenRow with the CREATED cell given: "3 days ago" for a
// person, a timestamp for a script.
func tokenRowCreated(t *apitoken.Token, created string) []string {
	return []string{t.ID, t.Name, t.Description, t.Scope, t.Role, created, t.CreatedBy, apitoken.KindText(t)}
}

// renderTokenSecret renders a token that carries its secret: the new one, or
// the rotated one. kind (Deployment, Workspace, Organization), verb and name
// complete the receipt's first line.
//
// In text, stdout is the secret and a newline, so `$(astro ... create)` and a
// pipe get exactly the value; the receipt and the warning that the value is
// not shown again are for the person, on stderr.
func renderTokenSecret(format cliout.Format, out, errOut io.Writer, t *apitoken.Token, kind, verb, name string) error {
	if err := (cliout.Renderer{Format: format, Out: out, Err: errOut}).Emit(t, cliout.Text(func(b *bufio.Writer) {
		if t.Token != "" {
			fmt.Fprintln(b, t.Token)
		}
	})); err != nil {
		return err
	}
	// The receipt follows the secret, and only when the secret was written.
	if format != cliout.FormatJSON {
		fmt.Fprintf(errOut, "\n%s %s API token %s\n", verb, kind, input.Plain(name))
		fmt.Fprintln(errOut, "Copy this API token for your records.")
		fmt.Fprintln(errOut, "This API token value is not shown again.")
	}
	return nil
}

// renderTokenLine renders v as one line of text, or as itself in json.
func renderTokenLine(format cliout.Format, out io.Writer, v any, line string) error {
	return cliout.Renderer{Format: format, Out: out}.Emit(v, cliout.Text(func(b *bufio.Writer) {
		fmt.Fprintln(b, line)
	}))
}

// newTokenPicker asks a person to choose a token, on stderr like every
// question, from a numbered table of
// the columns header names, each token's cells laid out by row, refusing an
// answer that is not one of its numbers with invalid. Its heading is
// question, which says what the command will do with the token, unless the
// platform gave one of its own (a name several tokens share). Under --output
// json it refuses before it writes anything, naming the token ID or nameFlag
// as what answers it (the token ID alone when nameFlag is "", for a command
// with no name flag).
func newTokenPicker(header []string, row func(*apitoken.Token) []string, invalid error, nameFlag, question string) apitoken.Picker {
	answers := "the token ID"
	if nameFlag != "" {
		answers += " or " + nameFlag
	}
	return func(heading string, tokens []apitoken.Token) (int, error) {
		if heading == "" {
			heading = "\n" + question
		}
		list := picker.List{
			Title:   heading,
			Header:  header,
			Ask:     []input.Option{input.About("an API token"), input.AnsweredBy(answers)},
			Invalid: invalid,
		}
		for i := range tokens {
			list.AddRow(false, row(&tokens[i])...)
		}
		return list.Pick(os.Stderr, os.Stdin)
	}
}

var (
	errInvalidDeploymentTokenKey   = errors.New("invalid Deployment API token selection")
	errInvalidWorkspaceTokenKey    = errors.New("invalid Workspace API token selection")
	errInvalidOrganizationTokenKey = errors.New("invalid Organization API token selection")
)

// deploymentTokenPicker picks among a Deployment's tokens, by their role on
// it. Its question names no action; it has always been the same for every
// command that asks it.
func deploymentTokenPicker() apitoken.Picker {
	return newTokenPicker(tokenHeader(deploymentRoleHeader), tokenRow, errInvalidDeploymentTokenKey, "--name", "Select the Deployment API token:")
}

// workspaceTokenPicker picks among a Workspace's tokens, by their role on it,
// asking question.
func workspaceTokenPicker(question string) apitoken.Picker {
	return newTokenPicker(tokenHeader(workspaceRoleHeader), tokenRow, errInvalidWorkspaceTokenKey, "--name", question)
}

// organizationTokenPicker picks among the Organization's tokens, asking
// question. Its table has always been shorter than the others: no ID, scope
// or creator, and the lifetime the token was created with. It shows the kind
// as they do: rotating or deleting is where telling a DIRECT_ACCESS token
// from a STANDARD one matters.
func organizationTokenPicker(nameFlag, question string) apitoken.Picker {
	return newTokenPicker([]string{"NAME", "DESCRIPTION", "ROLE", "EXPIRES", "KIND"}, func(t *apitoken.Token) []string {
		expires := ""
		if t.ExpiryPeriodInDays != nil {
			expires = strconv.Itoa(*t.ExpiryPeriodInDays)
		}
		return []string{t.Name, t.Description, t.Role, expires, apitoken.KindText(t)}
	}, errInvalidOrganizationTokenKey, nameFlag, question)
}

// confirmTokenChange asks question, after warning when there is one. Under
// --output json it refuses, naming --yes, before it writes anything. Answered
// no, it is input.ErrAborted.
func confirmTokenChange(warning, question string) error {
	if err := input.MayAsk(question, input.AnsweredBy("--yes")); err != nil {
		return err
	}
	if warning != "" {
		fmt.Fprintln(os.Stderr, cliout.DangerLabel(os.Stderr), warning)
	}
	return input.ConfirmOrAbort(question, input.AnsweredBy("--yes"))
}

// mayPickRole refuses, under --output json, a command about to offer a role
// to pick, before it prints the line introducing the choice. In text mode it
// does nothing.
func mayPickRole() error {
	return input.MayAsk("\n> ", input.About("a role"), input.AnsweredBy("--role"))
}

// scopeNoun names the scope of a token as the messages do.
func scopeNoun(scope astrov1.ApiTokenScope) string {
	switch scope {
	case astrov1.ApiTokenScopeWORKSPACE:
		return "Workspace"
	case astrov1.ApiTokenScopeORGANIZATION:
		return "Organization"
	case astrov1.ApiTokenScopeDEPLOYMENT:
		return "Deployment"
	}
	return string(scope)
}

// groupPath is the path of the command group cmd is a verb of
// (astro workspace token), which the other verb is under.
func groupPath(cmd *cobra.Command) string {
	if cmd.HasParent() {
		return cmd.Parent().CommandPath()
	}
	return ""
}

// refuseDeleteOfBroader is the refusal of a delete that names a token of a
// broader scope than the one the command is about (a Workspace's delete given
// an Organization token): the token is not the scope's to delete, and what
// the command can do with it is take its role away, which remove does. It is
// a usage error: the verb does not fit the token.
func refuseDeleteOfBroader(deleteCmd *cobra.Command, token *astrov1.ApiToken, own string) error {
	return cliout.UsageNoHelp(fmt.Errorf("%s is %s API token, not one of this %s's own. %s deletes a %s's own tokens; use %s remove to take the token's role off the %s",
		token.Name, article(scopeNoun(token.Scope)), own, deleteCmd.CommandPath(), own, groupPath(deleteCmd), own))
}

// refuseRemoveOfOwn is the refusal of a remove that names the scope's own
// token: it has no role on its own scope to take away, and delete is what ends
// it. A usage error, like refuseDeleteOfBroader.
func refuseRemoveOfOwn(removeCmd *cobra.Command, token *astrov1.ApiToken, own string) error {
	return cliout.UsageNoHelp(fmt.Errorf("%s is this %s's own API token. %s takes a broader token's role off the %s; use %s delete to delete it",
		token.Name, own, removeCmd.CommandPath(), own, groupPath(removeCmd)))
}

// deprecatedTokenGroup hides group, an old subgroup of a token command whose
// verbs moved up to the token command, and marks each verb it keeps for
// scripts as a deprecated spelling of the same verb one level up. kind is the
// scope of the tokens the group handled (Workspace, Organization), which the
// warning of its list verb names; it is given, not read from the group's name,
// so that renaming a group cannot quietly change it.
func deprecatedTokenGroup(group *cobra.Command, kind string) *cobra.Command {
	group.Hidden = true
	for _, verb := range group.Commands() {
		// Two of the new verbs are not quite the old ones, and the warning
		// says how: the flat list shows every scope, where the old one showed
		// one kind, and the flat remove asks to confirm, where the old one
		// did not, which a run with no terminal fails on without --yes.
		suffix := ""
		switch verb.Name() {
		case "list":
			suffix = ", which lists the tokens of every scope, not only " + kind + " tokens: tell them apart by the scope field of -o json"
		case "remove":
			suffix = " --yes (without --yes, remove asks for confirmation, which this spelling never did)"
		}
		cliout.DeprecateCommand(verb, func(c *cobra.Command) string {
			return c.Parent().Parent().CommandPath() + " " + c.Name()
		}, suffix)
	}
	return group
}
