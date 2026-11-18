package astro

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
)

// What the token groups' verbs do now that each group has one flat set:
// create, rotate and delete act on the scope's own tokens, add and remove
// grant and revoke a broader token's role, update changes either. The
// per-command texts are pinned in the token tables beside these; these are the
// refusals, the scope the deployment verbs work out, and the old spellings.

// What the warning of a deprecated remove or list adds to the path to use:
// the new verb is not quite the old one.
const removeNote = " --yes (without --yes, remove asks for confirmation, which this spelling never did)"

func listNote(kind string) string {
	return ", which lists the tokens of every scope, not only " + kind + " tokens: tell them apart by the scope field of -o json"
}

// warnings are the deprecation warnings a run printed, without their label.
func warnings(stderr string) []string {
	var lines []string
	for line := range strings.Lines(stderr) {
		if rest, ok := strings.CutPrefix(line, "Warning: "); ok && strings.Contains(rest, " is deprecated: ") {
			lines = append(lines, strings.TrimSuffix(rest, "\n"))
		}
	}
	return lines
}

// A verb that does not fit the token it names is refused as a usage error
// (exit 2), saying which verb does, before anything is asked or changed: the
// clients have no call that changes a token, and a stdin that would answer yes
// is waiting.
func TestAVerbThatDoesNotFitTheTokenIsRefusedAsUsage(t *testing.T) {
	dep, ws, org := jsonTokenFixtures()
	wsTok, orgTok := wsOrgTokenFixtures() // with roles in the current Workspace
	wsOrg := []astrov1.ApiToken{wsTok, orgTok}
	for _, tc := range []struct {
		name   string
		root   func(io.Writer) *cobra.Command
		client func(t *testing.T) astrov1.APIClient
		args   []string
		want   string
	}{
		{
			"workspace delete of an organization token", newWorkspaceCmd, fakeTokens(wsOrg),
			[]string{"workspace", "token", "delete", "tok-org", "--yes"},
			"org-token is an Organization API token, not one of this Workspace's own. astro workspace token delete deletes a Workspace's own tokens; use astro workspace token remove to take the token's role off the Workspace",
		},
		{
			"workspace remove of the workspace's own token", newWorkspaceCmd, fakeTokens(wsOrg),
			[]string{"workspace", "token", "remove", "tok-ws", "--yes"},
			"ws-token is this Workspace's own API token. astro workspace token remove takes a broader token's role off the Workspace; use astro workspace token delete to delete it",
		},
		{
			"deployment delete of a workspace token", newDeploymentRootCmd, func(t *testing.T) astrov1.APIClient { return tokenMock(t, ws) },
			[]string{"deployment", "token", "delete", "tok-ws", "--deployment=" + tokDeploymentID, "--yes"},
			"ws-token is a Workspace API token, not one of this Deployment's own. astro deployment token delete deletes a Deployment's own tokens; use astro deployment token remove to take the token's role off the Deployment",
		},
		{
			"deployment delete of an organization token", newDeploymentRootCmd, func(t *testing.T) astrov1.APIClient { return tokenMock(t, org) },
			[]string{"deployment", "token", "delete", "tok-org", "--deployment=" + tokDeploymentID, "--yes"},
			"org-token is an Organization API token, not one of this Deployment's own. astro deployment token delete deletes a Deployment's own tokens; use astro deployment token remove to take the token's role off the Deployment",
		},
		{
			"deployment remove of the deployment's own token", newDeploymentRootCmd, func(t *testing.T) astrov1.APIClient { return tokenMock(t, dep) },
			[]string{"deployment", "token", "remove", "tok-dep", "--deployment=" + tokDeploymentID, "--yes"},
			"ci-deploy is this Deployment's own API token. astro deployment token remove takes a broader token's role off the Deployment; use astro deployment token delete to delete it",
		},
		{
			"deployment add of the deployment's own token", newDeploymentRootCmd, func(t *testing.T) astrov1.APIClient { return tokenMock(t, dep) },
			[]string{"deployment", "token", "add", "tok-dep", "--deployment=" + tokDeploymentID, "--role", "DEPLOYMENT_ADMIN"},
			"this is a Deployment API token, which has its role on its own Deployment already: update changes it",
		},
		{
			"workspace rename of an organization token", newWorkspaceCmd, fakeTokens(wsOrg),
			[]string{"workspace", "token", "update", "tok-org", "--new-name", "renamed"},
			"an Organization API token has its name and description changed in the Organization, not in a Workspace",
		},
		{
			"deployment rename of a workspace token", newDeploymentRootCmd, func(t *testing.T) astrov1.APIClient { return tokenMock(t, ws) },
			[]string{"deployment", "token", "update", "tok-ws", "--deployment=" + tokDeploymentID, "--description", "d"},
			"a Workspace or Organization API token has its name and description changed where it lives, not in a Deployment",
		},
	} {
		for _, asJSON := range []bool{false, true} {
			args := tc.args
			if asJSON {
				args = append(append([]string{}, args...), "-o", "json")
			}
			r := execAstroCmd(t, tc.client(t), "y\n", tc.root, args...)
			require.Error(t, r.err, tc.name)
			assert.Equal(t, tc.want, r.err.Error(), tc.name)
			assert.True(t, cliout.IsUsage(r.err), "%s: the verb does not fit the token, which is a usage error", tc.name)
			assert.Equal(t, cliout.ExitUsage, r.code, tc.name)
			assert.NotContains(t, r.asked, "Are you sure", "%s asked before refusing", tc.name)
			if asJSON {
				var got errorJSON
				decodeOne(t, r.stdout, &got)
				assert.Equal(t, string(cliout.KindUsage), got.Kind, tc.name)
				assert.Equal(t, cliout.ExitUsage, got.Code, tc.name)
			}
		}
	}
}

// addScopeClient serves what `deployment token add` looks among, telling the
// Workspace's tokens from the Organization's by the filter the request carries,
// and each token by its ID. A token's role is set through the one call that
// sets roles, which the test reads.
func addScopeClient(t *testing.T, ws, org astrov1.ApiToken) *astrov1_mocks.ClientWithResponsesInterface { //nolint:gocritic // test fixtures
	t.Helper()
	m := new(astrov1_mocks.ClientWithResponsesInterface)
	m.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListApiTokensParams) bool {
		return p.WorkspaceId != nil
	})).Return(listTokensResp(ws), nil).Maybe()
	m.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListApiTokensParams) bool {
		return p.IncludeOnlyOrganizationTokens != nil
	})).Return(listTokensResp(org), nil).Maybe()
	for _, tok := range []astrov1.ApiToken{ws, org} { //nolint:gocritic // test fixtures
		m.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, tok.Id).Return(getTokenResp(tok), nil).Maybe()
		m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, tok.Id, mock.Anything).Return(rolesOK(), nil).Maybe()
	}
	return m
}

// `deployment token add` works out from the token whether it is a Workspace
// token or an Organization token, by its ID or its name, and says which it
// found; the same goes for `update` and `remove`, which find it among the
// Deployment's.
func TestDeploymentTokenAddUpdateRemoveWorkOutTheScope(t *testing.T) {
	_, ws, org := jsonTokenFixtures()
	// Neither holds a role on the Deployment yet, which is what an add is for.
	for _, tok := range []*astrov1.ApiToken{&ws, &org} {
		roles := []astrov1.ApiTokenRole{}
		for _, r := range *tok.Roles {
			if r.EntityType != astrov1.ApiTokenRoleEntityTypeDEPLOYMENT {
				roles = append(roles, r)
			}
		}
		tok.Roles = &roles
	}
	d := "--deployment=" + tokDeploymentID

	for _, tc := range []struct {
		name      string
		args      []string
		wantScope string
		wantID    string
	}{
		{"a workspace token by ID", []string{"token", "add", "tok-ws", d, "--role", "DEPLOYMENT_ADMIN"}, "WORKSPACE", "tok-ws"},
		{"an organization token by ID", []string{"token", "add", "tok-org", d, "--role", "DEPLOYMENT_ADMIN"}, "ORGANIZATION", "tok-org"},
		{"a workspace token by name, in the workspace", []string{"token", "add", "--name", "ws-token", "--workspace=" + tokWorkspaceID, d, "--role", "DEPLOYMENT_ADMIN"}, "WORKSPACE", "tok-ws"},
		{"an organization token by name", []string{"token", "add", "--name", "org-token", "--workspace=" + tokWorkspaceID, d, "--role", "DEPLOYMENT_ADMIN"}, "ORGANIZATION", "tok-org"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := addScopeClient(t, ws, org)
			r := execTokenCmd(t, m, "", append(tc.args, "-o", "json")...)
			require.NoError(t, r.err, r.stdout)
			var got tokenJSON
			decodeOne(t, r.stdout, &got)
			assert.Equal(t, tc.wantScope, got.Scope)
			assert.Equal(t, tc.wantID, got.ID)
			assert.Equal(t, "DEPLOYMENT_ADMIN", got.Role)
			m.AssertCalled(t, "UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, tc.wantID, mock.MatchedBy(func(b astrov1.UpdateApiTokenRolesRequest) bool {
				for _, role := range b.Roles {
					if role.EntityType == astrov1.ApiTokenRoleEntityTypeDEPLOYMENT && role.EntityId == tokDeploymentID && role.Role == "DEPLOYMENT_ADMIN" {
						return true
					}
				}
				return false
			}))
			assert.Empty(t, r.stderr)
		})
	}

	t.Run("text says which it found", func(t *testing.T) {
		r := execTokenCmd(t, addScopeClient(t, ws, org), "", "token", "add", "tok-org", d, "--role", "DEPLOYMENT_ADMIN")
		require.NoError(t, r.err)
		assert.Contains(t, r.stdout, "Astro Organization API token org-token was successfully added to the Deployment")
		r = execTokenCmd(t, addScopeClient(t, ws, org), "", "token", "add", "tok-ws", d, "--role", "DEPLOYMENT_ADMIN")
		require.NoError(t, r.err)
		assert.Contains(t, r.stdout, "Astro Workspace API token ws-token was successfully added to the Deployment")
	})

	t.Run("update and remove find the token among the Deployment's, of either scope", func(t *testing.T) {
		_, wsHeld, orgHeld := jsonTokenFixtures() // both hold a role on the Deployment
		for _, tok := range []astrov1.ApiToken{wsHeld, orgHeld} {
			// A role other than the one each holds: DEPLOYMENT_MEMBER and
			// DEPLOYMENT_ADMIN, respectively.
			newRole := "DEPLOYMENT_MEMBER"
			if tok.Scope == astrov1.ApiTokenScopeWORKSPACE {
				newRole = "DEPLOYMENT_ADMIN"
			}
			m := tokenMock(t, tok)
			m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, tok.Id, mock.Anything).Return(rolesOK(), nil)
			r := execTokenCmd(t, m, "", "token", "update", "--name", tok.Name, d, "--role", newRole, "-o", "json")
			require.NoError(t, r.err, tok.Name)
			var got tokenJSON
			decodeOne(t, r.stdout, &got)
			assert.Equal(t, string(tok.Scope), got.Scope)
			assert.Equal(t, newRole, got.Role)

			m = tokenMock(t, tok)
			m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, tok.Id, mock.Anything).Return(rolesOK(), nil)
			r = execTokenCmd(t, m, "", "token", "remove", "--name", tok.Name, d, "--yes", "-o", "json")
			require.NoError(t, r.err, tok.Name)
			var removed tokenRemovalJSON
			decodeOne(t, r.stdout, &removed)
			assert.Equal(t, string(tok.Scope), removed.Scope)
			assert.Equal(t, "removed", removed.Action)
		}
	})
}

// The API serves any token of the Organization by its ID. A token given by ID
// that holds no role in the Workspace or on the Deployment is not that scope's
// to change, whichever verb names it: it is refused as a usage error naming the
// scope, and nothing is changed (the clients have no call that changes a token).
func TestATokenGivenByIDMustHaveARoleInTheScope(t *testing.T) {
	// Tokens with a role only on or in some other Deployment or Workspace.
	otherDep, otherWs, otherOrg := jsonTokenFixtures()
	otherDep.Roles = &[]astrov1.ApiTokenRole{{EntityType: astrov1.ApiTokenRoleEntityTypeDEPLOYMENT, EntityId: "clother0000000000000000001", Role: "DEPLOYMENT_ADMIN"}}
	otherWs.Roles = &[]astrov1.ApiTokenRole{{EntityType: astrov1.ApiTokenRoleEntityTypeWORKSPACE, EntityId: "clotherws00000000000000001", Role: "WORKSPACE_MEMBER"}}
	otherOrg.Roles = &[]astrov1.ApiTokenRole{{EntityType: astrov1.ApiTokenRoleEntityTypeORGANIZATION, EntityId: "test-org-id", Role: "ORGANIZATION_MEMBER"}}
	d := "--deployment=" + tokDeploymentID
	depWant := "the API token %s has no role on this Deployment: only a Deployment's own tokens, and Workspace and Organization tokens with a role on it, are found here; to give a token a role on it, use astro deployment token add"
	wsWant := "the API token %s has no role in this Workspace: only a Workspace's own tokens, and Organization tokens with a role in it, are found here; to give an Organization token a role in it, use astro workspace token add"

	for _, tc := range []struct {
		name string
		root func(io.Writer) *cobra.Command
		tok  astrov1.ApiToken
		args []string
		want string
	}{
		{"deployment delete", newDeploymentRootCmd, otherDep, []string{"deployment", "token", "delete", "tok-dep", d, "--yes"}, depWant},
		{"deployment remove", newDeploymentRootCmd, otherOrg, []string{"deployment", "token", "remove", "tok-org", d, "--yes"}, depWant},
		{"deployment update", newDeploymentRootCmd, otherDep, []string{"deployment", "token", "update", "tok-dep", d, "--role", "DEPLOYMENT_MEMBER"}, depWant},
		{"deployment rotate", newDeploymentRootCmd, otherDep, []string{"deployment", "token", "rotate", "tok-dep", d, "--yes"}, depWant},
		{"workspace delete", newWorkspaceCmd, otherWs, []string{"workspace", "token", "delete", "tok-ws", "--yes"}, wsWant},
		{"workspace remove", newWorkspaceCmd, otherOrg, []string{"workspace", "token", "remove", "tok-org", "--yes"}, wsWant},
		{"workspace update", newWorkspaceCmd, otherWs, []string{"workspace", "token", "update", "tok-ws", "--role", "WORKSPACE_OWNER"}, wsWant},
		{"workspace rotate", newWorkspaceCmd, otherWs, []string{"workspace", "token", "rotate", "tok-ws", "--yes"}, wsWant},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := execAstroCmd(t, fakeTokens([]astrov1.ApiToken{tc.tok})(t), "", tc.root, tc.args...)
			require.Error(t, r.err)
			assert.Equal(t, fmt.Sprintf(tc.want, tc.tok.Id), r.err.Error())
			assert.True(t, cliout.IsUsage(r.err))
			assert.Equal(t, cliout.ExitUsage, r.code)
		})
	}
}

// Two tokens that share the name given are told apart by ID, which is what a
// run that may not ask says: --name is already given, so it is not offered as
// the answer.
func TestASharedNameIsAnsweredByTheTokenIDAlone(t *testing.T) {
	dep, ws, _ := jsonTokenFixtures()
	twin := ws
	twin.Id, twin.Name = "tok-twin", dep.Name // a Workspace token named like the Deployment's own
	d := "--deployment=" + tokDeploymentID
	for _, args := range [][]string{
		{"token", "delete", "--name", dep.Name, d, "--yes", "-o", "json"},
		{"token", "update", "--name", dep.Name, d, "--role", "DEPLOYMENT_MEMBER", "-o", "json"},
		{"token", "remove", "--name", dep.Name, d, "--yes", "-o", "json"},
	} {
		r := execTokenCmd(t, tokenMock(t, dep, twin), "", args...)
		require.Error(t, r.err, args)
		var got errorJSON
		decodeOne(t, r.stdout, &got)
		assert.Equal(t, string(cliout.KindInputRequired), got.Kind, args)
		assert.Contains(t, got.Error, "the token's ID instead of its name", args)
		assert.NotContains(t, got.Error, "or --name", args)
	}
}

// A delete or a remove that has to ask offers only the tokens it can act on,
// so no row of its picker is one it would refuse; a token it is told by name
// or ID is found among all of them, and refused with what to use instead.
func TestDeleteAndRemovePickersOfferOnlyWhatTheyCanActOn(t *testing.T) {
	dep, ws, org := jsonTokenFixtures()
	wsTok, orgTok := wsOrgTokenFixtures()

	// Workspace: delete offers the Workspace's own, remove the Organization's.
	r := execAstroCmd(t, fakeTokens([]astrov1.ApiToken{wsTok, orgTok}, deletesToken("tok-ws"))(t), "1\n", newWorkspaceCmd, "workspace", "token", "delete", "--yes")
	require.NoError(t, r.err)
	assert.Equal(t, []string{"tok-ws"}, pickedIDs(t, r.asked))
	r = execAstroCmd(t, fakeTokens([]astrov1.ApiToken{wsTok, orgTok}, setsRoles("tok-org"))(t), "1\n", newWorkspaceCmd, "workspace", "token", "remove", "--yes")
	require.NoError(t, r.err)
	assert.Equal(t, []string{"tok-org"}, pickedIDs(t, r.asked))

	// Deployment: delete offers the Deployment's own, remove the Workspace's
	// and the Organization's.
	d := "--deployment=" + tokDeploymentID
	m := tokenMock(t, dep, ws, org)
	m.On("DeleteApiTokenWithResponse", mock.Anything, mock.Anything, "tok-dep").Return(&astrov1.DeleteApiTokenResponse{HTTPResponse: ok200()}, nil)
	r = execTokenCmd(t, m, "1\n", "token", "delete", d, "--yes")
	require.NoError(t, r.err)
	assert.Equal(t, []string{"tok-dep"}, pickedIDs(t, r.asked))
	m = tokenMock(t, dep, ws, org)
	m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "tok-ws", mock.Anything).Return(rolesOK(), nil)
	r = execTokenCmd(t, m, "1\n", "token", "remove", d, "--yes")
	require.NoError(t, r.err)
	assert.Equal(t, []string{"tok-ws", "tok-org"}, pickedIDs(t, r.asked))
}

// pickedIDs is the ID column of the picker table in what a run asked.
func pickedIDs(t *testing.T, asked string) []string {
	t.Helper()
	var ids []string
	for _, row := range tableRows(t, asked, "#") {
		ids = append(ids, row["ID"])
	}
	return ids
}

// `link remove` is what `link delete` was: the old spelling still unlinks, and
// says once per run to type remove, while remove and its alias rm say nothing.
func TestEnvLinkDeleteStillUnlinksAndWarns(t *testing.T) {
	ws := "--workspace=" + wjWorkspace
	dep := "--deployment=" + wjDeployment
	for _, n := range []struct{ noun, keyFlag string }{
		{"variable", "--variable-key"}, {"connection", "--connection-key"}, {"airflow-variable", "--airflow-variable-key"},
	} {
		t.Run(n.noun, func(t *testing.T) {
			for verb, wantWarning := range map[string][]string{
				"delete": {"astro env " + n.noun + " link delete is deprecated: use astro env " + n.noun + " link remove"},
				"remove": nil,
				"rm":     nil,
			} {
				mc := wjClient(t)
				r := execEnvJSON(t, mc, n.noun, "link", verb, n.keyFlag, "LINKED", dep, ws, "-o", "json")
				require.NoError(t, r.err, "%s: %s", verb, r.stdout)
				assert.Equal(t, wantWarning, warnings(r.stderr), verb)
				mc.AssertCalled(t, "UpdateEnvironmentObjectWithResponse", mock.Anything, mock.Anything, wjID, mock.Anything)
				var doc map[string]any
				decodeOne(t, r.stdout, &doc)
			}
		})
	}
}

// The old spellings still do what they did, and each says once per run what
// to type instead, naming the verb it was typed as and the flag, if it was
// typed through the old one. --output json stays one document on stdout.
func TestTheOldTokenSpellingsStillRunAndWarn(t *testing.T) {
	_, ws, org := jsonTokenFixtures()
	wsTok, orgTok := wsOrgTokenFixtures()
	wsOrg := []astrov1.ApiToken{wsTok, orgTok}
	d := "--deployment=" + tokDeploymentID
	for _, tc := range []struct {
		name   string
		root   func(io.Writer) *cobra.Command
		client func(t *testing.T) astrov1.APIClient
		args   []string
		want   []string
	}{
		{
			"workspace organization-token remove, by the old flag", newWorkspaceCmd, fakeTokens(wsOrg, setsRoles("tok-org")),
			[]string{"workspace", "token", "organization-token", "remove", "--org-token-name", "org-token"},
			[]string{
				"--org-token-name is deprecated: use --name",
				"astro workspace token organization-token remove is deprecated: use astro workspace token remove" + removeNote,
			},
		},
		{
			"workspace organization-token update, by the new flag", newWorkspaceCmd, fakeTokens(wsOrg, setsRoles("tok-org")),
			[]string{"workspace", "token", "organization-token", "update", "--name", "org-token", "--role", "WORKSPACE_MEMBER"},
			[]string{"astro workspace token organization-token update is deprecated: use astro workspace token update"},
		},
		{
			"workspace organization-token list", newWorkspaceCmd, fakeTokens(wsOrg),
			[]string{"workspace", "token", "organization-token", "list"},
			[]string{"astro workspace token organization-token list is deprecated: use astro workspace token list" + listNote("Organization")},
		},
		{
			"deployment workspace-token remove, by the old flag", newDeploymentRootCmd, func(t *testing.T) astrov1.APIClient {
				m := tokenMock(t, ws)
				m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "tok-ws", mock.Anything).Return(rolesOK(), nil)
				return m
			},
			[]string{"deployment", "token", "workspace-token", "remove", "--workspace-token-name", "ws-token", d, "--workspace=" + tokWorkspaceID},
			[]string{
				"--workspace-token-name is deprecated: use --name",
				"astro deployment token workspace-token remove is deprecated: use astro deployment token remove" + removeNote,
			},
		},
		{
			"deployment organization-token list", newDeploymentRootCmd, func(t *testing.T) astrov1.APIClient { return tokenMock(t, org) },
			[]string{"deployment", "token", "organization-token", "list", d},
			[]string{"astro deployment token organization-token list is deprecated: use astro deployment token list" + listNote("Organization")},
		},
	} {
		for _, asJSON := range []bool{false, true} {
			args := tc.args
			if asJSON {
				args = append(append([]string{}, args...), "-o", "json")
			}
			r := execAstroCmd(t, tc.client(t), "", tc.root, args...)
			require.NoError(t, r.err, "%s: %s", tc.name, r.stderr)
			assert.ElementsMatch(t, tc.want, warnings(r.stderr), tc.name)
			if asJSON {
				var doc map[string]any
				_ = decodeOne(t, r.stdout, &doc)
			}
		}
	}
}
