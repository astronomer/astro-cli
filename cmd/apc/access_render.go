package apc

import (
	"bufio"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/apc/deployment"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	sa "github.com/astronomer/astro-cli/internal/platform/apc/service_account"
	"github.com/astronomer/astro-cli/internal/platform/apc/teams"
	"github.com/astronomer/astro-cli/internal/platform/apc/user"
	"github.com/astronomer/astro-cli/internal/platform/apc/workspace"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

// The shapes the APC access commands publish under --output json, and the
// text each has always printed, drawn from the same values: `deployment
// service-account|team|user`, `workspace service-account|team|user`, `team`
// and `user create`. The platform functions return what Houston said; this is
// the one place that decides how it looks.
//
// A field that Houston may give no value for is null then, never "": either
// the command's query does not ask for it, or the object has none. Houston
// returns both as no value, so they cannot be told apart.

// accessOutput is the --output of every access command. One process runs one
// command, and registering the flag resets it, so they can share it.
var accessOutput string

// accessRenderer parses -o, before anything else so a bad value is a usage
// error, and returns the Renderer the command publishes through.
func accessRenderer(out io.Writer) (cliout.Renderer, error) {
	format, err := cliout.ParseFormat(accessOutput)
	if err != nil {
		return cliout.Renderer{}, err
	}
	return cliout.Renderer{Format: format, Out: out}, nil
}

func addAccessOutputFlag(cmd *cobra.Command) {
	cliout.AddOutputFlag(cmd, &accessOutput)
}

// orNull is s when Houston gave a value, and nil when it gave none.
func orNull(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Removal actions.
const (
	accessActionDeleted = "deleted"
	accessActionRemoved = "removed"
)

// serviceAccountJSON is a Workspace or Deployment service account.
type serviceAccountJSON struct {
	ID    string  `json:"id"`
	Label *string `json:"label"`
	// Category is a free-form grouping the account was created with.
	Category   *string `json:"category"`
	Active     bool    `json:"active"`
	CreatedAt  *string `json:"created_at"`
	LastUsedAt *string `json:"last_used_at"`
	// APIKey is the account's credential, published by a create only: it is
	// the result a script runs the command for, the one moment it is meant
	// to be read. A list publishes null. Houston returns the key whole for
	// ten minutes after the create and masked after that (houston-api
	// ), and a list run in
	// CI within those ten minutes would put it in the job's log. The text
	// table shows what Houston returns, as it always has. It is never
	// written to an error or a log.
	APIKey *string `json:"api_key"`
}

type serviceAccountListJSON struct {
	ServiceAccounts []serviceAccountJSON `json:"service_accounts"`
}

// deploymentServiceAccountRemovalJSON is what `astro deployment
// service-account delete` deleted.
type deploymentServiceAccountRemovalJSON struct {
	ID           string  `json:"id"`
	Label        *string `json:"label"`
	DeploymentID string  `json:"deployment_id"`
	Action       string  `json:"action"`
}

// workspaceServiceAccountRemovalJSON is what `astro workspace
// service-account delete` deleted.
type workspaceServiceAccountRemovalJSON struct {
	ID          string  `json:"id"`
	Label       *string `json:"label"`
	WorkspaceID string  `json:"workspace_id"`
	Action      string  `json:"action"`
}

// newServiceAccountJSON is s without its key: what a list publishes.
func newServiceAccountJSON(s *sa.ServiceAccount) serviceAccountJSON {
	return serviceAccountJSON{
		ID: s.ID, Label: orNull(s.Label), Category: orNull(s.Category), Active: s.Active,
		CreatedAt: orNull(s.CreatedAt), LastUsedAt: orNull(s.LastUsedAt),
	}
}

func serviceAccountTable() *printutil.Table {
	return &printutil.Table{
		Padding:        []int{40, 40, 50, 50},
		DynamicPadding: true,
		Header:         []string{"NAME", "CATEGORY", "ID", "APIKEY"},
	}
}

func renderServiceAccountCreated(r cliout.Renderer, s *sa.ServiceAccount) error {
	out := newServiceAccountJSON(s)
	out.APIKey = orNull(s.APIKey)
	return r.Emit(out, func(w io.Writer) error {
		tab := serviceAccountTable()
		tab.AddRow([]string{s.Label, s.Category, s.ID, s.APIKey}, false)
		tab.SuccessMsg = "\n Service account successfully created."
		return tab.Print(w)
	})
}

func renderServiceAccountList(r cliout.Renderer, sas []sa.ServiceAccount) error {
	out := serviceAccountListJSON{ServiceAccounts: make([]serviceAccountJSON, 0, len(sas))}
	for i := range sas {
		out.ServiceAccounts = append(out.ServiceAccounts, newServiceAccountJSON(&sas[i]))
	}
	return r.Emit(out, func(w io.Writer) error {
		tab := serviceAccountTable()
		for i := range sas {
			tab.AddRow([]string{sas[i].Label, sas[i].Category, sas[i].ID, sas[i].APIKey}, false)
		}
		return tab.Print(w)
	})
}

func serviceAccountDeletedText(s *sa.ServiceAccount) func(io.Writer) error {
	return cliout.Text(func(b *bufio.Writer) {
		fmt.Fprintf(b, "Service Account %s (%s) successfully deleted\n", s.Label, s.ID)
	})
}

// userJSON is an APC user and the role they hold on the Workspace or
// Deployment the command is about; the other role is null. ID is null when
// Houston's answer did not name the user, full_name when it did not give it.
type userJSON struct {
	ID *string `json:"id"`
	// Username is the user's APC username, which is the email they signed
	// up with.
	Username       *string `json:"username"`
	FullName       *string `json:"full_name"`
	WorkspaceRole  *string `json:"workspace_role"`
	DeploymentRole *string `json:"deployment_role"`
}

type userListJSON struct {
	Users []userJSON `json:"users"`
}

// workspaceUserRemovalJSON names the user `astro workspace user remove` took
// off the Workspace.
type workspaceUserRemovalJSON struct {
	ID          string  `json:"id"`
	Username    *string `json:"username"`
	WorkspaceID string  `json:"workspace_id"`
	Action      string  `json:"action"`
}

// deploymentUserRemovalJSON names the user `astro deployment user remove`
// took the Deployment role from, and the role they held. The user stays in
// the Workspace.
type deploymentUserRemovalJSON struct {
	ID           *string `json:"id"`
	Username     *string `json:"username"`
	DeploymentID string  `json:"deployment_id"`
	Role         *string `json:"role"`
	Action       string  `json:"action"`
}

func newDeploymentUserJSON(u *deployment.UserRole) userJSON {
	return userJSON{ID: orNull(u.ID), Username: orNull(u.Username), FullName: orNull(u.FullName), DeploymentRole: orNull(u.Role)}
}

func newWorkspaceUserJSON(u *workspace.UserRole) userJSON {
	return userJSON{ID: orNull(u.ID), Username: orNull(u.Username), FullName: orNull(u.FullName), WorkspaceRole: orNull(u.Role)}
}

// deploymentUserTable is the table a Deployment user add, update and
// remove print.
func deploymentUserTable() *printutil.Table {
	return &printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"DEPLOYMENT ID", "USER", "ROLE"},
	}
}

func renderDeploymentUserList(r cliout.Renderer, users []deployment.UserRole) error {
	out := userListJSON{Users: make([]userJSON, 0, len(users))}
	for i := range users {
		out.Users = append(out.Users, newDeploymentUserJSON(&users[i]))
	}
	return r.Emit(out, func(w io.Writer) error {
		tab := printutil.Table{
			Padding:        []int{44, 50},
			DynamicPadding: true,
			Header:         []string{"USER ID", "NAME", "EMAIL", "ROLE"},
		}
		for i := range users {
			tab.AddRow([]string{users[i].ID, users[i].FullName, users[i].Username, users[i].Role}, false)
		}
		return tab.Print(w)
	})
}

// renderNoDeploymentUsers is the list when Houston listed nobody: an empty
// list, and in text the sentence the command has always printed.
func renderNoDeploymentUsers(r cliout.Renderer) error {
	return r.Emit(userListJSON{Users: []userJSON{}}, cliout.Text(func(b *bufio.Writer) {
		fmt.Fprintln(b, "No users were found for this deployment.  Check the deploymentId and try again.")
	}))
}

// renderDeploymentUserChange is what a Deployment user add or update did.
// The table names the user as given (email) after an add, and as Houston
// names them after an update, as each always has.
func renderDeploymentUserChange(r cliout.Renderer, deploymentID, email string, u *deployment.UserRole, added bool) error {
	return r.Emit(newDeploymentUserJSON(u), func(w io.Writer) error {
		tab := deploymentUserTable()
		if added {
			tab.AddRow([]string{deploymentID, email, u.Role}, false)
			tab.SuccessMsg = fmt.Sprintf("\n Successfully added %s as a %s", email, u.Role)
		} else {
			tab.AddRow([]string{deploymentID, u.Username, u.Role}, false)
			tab.SuccessMsg = fmt.Sprintf("\n Successfully updated %s to a %s", email, u.Role)
		}
		return tab.Print(w)
	})
}

func renderDeploymentUserRemoval(r cliout.Renderer, deploymentID, email string, u *deployment.UserRole) error {
	role := u.Role
	// The json names the user as Houston does; the text as given.
	out := deploymentUserRemovalJSON{ID: orNull(u.ID), Username: orNull(u.Username), DeploymentID: deploymentID, Role: orNull(role), Action: accessActionRemoved}
	return r.Emit(out, func(w io.Writer) error {
		tab := deploymentUserTable()
		tab.AddRow([]string{deploymentID, email, role}, false)
		tab.SuccessMsg = fmt.Sprintf("\n Successfully removed the %s role for %s from deployment %s", role, email, deploymentID)
		return tab.Print(w)
	})
}

func workspaceUserListTable(users []workspace.UserRole) *printutil.Table {
	tab := &printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"USERNAME", "ID", "ROLE"},
	}
	for i := range users {
		tab.AddRow([]string{users[i].Username, users[i].ID, users[i].Role}, false)
	}
	return tab
}

func renderWorkspaceUserList(r cliout.Renderer, users []workspace.UserRole) error {
	out := userListJSON{Users: make([]userJSON, 0, len(users))}
	for i := range users {
		out.Users = append(out.Users, newWorkspaceUserJSON(&users[i]))
	}
	return r.Emit(out, func(w io.Writer) error { return workspaceUserListTable(users).Print(w) })
}

func renderWorkspaceUserAdded(r cliout.Renderer, w *houston.Workspace, email string, u *workspace.UserRole) error {
	return r.Emit(newWorkspaceUserJSON(u), func(out io.Writer) error {
		tab := printutil.Table{
			Padding:        []int{44, 50},
			DynamicPadding: true,
			Header:         []string{"NAME", "WORKSPACE ID", "EMAIL", "ROLE"},
		}
		tab.AddRow([]string{w.Label, w.ID, email, u.Role}, false)
		tab.SuccessMsg = fmt.Sprintf("Successfully added %s to %s", email, w.Label)
		return tab.Print(out)
	})
}

func renderWorkspaceUserUpdated(r cliout.Renderer, email string, c *workspace.UserRoleChange) error {
	return r.Emit(newWorkspaceUserJSON(&c.User), cliout.Text(func(b *bufio.Writer) {
		fmt.Fprintf(b, "Role has been changed from %s to %s for user %s", c.Previous, c.User.Role, email)
	}))
}

// renderWorkspaceUserRemoved is what a Workspace user remove did. The json
// names the user as Houston does.
func renderWorkspaceUserRemoved(r cliout.Renderer, w *houston.Workspace, u *workspace.UserRole) error {
	userID := u.ID
	out := workspaceUserRemovalJSON{ID: userID, Username: orNull(u.Username), WorkspaceID: w.ID, Action: accessActionRemoved}
	return r.Emit(out, func(o io.Writer) error {
		tab := printutil.Table{
			Padding: []int{30, 50, 50},
			Header:  []string{"NAME", "WORKSPACE ID", "USER_ID"},
		}
		tab.AddRow([]string{w.Label, w.ID, userID}, false)
		tab.SuccessMsg = "Successfully removed user from workspace"
		return tab.Print(o)
	})
}

// createdUserJSON is the user `astro user create` created. The session token
// Houston returns with it is not published: it is the new user's credential,
// not the caller's.
type createdUserJSON struct {
	ID       *string `json:"id"`
	Username *string `json:"username"`
	// Status is Houston's: "pending" until the user verifies their email,
	// "active" once they can log in.
	Status *string `json:"status"`
}

func renderUserCreated(r cliout.Renderer, c *user.Created) error {
	out := createdUserJSON{ID: orNull(c.ID), Username: orNull(c.Username), Status: orNull(c.Status)}
	return r.Emit(out, cliout.Text(func(b *bufio.Writer) {
		loginMsg := "You may now login to the platform."
		if c.Status == "pending" {
			loginMsg = "Check your email for a verification."
		}
		fmt.Fprintf(b, "Successfully created user %s. %s\n", c.Email, loginMsg)
	}))
}

// teamJSON is an APC team and the role it holds on what the command is
// about: the platform (system_role), a Workspace or a Deployment; the other
// roles are null. A role is NONE when the team holds none there. Name is
// null when Houston's answer did not give it.
type teamJSON struct {
	ID             string  `json:"id"`
	Name           *string `json:"name"`
	SystemRole     *string `json:"system_role"`
	WorkspaceRole  *string `json:"workspace_role"`
	DeploymentRole *string `json:"deployment_role"`
}

type teamListJSON struct {
	Teams []teamJSON `json:"teams"`
}

// teamDetailJSON is `astro team get`: the team, the roles it holds on
// Workspaces and Deployments, and its users. Users is null unless --users or
// --all asked for them, which costs a second request.
type teamDetailJSON struct {
	ID   string  `json:"id"`
	Name *string `json:"name"`
	// SystemRole is never null: the CLI derives it from the team's
	// bindings, NONE when it holds no system role, as team list does.
	SystemRole      string                   `json:"system_role"`
	CreatedAt       *string                  `json:"created_at"`
	WorkspaceRoles  []teamWorkspaceRoleJSON  `json:"workspace_roles"`
	DeploymentRoles []teamDeploymentRoleJSON `json:"deployment_roles"`
	Users           []teamUserJSON           `json:"users"`
}

type teamWorkspaceRoleJSON struct {
	WorkspaceID string  `json:"workspace_id"`
	Label       *string `json:"label"`
	Role        string  `json:"role"`
}

type teamDeploymentRoleJSON struct {
	DeploymentID string  `json:"deployment_id"`
	Label        *string `json:"label"`
	Role         string  `json:"role"`
}

type teamUserJSON struct {
	ID       string  `json:"id"`
	Username *string `json:"username"`
}

// workspaceTeamRemovalJSON names the team `astro workspace team remove` took
// off the Workspace.
type workspaceTeamRemovalJSON struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Action      string `json:"action"`
	// MembershipVerified is false when the login could not read the team's
	// role on the Workspace, so the removal was sent without knowing the
	// team was in it. Houston removes nothing for a team that is not, and
	// says nothing.
	MembershipVerified bool `json:"membership_verified"`
}

// deploymentTeamRemovalJSON names the team `astro deployment team remove`
// took the Deployment role from. The team stays in the Workspace.
type deploymentTeamRemovalJSON struct {
	ID           string `json:"id"`
	DeploymentID string `json:"deployment_id"`
	Action       string `json:"action"`
}

func renderDeploymentTeamList(r cliout.Renderer, deploymentID string, ts []deployment.TeamRole) error {
	rows := make([]teamRoleRow, 0, len(ts))
	for i := range ts {
		rows = append(rows, teamRoleRow(ts[i]))
	}
	return renderTeamRoleList(r, "DEPLOYMENT ID", deploymentID, rows, func(t teamRoleRow) teamJSON {
		return teamJSON{ID: t.ID, Name: orNull(t.Name), DeploymentRole: orNull(t.Role)}
	})
}

// teamRoleRow is a team and the role it holds on a Workspace or a
// Deployment, as their team lists show it.
type teamRoleRow struct {
	ID   string
	Name string
	Role string
}

// renderTeamRoleList is `workspace team list` and `deployment team list`:
// the table has the Workspace or Deployment in its first column, named by
// scopeHeader.
func renderTeamRoleList(r cliout.Renderer, scopeHeader, scopeID string, rows []teamRoleRow, toJSON func(teamRoleRow) teamJSON) error {
	out := teamListJSON{Teams: make([]teamJSON, 0, len(rows))}
	for _, t := range rows {
		out.Teams = append(out.Teams, toJSON(t))
	}
	return r.Emit(out, func(w io.Writer) error {
		tab := printutil.Table{
			Padding:        []int{44, 50},
			DynamicPadding: true,
			Header:         []string{scopeHeader, "TEAM ID", "TEAM NAME", "ROLE"},
		}
		for _, t := range rows {
			tab.AddRow([]string{scopeID, t.ID, t.Name, t.Role}, false)
		}
		return tab.Print(w)
	})
}

func renderDeploymentTeamChange(r cliout.Renderer, deploymentID, teamID, role string, added bool) error {
	return r.Emit(teamJSON{ID: teamID, DeploymentRole: orNull(role)}, func(w io.Writer) error {
		tab := printutil.Table{
			Padding:        []int{44, 50},
			DynamicPadding: true,
			Header:         []string{"DEPLOYMENT ID", "TEAM ID", "ROLE"},
		}
		tab.AddRow([]string{deploymentID, teamID, role}, false)
		if added {
			tab.SuccessMsg = fmt.Sprintf("\nSuccessfully added team %s to deployment %s as a %s", teamID, deploymentID, role)
		} else {
			tab.SuccessMsg = fmt.Sprintf("\n Successfully updated team %s to a %s", teamID, role)
		}
		return tab.Print(w)
	})
}

func renderDeploymentTeamRemoval(r cliout.Renderer, deploymentID, teamID string) error {
	out := deploymentTeamRemovalJSON{ID: teamID, DeploymentID: deploymentID, Action: accessActionRemoved}
	return r.Emit(out, func(w io.Writer) error {
		tab := printutil.Table{
			Padding:        []int{44, 50},
			DynamicPadding: true,
			Header:         []string{"DEPLOYMENT ID", "TEAM ID"},
		}
		tab.AddRow([]string{deploymentID, teamID}, false)
		tab.SuccessMsg = fmt.Sprintf("\n Successfully removed team %s from deployment %s", teamID, deploymentID)
		return tab.Print(w)
	})
}

func renderWorkspaceTeamList(r cliout.Renderer, workspaceID string, ts []workspace.TeamRole) error {
	rows := make([]teamRoleRow, 0, len(ts))
	for i := range ts {
		rows = append(rows, teamRoleRow(ts[i]))
	}
	return renderTeamRoleList(r, "WORKSPACE ID", workspaceID, rows, func(t teamRoleRow) teamJSON {
		return teamJSON{ID: t.ID, Name: orNull(t.Name), WorkspaceRole: orNull(t.Role)}
	})
}

func renderWorkspaceTeamAdded(r cliout.Renderer, w *houston.Workspace, teamID, role string) error {
	return r.Emit(teamJSON{ID: teamID, WorkspaceRole: orNull(role)}, func(out io.Writer) error {
		tab := printutil.Table{
			Padding:        []int{44, 50},
			DynamicPadding: true,
			Header:         []string{"NAME", "WORKSPACE ID", "TEAM ID", "ROLE"},
		}
		tab.AddRow([]string{w.Label, w.ID, teamID, role}, false)
		tab.SuccessMsg = fmt.Sprintf("Successfully added %s to %s", teamID, w.Label)
		return tab.Print(out)
	})
}

func renderWorkspaceTeamUpdated(r cliout.Renderer, c *workspace.TeamRoleChange) error {
	out := teamJSON{ID: c.Team.ID, Name: orNull(c.Team.Name), WorkspaceRole: orNull(c.Team.Role)}
	return r.Emit(out, cliout.Text(func(b *bufio.Writer) {
		fmt.Fprintf(b, "Role has been changed from %s to %s for team %s\n", c.Previous, c.Team.Role, c.Team.ID)
	}))
}

// renderWorkspaceTeamRemoved is what a Workspace team remove did. When the
// team's membership could not be checked, a note says so first, on notes.
func renderWorkspaceTeamRemoved(r cliout.Renderer, notes io.Writer, removal *workspace.TeamRemoval, teamID string) error {
	w := removal.Workspace
	if !removal.Verified {
		fmt.Fprintf(notes, "Could not confirm that team %s is in Workspace %s: this login cannot read the Workspace's teams. The removal was sent anyway; for a team that is not in the Workspace, nothing is removed.\n", teamID, w.ID)
	}
	out := workspaceTeamRemovalJSON{ID: teamID, WorkspaceID: w.ID, Action: accessActionRemoved, MembershipVerified: removal.Verified}
	return r.Emit(out, func(o io.Writer) error {
		tab := printutil.Table{
			Padding: []int{30, 50, 50},
			Header:  []string{"NAME", "WORKSPACE ID", "TEAM ID"},
		}
		tab.AddRow([]string{w.Label, w.ID, teamID}, false)
		tab.SuccessMsg = "Successfully removed team from workspace"
		return tab.Print(o)
	})
}

// systemTeamsTable is the table `astro team list` prints, a page of it when
// paginated.
func systemTeamsTable(ts []houston.Team) *printutil.Table {
	tab := &printutil.Table{
		Padding:        []int{50, 50},
		DynamicPadding: true,
		Header:         []string{"TEAM ID", "TEAM NAME", "ROLE"},
		ColorRowCode:   [2]string{"\033[1;32m", "\033[0m"},
	}
	for i := range ts {
		tab.AddRow([]string{ts[i].ID, ts[i].Name, teams.SystemRole(ts[i].RoleBindings)}, false)
	}
	return tab
}

func renderTeamList(r cliout.Renderer, ts []houston.Team) error {
	out := teamListJSON{Teams: make([]teamJSON, 0, len(ts))}
	for i := range ts {
		out.Teams = append(out.Teams, teamJSON{ID: ts[i].ID, Name: orNull(ts[i].Name), SystemRole: orNull(teams.SystemRole(ts[i].RoleBindings))})
	}
	return r.Emit(out, func(w io.Writer) error { return systemTeamsTable(ts).Print(w) })
}

func renderTeamUpdated(r cliout.Renderer, c *teams.RoleChange) error {
	return r.Emit(teamJSON{ID: c.TeamID, SystemRole: orNull(c.Role)}, cliout.Text(func(b *bufio.Writer) {
		switch {
		case !c.Changed:
			fmt.Fprintf(b, "Role for the team %s already set to None, nothing to update\n", c.TeamID)
		case c.Previous != "":
			fmt.Fprintf(b, "Role has been changed from %s to %s for team %s\n\n", c.Previous, c.Role, c.TeamID)
		default:
			fmt.Fprintf(b, "Role has been changed to %s for team %s\n\n", c.Role, c.TeamID)
		}
	}))
}

// renderTeamDetail is `astro team get`. The text shows the roles only with
// --roles or --all, and the users only when they were fetched; json has the
// roles always, since the one query returns them.
func renderTeamDetail(r cliout.Renderer, d *teams.Detail, showRoles bool) error {
	team := d.Team
	role := teams.SystemRole(team.RoleBindings)
	out := teamDetailJSON{
		ID: team.ID, Name: orNull(team.Name), SystemRole: role, CreatedAt: orNull(team.CreatedAt),
		WorkspaceRoles: []teamWorkspaceRoleJSON{}, DeploymentRoles: []teamDeploymentRoleJSON{},
	}
	workspaceRolesTable := printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"WORKSPACE ID", "WORKSPACE NAME", "ROLE"},
		ColorRowCode:   [2]string{"\033[1;32m", "\033[0m"},
	}
	deploymentRolesTable := printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"DEPLOYMENT ID", "DEPLOYMENT NAME", "ROLE"},
		ColorRowCode:   [2]string{"\033[1;32m", "\033[0m"},
	}
	for i := range team.RoleBindings {
		rb := &team.RoleBindings[i]
		if rb.Role == houston.NoneRole {
			continue
		}
		if workspace.IsValidWorkspaceLevelRole(rb.Role) {
			workspaceRolesTable.AddRow([]string{rb.Workspace.ID, rb.Workspace.Label, rb.Role}, false)
			out.WorkspaceRoles = append(out.WorkspaceRoles, teamWorkspaceRoleJSON{WorkspaceID: rb.Workspace.ID, Label: orNull(rb.Workspace.Label), Role: rb.Role})
		}
		if deployment.IsValidDeploymentLevelRole(rb.Role) {
			deploymentRolesTable.AddRow([]string{rb.Deployment.ID, rb.Deployment.Label, rb.Role}, false)
			out.DeploymentRoles = append(out.DeploymentRoles, teamDeploymentRoleJSON{DeploymentID: rb.Deployment.ID, Label: orNull(rb.Deployment.Label), Role: rb.Role})
		}
	}
	teamUsersTable := printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"USERNAME", "ID"},
		ColorRowCode:   [2]string{"\033[1;32m", "\033[0m"},
	}
	if d.Users != nil {
		out.Users = make([]teamUserJSON, 0, len(d.Users))
		for i := range d.Users {
			teamUsersTable.AddRow([]string{d.Users[i].Username, d.Users[i].ID}, false)
			out.Users = append(out.Users, teamUserJSON{ID: d.Users[i].ID, Username: orNull(d.Users[i].Username)})
		}
	}
	return r.Emit(out, cliout.Text(func(b *bufio.Writer) {
		fmt.Fprintf(b, "\nTeam Name: %s\nTeam ID: %s \nSystem Role: %s\n", team.Name, team.ID, role)
		if showRoles {
			if len(workspaceRolesTable.Rows) > 0 {
				fmt.Fprintln(b, "\nWorkspace Level Roles:")
				workspaceRolesTable.Print(b) //nolint:errcheck // a bufio.Writer keeps the error for the flush
			}
			if len(deploymentRolesTable.Rows) > 0 {
				fmt.Fprintln(b, "\nDeployment Level Roles:")
				deploymentRolesTable.Print(b) //nolint:errcheck // a bufio.Writer keeps the error for the flush
			}
		}
		if d.Users != nil {
			fmt.Fprintln(b, "\nUsers part of Team:")
			teamUsersTable.Print(b) //nolint:errcheck // a bufio.Writer keeps the error for the flush
		}
	}))
}
