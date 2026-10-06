package astro

import (
	"encoding/json"
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

// What the `astro workspace token` and `astro organization token` families
// do, in both formats. They publish the shapes `astro deployment token` does
// (apitoken, pinned by the deployment-token*.json goldens), plus a removal
// naming the Workspace or the Organization and the roles of an Organization
// token (workspace-token-removal.json, organization-token-*.json). As in
// deployment_token_json_test.go, these tests decode what a run printed and
// assert what it means, and in text they assert the messages in order and
// each table cell under its header, not the padding. --clean-output alone is
// pinned byte for byte, because a script captures it whole.

// curWorkspaceID is the current Workspace of the test config, which these
// families use when no --workspace-id is given.
const curWorkspaceID = "ck05r3bor07h40d02y2hw4n4v"

// wsOrgTokenFixtures builds a Workspace token and an Organization token that
// also holds a role on the current Workspace, created at times whose "ago"
// rendering is stable for the length of a test run.
func wsOrgTokenFixtures() (ws, org astrov1.ApiToken) {
	now := time.Now()
	ws = astrov1.ApiToken{
		Id: "tok-ws", Name: "ws-token", Description: "Workspace CI",
		Scope:     astrov1.ApiTokenScopeWORKSPACE,
		Roles:     &[]astrov1.ApiTokenRole{{EntityType: astrov1.ApiTokenRoleEntityTypeWORKSPACE, EntityId: curWorkspaceID, Role: "WORKSPACE_MEMBER"}},
		CreatedAt: now.Add(-50 * time.Hour),
		CreatedBy: &astrov1.BasicSubjectProfile{FullName: tokPtr("Ada Lovelace")},
	}
	org = astrov1.ApiToken{
		Id: "tok-org", Name: "org-token", Description: "Org wide",
		Scope: astrov1.ApiTokenScopeORGANIZATION,
		Roles: &[]astrov1.ApiTokenRole{
			{EntityType: astrov1.ApiTokenRoleEntityTypeORGANIZATION, EntityId: "test-org-id", Role: "ORGANIZATION_MEMBER"},
			{EntityType: astrov1.ApiTokenRoleEntityTypeWORKSPACE, EntityId: curWorkspaceID, Role: "WORKSPACE_OPERATOR"},
		},
		ExpiryPeriodInDays: tokPtr(30),
		CreatedAt:          now.Add(-3*time.Hour - time.Minute),
		CreatedBy:          &astrov1.BasicSubjectProfile{ApiTokenName: tokPtr("bootstrap")},
	}
	return ws, org
}

// orgOnly is org with no role but its Organization one, as an Organization
// token is before it is added to a Workspace or a Deployment.
func orgOnly(org astrov1.ApiToken) astrov1.ApiToken { //nolint:gocritic // a test fixture
	org.Roles = &[]astrov1.ApiTokenRole{{EntityType: astrov1.ApiTokenRoleEntityTypeORGANIZATION, EntityId: "test-org-id", Role: "ORGANIZATION_MEMBER"}}
	return org
}

// fakeTokens is a client answering List with tokens and Get with any of
// them, plus whatever more adds.
func fakeTokens(tokens []astrov1.ApiToken, more ...func(m *astrov1_mocks.ClientWithResponsesInterface)) func(t *testing.T) astrov1.APIClient {
	return func(t *testing.T) astrov1.APIClient {
		m := tokenMock(t, tokens...)
		for _, f := range more {
			f(m)
		}
		return m
	}
}

func createsToken(tok astrov1.ApiToken) func(m *astrov1_mocks.ClientWithResponsesInterface) { //nolint:gocritic // a test fixture
	return func(m *astrov1_mocks.ClientWithResponsesInterface) {
		m.On("CreateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.CreateApiTokenResponse{HTTPResponse: ok200(), JSON200: withSecret(tok)}, nil)
	}
}

func rotatesToken(tok astrov1.ApiToken) func(m *astrov1_mocks.ClientWithResponsesInterface) { //nolint:gocritic // a test fixture
	return func(m *astrov1_mocks.ClientWithResponsesInterface) {
		m.On("RotateApiTokenWithResponse", mock.Anything, mock.Anything, tok.Id).Return(&astrov1.RotateApiTokenResponse{HTTPResponse: ok200(), JSON200: withSecret(tok)}, nil)
	}
}

// updatesToken answers an update of tok's name and description with after,
// and any change to its roles.
func updatesToken(tok, after astrov1.ApiToken) func(m *astrov1_mocks.ClientWithResponsesInterface) { //nolint:gocritic // a test fixture
	return func(m *astrov1_mocks.ClientWithResponsesInterface) {
		m.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, tok.Id, mock.Anything).Return(&astrov1.UpdateApiTokenResponse{HTTPResponse: ok200(), JSON200: &after}, nil)
		m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, tok.Id, mock.Anything).Return(rolesOK(), nil).Maybe()
	}
}

func setsRoles(id string) func(m *astrov1_mocks.ClientWithResponsesInterface) {
	return func(m *astrov1_mocks.ClientWithResponsesInterface) {
		m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, id, mock.Anything).Return(rolesOK(), nil)
	}
}

func deletesToken(id string) func(m *astrov1_mocks.ClientWithResponsesInterface) {
	return func(m *astrov1_mocks.ClientWithResponsesInterface) {
		m.On("DeleteApiTokenWithResponse", mock.Anything, mock.Anything, id).Return(&astrov1.DeleteApiTokenResponse{HTTPResponse: ok200()}, nil)
	}
}

// tokenCase is one run of a token command and what to check of it.
type tokenCase struct {
	name    string
	root    func(io.Writer) *cobra.Command
	client  func(t *testing.T) astrov1.APIClient
	answers string
	args    []string
	check   func(t *testing.T, stdout string)
	wantErr string
}

func runTokenCases(t *testing.T, cases []tokenCase, extra ...string) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := execAstroCmd(t, tc.client(t), tc.answers, tc.root, append(tc.args, extra...)...)
			if tc.wantErr != "" {
				require.Error(t, r.err)
				assert.Equal(t, tc.wantErr, r.err.Error())
				assert.Equal(t, cliout.ExitFailure, r.code)
			} else {
				require.NoError(t, r.err)
				assert.Equal(t, 0, r.code)
			}
			tc.check(t, r.stdout)
		})
	}
}

// Text checks.
func says(parts ...string) func(t *testing.T, out string) {
	return func(t *testing.T, out string) { requireInOrder(t, out, parts...) }
}

func listsRows(first string, rows ...map[string]string) func(t *testing.T, out string) {
	return func(t *testing.T, out string) {
		assert.Equal(t, append([]map[string]string{}, rows...), tableRows(t, out, first))
	}
}

// secretShown is what a create or a rotate prints around the secret.
func secretShown(kind, verb, name string) func(t *testing.T, out string) {
	return func(t *testing.T, out string) {
		requireInOrder(t, out,
			"Astro "+kind+" API token "+name+" was successfully "+verb,
			"Copy and paste this API token for your records.",
			tokSecret,
			"You will not be shown this API token value again.")
		requireLine(t, out, tokSecret)
	}
}

// onlyTheSecret is --clean-output's contract: the secret and a newline, nothing
// else (`TOKEN=$(astro workspace token create ... --clean-output)`).
func onlyTheSecret(t *testing.T, out string) { assert.Equal(t, tokSecret+"\n", out) }

// withNumber is row with the picker's number.
func withNumber(n string, row map[string]string) map[string]string {
	out := map[string]string{"#": n}
	for k, v := range row {
		out[k] = v
	}
	return out
}

func wsRoleRow(id, name, desc, scope, role, created, by string) map[string]string {
	return map[string]string{
		"ID": id, "NAME": name, "DESCRIPTION": desc, "SCOPE": scope,
		"WORKSPACE ROLE": role, "CREATED": created, "CREATED BY": by,
	}
}

var (
	wsTokRow  = wsRoleRow("tok-ws", "ws-token", "Workspace CI", "WORKSPACE", "WORKSPACE_MEMBER", "2 days ago", "Ada Lovelace")
	orgTokRow = wsRoleRow("tok-org", "org-token", "Org wide", "ORGANIZATION", "WORKSPACE_OPERATOR", "3 hours ago", "bootstrap")
	// The Organization picker's row: no ID, scope or creator, and the
	// lifetime the token was created with.
	orgPickRow = map[string]string{"#": "1", "NAME": "org-token", "DESCRIPTION": "Org wide", "ROLE": "ORGANIZATION_MEMBER", "EXPIRES": "30"}
)

// What `astro workspace token` prints in text: what it printed before it
// gained --output, recorded byte for byte against v2 and checked here by
// meaning. Three quirks are kept, not fixed: a rotate by ID names an empty
// token, the Workspace picker always asks for a token "to add to the
// Deployment", and an update to a role the token holds says "the Deployment"
// after it has already sent the new name.
func TestWorkspaceTokenText(t *testing.T) {
	ws, org := wsOrgTokenFixtures()
	both := []astrov1.ApiToken{ws, org}
	twin := ws
	twin.Id = "tok-ws-2"
	renamed := ws
	renamed.Name = "ws-token-2"
	wsPickHeading := "Please select the Workspace API token you would like to add to the Deployment:"

	runTokenCases(t, []tokenCase{
		{name: "list", root: newWorkspaceCmd, client: fakeTokens(both), args: []string{"workspace", "token", "list"}, check: listsRows("ID", wsTokRow, orgTokRow)},
		{name: "list empty", root: newWorkspaceCmd, client: fakeTokens(nil), args: []string{"workspace", "token", "list"}, check: listsRows("ID")},
		{name: "organization-token list", root: newWorkspaceCmd, client: fakeTokens(both), args: []string{"workspace", "token", "organization-token", "list"}, check: listsRows("ID", orgTokRow)},
		{name: "create", root: newWorkspaceCmd, client: fakeTokens(nil, createsToken(ws)), args: []string{"workspace", "token", "create", "--name", "ws-token", "--role", "WORKSPACE_MEMBER"}, check: secretShown("Workspace", "created", "ws-token")},
		{
			name: "create with the role picked", root: newWorkspaceCmd, client: fakeTokens(nil, createsToken(ws)), answers: "1\n",
			args: []string{"workspace", "token", "create", "--name", "ws-token"},
			check: func(t *testing.T, out string) {
				requireInOrder(t, out, "select a Workspace Role for the new API token:", "WORKSPACE_MEMBER", "Astro Workspace API token ws-token was successfully created")
				secretShown("Workspace", "created", "ws-token")(t, out)
			},
		},
		{name: "create --clean-output", root: newWorkspaceCmd, client: fakeTokens(nil, createsToken(ws)), args: []string{"workspace", "token", "create", "--name", "ws-token", "--role", "WORKSPACE_MEMBER", "--clean-output"}, check: onlyTheSecret},
		{name: "update", root: newWorkspaceCmd, client: fakeTokens(both, updatesToken(ws, renamed)), args: []string{"workspace", "token", "update", "tok-ws", "--new-name", "ws-token-2", "--role", "WORKSPACE_OWNER"}, check: says("Astro Workspace API token ws-token was successfully updated")},
		{
			name: "update through the picker", root: newWorkspaceCmd, client: fakeTokens([]astrov1.ApiToken{ws}, updatesToken(ws, ws)), answers: "1\n",
			args: []string{"workspace", "token", "update", "--description", "d"},
			check: func(t *testing.T, out string) {
				requireInOrder(t, out, wsPickHeading, "> Astro Workspace API token ws-token was successfully updated")
				assert.Equal(t, []map[string]string{withNumber("1", wsTokRow)}, tableRows(t, out, "#"))
			},
		},
		{
			name: "update by a shared name", root: newWorkspaceCmd, client: fakeTokens([]astrov1.ApiToken{ws, twin}, updatesToken(twin, twin)), answers: "2\n",
			args:  []string{"workspace", "token", "update", "--name", "ws-token", "--description", "d"},
			check: says("There are more than one API tokens with name ws-token. Please select an API token:", "> Astro Workspace API token ws-token was successfully updated"),
		},
		{
			name: "update to the role it holds", root: newWorkspaceCmd, client: fakeTokens(both, updatesToken(ws, ws)),
			args:    []string{"workspace", "token", "update", "tok-ws", "--role", "WORKSPACE_MEMBER"},
			check:   func(t *testing.T, out string) { assert.Empty(t, out) },
			wantErr: "this Workspace API token has already been added to the Deployment with that role",
		},
		{name: "rotate --yes by id names an empty token", root: newWorkspaceCmd, client: fakeTokens(both, rotatesToken(ws)), args: []string{"workspace", "token", "rotate", "tok-ws", "--yes"}, check: secretShown("Workspace", "rotated", "")},
		{
			name: "rotate confirmed by name", root: newWorkspaceCmd, client: fakeTokens(both, rotatesToken(ws)), answers: "y\n",
			args: []string{"workspace", "token", "rotate", "--name", "ws-token"},
			check: func(t *testing.T, out string) {
				requireInOrder(t, out,
					"WARNING: API Token rotation will invalidate the current token and cannot be undone.",
					"Are you sure you want to rotate the ws-token API token? (y/n)")
				secretShown("Workspace", "rotated", "ws-token")(t, out)
			},
		},
		{
			// The client mocks no rotate, so going on after "n" would panic.
			name: "rotate declined", root: newWorkspaceCmd, client: fakeTokens(both), answers: "n\n",
			args:  []string{"workspace", "token", "rotate", "tok-ws"},
			check: says("WARNING: API Token rotation", "Are you sure you want to rotate the ws-token API token? (y/n)", "Canceling token rotation"),
		},
		{name: "rotate --clean-output", root: newWorkspaceCmd, client: fakeTokens(both, rotatesToken(ws)), args: []string{"workspace", "token", "rotate", "tok-ws", "--yes", "--clean-output"}, check: onlyTheSecret},
		{
			name: "delete --yes", root: newWorkspaceCmd, client: fakeTokens(both, deletesToken("tok-ws")),
			args: []string{"workspace", "token", "delete", "tok-ws", "--yes"},
			check: func(t *testing.T, out string) {
				requireInOrder(t, out, "Astro Workspace API token ws-token was successfully deleted")
				assert.NotContains(t, out, "Are you sure", "--yes answers the question")
			},
		},
		{
			name: "delete confirmed", root: newWorkspaceCmd, client: fakeTokens(both, deletesToken("tok-ws")), answers: "y\n",
			args:  []string{"workspace", "token", "delete", "tok-ws"},
			check: says("WARNING: API token deletion cannot be undone.", "Are you sure you want to delete the ws-token API token? (y/n)", "Astro Workspace API token ws-token was successfully deleted"),
		},
		{
			name: "delete declined", root: newWorkspaceCmd, client: fakeTokens(both), answers: "n\n",
			args:  []string{"workspace", "token", "delete", "tok-ws"},
			check: says("WARNING: API token deletion cannot be undone.", "Are you sure you want to delete the ws-token API token? (y/n)", "Canceling API Token deletion"),
		},
		{
			name: "delete through the picker", root: newWorkspaceCmd, client: fakeTokens(both, deletesToken("tok-ws")), answers: "1\n",
			args: []string{"workspace", "token", "delete", "--yes"},
			check: func(t *testing.T, out string) {
				requireInOrder(t, out, wsPickHeading, "> Astro Workspace API token ws-token was successfully deleted")
				assert.Equal(t, []map[string]string{withNumber("1", wsTokRow), withNumber("2", orgTokRow)}, tableRows(t, out, "#"))
			},
		},
		{
			// An Organization token is not the Workspace's to delete:
			// deleting it here removes it from the Workspace, and says so.
			name: "delete an organization token removes it", root: newWorkspaceCmd, client: fakeTokens(both, setsRoles("tok-org")), answers: "y\n",
			args: []string{"workspace", "token", "delete", "tok-org"},
			check: func(t *testing.T, out string) {
				requireInOrder(t, out,
					"Are you sure you want to remove the org-token API token from the Workspace? (y/n)",
					"Astro Organization API token org-token was successfully removed from the Workspace")
				assert.NotContains(t, out, "WARNING", "a removal can be undone")
			},
		},
		{
			name: "remove declined", root: newWorkspaceCmd, client: fakeTokens(both), answers: "n\n",
			args:  []string{"workspace", "token", "delete", "tok-org"},
			check: says("Are you sure you want to remove the org-token API token from the Workspace? (y/n)", "Canceling API Token removal"),
		},
		{name: "add", root: newWorkspaceCmd, client: fakeTokens([]astrov1.ApiToken{orgOnly(org)}, setsRoles("tok-org")), args: []string{"workspace", "token", "add", "tok-org", "--role", "WORKSPACE_MEMBER"}, check: says("Astro Organization API token org-token was successfully added to the Workspace")},
		{
			name: "add through the picker", root: newWorkspaceCmd, client: fakeTokens([]astrov1.ApiToken{orgOnly(org)}, setsRoles("tok-org")), answers: "1\n",
			args: []string{"workspace", "token", "add", "--role", "WORKSPACE_MEMBER"},
			check: func(t *testing.T, out string) {
				requireInOrder(t, out, "Please select the Organization API token you would like to update:", "> Astro Organization API token org-token was successfully added to the Workspace")
				assert.Equal(t, []map[string]string{orgPickRow}, tableRows(t, out, "#"))
			},
		},
		{
			name: "add with the role it holds", root: newWorkspaceCmd, client: fakeTokens(both),
			args:    []string{"workspace", "token", "add", "tok-org", "--role", "WORKSPACE_OPERATOR"},
			check:   func(t *testing.T, out string) { assert.Empty(t, out) },
			wantErr: "this Organization API token has already been added to the Workspace with that role",
		},
		{name: "organization-token add", root: newWorkspaceCmd, client: fakeTokens([]astrov1.ApiToken{orgOnly(org)}, setsRoles("tok-org")), args: []string{"workspace", "token", "organization-token", "add", "tok-org", "--role", "WORKSPACE_MEMBER"}, check: says("Astro Organization API token org-token was successfully added/updated to the Workspace")},
		{
			name: "organization-token add through the picker", root: newWorkspaceCmd, client: fakeTokens([]astrov1.ApiToken{orgOnly(org)}, setsRoles("tok-org")), answers: "1\n",
			args: []string{"workspace", "token", "organization-token", "add", "--role", "WORKSPACE_MEMBER"},
			check: func(t *testing.T, out string) {
				requireInOrder(t, out, "Please select the Organization API token you would like to update:", "> Astro Organization API token org-token was successfully added/updated to the Workspace")
				assert.Equal(t, []map[string]string{orgPickRow}, tableRows(t, out, "#"))
			},
		},
		{name: "organization-token update", root: newWorkspaceCmd, client: fakeTokens(both, setsRoles("tok-org")), args: []string{"workspace", "token", "organization-token", "update", "tok-org", "--role", "WORKSPACE_OWNER"}, check: says("Astro Organization API token org-token was successfully added/updated to the Workspace")},
		{
			name: "organization-token update through the picker", root: newWorkspaceCmd, client: fakeTokens(both, setsRoles("tok-org")), answers: "1\n",
			args: []string{"workspace", "token", "organization-token", "update", "--role", "WORKSPACE_OWNER"},
			check: func(t *testing.T, out string) {
				requireInOrder(t, out, wsPickHeading, "> Astro Organization API token org-token was successfully added/updated to the Workspace")
				assert.Equal(t, []map[string]string{withNumber("1", orgTokRow)}, tableRows(t, out, "#"))
			},
		},
		{name: "organization-token remove", root: newWorkspaceCmd, client: fakeTokens(both, setsRoles("tok-org")), args: []string{"workspace", "token", "organization-token", "remove", "tok-org"}, check: says("Astro Organization API token org-token was successfully removed from the Workspace")},
	})
}

// `workspace token organization-token list` lists the Workspace --workspace-id
// names. It used to read the Deployment's id there, which no `workspace`
// command sets, and so listed the current Workspace whatever was asked.
func TestWorkspaceOrgTokenListReadsWorkspaceID(t *testing.T) {
	_, org := wsOrgTokenFixtures()
	m := tokenMock(t, org)
	r := execAstroCmd(t, m, "", newWorkspaceCmd, "workspace", "token", "organization-token", "list", "--workspace-id", "clother", "-o", "json")
	require.NoError(t, r.err)
	m.AssertCalled(t, "ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListApiTokensParams) bool {
		return p.WorkspaceId != nil && *p.WorkspaceId == "clother"
	}))
	var got tokenListJSON
	decodeOne(t, r.stdout, &got)
	require.Len(t, got.Tokens, 1)
	assert.Empty(t, got.Tokens[0].Role, "its role is read on the Workspace named, where it holds none")
}

// The deployment family picks a Workspace or an Organization token to add
// with these families' pickers, and their tables.
func TestDeploymentTokenPicksWithTheOtherFamiliesTables(t *testing.T) {
	ws, org := wsOrgTokenFixtures()
	d := "--deployment=" + tokDeploymentID
	// The deployment family names no Workspace here, so the Workspace picker
	// shows no Workspace role, as it always has.
	wsNoRole := withNumber("1", wsTokRow)
	wsNoRole["WORKSPACE ROLE"] = ""

	runTokenCases(t, []tokenCase{
		{
			name: "workspace-token add", root: newDeploymentRootCmd, client: fakeTokens([]astrov1.ApiToken{ws}, setsRoles("tok-ws")), answers: "1\n",
			args: []string{"deployment", "token", "workspace-token", "add", d, "--role", "DEPLOYMENT_ADMIN"},
			check: func(t *testing.T, out string) {
				requireInOrder(t, out, "Please select the Workspace API token you would like to add to the Deployment:", "> Astro Workspace API token ws-token was successfully added/updated to the Deployment")
				assert.Equal(t, []map[string]string{wsNoRole}, tableRows(t, out, "#"))
			},
		},
		{
			name: "workspace-token remove", root: newDeploymentRootCmd, client: fakeTokens([]astrov1.ApiToken{ws}, setsRoles("tok-ws")), answers: "1\n",
			args: []string{"deployment", "token", "workspace-token", "remove", d},
			check: func(t *testing.T, out string) {
				requireInOrder(t, out, "Please select the Workspace API token you would like to add to the Deployment:", "> Astro Workspace API token ws-token was successfully removed from the Deployment")
				assert.Equal(t, []map[string]string{wsNoRole}, tableRows(t, out, "#"))
			},
		},
		{
			name: "organization-token add", root: newDeploymentRootCmd, client: fakeTokens([]astrov1.ApiToken{orgOnly(org)}, setsRoles("tok-org")), answers: "1\n",
			args: []string{"deployment", "token", "organization-token", "add", d, "--role", "DEPLOYMENT_ADMIN"},
			check: func(t *testing.T, out string) {
				requireInOrder(t, out, "Please select the Organization API token you would like to update:", "> Astro Organization API token org-token was successfully added/updated to the Deployment")
				assert.Equal(t, []map[string]string{orgPickRow}, tableRows(t, out, "#"))
			},
		},
	})
}

// What `astro organization token` prints in text, checked by meaning as for
// the Workspace family. A rotate by ID names an empty token, as it always has.
func TestOrganizationTokenText(t *testing.T) {
	ws, org := wsOrgTokenFixtures()
	orgs := []astrov1.ApiToken{org}
	renamed := org
	renamed.Name = "org-token-2"
	orgListRow := map[string]string{
		"ID": "tok-org", "NAME": "org-token", "DESCRIPTION": "Org wide", "SCOPE": "ORGANIZATION",
		"ORGANIZATION ROLE": "ORGANIZATION_MEMBER", "CREATED": "3 hours ago", "CREATED BY": "bootstrap",
	}
	roleRows := []map[string]string{
		{"ENTITY_TYPE": "ORGANIZATION", "ENTITY_ID": "test-org-id", "ROLE": "ORGANIZATION_MEMBER"},
		{"ENTITY_TYPE": "WORKSPACE", "ENTITY_ID": curWorkspaceID, "ROLE": "WORKSPACE_OPERATOR"},
	}
	orgPickHeading := "Please select the Organization API token you would like to update:"

	runTokenCases(t, []tokenCase{
		{name: "list", root: newOrganizationCmd, client: fakeTokens(orgs), args: []string{"organization", "token", "list"}, check: listsRows("ID", orgListRow)},
		{name: "list empty", root: newOrganizationCmd, client: fakeTokens(nil), args: []string{"organization", "token", "list"}, check: listsRows("ID")},
		{name: "roles", root: newOrganizationCmd, client: fakeTokens(orgs), args: []string{"organization", "token", "roles", "tok-org"}, check: listsRows("ENTITY_TYPE", roleRows...)},
		{
			name: "roles through the picker", root: newOrganizationCmd, client: fakeTokens(orgs), answers: "1\n",
			args: []string{"organization", "token", "roles"},
			check: func(t *testing.T, out string) {
				// The roles' header shares the prompt's line ("> "), so
				// they are read in order rather than as a table.
				requireInOrder(t, out, orgPickHeading, "> ", "ENTITY_TYPE", "ENTITY_ID", "ROLE",
					"ORGANIZATION", "test-org-id", "ORGANIZATION_MEMBER",
					"WORKSPACE", curWorkspaceID, "WORKSPACE_OPERATOR")
				assert.Equal(t, []map[string]string{orgPickRow}, tableRows(t, out, "#"))
			},
		},
		{name: "create", root: newOrganizationCmd, client: fakeTokens(nil, createsToken(org)), args: []string{"organization", "token", "create", "--name", "org-token", "--role", "ORGANIZATION_MEMBER"}, check: secretShown("Organization", "created", "org-token")},
		{
			name: "create with the role picked", root: newOrganizationCmd, client: fakeTokens(nil, createsToken(org)), answers: "1\n",
			args: []string{"organization", "token", "create", "--name", "org-token"},
			check: func(t *testing.T, out string) {
				requireInOrder(t, out, "select a Organization Role for the new API token:", "ORGANIZATION_MEMBER", "Astro Organization API token org-token was successfully created")
				secretShown("Organization", "created", "org-token")(t, out)
			},
		},
		{name: "create --clean-output", root: newOrganizationCmd, client: fakeTokens(nil, createsToken(org)), args: []string{"organization", "token", "create", "--name", "org-token", "--role", "ORGANIZATION_MEMBER", "--clean-output"}, check: onlyTheSecret},
		{name: "update", root: newOrganizationCmd, client: fakeTokens(orgs, updatesToken(org, renamed)), args: []string{"organization", "token", "update", "tok-org", "--new-name", "org-token-2", "--role", "ORGANIZATION_OWNER"}, check: says("Astro Organization API token org-token was successfully updated")},
		{
			name: "update through the picker", root: newOrganizationCmd, client: fakeTokens(orgs, updatesToken(org, org)), answers: "1\n",
			args: []string{"organization", "token", "update", "--description", "d"},
			check: func(t *testing.T, out string) {
				requireInOrder(t, out, orgPickHeading, "> Astro Organization API token org-token was successfully updated")
				assert.Equal(t, []map[string]string{orgPickRow}, tableRows(t, out, "#"))
			},
		},
		{name: "rotate --yes by id names an empty token", root: newOrganizationCmd, client: fakeTokens(orgs, rotatesToken(org)), args: []string{"organization", "token", "rotate", "tok-org", "--yes"}, check: secretShown("Organization", "rotated", "")},
		{
			name: "rotate confirmed by name", root: newOrganizationCmd, client: fakeTokens(orgs, rotatesToken(org)), answers: "y\n",
			args: []string{"organization", "token", "rotate", "--name", "org-token"},
			check: func(t *testing.T, out string) {
				requireInOrder(t, out,
					"WARNING: API Token rotation will invalidate the current token and cannot be undone.",
					"Are you sure you want to rotate the org-token API token? (y/n)")
				secretShown("Organization", "rotated", "org-token")(t, out)
			},
		},
		{
			name: "rotate declined", root: newOrganizationCmd, client: fakeTokens(orgs), answers: "n\n",
			args:  []string{"organization", "token", "rotate", "tok-org"},
			check: says("WARNING: API Token rotation", "Are you sure you want to rotate the org-token API token? (y/n)", "Canceling token rotation"),
		},
		{name: "rotate --clean-output", root: newOrganizationCmd, client: fakeTokens(orgs, rotatesToken(org)), args: []string{"organization", "token", "rotate", "tok-org", "--yes", "--clean-output"}, check: onlyTheSecret},
		{name: "delete --yes", root: newOrganizationCmd, client: fakeTokens(orgs, deletesToken("tok-org")), args: []string{"organization", "token", "delete", "tok-org", "--yes"}, check: says("Astro Organization API token org-token was successfully deleted")},
		{
			name: "delete confirmed", root: newOrganizationCmd, client: fakeTokens(orgs, deletesToken("tok-org")), answers: "y\n",
			args:  []string{"organization", "token", "delete", "tok-org"},
			check: says("WARNING: API token deletion cannot be undone.", "Are you sure you want to delete the org-token API token? (y/n)", "Astro Organization API token org-token was successfully deleted"),
		},
		{
			name: "delete declined", root: newOrganizationCmd, client: fakeTokens(orgs), answers: "n\n",
			args:  []string{"organization", "token", "delete", "tok-org"},
			check: says("WARNING: API token deletion cannot be undone.", "Are you sure you want to delete the org-token API token? (y/n)", "Canceling API Token deletion"),
		},
		{
			name: "delete a workspace token", root: newOrganizationCmd, client: fakeTokens([]astrov1.ApiToken{ws}),
			args:    []string{"organization", "token", "delete", "tok-ws", "--yes"},
			check:   func(t *testing.T, out string) { assert.Empty(t, out) },
			wantErr: "the token selected is not of the type you are trying to modify",
		},
	})
}

// The json the Workspace and Organization families add to the deployment
// family's: a removal naming the Workspace or the Organization, and a token's
// roles.
type (
	wsRemovalJSON struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Scope       string `json:"scope"`
		WorkspaceID string `json:"workspace_id"`
		Action      string `json:"action"`
	}
	orgRemovalJSON struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Scope          string `json:"scope"`
		OrganizationID string `json:"organization_id"`
		Action         string `json:"action"`
	}
	roleJSON struct {
		EntityType string `json:"entity_type"`
		EntityID   string `json:"entity_id"`
		Role       string `json:"role"`
	}
	roleListJSON struct {
		Roles []roleJSON `json:"roles"`
	}
)

// jsonWsOrgTokenFixtures are wsOrgTokenFixtures at a fixed time, so a
// decoded token compares equal. The Organization token expires; the
// Workspace token does not.
func jsonWsOrgTokenFixtures() (ws, org astrov1.ApiToken) {
	ws, org = wsOrgTokenFixtures()
	ws.CreatedAt, org.CreatedAt = tokCreated, tokCreated
	org.EndAt = tokPtr(time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC))
	return ws, org
}

// What jsonWsOrgTokenFixtures publish. role is the role on the object the
// command is about, which differs by family.
func wsTokJSON(role string) tokenJSON {
	return tokenJSON{
		ID: "tok-ws", Name: "ws-token", Description: "Workspace CI", Scope: "WORKSPACE",
		Role: role, CreatedAt: tokCreated, CreatedBy: "Ada Lovelace",
	}
}

func orgTokJSON(role string) tokenJSON {
	return tokenJSON{
		ID: "tok-org", Name: "org-token", Description: "Org wide", Scope: "ORGANIZATION",
		Role: role, CreatedAt: tokCreated, CreatedBy: "bootstrap",
		ExpiresAt: tokPtr(time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC)),
	}
}

func jsonLists(want ...tokenJSON) func(t *testing.T, stdout string) {
	return func(t *testing.T, stdout string) {
		var got tokenListJSON
		fields := decodeOne(t, stdout, &got)
		var raw []map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(fields["tokens"], &raw), "tokens is an array, never null")
		require.NotNil(t, raw)
		assert.Equal(t, append([]tokenJSON{}, want...), append([]tokenJSON{}, got.Tokens...))
		require.Len(t, raw, len(want))
		for i := range want {
			assert.Equal(t, tokenKeys(&want[i]), objectKeys(raw[i]), "keys of token %d", i)
		}
	}
}

func jsonIsToken(want tokenJSON) func(t *testing.T, stdout string) { //nolint:gocritic // a test fixture
	return func(t *testing.T, stdout string) {
		var got tokenJSON
		fields := decodeOne(t, stdout, &got)
		assert.Equal(t, want, got)
		assert.Equal(t, tokenKeys(&want), objectKeys(fields), "keys of the token")
	}
}

func jsonWithSecret(tok tokenJSON) tokenJSON { //nolint:gocritic // a test fixture
	tok.Token = tokPtr(tokSecret)
	return tok
}

func jsonWsRemoval(want wsRemovalJSON) func(t *testing.T, stdout string) { //nolint:gocritic // a test fixture
	return func(t *testing.T, stdout string) {
		var got wsRemovalJSON
		decodeOne(t, stdout, &got)
		assert.Equal(t, want, got)
	}
}

// What `astro workspace token` publishes under --output json, and that it
// publishes nothing else: stdout is the one object, stderr is empty, the exit
// is 0.
func TestWorkspaceTokenJSON(t *testing.T) {
	ws, org := jsonWsOrgTokenFixtures()
	both := []astrov1.ApiToken{ws, org}
	renamed := ws
	renamed.Name = "ws-token-2"

	for _, tc := range []tokenCase{
		{name: "list", client: fakeTokens(both), args: []string{"workspace", "token", "list"}, check: jsonLists(wsTokJSON("WORKSPACE_MEMBER"), orgTokJSON("WORKSPACE_OPERATOR"))},
		{name: "list empty", client: fakeTokens(nil), args: []string{"workspace", "token", "list"}, check: jsonLists()},
		{name: "organization-token list", client: fakeTokens(both), args: []string{"workspace", "token", "organization-token", "list"}, check: jsonLists(orgTokJSON("WORKSPACE_OPERATOR"))},
		{name: "create carries the secret", client: fakeTokens(nil, createsToken(ws)), args: []string{"workspace", "token", "create", "--name", "ws-token", "--role", "WORKSPACE_MEMBER"}, check: jsonIsToken(jsonWithSecret(wsTokJSON("WORKSPACE_MEMBER")))},
		{
			name: "update is the token as it now is", client: fakeTokens(both, updatesToken(ws, renamed)),
			args: []string{"workspace", "token", "update", "tok-ws", "--new-name", "ws-token-2", "--role", "WORKSPACE_OWNER"},
			check: jsonIsToken(func() tokenJSON {
				tok := wsTokJSON("WORKSPACE_OWNER")
				tok.Name = "ws-token-2"
				return tok
			}()),
		},
		{name: "update without --role keeps the role", client: fakeTokens(both, updatesToken(ws, ws)), args: []string{"workspace", "token", "update", "tok-ws", "--description", "d"}, check: jsonIsToken(wsTokJSON("WORKSPACE_MEMBER"))},
		{name: "rotate carries the new secret", client: fakeTokens(both, rotatesToken(ws)), args: []string{"workspace", "token", "rotate", "tok-ws", "--yes"}, check: jsonIsToken(jsonWithSecret(wsTokJSON("WORKSPACE_MEMBER")))},
		{name: "delete", client: fakeTokens(both, deletesToken("tok-ws")), args: []string{"workspace", "token", "delete", "tok-ws", "--yes"}, check: jsonWsRemoval(wsRemovalJSON{ID: "tok-ws", Name: "ws-token", Scope: "WORKSPACE", WorkspaceID: curWorkspaceID, Action: "deleted"})},
		{name: "delete of an organization token removes it", client: fakeTokens(both, setsRoles("tok-org")), args: []string{"workspace", "token", "delete", "tok-org", "--yes"}, check: jsonWsRemoval(wsRemovalJSON{ID: "tok-org", Name: "org-token", Scope: "ORGANIZATION", WorkspaceID: curWorkspaceID, Action: "removed"})},
		{name: "add", client: fakeTokens([]astrov1.ApiToken{orgOnly(org)}, setsRoles("tok-org")), args: []string{"workspace", "token", "add", "tok-org", "--role", "WORKSPACE_MEMBER"}, check: jsonIsToken(orgTokJSON("WORKSPACE_MEMBER"))},
		{name: "organization-token add", client: fakeTokens([]astrov1.ApiToken{orgOnly(org)}, setsRoles("tok-org")), args: []string{"workspace", "token", "organization-token", "add", "tok-org", "--role", "WORKSPACE_AUTHOR"}, check: jsonIsToken(orgTokJSON("WORKSPACE_AUTHOR"))},
		{name: "organization-token update", client: fakeTokens(both, setsRoles("tok-org")), args: []string{"workspace", "token", "organization-token", "update", "tok-org", "--role", "WORKSPACE_OWNER"}, check: jsonIsToken(orgTokJSON("WORKSPACE_OWNER"))},
		{name: "organization-token remove", client: fakeTokens(both, setsRoles("tok-org")), args: []string{"workspace", "token", "organization-token", "remove", "tok-org"}, check: jsonWsRemoval(wsRemovalJSON{ID: "tok-org", Name: "org-token", Scope: "ORGANIZATION", WorkspaceID: curWorkspaceID, Action: "removed"})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := execAstroCmd(t, tc.client(t), "", newWorkspaceCmd, append(tc.args, "-o", "json")...)
			require.NoError(t, r.err)
			assert.Equal(t, 0, r.code)
			tc.check(t, r.stdout)
			assert.Empty(t, r.stderr)
		})
	}
}

// What `astro organization token` publishes under --output json.
func TestOrganizationTokenJSON(t *testing.T) {
	_, org := jsonWsOrgTokenFixtures()
	orgs := []astrov1.ApiToken{org}
	renamed := org
	renamed.Name = "org-token-2"
	noRoles := org
	noRoles.Roles = nil

	for _, tc := range []tokenCase{
		{name: "list", client: fakeTokens(orgs), args: []string{"organization", "token", "list"}, check: jsonLists(orgTokJSON("ORGANIZATION_MEMBER"))},
		{name: "list empty", client: fakeTokens(nil), args: []string{"organization", "token", "list"}, check: jsonLists()},
		{
			name: "roles", client: fakeTokens(orgs), args: []string{"organization", "token", "roles", "tok-org"},
			check: func(t *testing.T, stdout string) {
				var got roleListJSON
				decodeOne(t, stdout, &got)
				assert.Equal(t, []roleJSON{
					{EntityType: "ORGANIZATION", EntityID: "test-org-id", Role: "ORGANIZATION_MEMBER"},
					{EntityType: "WORKSPACE", EntityID: curWorkspaceID, Role: "WORKSPACE_OPERATOR"},
				}, got.Roles)
			},
		},
		{
			name: "roles of a token with none", client: fakeTokens([]astrov1.ApiToken{noRoles}), args: []string{"organization", "token", "roles", "tok-org"},
			check: func(t *testing.T, stdout string) {
				var got roleListJSON
				fields := decodeOne(t, stdout, &got)
				assert.JSONEq(t, `[]`, string(fields["roles"]), "an empty array, not null and not a missing key")
			},
		},
		{name: "create carries the secret", client: fakeTokens(nil, createsToken(org)), args: []string{"organization", "token", "create", "--name", "org-token", "--role", "ORGANIZATION_MEMBER"}, check: jsonIsToken(jsonWithSecret(orgTokJSON("ORGANIZATION_MEMBER")))},
		{
			name: "update is the token as it now is", client: fakeTokens(orgs, updatesToken(org, renamed)),
			args: []string{"organization", "token", "update", "tok-org", "--new-name", "org-token-2", "--role", "ORGANIZATION_OWNER"},
			check: jsonIsToken(func() tokenJSON {
				tok := orgTokJSON("ORGANIZATION_OWNER")
				tok.Name = "org-token-2"
				return tok
			}()),
		},
		{name: "rotate carries the new secret", client: fakeTokens(orgs, rotatesToken(org)), args: []string{"organization", "token", "rotate", "tok-org", "--yes"}, check: jsonIsToken(jsonWithSecret(orgTokJSON("ORGANIZATION_MEMBER")))},
		{
			name: "delete", client: fakeTokens(orgs, deletesToken("tok-org")), args: []string{"organization", "token", "delete", "tok-org", "--yes"},
			check: func(t *testing.T, stdout string) {
				var got orgRemovalJSON
				decodeOne(t, stdout, &got)
				assert.Equal(t, orgRemovalJSON{ID: "tok-org", Name: "org-token", Scope: "ORGANIZATION", OrganizationID: "test-org-id", Action: "deleted"}, got)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := execAstroCmd(t, tc.client(t), "", newOrganizationCmd, append(tc.args, "-o", "json")...)
			require.NoError(t, r.err)
			assert.Equal(t, 0, r.code)
			tc.check(t, r.stdout)
			assert.Empty(t, r.stderr)
		})
	}
}

// A list never carries a secret, even when the API sent one, and a token's
// lifetime in days (which only the Organization picker shows) is never
// published.
func TestWorkspaceOrganizationTokenListsCarryNoSecret(t *testing.T) {
	ws, org := jsonWsOrgTokenFixtures()
	for _, run := range []struct {
		root func(io.Writer) *cobra.Command
		args []string
	}{
		{newWorkspaceCmd, []string{"workspace", "token", "list"}},
		{newWorkspaceCmd, []string{"workspace", "token", "organization-token", "list"}},
		{newOrganizationCmd, []string{"organization", "token", "list"}},
	} {
		r := execAstroCmd(t, tokenMock(t, *withSecret(ws), *withSecret(org)), "", run.root, append(run.args, "-o", "json")...)
		require.NoError(t, r.err, run.args)
		var got struct {
			Tokens []map[string]json.RawMessage `json:"tokens"`
		}
		decodeOne(t, r.stdout, &got)
		require.NotEmpty(t, got.Tokens, run.args)
		for _, tok := range got.Tokens {
			assert.NotContains(t, tok, "token", run.args)
			assert.NotContains(t, tok, "expiry_period_in_days", run.args)
		}
		assert.NotContains(t, r.stdout, tokSecret, run.args)
	}
}

// Under --output json a command that would ask something fails as
// input_required, naming what answers it, with that object as the whole of
// stdout: no warning, no table, no prompt ahead of it. The clients mock
// nothing that changes a token, so a refused question that went on to act
// would panic.
func TestWorkspaceOrganizationTokenJSONNeverAsks(t *testing.T) {
	ws, org := jsonWsOrgTokenFixtures()
	twin := ws
	twin.Id = "tok-ws-2"
	both := fakeTokens([]astrov1.ApiToken{ws, org})
	orgs := fakeTokens([]astrov1.ApiToken{org})

	cases := []struct {
		name     string
		root     func(io.Writer) *cobra.Command
		client   func(t *testing.T) astrov1.APIClient
		args     []string
		answered string
	}{
		{"ws rotate without --yes", newWorkspaceCmd, both, []string{"workspace", "token", "rotate", "tok-ws"}, "pass --yes"},
		{"ws delete without --yes", newWorkspaceCmd, both, []string{"workspace", "token", "delete", "tok-ws"}, "pass --yes"},
		{"ws remove without --yes", newWorkspaceCmd, both, []string{"workspace", "token", "delete", "tok-org"}, "pass --yes"},
		{"ws update naming no token", newWorkspaceCmd, both, []string{"workspace", "token", "update", "--description", "d"}, "pass the token ID or --name"},
		{"ws rotate naming no token", newWorkspaceCmd, both, []string{"workspace", "token", "rotate", "--yes"}, "pass the token ID or --name"},
		{"ws delete naming no token", newWorkspaceCmd, both, []string{"workspace", "token", "delete", "--yes"}, "pass the token ID or --name"},
		{"ws update by a shared name", newWorkspaceCmd, fakeTokens([]astrov1.ApiToken{ws, twin}), []string{"workspace", "token", "update", "--name", "ws-token", "--description", "d"}, "pass the token's ID instead of its name"},
		{"ws create naming no name", newWorkspaceCmd, both, []string{"workspace", "token", "create", "--role", "WORKSPACE_MEMBER"}, "pass --name"},
		{"ws create naming no role", newWorkspaceCmd, both, []string{"workspace", "token", "create", "--name", "n"}, "pass --role"},
		{"ws add naming no role", newWorkspaceCmd, both, []string{"workspace", "token", "add", "tok-org"}, "pass --role"},
		{"ws add naming no token", newWorkspaceCmd, both, []string{"workspace", "token", "add", "--role", "WORKSPACE_MEMBER"}, "pass the token ID or --org-token-name"},
		{"ws organization-token add naming no role", newWorkspaceCmd, both, []string{"workspace", "token", "organization-token", "add", "tok-org"}, "pass --role"},
		{"ws organization-token add naming no token", newWorkspaceCmd, both, []string{"workspace", "token", "organization-token", "add", "--role", "WORKSPACE_MEMBER"}, "pass the token ID or --org-token-name"},
		{"ws organization-token update naming no token", newWorkspaceCmd, both, []string{"workspace", "token", "organization-token", "update", "--role", "WORKSPACE_MEMBER"}, "pass the token ID or --org-token-name"},
		{"ws organization-token remove naming no token", newWorkspaceCmd, both, []string{"workspace", "token", "organization-token", "remove"}, "pass the token ID or --org-token-name"},
		{"org rotate without --yes", newOrganizationCmd, orgs, []string{"organization", "token", "rotate", "tok-org"}, "pass --yes"},
		{"org delete without --yes", newOrganizationCmd, orgs, []string{"organization", "token", "delete", "tok-org"}, "pass --yes"},
		{"org update naming no token", newOrganizationCmd, orgs, []string{"organization", "token", "update", "--description", "d"}, "pass the token ID or --name"},
		{"org roles naming no token", newOrganizationCmd, orgs, []string{"organization", "token", "roles"}, "pass the token ID"},
		{"org create naming no role", newOrganizationCmd, orgs, []string{"organization", "token", "create", "--name", "n"}, "pass --role"},
		{"deployment workspace-token update naming no token", newDeploymentRootCmd, both, []string{"deployment", "token", "workspace-token", "update", "--deployment=" + tokDeploymentID, "--role", "DEPLOYMENT_ADMIN"}, "pass the token ID or --workspace-token-name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// An answer is waiting, so a prompt that did read would go on.
			r := execAstroCmd(t, tc.client(t), "y\n1\n", tc.root, append(tc.args, "-o", "json")...)
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

// --output takes text or json, and --clean-output is a text format of its own:
// either mistake is a usage error, exit 2, before anything is asked or done.
func TestWorkspaceOrganizationTokenOutputUsage(t *testing.T) {
	for _, run := range []struct {
		root func(io.Writer) *cobra.Command
		args []string
	}{
		{newWorkspaceCmd, []string{"workspace", "token", "list", "-o", "yaml"}},
		{newWorkspaceCmd, []string{"workspace", "token", "create", "-o", "yaml"}},
		{newWorkspaceCmd, []string{"workspace", "token", "create", "--name", "n", "--role", "WORKSPACE_MEMBER", "--clean-output", "-o", "json"}},
		{newWorkspaceCmd, []string{"workspace", "token", "rotate", "tok-ws", "--yes", "--clean-output", "-o", "json"}},
		{newOrganizationCmd, []string{"organization", "token", "roles", "tok-org", "-o", "yaml"}},
		{newOrganizationCmd, []string{"organization", "token", "create", "--name", "n", "--role", "ORGANIZATION_MEMBER", "--clean-output", "-o", "json"}},
		{newOrganizationCmd, []string{"organization", "token", "rotate", "tok-org", "--yes", "--clean-output", "-o", "json"}},
	} {
		r := execAstroCmd(t, tokenMock(t), "", run.root, run.args...)
		require.Error(t, r.err, run.args)
		assert.Equal(t, cliout.ExitUsage, r.code, run.args)
	}
}
