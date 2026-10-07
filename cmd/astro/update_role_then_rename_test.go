package astro

import (
	"io"
	"net/http"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
)

// The updates that change a role and a name (`organization team update` and
// the three token families' `update`) send the role first, so a refused role
// leaves the object as it was. Two things follow, checked here for each:
//   - the name and description are sent only when one was given, so a
//     role-only update is one call that changes anything, and cannot fail
//     after the role went through;
//   - when they are given and fail after the role went through, the error
//     says the role changed, in text and as the one error object in json.

// nameTaken is the API refusing a rename.
var nameTaken = struct {
	resp *http.Response
	body []byte
}{&http.Response{StatusCode: http.StatusConflict}, []byte(`{"message":"name taken"}`)}

type roleThenRename struct {
	name   string
	root   func(io.Writer) *cobra.Command
	update []string // the update, naming the object and its scope
	role   string   // a role the object does not hold
	client func(t *testing.T) *astrov1_mocks.ClientWithResponsesInterface
	// the calls that change the role, and the name and description
	roleCall, renameCall string
	// renameFails makes the rename call fail as nameTaken
	renameFails func(m *astrov1_mocks.ClientWithResponsesInterface)
	// renameOK lets the rename call through, as Maybe, so that a regression
	// fails its assertion rather than panicking the package
	renameOK func(m *astrov1_mocks.ClientWithResponsesInterface)
	partial  string
}

func roleThenRenameCases() []roleThenRename {
	ada, bob, eng, idp := userTeamFixtures()
	dep, _, _ := tokenFixtures()
	ws, org := jsonWsOrgTokenFixtures()
	tokenRenameFails := func(m *astrov1_mocks.ClientWithResponsesInterface) {
		m.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.UpdateApiTokenResponse{HTTPResponse: nameTaken.resp, Body: nameTaken.body}, nil)
	}
	tokenRenameOK := func(tok astrov1.ApiToken) func(m *astrov1_mocks.ClientWithResponsesInterface) {
		return func(m *astrov1_mocks.ClientWithResponsesInterface) {
			m.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.UpdateApiTokenResponse{HTTPResponse: ok200(), JSON200: &tok}, nil).Maybe()
		}
	}
	tokens := func(tok astrov1.ApiToken) func(t *testing.T) *astrov1_mocks.ClientWithResponsesInterface {
		return func(t *testing.T) *astrov1_mocks.ClientWithResponsesInterface {
			m := tokenMock(t, tok)
			m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, tok.Id, mock.Anything).Return(rolesOK(), nil)
			return m
		}
	}
	tokenPartial := func(role string) string {
		return "the token's role was updated to " + role + ", but updating its name and description failed: name taken"
	}
	return []roleThenRename{
		{
			name: "organization team", root: newOrganizationCmd, update: []string{"organization", "team", "update", "team-eng"}, role: "ORGANIZATION_OWNER",
			client: func(t *testing.T) *astrov1_mocks.ClientWithResponsesInterface {
				m := userTeamMock([]astrov1.User{ada, bob}, []astrov1.Team{eng, idp}, nil)(t)
				setsTeamRoles("team-eng")(m)
				return m
			},
			roleCall: "UpdateTeamRolesWithResponse", renameCall: "UpdateTeamWithResponse",
			renameFails: func(m *astrov1_mocks.ClientWithResponsesInterface) {
				m.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, "team-eng", mock.Anything).Return(&astrov1.UpdateTeamResponse{HTTPResponse: nameTaken.resp, Body: nameTaken.body}, nil)
			},
			renameOK: func(m *astrov1_mocks.ClientWithResponsesInterface) {
				m.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, "team-eng", mock.Anything).Return(&astrov1.UpdateTeamResponse{HTTPResponse: ok200(), JSON200: &eng}, nil).Maybe()
			},
			partial: "the team's role was updated to ORGANIZATION_OWNER, but updating its name and description failed: name taken",
		},
		{
			name: "deployment token", root: newDeploymentRootCmd, update: []string{"deployment", "token", "update", "tok-dep", "--deployment=" + tokDeploymentID}, role: "custom-role",
			client: tokens(dep), roleCall: "UpdateApiTokenRolesWithResponse", renameCall: "UpdateApiTokenWithResponse", renameFails: tokenRenameFails, renameOK: tokenRenameOK(dep), partial: tokenPartial("custom-role"),
		},
		{
			name: "workspace token", root: newWorkspaceCmd, update: []string{"workspace", "token", "update", "tok-ws"}, role: "WORKSPACE_OWNER",
			client: tokens(ws), roleCall: "UpdateApiTokenRolesWithResponse", renameCall: "UpdateApiTokenWithResponse", renameFails: tokenRenameFails, renameOK: tokenRenameOK(ws), partial: tokenPartial("WORKSPACE_OWNER"),
		},
		{
			name: "organization token", root: newOrganizationCmd, update: []string{"organization", "token", "update", "tok-org"}, role: "ORGANIZATION_OWNER",
			client: tokens(org), roleCall: "UpdateApiTokenRolesWithResponse", renameCall: "UpdateApiTokenWithResponse", renameFails: tokenRenameFails, renameOK: tokenRenameOK(org), partial: tokenPartial("ORGANIZATION_OWNER"),
		},
	}
}

func TestUpdateSendsTheRoleThenTheRename(t *testing.T) {
	for _, c := range roleThenRenameCases() {
		for _, format := range []string{"text", "json"} {
			t.Run(c.name+"/a role-only update is one call/"+format, func(t *testing.T) {
				m := c.client(t)
				c.renameOK(m)
				r := execAstroCmd(t, m, "", c.root, append(c.update, "--role", c.role, "-o", format)...)
				require.NoError(t, r.err)
				assert.Equal(t, 0, r.code)
				m.AssertNumberOfCalls(t, c.roleCall, 1)
				m.AssertNotCalled(t, c.renameCall, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			})

			t.Run(c.name+"/a rename that fails after the role says the role changed/"+format, func(t *testing.T) {
				m := c.client(t)
				c.renameFails(m)
				r := execAstroCmd(t, m, "", c.root, append(c.update, "--role", c.role, "--description", "new words", "-o", format)...)
				require.Error(t, r.err)
				assert.Equal(t, c.partial, r.err.Error())
				assert.Equal(t, cliout.ExitFailure, r.code)
				m.AssertNumberOfCalls(t, c.roleCall, 1)
				m.AssertNumberOfCalls(t, c.renameCall, 1)
				if format == "text" {
					assert.Empty(t, r.stdout)
					return
				}
				var got errorJSON
				decodeOne(t, r.stdout, &got)
				assert.Equal(t, c.partial, got.Error)
				assert.Equal(t, cliout.ExitFailure, got.Code)
				assert.Empty(t, r.stderr)
			})

			t.Run(c.name+"/a rename without a role is one call/"+format, func(t *testing.T) {
				m := c.client(t)
				c.renameFails(m)
				r := execAstroCmd(t, m, "", c.root, append(c.update, "--description", "new words", "-o", format)...)
				require.Error(t, r.err)
				assert.Contains(t, r.err.Error(), "name taken")
				assert.NotContains(t, r.err.Error(), "role was updated", "no role was changed")
				m.AssertNotCalled(t, c.roleCall, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			})
		}
	}
}
