package astro

import (
	"io"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
)

// The user and team commands of `astro workspace` and `astro organization`,
// in both formats. They publish the user and team objects their lists already
// published (user.UserInfo and team.TeamInfo, pinned by user.json and
// team.json beside user-list.json and team-list.json), a removal naming the
// Workspace or the Organization, an invite, and a team's members. As in
// workspace_org_token_json_test.go, these tests decode what a run printed and
// assert what it means, and in text they assert the messages in order and each
// table cell under its header, not the padding.

var userTeamCreated = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// userTeamFixtures builds two users of the Organization, one of them a member
// of the current Workspace, and two teams, one of them managed by an IdP.
func userTeamFixtures() (ada, bob astrov1.User, eng, idp astrov1.Team) {
	member := astrov1.UserOrganizationRoleORGANIZATIONMEMBER
	owner := astrov1.UserOrganizationRole("ORGANIZATION_OWNER")
	ada = astrov1.User{
		Id: "user-ada", Username: "ada@example.com", FullName: "Ada Lovelace",
		OrganizationRole: &member,
		WorkspaceRoles:   &[]astrov1.WorkspaceRole{{WorkspaceId: curWorkspaceID, Role: "WORKSPACE_MEMBER"}},
		DeploymentRoles:  &[]astrov1.DeploymentRole{{DeploymentId: "dep-1", Role: "DEPLOYMENT_ADMIN"}},
		CreatedAt:        userTeamCreated,
	}
	bob = astrov1.User{
		Id: "user-bob", Username: "bob@example.com", FullName: "Bob Builder",
		OrganizationRole: &owner,
		CreatedAt:        userTeamCreated,
	}
	eng = astrov1.Team{
		Id: "team-eng", Name: "Engineering", Description: tokPtr("Builds things"),
		OrganizationRole: astrov1.TeamOrganizationRoleORGANIZATIONMEMBER,
		WorkspaceRoles:   &[]astrov1.WorkspaceRole{{WorkspaceId: curWorkspaceID, Role: "WORKSPACE_OPERATOR"}},
		DeploymentRoles:  &[]astrov1.DeploymentRole{{DeploymentId: "dep-1", Role: "DEPLOYMENT_ADMIN"}},
		CreatedAt:        userTeamCreated,
	}
	idp = astrov1.Team{
		Id: "team-idp", Name: "Okta Ops",
		OrganizationRole: astrov1.TeamOrganizationRoleORGANIZATIONMEMBER,
		IsIdpManaged:     true,
		CreatedAt:        userTeamCreated,
	}
	return ada, bob, eng, idp
}

// member is u as a member of a team.
func member(u astrov1.User) astrov1.TeamMember { //nolint:gocritic // a test fixture
	return astrov1.TeamMember{UserId: u.Id, Username: u.Username, FullName: tokPtr(u.FullName), CreatedAt: tokPtr(userTeamCreated)}
}

// userTeamMock is a client that lists users and teams, gets any of them, and
// lists members as the team's.
func userTeamMock(users []astrov1.User, teams []astrov1.Team, members []astrov1.TeamMember) func(t *testing.T) *astrov1_mocks.ClientWithResponsesInterface {
	return func(t *testing.T) *astrov1_mocks.ClientWithResponsesInterface {
		t.Helper()
		if users == nil {
			users = []astrov1.User{}
		}
		if teams == nil {
			teams = []astrov1.Team{}
		}
		if members == nil {
			members = []astrov1.TeamMember{}
		}
		m := new(astrov1_mocks.ClientWithResponsesInterface)
		m.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.ListUsersResponse{
			HTTPResponse: ok200(), JSON200: &astrov1.UsersPaginated{Users: users, Limit: 100, TotalCount: len(users)},
		}, nil).Maybe()
		for i := range users {
			m.On("GetUserWithResponse", mock.Anything, mock.Anything, users[i].Id).Return(&astrov1.GetUserResponse{HTTPResponse: ok200(), JSON200: &users[i]}, nil).Maybe()
		}
		m.On("ListTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.ListTeamsResponse{
			HTTPResponse: ok200(), JSON200: &astrov1.TeamsPaginated{Teams: teams, Limit: 100, TotalCount: len(teams)},
		}, nil).Maybe()
		for i := range teams {
			m.On("GetTeamWithResponse", mock.Anything, mock.Anything, teams[i].Id).Return(&astrov1.GetTeamResponse{HTTPResponse: ok200(), JSON200: &teams[i]}, nil).Maybe()
		}
		m.On("ListTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.ListTeamMembersResponse{
			HTTPResponse: ok200(), JSON200: &astrov1.TeamMembersPaginated{TeamMembers: members, Limit: 100, TotalCount: len(members)},
		}, nil).Maybe()
		return m
	}
}

// with adds the calls that change something to a userTeamMock.
func with(base func(t *testing.T) *astrov1_mocks.ClientWithResponsesInterface, more ...func(m *astrov1_mocks.ClientWithResponsesInterface)) func(t *testing.T) astrov1.APIClient {
	return func(t *testing.T) astrov1.APIClient {
		m := base(t)
		for _, f := range more {
			f(m)
		}
		return m
	}
}

func setsUserRoles(id string) func(m *astrov1_mocks.ClientWithResponsesInterface) {
	return func(m *astrov1_mocks.ClientWithResponsesInterface) {
		m.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, id, mock.Anything).Return(&astrov1.UpdateUserRolesResponse{HTTPResponse: ok200(), JSON200: &astrov1.SubjectRoles{}}, nil)
	}
}

func setsTeamRoles(id string) func(m *astrov1_mocks.ClientWithResponsesInterface) {
	return func(m *astrov1_mocks.ClientWithResponsesInterface) {
		m.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, id, mock.Anything).Return(&astrov1.UpdateTeamRolesResponse{HTTPResponse: ok200(), JSON200: &astrov1.SubjectRoles{}}, nil)
	}
}

func invites(inv *astrov1.Invite) func(m *astrov1_mocks.ClientWithResponsesInterface) {
	return func(m *astrov1_mocks.ClientWithResponsesInterface) {
		m.On("CreateUserInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.CreateUserInviteResponse{HTTPResponse: ok200(), JSON200: inv}, nil)
	}
}

func createsTeam(after *astrov1.Team) func(m *astrov1_mocks.ClientWithResponsesInterface) {
	return func(m *astrov1_mocks.ClientWithResponsesInterface) {
		m.On("CreateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.CreateTeamResponse{HTTPResponse: ok200(), JSON200: after}, nil)
	}
}

func updatesTeam(id string, after *astrov1.Team) func(m *astrov1_mocks.ClientWithResponsesInterface) {
	return func(m *astrov1_mocks.ClientWithResponsesInterface) {
		m.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, id, mock.Anything).Return(&astrov1.UpdateTeamResponse{HTTPResponse: ok200(), JSON200: after}, nil)
	}
}

func deletesTeam(id string) func(m *astrov1_mocks.ClientWithResponsesInterface) {
	return func(m *astrov1_mocks.ClientWithResponsesInterface) {
		m.On("DeleteTeamWithResponse", mock.Anything, mock.Anything, id).Return(&astrov1.DeleteTeamResponse{HTTPResponse: ok200()}, nil)
	}
}

func addsMember(teamID string) func(m *astrov1_mocks.ClientWithResponsesInterface) {
	return func(m *astrov1_mocks.ClientWithResponsesInterface) {
		m.On("AddTeamMembersWithResponse", mock.Anything, mock.Anything, teamID, mock.Anything).Return(&astrov1.AddTeamMembersResponse{HTTPResponse: ok200()}, nil)
	}
}

func removesMember(teamID, userID string) func(m *astrov1_mocks.ClientWithResponsesInterface) {
	return func(m *astrov1_mocks.ClientWithResponsesInterface) {
		m.On("RemoveTeamMemberWithResponse", mock.Anything, mock.Anything, teamID, userID).Return(&astrov1.RemoveTeamMemberResponse{HTTPResponse: ok200()}, nil)
	}
}

// The rows a user picker shows. In the Workspace picker the role column is
// empty whatever role the user holds (pinned: SelectUser looks the role up on
// the Workspace ""); the Organization picker shows the Organization role.
func userPickRow(n, name, email, id, roleHeader, role string) map[string]string {
	return map[string]string{"#": n, "FULLNAME": name, "EMAIL": email, "ID": id, roleHeader: role, "CREATE DATE": "2026-01-02T03:04:05Z"}
}

var (
	adaOrgPickRow = userPickRow("1", "Ada Lovelace", "ada@example.com", "user-ada", "ORGANIZATION ROLE", "ORGANIZATION_MEMBER")
	bobOrgPickRow = userPickRow("2", "Bob Builder", "bob@example.com", "user-bob", "ORGANIZATION ROLE", "ORGANIZATION_OWNER")
)

var (
	teamPickRows = []map[string]string{
		{"#": "1", "TEAMNAME": "Engineering", "ID": "team-eng"},
		{"#": "2", "TEAMNAME": "Okta Ops", "ID": "team-idp"},
	}
	adaMemberRow = map[string]string{"ID": "user-ada", "FullName": "Ada Lovelace", "Email": "ada@example.com"}
)

// What the user commands print in text: what they printed before they gained
// --output, recorded byte for byte against v2 and checked here by meaning.
func TestWorkspaceOrganizationUserText(t *testing.T) {
	ada, bob, _, _ := userTeamFixtures()
	users := userTeamMock([]astrov1.User{ada, bob}, nil, nil)
	inv := &astrov1.Invite{InviteId: "inv-1", OrganizationId: "test-org-id", ExpiresAt: userTeamCreated.Add(7 * 24 * time.Hour)}

	runTokenCases(t, []tokenCase{
		{name: "invite", root: newOrganizationCmd, client: with(users, invites(inv)), args: []string{"organization", "user", "invite", "Ada@Example.com", "--role", "ORGANIZATION_BILLING_ADMIN"}, check: says("invite for ada@example.com with role ORGANIZATION_BILLING_ADMIN created")},
		{name: "invite with the default role", root: newOrganizationCmd, client: with(users, invites(inv)), args: []string{"organization", "user", "invite", "ada@example.com"}, check: says("invite for ada@example.com with role ORGANIZATION_MEMBER created")},
		{name: "invite with the email asked", root: newOrganizationCmd, client: with(users, invites(inv)), answers: "ada@example.com\n", args: []string{"organization", "user", "invite"}, check: says("enter email address to invite a user: ", "invite for ada@example.com with role ORGANIZATION_MEMBER created")},
		{name: "invite with no email", root: newOrganizationCmd, client: with(users), answers: "\n", args: []string{"organization", "user", "invite"}, check: says("enter email address to invite a user: "), wantErr: "no email provided for the invite. Retry with a valid email address"},
		{name: "invite with a role that is not one", root: newOrganizationCmd, client: with(users), args: []string{"organization", "user", "invite", "ada@example.com", "--role", "NOPE"}, check: func(t *testing.T, out string) { assert.Empty(t, out) }, wantErr: "requested role is invalid. Possible values are ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN and ORGANIZATION_OWNER "},
		{name: "organization update", root: newOrganizationCmd, client: with(users, setsUserRoles("user-ada")), args: []string{"organization", "user", "update", "ada@example.com", "--role", "ORGANIZATION_OWNER"}, check: says("The user ada@example.com role was successfully updated to ORGANIZATION_OWNER")},
		{name: "organization update with the role asked", root: newOrganizationCmd, client: with(users, setsUserRoles("user-ada")), answers: "ORGANIZATION_OWNER\n", args: []string{"organization", "user", "update", "ada@example.com"}, check: says("enter a user Organization role(", ") to update user: ", "The user ada@example.com role was successfully updated to ORGANIZATION_OWNER")},
		{
			name: "organization update through the picker", root: newOrganizationCmd, client: with(users, setsUserRoles("user-bob")), answers: "2\n",
			args: []string{"organization", "user", "update", "--role", "ORGANIZATION_MEMBER"},
			check: func(t *testing.T, out string) {
				requireInOrder(t, out, "Please select the user:", "> The user bob@example.com role was successfully updated to ORGANIZATION_MEMBER")
				assert.Equal(t, []map[string]string{adaOrgPickRow, bobOrgPickRow}, tableRows(t, out, "#"))
			},
		},
		{name: "organization update of nobody", root: newOrganizationCmd, client: with(users), args: []string{"organization", "user", "update", "nobody@example.com", "--role", "ORGANIZATION_OWNER"}, check: func(t *testing.T, out string) { assert.Empty(t, out) }, wantErr: "no user was found for the email you provided"},
		{name: "workspace add", root: newWorkspaceCmd, client: with(users, setsUserRoles("user-bob")), args: []string{"workspace", "user", "add", "bob@example.com", "--role", "WORKSPACE_AUTHOR"}, check: says("The user bob@example.com was successfully added to the workspace with the role WORKSPACE_AUTHOR")},
		{name: "workspace add with the default role", root: newWorkspaceCmd, client: with(users, setsUserRoles("user-bob")), args: []string{"workspace", "user", "add", "bob@example.com"}, check: says("The user bob@example.com was successfully added to the workspace with the role WORKSPACE_MEMBER")},
		{
			name: "workspace add through the picker", root: newWorkspaceCmd, client: with(users, setsUserRoles("user-bob")), answers: "2\n",
			args: []string{"workspace", "user", "add"},
			check: func(t *testing.T, out string) {
				requireInOrder(t, out, "Please select the user:", "> The user bob@example.com was successfully added to the workspace with the role WORKSPACE_MEMBER")
				// The picker offers the Organization's users, by their
				// Organization role.
				assert.Equal(t, []map[string]string{adaOrgPickRow, bobOrgPickRow}, tableRows(t, out, "#"))
			},
		},
		{name: "workspace update", root: newWorkspaceCmd, client: with(users, setsUserRoles("user-ada")), args: []string{"workspace", "user", "update", "ada@example.com", "--role", "WORKSPACE_OWNER"}, check: says("The workspace user ada@example.com role was successfully updated to WORKSPACE_OWNER")},
		{name: "workspace update with the role asked", root: newWorkspaceCmd, client: with(users, setsUserRoles("user-ada")), answers: "WORKSPACE_OWNER\n", args: []string{"workspace", "user", "update", "ada@example.com"}, check: says("Enter a user Workspace role(", ") to update user: ", "The workspace user ada@example.com role was successfully updated to WORKSPACE_OWNER")},
		{
			name: "workspace update through the picker", root: newWorkspaceCmd, client: with(users, setsUserRoles("user-ada")), answers: "1\n",
			args: []string{"workspace", "user", "update", "--role", "WORKSPACE_OWNER"},
			check: func(t *testing.T, out string) {
				requireInOrder(t, out, "Please select the user:", "> The workspace user ada@example.com role was successfully updated to WORKSPACE_OWNER")
				assert.Equal(t, []map[string]string{
					userPickRow("1", "Ada Lovelace", "ada@example.com", "user-ada", "WORKSPACE ROLE", ""),
					userPickRow("2", "Bob Builder", "bob@example.com", "user-bob", "WORKSPACE ROLE", ""),
				}, tableRows(t, out, "#"))
			},
		},
		{name: "workspace update with a role that is not one", root: newWorkspaceCmd, client: with(users), args: []string{"workspace", "user", "update", "ada@example.com", "--role", "NOPE"}, check: func(t *testing.T, out string) { assert.Empty(t, out) }, wantErr: "requested role is invalid. Possible values are WORKSPACE_MEMBER, WORKSPACE_AUTHOR, WORKSPACE_OPERATOR and WORKSPACE_OWNER "},
		{name: "workspace remove", root: newWorkspaceCmd, client: with(users, setsUserRoles("user-ada")), args: []string{"workspace", "user", "remove", "ada@example.com"}, check: says("The user ada@example.com was successfully removed from the workspace")},
		{
			name: "workspace remove through the picker", root: newWorkspaceCmd, client: with(users, setsUserRoles("user-ada")), answers: "1\n",
			args:  []string{"workspace", "user", "remove"},
			check: says("Please select the user:", "> The user ada@example.com was successfully removed from the workspace"),
		},
		{name: "workspace remove of nobody", root: newWorkspaceCmd, client: with(users), args: []string{"workspace", "user", "remove", "nobody@example.com"}, check: func(t *testing.T, out string) { assert.Empty(t, out) }, wantErr: "no user was found for the email you provided"},
	})
}

// What the team commands print in text, recorded the same way.
func TestWorkspaceOrganizationTeamText(t *testing.T) {
	ada, bob, eng, idp := userTeamFixtures()
	teams := userTeamMock([]astrov1.User{ada, bob}, []astrov1.Team{eng, idp}, []astrov1.TeamMember{member(ada)})
	noMembers := userTeamMock([]astrov1.User{ada, bob}, []astrov1.Team{eng, idp}, nil)
	noTeams := userTeamMock(nil, nil, nil)
	renamed := eng
	renamed.Name = "Platform"
	empty := func(t *testing.T, out string) { assert.Empty(t, out) }

	runTokenCases(t, []tokenCase{
		{name: "create", root: newOrganizationCmd, client: with(teams, createsTeam(&eng)), args: []string{"organization", "team", "create", "--name", "Engineering", "--description", "Builds things", "--role", "ORGANIZATION_MEMBER"}, check: says("Astro Team Engineering was successfully created")},
		{
			name: "create with the role picked", root: newOrganizationCmd, client: with(teams, createsTeam(&eng)), answers: "1\n",
			args: []string{"organization", "team", "create", "--name", "Engineering"},
			check: func(t *testing.T, out string) {
				requireInOrder(t, out, "select a Organization Role for the new team:", "ORGANIZATION_MEMBER", "> Astro Team Engineering was successfully created")
				assert.Equal(t, []map[string]string{{"#": "1", "ROLE": "ORGANIZATION_MEMBER"}, {"#": "2", "ROLE": "ORGANIZATION_BILLING_ADMIN"}, {"#": "3", "ROLE": "ORGANIZATION_OWNER"}}, tableRows(t, out, "#"))
			},
		},
		{name: "create with the name asked", root: newOrganizationCmd, client: with(teams, createsTeam(&eng)), answers: "Engineering\n", args: []string{"organization", "team", "create", "--role", "ORGANIZATION_MEMBER"}, check: says("Please specify a name for your Team", "Team name: ", "Astro Team Engineering was successfully created")},
		{name: "update", root: newOrganizationCmd, client: with(teams, updatesTeam("team-eng", &renamed), setsTeamRoles("team-eng")), args: []string{"organization", "team", "update", "team-eng", "--name", "Platform", "--role", "ORGANIZATION_OWNER"}, check: says("Astro Team Engineering was successfully updated", "Astro Team role Engineering was successfully updated to ORGANIZATION_OWNER")},
		{name: "update without --role", root: newOrganizationCmd, client: with(teams, updatesTeam("team-eng", &eng)), args: []string{"organization", "team", "update", "team-eng", "--description", "d"}, check: func(t *testing.T, out string) {
			assert.Equal(t, "Astro Team Engineering was successfully updated\n", out)
		}},
		{
			name: "update through the picker", root: newOrganizationCmd, client: with(teams, updatesTeam("team-eng", &eng)), answers: "1\n",
			args: []string{"organization", "team", "update", "--description", "d"},
			check: func(t *testing.T, out string) {
				requireInOrder(t, out, "Please select a team:", "> Astro Team Engineering was successfully updated")
				assert.Equal(t, teamPickRows, tableRows(t, out, "#"))
			},
		},
		{name: "update an IdP team confirmed", root: newOrganizationCmd, client: with(teams, updatesTeam("team-idp", &idp)), answers: "y\n", args: []string{"organization", "team", "update", "team-idp", "--description", "d"}, check: says("This is an IDP-managed team. Are you sure you want to continue the operation? (y/n)", "Astro Team Okta Ops was successfully updated")},
		{name: "update an IdP team declined", root: newOrganizationCmd, client: with(teams), answers: "n\n", args: []string{"organization", "team", "update", "team-idp", "--description", "d"}, check: func(t *testing.T, out string) {
			assert.Equal(t, "This is an IDP-managed team. Are you sure you want to continue the operation? (y/n) ", out)
		}},
		{name: "update an IdP team --yes", root: newOrganizationCmd, client: with(teams, updatesTeam("team-idp", &idp)), args: []string{"organization", "team", "update", "team-idp", "--description", "d", "--yes"}, check: says("Astro Team Okta Ops was successfully updated")},
		{
			// Pinned: the rename is sent before the role is checked, so a
			// role that is not one fails after the team was renamed.
			name: "update with a role that is not one", root: newOrganizationCmd, client: with(teams, updatesTeam("team-eng", &renamed)),
			args:    []string{"organization", "team", "update", "team-eng", "--name", "Platform", "--role", "NOPE"},
			check:   says("Astro Team Engineering was successfully updated"),
			wantErr: "requested role is invalid. Possible values are ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN and ORGANIZATION_OWNER ",
		},
		{name: "delete", root: newOrganizationCmd, client: with(teams, deletesTeam("team-eng")), args: []string{"organization", "team", "delete", "team-eng"}, check: says("Astro Team Engineering was successfully deleted")},
		{name: "delete through the picker", root: newOrganizationCmd, client: with(teams, deletesTeam("team-eng")), answers: "1\n", args: []string{"organization", "team", "delete"}, check: says("Please select a team:", "> Astro Team Engineering was successfully deleted")},
		{name: "delete an IdP team --yes", root: newOrganizationCmd, client: with(teams, deletesTeam("team-idp")), args: []string{"organization", "team", "delete", "team-idp", "--yes"}, check: says("Astro Team Okta Ops was successfully deleted")},
		{name: "delete an IdP team declined", root: newOrganizationCmd, client: with(teams), answers: "n\n", args: []string{"organization", "team", "delete", "team-idp"}, check: says("Are you sure you want to continue the operation? (y/n)")},
		{name: "delete with no teams", root: newOrganizationCmd, client: with(noTeams), args: []string{"organization", "team", "delete"}, check: empty, wantErr: "no teams found in your organization"},
		// Pinned: the user is named by ID, and the line ends in a space.
		{name: "user add", root: newOrganizationCmd, client: with(teams, addsMember("team-eng")), args: []string{"organization", "team", "user", "add", "--team-id", "team-eng", "--user-id", "user-bob"}, check: says("Astro User user-bob was successfully added to team Engineering \n")},
		{
			name: "user add through the user picker", root: newOrganizationCmd, client: with(teams, addsMember("team-eng")), answers: "2\n",
			args:  []string{"organization", "team", "user", "add", "--team-id", "team-eng"},
			check: says("Please select the user:", "> Astro User user-bob was successfully added to team Engineering"),
		},
		{
			name: "user add through the team picker", root: newOrganizationCmd, client: with(teams, addsMember("team-eng")), answers: "1\n",
			args:  []string{"organization", "team", "user", "add", "--user-id", "user-bob"},
			check: says("Please select a team:", "> Astro User user-bob was successfully added to team Engineering"),
		},
		{name: "user add to an IdP team declined", root: newOrganizationCmd, client: with(teams), answers: "n\n", args: []string{"organization", "team", "user", "add", "--team-id", "team-idp", "--user-id", "user-bob"}, check: says("Are you sure you want to continue the operation? (y/n)")},
		{name: "user remove", root: newOrganizationCmd, client: with(teams, removesMember("team-eng", "user-ada")), args: []string{"organization", "team", "user", "remove", "--team-id", "team-eng", "--user-id", "user-ada"}, check: says("Astro User user-ada was successfully removed from team Engineering \n")},
		{
			name: "user remove through the member picker", root: newOrganizationCmd, client: with(teams, removesMember("team-eng", "user-ada")), answers: "1\n",
			args: []string{"organization", "team", "user", "remove", "--team-id", "team-eng"},
			check: func(t *testing.T, out string) {
				requireInOrder(t, out, "Please select the teamMember who's membership you'd like to modify:", "> Astro User user-ada was successfully removed from team Engineering")
				assert.Equal(t, []map[string]string{{"#": "1", "FULLNAME": "Ada Lovelace", "EMAIL": "ada@example.com", "ID": "user-ada"}}, tableRows(t, out, "#"))
			},
		},
		// Pinned: a user who is not a member is reported as a team not found.
		{name: "user remove of a non-member", root: newOrganizationCmd, client: with(teams), args: []string{"organization", "team", "user", "remove", "--team-id", "team-eng", "--user-id", "user-bob"}, check: empty, wantErr: "no team was found for the ID you provided"},
		{name: "user remove from a team with no members", root: newOrganizationCmd, client: with(noMembers), args: []string{"organization", "team", "user", "remove", "--team-id", "team-eng", "--user-id", "user-ada"}, check: empty, wantErr: "no team members found in team"},
		{name: "user list", root: newOrganizationCmd, client: with(teams), args: []string{"organization", "team", "user", "list", "--team-id", "team-eng"}, check: listsRows("ID", adaMemberRow)},
		{name: "user list empty", root: newOrganizationCmd, client: with(noMembers), args: []string{"organization", "team", "user", "list", "--team-id", "team-eng"}, check: func(t *testing.T, out string) {
			assert.Equal(t, "The selected team has no members\n", out)
		}},
		{
			name: "user list through the picker", root: newOrganizationCmd, client: with(teams), answers: "1\n",
			args: []string{"organization", "team", "user", "list"},
			check: func(t *testing.T, out string) {
				// The members' header shares the prompt's line ("> "), so
				// they are read in order rather than as a table.
				requireInOrder(t, out, "Please select a team:", "> ", "ID", "FullName", "Email", "user-ada", "Ada Lovelace", "ada@example.com")
				assert.Equal(t, teamPickRows, tableRows(t, out, "#"))
			},
		},
		// Pinned: add and update name the team by ID, remove by name.
		{name: "workspace add", root: newWorkspaceCmd, client: with(teams, setsTeamRoles("team-idp")), args: []string{"workspace", "team", "add", "team-idp", "--role", "WORKSPACE_AUTHOR"}, check: says("The team team-idp was successfully added to the workspace with the role WORKSPACE_AUTHOR")},
		{name: "workspace add with the default role", root: newWorkspaceCmd, client: with(teams, setsTeamRoles("team-idp")), args: []string{"workspace", "team", "add", "team-idp"}, check: says("The team team-idp was successfully added to the workspace with the role WORKSPACE_MEMBER")},
		{name: "workspace add through the picker", root: newWorkspaceCmd, client: with(teams, setsTeamRoles("team-idp")), answers: "2\n", args: []string{"workspace", "team", "add"}, check: says("Please select a team:", "> The team team-idp was successfully added to the workspace with the role WORKSPACE_MEMBER")},
		{name: "workspace update", root: newWorkspaceCmd, client: with(teams, setsTeamRoles("team-eng")), args: []string{"workspace", "team", "update", "team-eng", "--role", "WORKSPACE_OWNER"}, check: says("The workspace team team-eng role was successfully updated to WORKSPACE_OWNER")},
		{
			name: "workspace update with the role picked", root: newWorkspaceCmd, client: with(teams, setsTeamRoles("team-eng")), answers: "4\n",
			args: []string{"workspace", "team", "update", "team-eng"},
			check: func(t *testing.T, out string) {
				requireInOrder(t, out, "ROLE", "WORKSPACE_OWNER", "> The workspace team team-eng role was successfully updated to WORKSPACE_OWNER")
			},
		},
		{name: "workspace update through the picker", root: newWorkspaceCmd, client: with(teams, setsTeamRoles("team-eng")), answers: "1\n", args: []string{"workspace", "team", "update", "--role", "WORKSPACE_OWNER"}, check: says("Please select a team:", "> The workspace team team-eng role was successfully updated to WORKSPACE_OWNER")},
		{name: "workspace update with no teams", root: newWorkspaceCmd, client: with(noTeams), args: []string{"workspace", "team", "update", "--role", "WORKSPACE_OWNER"}, check: empty, wantErr: "no teams found in your workspace"},
		{name: "workspace remove", root: newWorkspaceCmd, client: with(teams, setsTeamRoles("team-eng")), args: []string{"workspace", "team", "remove", "team-eng"}, check: says("Astro Team Engineering was successfully removed from workspace " + curWorkspaceID)},
		{name: "workspace remove through the picker", root: newWorkspaceCmd, client: with(teams, setsTeamRoles("team-eng")), answers: "1\n", args: []string{"workspace", "team", "remove"}, check: says("Please select a team:", "> Astro Team Engineering was successfully removed from workspace "+curWorkspaceID)},
	})
}

// jsonIs checks that stdout is the one object want, every key and value: a
// key the object should not carry (a role on another object, an empty
// optional field) fails as surely as a wrong value.
func jsonIs(want map[string]any) func(t *testing.T, stdout string) {
	return func(t *testing.T, stdout string) {
		var got map[string]any
		decodeOne(t, stdout, &got)
		assert.Equal(t, want, got)
	}
}

// What the fixtures publish: each user or team with the role on the object
// the command is about, and no other.
func adaJSON(roleKey, role string) map[string]any {
	return map[string]any{"full_name": "Ada Lovelace", "email": "ada@example.com", "id": "user-ada", "created_at": "2026-01-02T03:04:05Z", roleKey: role}
}

func bobJSON(roleKey, role string) map[string]any {
	return map[string]any{"full_name": "Bob Builder", "email": "bob@example.com", "id": "user-bob", "created_at": "2026-01-02T03:04:05Z", roleKey: role}
}

func engJSON(roleKey, role string) map[string]any {
	return map[string]any{"id": "team-eng", "name": "Engineering", "description": "Builds things", "created_at": "2026-01-02T03:04:05Z", roleKey: role}
}

// What the user commands publish under --output json, and that they publish
// nothing else: stdout is the one object, stderr is empty, the exit is 0.
func TestWorkspaceOrganizationUserJSON(t *testing.T) {
	ada, bob, _, _ := userTeamFixtures()
	users := userTeamMock([]astrov1.User{ada, bob}, nil, nil)
	expires := userTeamCreated.Add(7 * 24 * time.Hour)
	inv := &astrov1.Invite{InviteId: "inv-1", OrganizationId: "test-org-id", ExpiresAt: expires}

	runJSONCases(t, []tokenCase{
		{
			name: "invite", root: newOrganizationCmd, client: with(users, invites(inv)),
			args: []string{"organization", "user", "invite", "Ada@Example.com", "--role", "ORGANIZATION_BILLING_ADMIN"},
			check: jsonIs(map[string]any{
				"invite_id": "inv-1", "email": "ada@example.com", "role": "ORGANIZATION_BILLING_ADMIN",
				"organization_id": "test-org-id", "expires_at": "2026-01-09T03:04:05Z",
			}),
		},
		{
			name: "invite of someone the API knows", root: newOrganizationCmd,
			client: with(users, invites(&astrov1.Invite{InviteId: "inv-2", OrganizationId: "test-org-id", ExpiresAt: expires, UserId: tokPtr("user-ada")})),
			args:   []string{"organization", "user", "invite", "ada@example.com"},
			check: jsonIs(map[string]any{
				"invite_id": "inv-2", "email": "ada@example.com", "role": "ORGANIZATION_MEMBER",
				"organization_id": "test-org-id", "user_id": "user-ada", "expires_at": "2026-01-09T03:04:05Z",
			}),
		},
		{name: "organization update", root: newOrganizationCmd, client: with(users, setsUserRoles("user-ada")), args: []string{"organization", "user", "update", "ada@example.com", "--role", "ORGANIZATION_OWNER"}, check: jsonIs(adaJSON("org_role", "ORGANIZATION_OWNER"))},
		{name: "workspace add", root: newWorkspaceCmd, client: with(users, setsUserRoles("user-bob")), args: []string{"workspace", "user", "add", "bob@example.com", "--role", "WORKSPACE_AUTHOR"}, check: jsonIs(bobJSON("workspace_role", "WORKSPACE_AUTHOR"))},
		{name: "workspace update", root: newWorkspaceCmd, client: with(users, setsUserRoles("user-ada")), args: []string{"workspace", "user", "update", "ada@example.com", "--role", "WORKSPACE_OWNER"}, check: jsonIs(adaJSON("workspace_role", "WORKSPACE_OWNER"))},
		{
			name: "workspace remove", root: newWorkspaceCmd, client: with(users, setsUserRoles("user-ada")),
			args:  []string{"workspace", "user", "remove", "ada@example.com"},
			check: jsonIs(map[string]any{"id": "user-ada", "email": "ada@example.com", "workspace_id": curWorkspaceID, "action": "removed"}),
		},
		{
			name: "workspace remove from a Workspace named", root: newWorkspaceCmd, client: with(users, setsUserRoles("user-ada")),
			args:  []string{"workspace", "user", "remove", "ada@example.com", "--workspace-id", "clws-other"},
			check: jsonIs(map[string]any{"id": "user-ada", "email": "ada@example.com", "workspace_id": "clws-other", "action": "removed"}),
		},
	})
}

// What the team commands publish under --output json.
func TestWorkspaceOrganizationTeamJSON(t *testing.T) {
	ada, bob, eng, idp := userTeamFixtures()
	teams := userTeamMock([]astrov1.User{ada, bob}, []astrov1.Team{eng, idp}, []astrov1.TeamMember{member(ada)})
	noMembers := userTeamMock([]astrov1.User{ada, bob}, []astrov1.Team{eng, idp}, nil)
	renamed := eng
	renamed.Name = "Platform"

	runJSONCases(t, []tokenCase{
		{name: "create", root: newOrganizationCmd, client: with(teams, createsTeam(&eng)), args: []string{"organization", "team", "create", "--name", "Engineering", "--description", "Builds things", "--role", "ORGANIZATION_MEMBER"}, check: jsonIs(engJSON("org_role", "ORGANIZATION_MEMBER"))},
		{
			name: "update is the team as it now is", root: newOrganizationCmd, client: with(teams, updatesTeam("team-eng", &renamed), setsTeamRoles("team-eng")),
			args: []string{"organization", "team", "update", "team-eng", "--name", "Platform", "--role", "ORGANIZATION_OWNER"},
			check: jsonIs(func() map[string]any {
				want := engJSON("org_role", "ORGANIZATION_OWNER")
				want["name"] = "Platform"
				return want
			}()),
		},
		{name: "update without --role keeps the role", root: newOrganizationCmd, client: with(teams, updatesTeam("team-eng", &eng)), args: []string{"organization", "team", "update", "team-eng", "--description", "d"}, check: jsonIs(engJSON("org_role", "ORGANIZATION_MEMBER"))},
		{
			name: "update of an IdP team --yes", root: newOrganizationCmd, client: with(teams, updatesTeam("team-idp", &idp)),
			args:  []string{"organization", "team", "update", "team-idp", "--description", "d", "--yes"},
			check: jsonIs(map[string]any{"id": "team-idp", "name": "Okta Ops", "created_at": "2026-01-02T03:04:05Z", "org_role": "ORGANIZATION_MEMBER", "is_idp_managed": true}),
		},
		{
			name: "delete", root: newOrganizationCmd, client: with(teams, deletesTeam("team-eng")),
			args:  []string{"organization", "team", "delete", "team-eng"},
			check: jsonIs(map[string]any{"id": "team-eng", "name": "Engineering", "organization_id": "test-org-id", "action": "deleted"}),
		},
		{
			name: "user add", root: newOrganizationCmd, client: with(teams, addsMember("team-eng")),
			args:  []string{"organization", "team", "user", "add", "--team-id", "team-eng", "--user-id", "user-bob"},
			check: jsonIs(map[string]any{"team_id": "team-eng", "team_name": "Engineering", "user_id": "user-bob", "email": "bob@example.com", "action": "added"}),
		},
		{
			name: "user add to an IdP team --yes", root: newOrganizationCmd, client: with(teams, addsMember("team-idp")),
			args:  []string{"organization", "team", "user", "add", "--team-id", "team-idp", "--user-id", "user-ada", "--yes"},
			check: jsonIs(map[string]any{"team_id": "team-idp", "team_name": "Okta Ops", "user_id": "user-ada", "email": "ada@example.com", "action": "added"}),
		},
		{
			name: "user remove", root: newOrganizationCmd, client: with(teams, removesMember("team-eng", "user-ada")),
			args:  []string{"organization", "team", "user", "remove", "--team-id", "team-eng", "--user-id", "user-ada"},
			check: jsonIs(map[string]any{"team_id": "team-eng", "team_name": "Engineering", "user_id": "user-ada", "email": "ada@example.com", "action": "removed"}),
		},
		{
			name: "user list", root: newOrganizationCmd, client: with(teams),
			args:  []string{"organization", "team", "user", "list", "--team-id", "team-eng"},
			check: jsonIs(map[string]any{"members": []any{map[string]any{"id": "user-ada", "full_name": "Ada Lovelace", "email": "ada@example.com"}}}),
		},
		{
			name: "user list empty", root: newOrganizationCmd, client: with(noMembers),
			args: []string{"organization", "team", "user", "list", "--team-id", "team-eng"},
			check: func(t *testing.T, stdout string) {
				var got map[string]any
				fields := decodeOne(t, stdout, &got)
				assert.JSONEq(t, `[]`, string(fields["members"]), "an empty array, not null and not a missing key")
			},
		},
		{
			name: "workspace add", root: newWorkspaceCmd, client: with(teams, setsTeamRoles("team-idp")),
			args:  []string{"workspace", "team", "add", "team-idp", "--role", "WORKSPACE_AUTHOR"},
			check: jsonIs(map[string]any{"id": "team-idp", "name": "Okta Ops", "created_at": "2026-01-02T03:04:05Z", "workspace_role": "WORKSPACE_AUTHOR", "is_idp_managed": true}),
		},
		{name: "workspace update", root: newWorkspaceCmd, client: with(teams, setsTeamRoles("team-eng")), args: []string{"workspace", "team", "update", "team-eng", "--role", "WORKSPACE_OWNER"}, check: jsonIs(engJSON("workspace_role", "WORKSPACE_OWNER"))},
		{
			name: "workspace remove", root: newWorkspaceCmd, client: with(teams, setsTeamRoles("team-eng")),
			args:  []string{"workspace", "team", "remove", "team-eng"},
			check: jsonIs(map[string]any{"id": "team-eng", "name": "Engineering", "workspace_id": curWorkspaceID, "action": "removed"}),
		},
	})
}

// runJSONCases runs each case with -o json, and checks that it succeeded with
// stdout the one object its check reads and nothing on stderr.
func runJSONCases(t *testing.T, cases []tokenCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := execAstroCmd(t, tc.client(t), "", tc.root, append(tc.args, "-o", "json")...)
			require.NoError(t, r.err)
			assert.Equal(t, 0, r.code)
			tc.check(t, r.stdout)
			assert.Empty(t, r.stderr)
		})
	}
}

// Under --output json a user or team command that would ask something fails
// as input_required, naming what answers it, with that object as the whole of
// stdout: no picker table, no prompt, no line introducing the choice ahead of
// it. The clients mock nothing that changes a user or a team, so a refused
// question that went on to act would panic.
func TestWorkspaceOrganizationUserTeamJSONNeverAsks(t *testing.T) {
	ada, bob, eng, idp := userTeamFixtures()
	all := func(t *testing.T) astrov1.APIClient {
		return userTeamMock([]astrov1.User{ada, bob}, []astrov1.Team{eng, idp}, []astrov1.TeamMember{member(ada)})(t)
	}
	ws, org := newWorkspaceCmd, newOrganizationCmd

	cases := []struct {
		name     string
		root     func(io.Writer) *cobra.Command
		args     []string
		answered string
	}{
		{"ws user add naming no user", ws, []string{"workspace", "user", "add", "--role", "WORKSPACE_MEMBER"}, "pass the user's email as an argument"},
		{"ws user update naming no user", ws, []string{"workspace", "user", "update", "--role", "WORKSPACE_MEMBER"}, "pass the user's email as an argument"},
		{"ws user update naming no role", ws, []string{"workspace", "user", "update", "ada@example.com"}, "pass --role"},
		{"ws user remove naming no user", ws, []string{"workspace", "user", "remove"}, "pass the user's email as an argument"},
		{"org user update naming no user", org, []string{"organization", "user", "update", "--role", "ORGANIZATION_OWNER"}, "pass the user's email as an argument"},
		{"org user update naming no role", org, []string{"organization", "user", "update", "ada@example.com"}, "pass --role"},
		{"org user invite naming no email", org, []string{"organization", "user", "invite"}, "pass the email address as an argument"},
		{"org team create naming no role", org, []string{"organization", "team", "create", "--name", "n"}, "pass --role"},
		{"org team create naming no name", org, []string{"organization", "team", "create", "--role", "ORGANIZATION_MEMBER"}, "pass --name"},
		{"org team update naming no team", org, []string{"organization", "team", "update", "--description", "d"}, "pass the team ID as an argument"},
		{"org team update of an IdP team without --yes", org, []string{"organization", "team", "update", "team-idp", "--description", "d"}, "pass --yes"},
		{"org team delete naming no team", org, []string{"organization", "team", "delete"}, "pass the team ID as an argument"},
		{"org team delete of an IdP team without --yes", org, []string{"organization", "team", "delete", "team-idp"}, "pass --yes"},
		{"org team user add naming no team", org, []string{"organization", "team", "user", "add", "--user-id", "user-bob"}, "pass --team-id"},
		{"org team user add naming no user", org, []string{"organization", "team", "user", "add", "--team-id", "team-eng"}, "pass --user-id"},
		{"org team user add to an IdP team without --yes", org, []string{"organization", "team", "user", "add", "--team-id", "team-idp", "--user-id", "user-bob"}, "pass --yes"},
		{"org team user remove naming no team", org, []string{"organization", "team", "user", "remove", "--user-id", "user-ada"}, "pass --team-id"},
		{"org team user remove naming no user", org, []string{"organization", "team", "user", "remove", "--team-id", "team-eng"}, "pass --user-id"},
		{"org team user list naming no team", org, []string{"organization", "team", "user", "list"}, "pass --team-id"},
		{"ws team add naming no team", ws, []string{"workspace", "team", "add", "--role", "WORKSPACE_MEMBER"}, "pass the team ID as an argument"},
		{"ws team update naming no team", ws, []string{"workspace", "team", "update", "--role", "WORKSPACE_MEMBER"}, "pass the team ID as an argument"},
		{"ws team update naming no role", ws, []string{"workspace", "team", "update", "team-eng"}, "pass --role"},
		{"ws team remove naming no team", ws, []string{"workspace", "team", "remove"}, "pass the team ID as an argument"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// An answer is waiting, so a prompt that did read would go on.
			r := execAstroCmd(t, all(t), "y\n1\n", tc.root, append(tc.args, "-o", "json")...)
			require.Error(t, r.err)
			assert.Equal(t, cliout.ExitFailure, r.code)
			var got errorJSON
			decodeOne(t, r.stdout, &got)
			assert.Equal(t, string(cliout.KindInputRequired), got.Kind)
			assert.Equal(t, cliout.ExitFailure, got.Code, "the code it reports is the one it exits with")
			assert.Contains(t, got.Error, tc.answered)
			assert.Empty(t, r.stderr)
		})
	}
}

// A team update whose role is refused after the rename went through says so
// in text, the rename's line and then the error, as it always has (pinned in
// TestWorkspaceOrganizationTeamText). Under --output json stdout is the error
// object alone: no line ahead of it.
func TestOrganizationTeamUpdateRefusedRoleJSON(t *testing.T) {
	ada, bob, eng, idp := userTeamFixtures()
	renamed := eng
	renamed.Name = "Platform"
	client := with(userTeamMock([]astrov1.User{ada, bob}, []astrov1.Team{eng, idp}, nil), updatesTeam("team-eng", &renamed))
	r := execAstroCmd(t, client(t), "", newOrganizationCmd, "organization", "team", "update", "team-eng", "--name", "Platform", "--role", "NOPE", "-o", "json")
	require.Error(t, r.err)
	assert.Equal(t, cliout.ExitFailure, r.code)
	var got errorJSON
	decodeOne(t, r.stdout, &got)
	assert.Contains(t, got.Error, "requested role is invalid")
	assert.Empty(t, r.stderr)
}

// --output takes text or json on every user and team command: anything else
// is a usage error, exit 2, before anything is asked or done.
func TestWorkspaceOrganizationUserTeamOutputUsage(t *testing.T) {
	for _, run := range []struct {
		root func(io.Writer) *cobra.Command
		args []string
	}{
		{newWorkspaceCmd, []string{"workspace", "user", "add", "ada@example.com", "-o", "yaml"}},
		{newWorkspaceCmd, []string{"workspace", "user", "list", "-o", "yaml"}},
		{newWorkspaceCmd, []string{"workspace", "team", "remove", "team-eng", "-o", "yaml"}},
		{newOrganizationCmd, []string{"organization", "user", "invite", "ada@example.com", "-o", "yaml"}},
		{newOrganizationCmd, []string{"organization", "team", "create", "-o", "yaml"}},
		{newOrganizationCmd, []string{"organization", "team", "user", "list", "--team-id", "team-eng", "-o", "yaml"}},
	} {
		// A client that answers nothing: a usage error makes no request.
		r := execAstroCmd(t, new(astrov1_mocks.ClientWithResponsesInterface), "", run.root, run.args...)
		require.Error(t, r.err, run.args)
		assert.Equal(t, cliout.ExitUsage, r.code, run.args)
	}
}

// Help text this change leaves as it was, pinned so that fixing it is a
// visible change: each is a follow-up, listed in the PR.
func TestUserTeamHelpQuirksArePinned(t *testing.T) {
	find := func(root func(io.Writer) *cobra.Command, path ...string) *cobra.Command {
		t.Helper()
		c, _, err := root(io.Discard).Find(path)
		require.NoError(t, err)
		return c
	}
	// The team's role is called the token's.
	assert.Contains(t, find(newOrganizationCmd, "team", "create").Flag("role").Usage, "The role for the token.")
	// The usage lines name a top-level `astro user` that does not exist.
	assert.Contains(t, find(newOrganizationCmd, "user", "invite").Long, "$astro user invite [email]")
	assert.Contains(t, find(newOrganizationCmd, "user", "update").Long, "$astro user update [email]")
	assert.Equal(t, "Update the role of a user your in Astro Organization", find(newOrganizationCmd, "user", "update").Short)
	// update and remove act on the current Workspace only; add and the user
	// commands take --workspace.
	assert.Nil(t, find(newWorkspaceCmd, "team", "update").Flag("workspace"))
	assert.Nil(t, find(newWorkspaceCmd, "team", "remove").Flag("workspace"))
	assert.NotNil(t, find(newWorkspaceCmd, "team", "add").Flag("workspace"))
}
