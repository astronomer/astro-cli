package workspacetoken

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/platform/astro/apitoken"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// What the token functions return, now that they return it instead of
// printing it. The command's rendering of these is pinned in cmd/astro.

var resultCreated = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// resultToken is a token of scope holding role on the current Workspace.
func resultToken(id, name string, scope astrov1.ApiTokenScope, role string) astrov1.ApiToken {
	return astrov1.ApiToken{
		Id: id, Name: name, Description: "about " + name, Scope: scope,
		Roles: &[]astrov1.ApiTokenRole{
			{EntityType: astrov1.ApiTokenRoleEntityTypeORGANIZATION, EntityId: "test-org-id", Role: "ORGANIZATION_MEMBER"},
			{EntityType: astrov1.ApiTokenRoleEntityTypeWORKSPACE, EntityId: workspaceID, Role: role},
		},
		CreatedAt: resultCreated,
		CreatedBy: &astrov1.BasicSubjectProfile{FullName: &fullName1},
	}
}

func resultClient(tokens ...astrov1.ApiToken) *astrov1_mocks.ClientWithResponsesInterface {
	m := new(astrov1_mocks.ClientWithResponsesInterface)
	m.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.ListApiTokensResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &astrov1.ApiTokensPaginated{Tokens: tokens, TotalCount: len(tokens)},
	}, nil).Maybe()
	for i := range tokens {
		m.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, tokens[i].Id).Return(&astrov1.GetApiTokenResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &tokens[i],
		}, nil).Maybe()
	}
	return m
}

func rolesOK() *astrov1.UpdateApiTokenRolesResponse {
	return &astrov1.UpdateApiTokenRolesResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &astrov1.SubjectRoles{}}
}

func noPicking(t *testing.T) apitoken.Picker {
	return func(string, []apitoken.Token) (int, error) {
		t.Fatal("the picker was asked, though the command named its token")
		return 0, nil
	}
}

func TestListTokensResult(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("each token with its role on the Workspace, and no secret", func(t *testing.T) {
		ws := resultToken("t1", "one", astrov1.ApiTokenScopeWORKSPACE, "WORKSPACE_MEMBER")
		org := resultToken("t2", "two", astrov1.ApiTokenScopeORGANIZATION, "WORKSPACE_OWNER")
		org.Token = &token
		got, err := ListTokens(resultClient(ws, org), "", nil)
		require.NoError(t, err)
		assert.Equal(t, []apitoken.Token{
			{ID: "t1", Name: "one", Description: "about one", Scope: "WORKSPACE", Role: "WORKSPACE_MEMBER", CreatedAt: resultCreated, CreatedBy: fullName1},
			{ID: "t2", Name: "two", Description: "about two", Scope: "ORGANIZATION", Role: "WORKSPACE_OWNER", CreatedAt: resultCreated, CreatedBy: fullName1},
		}, got)
	})

	t.Run("filtered by scope", func(t *testing.T) {
		ws := resultToken("t1", "one", astrov1.ApiTokenScopeWORKSPACE, "WORKSPACE_MEMBER")
		org := resultToken("t2", "two", astrov1.ApiTokenScopeORGANIZATION, "WORKSPACE_OWNER")
		got, err := ListTokens(resultClient(ws, org), "", []TokenType{TokenTypeORGANIZATION})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "t2", got[0].ID)
	})

	t.Run("the role is read on the Workspace named", func(t *testing.T) {
		ws := resultToken("t1", "one", astrov1.ApiTokenScopeWORKSPACE, "WORKSPACE_MEMBER")
		got, err := ListTokens(resultClient(ws), "another-workspace", nil)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Empty(t, got[0].Role, "no role on another Workspace")
	})

	t.Run("none is empty, not nil", func(t *testing.T) {
		got, err := ListTokens(resultClient(), "", nil)
		require.NoError(t, err)
		assert.NotNil(t, got)
		assert.Empty(t, got)
	})
}

func TestCreateTokenResult(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	created := resultToken("t1", "one", astrov1.ApiTokenScopeWORKSPACE, "WORKSPACE_MEMBER")
	created.Token = &token
	m := resultClient()
	m.On("CreateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.CreateApiTokenResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &created,
	}, nil)

	got, err := CreateToken("one", "about one", "WORKSPACE_MEMBER", "", 0, m)
	require.NoError(t, err)
	assert.Equal(t, apitoken.Token{
		ID: "t1", Name: "one", Description: "about one", Scope: "WORKSPACE", Role: "WORKSPACE_MEMBER",
		CreatedAt: resultCreated, CreatedBy: fullName1, Token: token,
	}, got)
}

func TestUpdateTokenResult(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	before := resultToken("t1", "one", astrov1.ApiTokenScopeWORKSPACE, "WORKSPACE_MEMBER")
	after := before
	after.Name = "uno"

	t.Run("a new role", func(t *testing.T) {
		m := resultClient(before)
		m.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, "t1", mock.Anything).Return(&astrov1.UpdateApiTokenResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &after,
		}, nil)
		m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "t1", mock.Anything).Return(rolesOK(), nil)

		got, err := UpdateToken("t1", "", "uno", "", "WORKSPACE_OWNER", "", noPicking(t), m)
		require.NoError(t, err)
		assert.Equal(t, "one", got.PreviousName, "the text names the token as it was")
		assert.Equal(t, "uno", got.Token.Name)
		assert.Equal(t, "WORKSPACE_OWNER", got.Token.Role)
		assert.Empty(t, got.Token.Token, "an update has no secret to report")
	})

	t.Run("no role keeps the one it holds", func(t *testing.T) {
		m := resultClient(before)
		m.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, "t1", mock.Anything).Return(&astrov1.UpdateApiTokenResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &after,
		}, nil)

		got, err := UpdateToken("t1", "", "uno", "", "", "", noPicking(t), m)
		require.NoError(t, err)
		assert.Equal(t, "WORKSPACE_MEMBER", got.Token.Role)
		m.AssertNotCalled(t, "UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})
}

// An update whose role is refused changes nothing: the mocks register no
// call that changes anything unless the case says so, and any such call
// would panic them.
func TestUpdateTokenRefusesBeforeChanging(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	before := resultToken("t1", "one", astrov1.ApiTokenScopeWORKSPACE, "WORKSPACE_MEMBER")

	t.Run("a role that is not a Workspace role", func(t *testing.T) {
		m := new(astrov1_mocks.ClientWithResponsesInterface)
		_, err := UpdateToken("", "", "uno", "", "NOT_A_ROLE", "", noPicking(t), m)
		require.Error(t, err)
		m.AssertExpectations(t)
	})

	t.Run("a role it already holds", func(t *testing.T) {
		m := resultClient(before)
		_, err := UpdateToken("t1", "", "uno", "", "WORKSPACE_MEMBER", "", noPicking(t), m)
		assert.EqualError(t, err, "this Workspace API token already has that role on the Workspace")
	})

	t.Run("a role the API refuses", func(t *testing.T) {
		m := resultClient(before)
		m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "t1", mock.Anything).Return(&astrov1.UpdateApiTokenRolesResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusBadRequest}, Body: []byte(`{"message":"no such role"}`),
		}, nil)
		_, err := UpdateToken("t1", "", "uno", "", "WORKSPACE_OWNER", "", noPicking(t), m)
		require.Error(t, err)
		m.AssertNotCalled(t, "UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})
}

func TestRotateTokenResult(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	tok := resultToken("t1", "one", astrov1.ApiTokenScopeWORKSPACE, "WORKSPACE_MEMBER")
	rotated := tok
	rotated.Token = &token
	m := resultClient(tok)
	m.On("RotateApiTokenWithResponse", mock.Anything, mock.Anything, "t1").Return(&astrov1.RotateApiTokenResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &rotated,
	}, nil)

	got, err := RotateToken(tok, "", m)
	require.NoError(t, err)
	assert.Equal(t, token, got.Token)
	assert.Equal(t, "WORKSPACE_MEMBER", got.Role)
}

func TestDeleteTokenResult(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("a Workspace token is deleted", func(t *testing.T) {
		tok := resultToken("t1", "one", astrov1.ApiTokenScopeWORKSPACE, "WORKSPACE_MEMBER")
		m := resultClient(tok)
		m.On("DeleteApiTokenWithResponse", mock.Anything, mock.Anything, "t1").Return(&DeleteWorkspaceAPITokenResponseOK, nil)
		got, err := DeleteToken(tok, "", m)
		require.NoError(t, err)
		assert.Equal(t, apitoken.WorkspaceRemoval{ID: "t1", Name: "one", Scope: "WORKSPACE", WorkspaceID: workspaceID, Action: apitoken.Deleted}, got)
	})

	t.Run("an Organization token loses its Workspace role, and keeps its others", func(t *testing.T) {
		tok := resultToken("t2", "two", astrov1.ApiTokenScopeORGANIZATION, "WORKSPACE_OWNER")
		m := resultClient(tok)
		m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "t2", mock.MatchedBy(func(r astrov1.UpdateApiTokenRolesRequest) bool {
			return len(r.Roles) == 1 && r.Roles[0].EntityType == astrov1.ApiTokenRoleEntityTypeORGANIZATION
		})).Return(rolesOK(), nil)
		got, err := DeleteToken(tok, "", m)
		require.NoError(t, err)
		assert.Equal(t, apitoken.WorkspaceRemoval{ID: "t2", Name: "two", Scope: "ORGANIZATION", WorkspaceID: workspaceID, Action: apitoken.Removed}, got)
	})
}

func TestOrgTokenWorkspaceRoleResult(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	org := resultToken("t2", "two", astrov1.ApiTokenScopeORGANIZATION, "WORKSPACE_MEMBER")

	t.Run("an update returns the token with its new role", func(t *testing.T) {
		m := resultClient(org)
		m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "t2", mock.Anything).Return(rolesOK(), nil)
		got, err := UpsertOrgTokenWorkspaceRole("t2", "", "WORKSPACE_OWNER", "", "update", noPicking(t), m)
		require.NoError(t, err)
		assert.Equal(t, "WORKSPACE_OWNER", got.Role)
		assert.Equal(t, "t2", got.ID)
	})

	t.Run("a remove names the Workspace", func(t *testing.T) {
		m := resultClient(org)
		m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "t2", mock.Anything).Return(rolesOK(), nil)
		got, err := RemoveOrgTokenWorkspaceRole("t2", "", "", noPicking(t), m)
		require.NoError(t, err)
		assert.Equal(t, apitoken.WorkspaceRemoval{ID: "t2", Name: "two", Scope: "ORGANIZATION", WorkspaceID: workspaceID, Action: apitoken.Removed}, got)
	})
}

func TestFindTokenAsksThePicker(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	a := resultToken("t1", "same", astrov1.ApiTokenScopeWORKSPACE, "WORKSPACE_MEMBER")
	b := resultToken("t2", "same", astrov1.ApiTokenScopeWORKSPACE, "WORKSPACE_OWNER")

	t.Run("naming nothing", func(t *testing.T) {
		var heading string
		var offered []apitoken.Token
		pick := func(h string, tokens []apitoken.Token) (int, error) {
			heading, offered = h, tokens
			return 1, nil
		}
		got, err := FindToken("", "", workspaceID, "test-org-id", nil, pick, resultClient(a, b))
		require.NoError(t, err)
		assert.Equal(t, "t2", got.Id)
		assert.Empty(t, heading, "the picker asks the plain choice in its own words")
		require.Len(t, offered, 2)
		assert.Equal(t, "WORKSPACE_OWNER", offered[1].Role)
	})

	t.Run("naming no Workspace offers each role on the current one", func(t *testing.T) {
		var offered []apitoken.Token
		pick := func(_ string, tokens []apitoken.Token) (int, error) {
			offered = tokens
			return 0, nil
		}
		_, err := FindToken("", "", "", "test-org-id", nil, pick, resultClient(a, b))
		require.NoError(t, err)
		require.Len(t, offered, 2)
		assert.Equal(t, "WORKSPACE_MEMBER", offered[0].Role)
		assert.Equal(t, "WORKSPACE_OWNER", offered[1].Role)
	})

	t.Run("naming a name two tokens share", func(t *testing.T) {
		var heading string
		pick := func(h string, _ []apitoken.Token) (int, error) {
			heading = h
			return 0, nil
		}
		got, err := FindToken("", "same", workspaceID, "test-org-id", nil, pick, resultClient(a, b))
		require.NoError(t, err)
		assert.Equal(t, "t1", got.Id)
		assert.Equal(t, "\nThere are more than one API tokens with name same. Please select an API token:", heading)
	})

	t.Run("a pick out of range is an error, not a panic", func(t *testing.T) {
		pick := func(string, []apitoken.Token) (int, error) { return 5, nil }
		_, err := FindToken("", "", workspaceID, "test-org-id", nil, pick, resultClient(a, b))
		assert.Error(t, err)
	})
}
