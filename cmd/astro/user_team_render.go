package astro

// How the user and team commands of `astro workspace` and `astro organization`
// look, in text and in json. They publish the user and team objects their
// lists publish (user.UserInfo, team.TeamInfo), with the role on the object
// the command is about set, plus what a remove did, an invite and a team's
// members. The platform packages return those; this file is the only place
// deciding how any of it looks.

import (
	"bufio"
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/team"
	"github.com/astronomer/astro-cli/pkg/input"
)

// The -o of each group: `workspace user`, `workspace team`, `organization
// user`, and `organization team` with its `user` commands.
var (
	workspaceUserOutput    string
	workspaceTeamOutput    string
	organizationUserOutput string
	organizationTeamOutput string
)

// What answers each picker these commands can open, for the refusal under
// --output json to name.
const (
	userEmailAnswer = "the user's email as an argument"
	teamIDAnswer    = "the team ID as an argument"
	teamIDFlag      = "--team-id"
	userIDFlag      = "--user-id"
)

// mayPick refuses, under --output json, a command about to ask which what it
// means, naming answeredBy as what answers it, before anything is fetched or
// printed. In text mode it does nothing. The pickers (user.SelectUser and the
// team package's) check for themselves too, but cannot say what answers them,
// because that depends on the command.
func mayPick(what, answeredBy string) error {
	return input.MayAsk("\n> ", input.About(what), input.AnsweredBy(answeredBy))
}

// renderLines renders v as lines of text, or as itself in json.
func renderLines(format cliout.Format, out io.Writer, v any, lines ...string) error {
	return cliout.Renderer{Format: format, Out: out}.Emit(v, cliout.Text(func(b *bufio.Writer) {
		for _, l := range lines {
			fmt.Fprintln(b, l)
		}
	}))
}

// renderTeamUpdate renders what an organization team update did. upd is nil
// when the person declined to change an IdP-managed team, which prints
// nothing. With err, the rename went through and the role did not: text says
// so the way it always has, with the first line and then the error, while
// json publishes only the error.
func renderTeamUpdate(format cliout.Format, out io.Writer, upd *team.Update, err error) error {
	if upd == nil {
		return err
	}
	lines := []string{fmt.Sprintf("Astro Team %s was successfully updated", upd.PreviousName)}
	if err != nil {
		if format == cliout.FormatText {
			if werr := cliout.WriteText(out, func(b *bufio.Writer) { fmt.Fprintln(b, lines[0]) }); werr != nil {
				return werr
			}
		}
		return err
	}
	if upd.RoleChanged {
		lines = append(lines, fmt.Sprintf("Astro Team role %s was successfully updated to %s", upd.PreviousName, upd.Team.OrgRole))
	}
	return renderLines(format, out, &upd.Team, lines...)
}

// renderMembership renders a team user add or remove. m is nil when the
// person declined to change an IdP-managed team, which prints nothing. The
// text names the user by ID, and has always ended in a space.
func renderMembership(format cliout.Format, out io.Writer, m *team.Membership) error {
	if m == nil {
		return nil
	}
	verb := "added to"
	if m.Action == team.Removed {
		verb = "removed from"
	}
	return renderLines(format, out, m, fmt.Sprintf("Astro User %s was successfully %s team %s ", m.UserID, verb, m.TeamName))
}

// renderTeamMembers renders a team's members.
func renderTeamMembers(format cliout.Format, out io.Writer, list *team.MemberList) error {
	tab := &cliout.Table{Header: []string{"ID", "FullName", "Email"}, Empty: "The selected team has no members"}
	for _, m := range list.Members {
		tab.AddRow(m.ID, m.FullName, m.Email)
	}
	return cliout.Renderer{Format: format, Out: out}.Emit(list, cliout.Text(tab.Render))
}
