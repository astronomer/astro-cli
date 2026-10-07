package astro

import (
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
)

// The user and team commands of `astro deployment`, in both formats. They
// publish what the workspace and organization user and team commands publish
// (workspace_org_user_team_json_test.go): the one user.UserInfo or
// team.TeamInfo with its deployment_role set, and a removal naming the
// Deployment. As there, these tests decode what a run printed and assert what
// it means, and in text they assert the messages and each picker cell under
// its header, not the padding.

// deploymentUserTeamFixtures is userTeamFixtures with Ada and the Engineering
// team holding a role on the Deployment the tests act on, so the pickers that
// list the Deployment's users and teams have something to show.
func deploymentUserTeamFixtures() (ada, bob astrov1.User, eng, idp astrov1.Team) {
	ada, bob, eng, idp = userTeamFixtures()
	ada.DeploymentRoles = &[]astrov1.DeploymentRole{{DeploymentId: tokDeploymentID, Role: "DEPLOYMENT_ADMIN"}}
	eng.DeploymentRoles = &[]astrov1.DeploymentRole{{DeploymentId: tokDeploymentID, Role: "DEPLOYMENT_ADMIN"}}
	return ada, bob, eng, idp
}

// refusesUserRoles and refusesTeamRoles answer the role change with the 403
// an API refusal is.
func refusesUserRoles(m *astrov1_mocks.ClientWithResponsesInterface) {
	m.On("UpdateUserRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.UpdateUserRolesResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusForbidden}, Body: []byte(`{"message":"you may not grant that role"}`),
	}, nil)
}

func refusesTeamRoles(m *astrov1_mocks.ClientWithResponsesInterface) {
	m.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.UpdateTeamRolesResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusForbidden}, Body: []byte(`{"message":"you may not grant that role"}`),
	}, nil)
}

// deploymentUserTeamTextCases are the runs whose text is pinned: every
// command, by argument, through each picker and prompt, and the ways each
// fails.
func deploymentUserTeamTextCases() []tokenCase {
	ada, bob, eng, idp := deploymentUserTeamFixtures()
	all := userTeamMock([]astrov1.User{ada, bob}, []astrov1.Team{eng, idp}, nil)
	none := userTeamMock(nil, nil, nil)
	d := "--deployment=" + tokDeploymentID
	dep := newDeploymentRootCmd
	empty := func(t *testing.T, out string) { assert.Empty(t, out) }
	exactly := func(want string) func(t *testing.T, out string) {
		return func(t *testing.T, out string) { assert.Equal(t, want, out) }
	}
	// The pickers' rows: the Organization's users by their Organization role
	// for an add, and the Deployment's users by their role on it for an
	// update or a remove: each user's role on this Deployment, empty for one
	// that holds none there.
	depPickRows := []map[string]string{
		userPickRow("1", "Ada Lovelace", "ada@example.com", "user-ada", "DEPLOYMENT ROLE", "DEPLOYMENT_ADMIN"),
		userPickRow("2", "Bob Builder", "bob@example.com", "user-bob", "DEPLOYMENT ROLE", ""),
	}
	addAda := "The user ada@example.com was successfully added to the deployment with the role DEPLOYMENT_ADMIN\n"

	return []tokenCase{
		{name: "user add", root: dep, client: with(all, setsUserRoles("user-bob")), args: []string{"deployment", "user", "add", "bob@example.com", "--role", "DEPLOYMENT_AUTHOR", d}, check: exactly("The user bob@example.com was successfully added to the deployment with the role DEPLOYMENT_AUTHOR\n")},
		{name: "user add with the default role", root: dep, client: with(all, setsUserRoles("user-ada")), args: []string{"deployment", "user", "add", "Ada@Example.com", d}, check: exactly(addAda)},
		{name: "user add with --deployment-id", root: dep, client: with(all, setsUserRoles("user-ada")), args: []string{"deployment", "user", "add", "ada@example.com", "--deployment-id", tokDeploymentID}, check: exactly(addAda)},
		{
			name: "user add through the picker", root: dep, client: with(all, setsUserRoles("user-bob")), answers: "2\n",
			args: []string{"deployment", "user", "add", d},
			check: func(t *testing.T, out string) {
				requireInOrder(t, out, "Please select the user:", "> The user bob@example.com was successfully added to the deployment with the role DEPLOYMENT_ADMIN\n")
				assert.Equal(t, []map[string]string{adaOrgPickRow, bobOrgPickRow}, tableRows(t, out, "#"))
			},
		},
		{name: "user add through the picker, a bad choice", root: dep, client: with(all), answers: "9\n", args: []string{"deployment", "user", "add", d}, check: says("Please select the user:", "> "), wantErr: "invalid User selected"},
		{name: "user add of nobody", root: dep, client: with(all), args: []string{"deployment", "user", "add", "nobody@example.com", d}, check: empty, wantErr: "no user was found for the email you provided"},
		{name: "user add the API refuses", root: dep, client: with(all, refusesUserRoles), args: []string{"deployment", "user", "add", "ada@example.com", d}, check: empty, wantErr: "you may not grant that role"},
		{name: "user update", root: dep, client: with(all, setsUserRoles("user-ada")), args: []string{"deployment", "user", "update", "ada@example.com", "--role", "custom-role", d}, check: exactly("The deployment user ada@example.com role was successfully updated to custom-role\n")},
		{name: "user update with the role asked", root: dep, client: with(all, setsUserRoles("user-ada")), answers: "custom-role\n", args: []string{"deployment", "user", "update", "ada@example.com", d}, check: exactly("Enter a user Deployment role or custom role name to update user: The deployment user ada@example.com role was successfully updated to custom-role\n")},
		{
			name: "user update through the picker", root: dep, client: with(all, setsUserRoles("user-ada")), answers: "1\n",
			args: []string{"deployment", "user", "update", "--role", "custom-role", d},
			check: func(t *testing.T, out string) {
				requireInOrder(t, out, "Please select the user:", "> The deployment user ada@example.com role was successfully updated to custom-role\n")
				assert.Equal(t, depPickRows, tableRows(t, out, "#"))
			},
		},
		{name: "user update of nobody", root: dep, client: with(all), args: []string{"deployment", "user", "update", "nobody@example.com", "--role", "custom-role", d}, check: empty, wantErr: "no user was found for the email you provided"},
		{name: "user remove", root: dep, client: with(all, setsUserRoles("user-ada")), args: []string{"deployment", "user", "remove", "ada@example.com", d}, check: exactly("The user ada@example.com was successfully removed from the deployment\n")},
		{
			name: "user remove through the picker", root: dep, client: with(all, setsUserRoles("user-ada")), answers: "1\n",
			args: []string{"deployment", "user", "remove", d},
			check: func(t *testing.T, out string) {
				requireInOrder(t, out, "Please select the user:", "> The user ada@example.com was successfully removed from the deployment\n")
				assert.Equal(t, depPickRows, tableRows(t, out, "#"))
			},
		},
		{name: "user remove of nobody", root: dep, client: with(all), args: []string{"deployment", "user", "remove", "nobody@example.com", d}, check: empty, wantErr: "no user was found for the email you provided"},
		{name: "user remove the API refuses", root: dep, client: with(all, refusesUserRoles), args: []string{"deployment", "user", "remove", "ada@example.com", d}, check: empty, wantErr: "you may not grant that role"},

		// The team add and update lines name the team by ID, and remove by
		// name, as the workspace team commands did before #467 fixed them.
		{name: "team add", root: dep, client: with(all, setsTeamRoles("team-eng")), args: []string{"deployment", "team", "add", "team-eng", "--role", "custom-role", d}, check: exactly("The team team-eng was successfully added to the deployment with the role custom-role\n")},
		{name: "team add with the default role", root: dep, client: with(all, setsTeamRoles("team-idp")), args: []string{"deployment", "team", "add", "team-idp", d}, check: exactly("The team team-idp was successfully added to the deployment with the role DEPLOYMENT_ADMIN\n")},
		{name: "team add with -w", root: dep, client: with(all, setsTeamRoles("team-idp")), args: []string{"deployment", "team", "add", "team-idp", "-w", tokDeploymentID}, check: exactly("The team team-idp was successfully added to the deployment with the role DEPLOYMENT_ADMIN\n")},
		{
			name: "team add through the picker", root: dep, client: with(all, setsTeamRoles("team-idp")), answers: "2\n",
			args: []string{"deployment", "team", "add", d},
			check: func(t *testing.T, out string) {
				requireInOrder(t, out, "Please select a team:", "> The team team-idp was successfully added to the deployment with the role DEPLOYMENT_ADMIN\n")
				assert.Equal(t, teamPickRows, tableRows(t, out, "#"))
			},
		},
		{name: "team add through the picker, a bad choice", root: dep, client: with(all), answers: "9\n", args: []string{"deployment", "team", "add", d}, check: says("Please select a team:", "> "), wantErr: "invalid team selection"},
		{name: "team add with no teams", root: dep, client: with(none), args: []string{"deployment", "team", "add", d}, check: empty, wantErr: "no teams found in your organization"},
		{name: "team add the API refuses", root: dep, client: with(all, refusesTeamRoles), args: []string{"deployment", "team", "add", "team-eng", d}, check: empty, wantErr: "you may not grant that role"},
		{name: "team update", root: dep, client: with(all, setsTeamRoles("team-eng")), args: []string{"deployment", "team", "update", "team-eng", "--role", "custom-role", d}, check: exactly("The deployment team team-eng role was successfully updated to custom-role\n")},
		{name: "team update with the role asked", root: dep, client: with(all, setsTeamRoles("team-eng")), answers: "custom-role\n", args: []string{"deployment", "team", "update", "team-eng", d}, check: exactly("Enter a Deployment role or custom role name to update team: The deployment team team-eng role was successfully updated to custom-role\n")},
		{
			name: "team update through the picker", root: dep, client: with(all, setsTeamRoles("team-eng")), answers: "1\n",
			args: []string{"deployment", "team", "update", "--role", "custom-role", d},
			check: func(t *testing.T, out string) {
				requireInOrder(t, out, "Please select a team:", "> The deployment team team-eng role was successfully updated to custom-role\n")
				assert.Equal(t, teamPickRows, tableRows(t, out, "#"))
			},
		},
		{name: "team update through the picker, a bad choice", root: dep, client: with(all), answers: "9\n", args: []string{"deployment", "team", "update", "--role", "custom-role", d}, check: says("Please select a team:", "> "), wantErr: "invalid team selection"},
		{name: "team update with no teams", root: dep, client: with(none), args: []string{"deployment", "team", "update", "--role", "custom-role", d}, check: empty, wantErr: "no teams found in your deployment"},
		{name: "team remove", root: dep, client: with(all, setsTeamRoles("team-eng")), args: []string{"deployment", "team", "remove", "team-eng", d}, check: exactly("Astro Team Engineering was successfully removed from deployment " + tokDeploymentID + "\n")},
		{
			name: "team remove through the picker", root: dep, client: with(all, setsTeamRoles("team-eng")), answers: "1\n",
			args:  []string{"deployment", "team", "remove", d},
			check: says("Please select a team:", "> Astro Team Engineering was successfully removed from deployment "+tokDeploymentID+"\n"),
		},
		{name: "team remove with no teams", root: dep, client: with(none), args: []string{"deployment", "team", "remove", d}, check: empty, wantErr: "no teams found in your deployment"},
		{name: "user add naming no Deployment", root: dep, client: with(all), args: []string{"deployment", "user", "add", "ada@example.com"}, check: empty, wantErr: "required flag --deployment not set. To find valid values, run: astro deployment list"},
		{name: "team remove naming no Deployment", root: dep, client: with(all), args: []string{"deployment", "team", "remove", "team-eng"}, check: empty, wantErr: "required flag --deployment not set. To find valid values, run: astro deployment list"},
		{name: "team remove the API refuses", root: dep, client: with(all, refusesTeamRoles), args: []string{"deployment", "team", "remove", "team-eng", d}, check: empty, wantErr: "you may not grant that role"},
	}
}

// What the commands print in text: what they printed before they gained
// --output, recorded against v2 byte for byte and checked here by meaning.
func TestDeploymentUserTeamText(t *testing.T) {
	runTokenCases(t, deploymentUserTeamTextCases())
}

// What the commands publish under --output json, and that they publish
// nothing else: stdout is the one object, stderr is empty, the exit is 0.
func TestDeploymentUserTeamJSON(t *testing.T) {
	ada, bob, eng, idp := deploymentUserTeamFixtures()
	all := userTeamMock([]astrov1.User{ada, bob}, []astrov1.Team{eng, idp}, nil)
	d := "--deployment=" + tokDeploymentID
	dep := newDeploymentRootCmd
	userRemoval := map[string]any{"id": "user-ada", "email": "ada@example.com", "deployment_id": tokDeploymentID, "action": "removed"}

	runJSONCases(t, []tokenCase{
		{name: "user add", root: dep, client: with(all, setsUserRoles("user-bob")), args: []string{"deployment", "user", "add", "bob@example.com", "--role", "DEPLOYMENT_AUTHOR", d}, check: jsonIs(bobJSON("deployment_role", "DEPLOYMENT_AUTHOR"))},
		{name: "user add with the default role", root: dep, client: with(all, setsUserRoles("user-ada")), args: []string{"deployment", "user", "add", "Ada@Example.com", d}, check: jsonIs(adaJSON("deployment_role", "DEPLOYMENT_ADMIN"))},
		{name: "user update", root: dep, client: with(all, setsUserRoles("user-ada")), args: []string{"deployment", "user", "update", "ada@example.com", "--role", "custom-role", d}, check: jsonIs(adaJSON("deployment_role", "custom-role"))},
		{name: "user remove", root: dep, client: with(all, setsUserRoles("user-ada")), args: []string{"deployment", "user", "remove", "ada@example.com", d}, check: jsonIs(userRemoval)},
		{name: "user remove with --deployment-id", root: dep, client: with(all, setsUserRoles("user-ada")), args: []string{"deployment", "user", "remove", "ada@example.com", "--deployment-id", tokDeploymentID}, check: jsonIs(userRemoval)},
		{name: "team add", root: dep, client: with(all, setsTeamRoles("team-eng")), args: []string{"deployment", "team", "add", "team-eng", "--role", "custom-role", d}, check: jsonIs(engJSON("deployment_role", "custom-role"))},
		{
			name: "team add of an IdP team with the default role", root: dep, client: with(all, setsTeamRoles("team-idp")),
			args:  []string{"deployment", "team", "add", "team-idp", d},
			check: jsonIs(map[string]any{"id": "team-idp", "name": "Okta Ops", "created_at": "2026-01-02T03:04:05Z", "deployment_role": "DEPLOYMENT_ADMIN", "is_idp_managed": true}),
		},
		{name: "team update", root: dep, client: with(all, setsTeamRoles("team-eng")), args: []string{"deployment", "team", "update", "team-eng", "--role", "custom-role", d}, check: jsonIs(engJSON("deployment_role", "custom-role"))},
		{
			name: "team remove", root: dep, client: with(all, setsTeamRoles("team-eng")),
			args:  []string{"deployment", "team", "remove", "team-eng", d},
			check: jsonIs(map[string]any{"id": "team-eng", "name": "Engineering", "deployment_id": tokDeploymentID, "action": "removed"}),
		},
	})
}

// A failure under --output json is the error object alone, with the code it
// exits with. (Its kind comes from the root's table, which this package's
// harness does not install.)
func TestDeploymentUserTeamJSONFailures(t *testing.T) {
	ada, bob, eng, idp := deploymentUserTeamFixtures()
	all := userTeamMock([]astrov1.User{ada, bob}, []astrov1.Team{eng, idp}, nil)
	d := "--deployment=" + tokDeploymentID
	for _, tc := range []struct {
		name   string
		client func(t *testing.T) astrov1.APIClient
		args   []string
		want   string
	}{
		{"user add of nobody", with(all), []string{"deployment", "user", "add", "nobody@example.com", d}, "no user was found for the email you provided"},
		{"user update the API refuses", with(all, refusesUserRoles), []string{"deployment", "user", "update", "ada@example.com", "--role", "custom-role", d}, "you may not grant that role"},
		{"team remove the API refuses", with(all, refusesTeamRoles), []string{"deployment", "team", "remove", "team-eng", d}, "you may not grant that role"},
		{"team add naming no Deployment", with(all), []string{"deployment", "team", "add", "team-eng"}, "required flag --deployment not set"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := execAstroCmd(t, tc.client(t), "", newDeploymentRootCmd, append(tc.args, "-o", "json")...)
			require.Error(t, r.err)
			assert.Equal(t, cliout.ExitFailure, r.code)
			var got errorJSON
			decodeOne(t, r.stdout, &got)
			assert.Contains(t, got.Error, tc.want)
			assert.Equal(t, cliout.ExitFailure, got.Code)
			assert.Empty(t, r.stderr)
		})
	}
}

// Under --output json a command that would ask something fails as
// input_required, naming what answers it, with that object as the whole of
// stdout. The clients mock nothing that changes a user or a team, so a
// refused question that went on to act would panic.
func TestDeploymentUserTeamJSONNeverAsks(t *testing.T) {
	ada, bob, eng, idp := deploymentUserTeamFixtures()
	d := "--deployment=" + tokDeploymentID
	for _, tc := range []struct {
		name     string
		args     []string
		answered string
	}{
		{"user add naming no user", []string{"deployment", "user", "add", d}, "pass the user's email as an argument"},
		{"user update naming no user", []string{"deployment", "user", "update", "--role", "custom-role", d}, "pass the user's email as an argument"},
		{"user update naming no role", []string{"deployment", "user", "update", "ada@example.com", d}, "pass --role"},
		{"user remove naming no user", []string{"deployment", "user", "remove", d}, "pass the user's email as an argument"},
		{"team add naming no team", []string{"deployment", "team", "add", d}, "pass the team ID as an argument"},
		{"team update naming no team", []string{"deployment", "team", "update", "--role", "custom-role", d}, "pass the team ID as an argument"},
		{"team update naming no role", []string{"deployment", "team", "update", "team-eng", d}, "pass --role"},
		{"team remove naming no team", []string{"deployment", "team", "remove", d}, "pass the team ID as an argument"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := userTeamMock([]astrov1.User{ada, bob}, []astrov1.Team{eng, idp}, nil)(t)
			// An answer is waiting, so a prompt that did read would go on.
			r := execAstroCmd(t, m, "1\n", newDeploymentRootCmd, append(tc.args, "-o", "json")...)
			require.Error(t, r.err)
			assert.Equal(t, cliout.ExitFailure, r.code)
			var got errorJSON
			decodeOne(t, r.stdout, &got)
			assert.Equal(t, string(cliout.KindInputRequired), got.Kind)
			assert.Equal(t, cliout.ExitFailure, got.Code, "the code it reports is the one it exits with")
			assert.Contains(t, got.Error, tc.answered)
			assert.Empty(t, r.stderr)
			assert.Empty(t, m.Calls, "refused before anything is fetched")
		})
	}
}

// -o is registered once on each group, so every command in it takes text or
// json, and anything else is a usage error, exit 2, before any request.
func TestDeploymentUserTeamOutputUsage(t *testing.T) {
	d := "--deployment=" + tokDeploymentID
	for _, args := range [][]string{
		{"deployment", "user", "add", "ada@example.com", d, "-o", "yaml"},
		{"deployment", "user", "list", d, "-o", "yaml"},
		{"deployment", "user", "update", "ada@example.com", "--role", "r", d, "-o", "yaml"},
		{"deployment", "user", "remove", "ada@example.com", d, "-o", "yaml"},
		{"deployment", "team", "add", "team-eng", d, "-o", "yaml"},
		{"deployment", "team", "list", d, "-o", "yaml"},
		{"deployment", "team", "update", "team-eng", "--role", "r", d, "-o", "yaml"},
		{"deployment", "team", "remove", "team-eng", d, "-o", "yaml"},
	} {
		m := new(astrov1_mocks.ClientWithResponsesInterface)
		r := execAstroCmd(t, m, "", newDeploymentRootCmd, args...)
		require.Error(t, r.err, args)
		assert.Equal(t, cliout.ExitUsage, r.code, args)
		assert.Empty(t, m.Calls, args)
	}
	for _, group := range []string{"user", "team"} {
		c, _, err := newDeploymentRootCmd(io.Discard).Find([]string{group})
		require.NoError(t, err)
		assert.NotNil(t, c.PersistentFlags().Lookup("output"), group)
		for _, sub := range c.Commands() {
			assert.Nil(t, sub.LocalNonPersistentFlags().Lookup("output"), "%s %s registers its own -o", group, sub.Name())
		}
	}
}
