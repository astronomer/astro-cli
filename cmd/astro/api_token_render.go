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
	"strconv"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/apitoken"
	"github.com/astronomer/astro-cli/pkg/input"
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
	tab := tokenTable(tokens, roleHeader, false)
	return cliout.Renderer{Format: format, Out: out}.Emit(apitoken.List{Tokens: tokens}, cliout.Text(tab.Render))
}

// tokenTable lays tokens out for a list, or, numbered, for a picker.
func tokenTable(tokens []apitoken.Token, roleHeader string, numbered bool) *cliout.Table {
	header := []string{"ID", "NAME", "DESCRIPTION", "SCOPE", roleHeader, "CREATED", "CREATED BY"}
	if numbered {
		header = append([]string{"#"}, header...)
	}
	tab := &cliout.Table{Header: header}
	for i := range tokens {
		t := &tokens[i]
		row := []string{t.ID, t.Name, t.Description, t.Scope, t.Role, apitoken.TimeAgo(t.CreatedAt), t.CreatedBy}
		if numbered {
			row = append([]string{strconv.Itoa(i + 1)}, row...)
		}
		tab.AddRow(row...)
	}
	return tab
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

// newTokenPicker asks a person to choose a token from the numbered table
// table lays out, refusing an answer that is not one of its numbers with
// invalid. Under --output json it refuses before it writes anything, naming
// the token ID or nameFlag as what answers it (the token ID alone when
// nameFlag is "", for a command with no name flag).
func newTokenPicker(out io.Writer, table func([]apitoken.Token) *cliout.Table, invalid error, nameFlag string) apitoken.Picker {
	answers := "the token ID"
	if nameFlag != "" {
		answers += " or " + nameFlag
	}
	return func(heading string, tokens []apitoken.Token) (int, error) {
		about := input.About("an API token")
		answeredBy := input.AnsweredBy(answers)
		if err := input.MayAsk("\n> ", about, answeredBy); err != nil {
			return 0, err
		}
		tab := table(tokens)
		cliout.WriteText(out, func(b *bufio.Writer) { //nolint:errcheck // best-effort render to the terminal; the answer is read either way
			fmt.Fprintln(b, heading)
			tab.Render(b)
		})
		choice, err := input.Text("\n> ", about, answeredBy)
		if err != nil {
			return 0, err
		}
		n, err := strconv.Atoi(choice)
		if err != nil || n < 1 || n > len(tokens) || strconv.Itoa(n) != choice {
			return 0, invalid
		}
		return n - 1, nil
	}
}

var (
	errInvalidDeploymentTokenKey   = errors.New("invalid Deployment API token selection")
	errInvalidWorkspaceTokenKey    = errors.New("invalid Workspace API token selection")
	errInvalidOrganizationTokenKey = errors.New("invalid Organization API token selection")
)

// deploymentTokenPicker picks among a Deployment's tokens, by their role on it.
func deploymentTokenPicker(out io.Writer, nameFlag string) apitoken.Picker {
	return newTokenPicker(out, func(tokens []apitoken.Token) *cliout.Table {
		return tokenTable(tokens, deploymentRoleHeader, true)
	}, errInvalidDeploymentTokenKey, nameFlag)
}

// workspaceTokenPicker picks among a Workspace's tokens, by their role on it.
func workspaceTokenPicker(out io.Writer, nameFlag string) apitoken.Picker {
	return newTokenPicker(out, func(tokens []apitoken.Token) *cliout.Table {
		return tokenTable(tokens, workspaceRoleHeader, true)
	}, errInvalidWorkspaceTokenKey, nameFlag)
}

// organizationTokenPicker picks among the Organization's tokens. Its table
// has always been shorter than the others: no ID, scope or creator, and the
// lifetime the token was created with.
func organizationTokenPicker(out io.Writer, nameFlag string) apitoken.Picker {
	return newTokenPicker(out, func(tokens []apitoken.Token) *cliout.Table {
		tab := &cliout.Table{Header: []string{"#", "NAME", "DESCRIPTION", "ROLE", "EXPIRES"}}
		for i := range tokens {
			t := &tokens[i]
			expires := ""
			if t.ExpiryPeriodInDays != nil {
				expires = strconv.Itoa(*t.ExpiryPeriodInDays)
			}
			tab.AddRow(strconv.Itoa(i+1), t.Name, t.Description, t.Role, expires)
		}
		return tab
	}, errInvalidOrganizationTokenKey, nameFlag)
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

// mayPickRole refuses, under --output json, a command about to offer a role
// to pick, before it prints the line introducing the choice. In text mode it
// does nothing.
func mayPickRole() error {
	return input.MayAsk("\n> ", input.About("a role"), input.AnsweredBy("--role"))
}
