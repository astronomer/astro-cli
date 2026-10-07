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

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/apitoken"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/picker"
)

var errCleanOutputWithJSON = errors.New("--clean-output prints the bare token for a script, and --output json the whole token with it in a \"token\" field; use one")

// The role column of each family's tables.
const (
	deploymentRoleHeader   = "DEPLOYMENT ROLE"
	workspaceRoleHeader    = "WORKSPACE ROLE"
	organizationRoleHeader = "ORGANIZATION ROLE"
)

// tokenFormatOf parses a token family's --output. A command calls it before
// it asks anything, so a bad value fails as usage before a prompt.
// --clean-output (create and rotate) is a text format of its own, so it and
// json together are a usage error rather than one silently winning.
func tokenFormatOf(output string) (cliout.Format, error) {
	format, err := cliout.ParseFormat(output)
	if err != nil {
		return "", err
	}
	if format == cliout.FormatJSON && cleanTokenOutput {
		return "", cliout.Usage(errCleanOutputWithJSON)
	}
	return format, nil
}

// renderTokenList renders a token list, its role column headed roleHeader.
func renderTokenList(format cliout.Format, out io.Writer, tokens []apitoken.Token, roleHeader string) error {
	tab := &cliout.Table{Header: tokenHeader(roleHeader)}
	for i := range tokens {
		tab.AddRow(tokenRow(&tokens[i])...)
	}
	return cliout.Renderer{Format: format, Out: out}.Emit(apitoken.List{Tokens: tokens}, cliout.Text(tab.Render))
}

// tokenHeader and tokenRow lay a token out, for a list and for a picker.
func tokenHeader(roleHeader string) []string {
	return []string{"ID", "NAME", "DESCRIPTION", "SCOPE", roleHeader, "CREATED", "CREATED BY"}
}

func tokenRow(t *apitoken.Token) []string {
	return []string{t.ID, t.Name, t.Description, t.Scope, t.Role, apitoken.TimeAgo(t.CreatedAt), t.CreatedBy}
}

// renderTokenSecret renders a token that carries its secret: the new one, or
// the rotated one. kind (Deployment, Workspace, Organization), verb and name
// complete the text's first line; clean prints the bare secret instead, for a
// script.
func renderTokenSecret(format cliout.Format, out io.Writer, t *apitoken.Token, kind, verb, name string, clean bool) error {
	return cliout.Renderer{Format: format, Out: out}.Emit(t, cliout.Text(func(b *bufio.Writer) {
		if clean {
			if t.Token != "" {
				fmt.Fprintln(b, t.Token)
			}
			return
		}
		fmt.Fprintf(b, "\nAstro %s API token %s was successfully %s\n", kind, name, verb)
		fmt.Fprintln(b, "Copy and paste this API token for your records.")
		if t.Token != "" {
			fmt.Fprintln(b, "\n"+t.Token)
		}
		fmt.Fprintln(b, "\nYou will not be shown this API token value again.")
	}))
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
func deploymentTokenPicker(nameFlag string) apitoken.Picker {
	return newTokenPicker(tokenHeader(deploymentRoleHeader), tokenRow, errInvalidDeploymentTokenKey, nameFlag, "Please select the Deployment API token:")
}

// workspaceTokenPicker picks among a Workspace's tokens, by their role on it,
// asking question.
func workspaceTokenPicker(nameFlag, question string) apitoken.Picker {
	return newTokenPicker(tokenHeader(workspaceRoleHeader), tokenRow, errInvalidWorkspaceTokenKey, nameFlag, question)
}

// organizationTokenPicker picks among the Organization's tokens, asking
// question. Its table has always been shorter than the others: no ID, scope
// or creator, and the lifetime the token was created with.
func organizationTokenPicker(nameFlag, question string) apitoken.Picker {
	return newTokenPicker([]string{"NAME", "DESCRIPTION", "ROLE", "EXPIRES"}, func(t *apitoken.Token) []string {
		expires := ""
		if t.ExpiryPeriodInDays != nil {
			expires = strconv.Itoa(*t.ExpiryPeriodInDays)
		}
		return []string{t.Name, t.Description, t.Role, expires}
	}, errInvalidOrganizationTokenKey, nameFlag, question)
}

// confirmTokenChange asks question, after warning when there is one. Under
// --output json it refuses, naming --yes, before it writes anything.
func confirmTokenChange(warning, question string) (bool, error) {
	if err := input.MayAsk(question, input.AnsweredBy("--yes")); err != nil {
		return false, err
	}
	if warning != "" {
		fmt.Fprintln(os.Stderr, warning)
	}
	return input.Confirm(question, input.AnsweredBy("--yes"))
}

// mayPickRole refuses, under --output json, a command about to offer a role
// to pick, before it prints the line introducing the choice. In text mode it
// does nothing.
func mayPickRole() error {
	return input.MayAsk("\n> ", input.About("a role"), input.AnsweredBy("--role"))
}
