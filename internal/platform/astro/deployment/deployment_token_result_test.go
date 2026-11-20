package deployment

import (
	"errors"
	"net/http"
	"slices"
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

func resultToken(id, name string, scope astrov1.ApiTokenScope, role string) astrov1.ApiToken {
	return astrov1.ApiToken{
		Id: id, Name: name, Description: "about " + name, Scope: scope,
		Roles:     &[]astrov1.ApiTokenRole{{EntityType: astrov1.ApiTokenRoleEntityTypeDEPLOYMENT, EntityId: deploymentID, Role: role}},
		CreatedAt: resultCreated,
		CreatedBy: &astrov1.BasicSubjectProfile{FullName: &fullName1},
	}
}

func resultClient(t *testing.T, tokens ...astrov1.ApiToken) *astrov1_mocks.ClientWithResponsesInterface {
	m := astrov1_mocks.NewClientWithResponsesInterface(t)
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

// ownTokens is the scope an update acted on before it took tokens of a
// broader scope: the Deployment's own. The tests of that behavior keep it.
var ownTokens = []DeploymentTokenType{DeploymentTokenTypeDEPLOYMENT}

func noPicking(t *testing.T) apitoken.Picker {
	return func(string, []apitoken.Token) (int, error) {
		t.Fatal("the picker was asked, though the command named its token")
		return 0, nil
	}
}

func TestListTokensResult(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("each token with its role on the Deployment", func(t *testing.T) {
		dep := resultToken("t1", "one", astrov1.ApiTokenScopeDEPLOYMENT, "DEPLOYMENT_ADMIN")
		org := resultToken("t2", "two", astrov1.ApiTokenScopeORGANIZATION, "DEPLOYMENT_MEMBER")
		org.CreatedBy = &astrov1.BasicSubjectProfile{ApiTokenName: &fullName2}
		org.Token = &token // a list must not pass a secret on
		got, err := ListTokens(resultClient(t, dep, org), deploymentID, nil)
		require.NoError(t, err)
		assert.Equal(t, []apitoken.Token{
			{ID: "t1", Name: "one", Description: "about one", Scope: "DEPLOYMENT", Role: "DEPLOYMENT_ADMIN", CreatedAt: resultCreated, CreatedBy: fullName1},
			{ID: "t2", Name: "two", Description: "about two", Scope: "ORGANIZATION", Role: "DEPLOYMENT_MEMBER", CreatedAt: resultCreated, CreatedBy: fullName2},
		}, got)
	})

	t.Run("filtered by scope", func(t *testing.T) {
		dep := resultToken("t1", "one", astrov1.ApiTokenScopeDEPLOYMENT, "DEPLOYMENT_ADMIN")
		org := resultToken("t2", "two", astrov1.ApiTokenScopeORGANIZATION, "DEPLOYMENT_MEMBER")
		got, err := ListTokens(resultClient(t, dep, org), deploymentID, []DeploymentTokenType{DeploymentTokenTypeORGANIZATION})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "t2", got[0].ID)
	})

	t.Run("none is empty, not nil", func(t *testing.T) {
		got, err := ListTokens(resultClient(t), deploymentID, nil)
		require.NoError(t, err)
		assert.NotNil(t, got)
		assert.Empty(t, got)
	})
}

func TestCreateTokenResult(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	created := resultToken("t1", "one", astrov1.ApiTokenScopeDEPLOYMENT, "DEPLOYMENT_ADMIN")
	created.Token = &token
	end := resultCreated.Add(24 * time.Hour)
	created.EndAt = &end
	m := resultClient(t)
	m.On("CreateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.CreateApiTokenResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &created,
	}, nil)

	got, err := CreateToken("one", "about one", "DEPLOYMENT_ADMIN", deploymentID, 1, m)
	require.NoError(t, err)
	assert.Equal(t, apitoken.Token{
		ID: "t1", Name: "one", Description: "about one", Scope: "DEPLOYMENT", Role: "DEPLOYMENT_ADMIN",
		CreatedAt: resultCreated, CreatedBy: fullName1, ExpiresAt: &end, Token: token,
	}, got)
}

func TestUpdateTokenResult(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	before := resultToken("t1", "one", astrov1.ApiTokenScopeDEPLOYMENT, "DEPLOYMENT_ADMIN")
	after := before
	after.Name = "uno"
	m := resultClient(t, before)
	m.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, "t1", mock.Anything).Return(&astrov1.UpdateApiTokenResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &after,
	}, nil)
	m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "t1", mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)

	got, err := UpdateToken("t1", "", "uno", "", "DEPLOYMENT_MEMBER", deploymentID, ownTokens, noPicking(t), m)
	require.NoError(t, err)
	assert.Equal(t, "one", got.PreviousName, "the text names the token as it was")
	assert.Equal(t, "uno", got.Token.Name)
	assert.Equal(t, "DEPLOYMENT_MEMBER", got.Token.Role)
	assert.Empty(t, got.Token.Token, "an update has no secret to report")
}

func TestRotateTokenResult(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	tok := resultToken("t1", "one", astrov1.ApiTokenScopeDEPLOYMENT, "DEPLOYMENT_ADMIN")
	rotated := tok
	rotated.Token = &token
	m := resultClient(t, tok)
	m.On("RotateApiTokenWithResponse", mock.Anything, mock.Anything, "t1").Return(&astrov1.RotateApiTokenResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &rotated,
	}, nil)

	got, err := RotateToken(tok, deploymentID, m)
	require.NoError(t, err)
	assert.Equal(t, token, got.Token)
	assert.Equal(t, "DEPLOYMENT_ADMIN", got.Role)
}

func TestDeleteTokenResult(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("a Deployment token is deleted", func(t *testing.T) {
		tok := resultToken("t1", "one", astrov1.ApiTokenScopeDEPLOYMENT, "DEPLOYMENT_ADMIN")
		m := resultClient(t, tok)
		m.On("DeleteApiTokenWithResponse", mock.Anything, mock.Anything, "t1").Return(&DeleteDeploymentAPITokenResponseOK, nil)
		got, err := DeleteToken(tok, deploymentID, m)
		require.NoError(t, err)
		assert.Equal(t, apitoken.DeploymentRemoval{ID: "t1", Name: "one", Scope: "DEPLOYMENT", DeploymentID: deploymentID, Action: apitoken.Deleted}, got)
	})

	t.Run("a token of a broader scope is not deleted, and nothing is sent", func(t *testing.T) {
		tok := resultToken("t2", "two", astrov1.ApiTokenScopeWORKSPACE, "DEPLOYMENT_MEMBER")
		m := resultClient(t, tok)
		_, err := DeleteToken(tok, deploymentID, m)
		require.ErrorIs(t, err, errNotDeploymentScoped)
		m.AssertNotCalled(t, "DeleteApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything)
		m.AssertNotCalled(t, "UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})
}

func TestRemoveTokenResult(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	for _, scope := range []astrov1.ApiTokenScope{astrov1.ApiTokenScopeWORKSPACE, astrov1.ApiTokenScopeORGANIZATION} {
		t.Run("a "+string(scope)+" token loses its Deployment role", func(t *testing.T) {
			tok := resultToken("t2", "two", scope, "DEPLOYMENT_MEMBER")
			m := resultClient(t, tok)
			m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "t2", mock.MatchedBy(func(r astrov1.UpdateApiTokenRolesRequest) bool {
				return len(r.Roles) == 0
			})).Return(&UpdateOrganizationAPITokenResponseOK, nil)
			got, err := RemoveToken(tok, deploymentID, m)
			require.NoError(t, err)
			assert.Equal(t, apitoken.DeploymentRemoval{ID: "t2", Name: "two", Scope: string(scope), DeploymentID: deploymentID, Action: apitoken.Removed}, got)
		})
	}

	t.Run("a Deployment token is not removed from its own Deployment, and nothing is sent", func(t *testing.T) {
		tok := resultToken("t1", "one", astrov1.ApiTokenScopeDEPLOYMENT, "DEPLOYMENT_ADMIN")
		m := resultClient(t, tok)
		_, err := RemoveToken(tok, deploymentID, m)
		require.ErrorIs(t, err, errDeploymentScoped)
		m.AssertNotCalled(t, "UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})
}

func TestUpdateTokenOfABroaderToken(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	for _, scope := range []astrov1.ApiTokenScope{astrov1.ApiTokenScopeWORKSPACE, astrov1.ApiTokenScopeORGANIZATION} {
		tok := resultToken("t2", "two", scope, "DEPLOYMENT_MEMBER")

		t.Run(string(scope)+": changes its role on the Deployment and nothing else", func(t *testing.T) {
			m := resultClient(t, tok)
			m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "t2", mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
			got, err := UpdateToken("t2", "", "", "", "DEPLOYMENT_ADMIN", deploymentID, nil, noPicking(t), m)
			require.NoError(t, err)
			assert.Equal(t, "DEPLOYMENT_ADMIN", got.Token.Role)
			assert.Equal(t, string(scope), got.Token.Scope)
			m.AssertNotCalled(t, "UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})

		t.Run(string(scope)+": is not renamed from a Deployment, and nothing is sent", func(t *testing.T) {
			m := resultClient(t, tok)
			_, err := UpdateToken("t2", "", "uno", "", "DEPLOYMENT_ADMIN", deploymentID, nil, noPicking(t), m)
			require.ErrorIs(t, err, ErrBroaderTokenNotRenamed)
			assert.True(t, usageMarked(err), "a rename of a broader token from a Deployment is a usage error")
			m.AssertNotCalled(t, "UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})

		t.Run(string(scope)+": refuses a role it already holds", func(t *testing.T) {
			m := resultClient(t, tok)
			_, err := UpdateToken("t2", "", "", "", "DEPLOYMENT_MEMBER", deploymentID, nil, noPicking(t), m)
			if scope == astrov1.ApiTokenScopeWORKSPACE {
				require.ErrorIs(t, err, errWorkspaceTokenInDeployment)
			} else {
				require.ErrorIs(t, err, errOrgTokenInDeployment)
			}
		})
	}
}

// usageMarked reports whether err is marked as a usage error the way cliout
// recognizes one from a package that cannot import it.
func usageMarked(err error) bool {
	var m interface{ IsUsageError() bool }
	return errors.As(err, &m) && m.IsUsageError()
}

// addClient serves what AddToken looks among: the Workspace's tokens, the
// Organization's, and each token by its ID, telling the two lists apart by
// the filter the request carries.
func addClient(t *testing.T, workspaceTokens, orgTokens []astrov1.ApiToken, byID ...astrov1.ApiToken) *astrov1_mocks.ClientWithResponsesInterface {
	t.Helper()
	m := astrov1_mocks.NewClientWithResponsesInterface(t)
	list := func(tokens []astrov1.ApiToken) *astrov1.ListApiTokensResponse {
		return &astrov1.ListApiTokensResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1.ApiTokensPaginated{Tokens: tokens, TotalCount: len(tokens)},
		}
	}
	m.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListApiTokensParams) bool {
		return p.WorkspaceId != nil
	})).Return(list(workspaceTokens), nil).Maybe()
	m.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListApiTokensParams) bool {
		return p.IncludeOnlyOrganizationTokens != nil
	})).Return(list(orgTokens), nil).Maybe()
	all := slices.Concat(workspaceTokens, orgTokens, byID)
	for i := range all {
		m.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, all[i].Id).Return(&astrov1.GetApiTokenResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &all[i],
		}, nil).Maybe()
	}
	return m
}

// AddToken works out from the token which scope it is: a Workspace token or
// an Organization token, found by its ID, its name or a pick.
func TestAddTokenWorksOutTheScope(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ws := resultToken("w1", "ws-token", astrov1.ApiTokenScopeWORKSPACE, "")
	org := resultToken("o1", "org-token", astrov1.ApiTokenScopeORGANIZATION, "")
	own := resultToken("d1", "dep-token", astrov1.ApiTokenScopeDEPLOYMENT, "DEPLOYMENT_ADMIN")
	setsRole := func(m *astrov1_mocks.ClientWithResponsesInterface, id string) {
		m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, id, mock.MatchedBy(func(r astrov1.UpdateApiTokenRolesRequest) bool {
			return len(r.Roles) == 1 && r.Roles[0].EntityId == deploymentID && r.Roles[0].Role == "DEPLOYMENT_ADMIN"
		})).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
	}

	t.Run("by ID, a Workspace token", func(t *testing.T) {
		m := addClient(t, nil, nil, ws)
		setsRole(m, "w1")
		got, err := AddToken("w1", "", "DEPLOYMENT_ADMIN", "", deploymentID, nil, noPicking(t), m)
		require.NoError(t, err)
		assert.Equal(t, "WORKSPACE", got.Scope)
		assert.Equal(t, "DEPLOYMENT_ADMIN", got.Role)
	})

	t.Run("by ID, an Organization token", func(t *testing.T) {
		m := addClient(t, nil, nil, org)
		setsRole(m, "o1")
		got, err := AddToken("o1", "", "DEPLOYMENT_ADMIN", "", deploymentID, nil, noPicking(t), m)
		require.NoError(t, err)
		assert.Equal(t, "ORGANIZATION", got.Scope)
	})

	t.Run("by name, a Workspace token is found in the Workspace and an Organization token in the Organization", func(t *testing.T) {
		m := addClient(t, []astrov1.ApiToken{ws}, []astrov1.ApiToken{org})
		setsRole(m, "w1")
		got, err := AddToken("", "ws-token", "DEPLOYMENT_ADMIN", "ws-id", deploymentID, nil, noPicking(t), m)
		require.NoError(t, err)
		assert.Equal(t, "WORKSPACE", got.Scope)

		m = addClient(t, []astrov1.ApiToken{ws}, []astrov1.ApiToken{org})
		setsRole(m, "o1")
		got, err = AddToken("", "org-token", "DEPLOYMENT_ADMIN", "ws-id", deploymentID, nil, noPicking(t), m)
		require.NoError(t, err)
		assert.Equal(t, "ORGANIZATION", got.Scope)
	})

	t.Run("by name, a name both scopes have is the picker's to settle", func(t *testing.T) {
		wsSame := resultToken("w2", "same", astrov1.ApiTokenScopeWORKSPACE, "")
		orgSame := resultToken("o2", "same", astrov1.ApiTokenScopeORGANIZATION, "")
		var offered []apitoken.Token
		pick := func(_ string, tokens []apitoken.Token) (int, error) {
			offered = tokens
			return 1, nil
		}
		m := addClient(t, []astrov1.ApiToken{wsSame}, []astrov1.ApiToken{orgSame})
		setsRole(m, "o2")
		got, err := AddToken("", "same", "DEPLOYMENT_ADMIN", "ws-id", deploymentID, nil, pick, m)
		require.NoError(t, err)
		assert.Equal(t, "ORGANIZATION", got.Scope, "the second of the two offered, the Workspace's first")
		require.Len(t, offered, 2)
		assert.Equal(t, "WORKSPACE", offered[0].Scope)
	})

	t.Run("naming nothing, the picker is offered both kinds", func(t *testing.T) {
		var offered []apitoken.Token
		pick := func(_ string, tokens []apitoken.Token) (int, error) {
			offered = tokens
			return 0, nil
		}
		m := addClient(t, []astrov1.ApiToken{ws}, []astrov1.ApiToken{org})
		setsRole(m, "w1")
		got, err := AddToken("", "", "DEPLOYMENT_ADMIN", "ws-id", deploymentID, nil, pick, m)
		require.NoError(t, err)
		assert.Equal(t, "WORKSPACE", got.Scope)
		assert.Len(t, offered, 2)
	})

	t.Run("with no Workspace to look in, a name is looked for among the Organization's alone", func(t *testing.T) {
		m := addClient(t, []astrov1.ApiToken{ws}, []astrov1.ApiToken{org})
		_, err := AddToken("", "ws-token", "DEPLOYMENT_ADMIN", "", deploymentID, nil, noPicking(t), m)
		require.ErrorIs(t, err, ErrTokenNotFound)

		setsRole(m, "o1")
		got, err := AddToken("", "org-token", "DEPLOYMENT_ADMIN", "", deploymentID, nil, noPicking(t), m)
		require.NoError(t, err)
		assert.Equal(t, "ORGANIZATION", got.Scope)
	})

	t.Run("a blank role is refused before anything is looked up", func(t *testing.T) {
		m := addClient(t, nil, nil, ws)
		_, err := AddToken("w1", "", "", "", deploymentID, nil, noPicking(t), m)
		require.ErrorIs(t, err, errRoleRequired)
		assert.True(t, usageMarked(err))
	})

	t.Run("a Deployment token is refused as a usage error, and nothing is sent", func(t *testing.T) {
		m := addClient(t, nil, nil, own)
		_, err := AddToken("d1", "", "DEPLOYMENT_ADMIN", "", deploymentID, nil, noPicking(t), m)
		require.ErrorIs(t, err, errDeploymentScopedAdd)
		assert.True(t, usageMarked(err))
	})

	t.Run("a command that takes one kind refuses the other", func(t *testing.T) {
		m := addClient(t, nil, nil, org)
		_, err := AddToken("o1", "", "DEPLOYMENT_ADMIN", "", deploymentID, []DeploymentTokenType{DeploymentTokenTypeWORKSPACE}, noPicking(t), m)
		require.ErrorIs(t, err, errWrongTokenTypeSelected)
	})

	t.Run("a role the token holds already is refused for what it is", func(t *testing.T) {
		held := resultToken("w1", "ws-token", astrov1.ApiTokenScopeWORKSPACE, "DEPLOYMENT_ADMIN")
		m := addClient(t, nil, nil, held)
		_, err := AddToken("w1", "", "DEPLOYMENT_ADMIN", "", deploymentID, nil, noPicking(t), m)
		require.ErrorIs(t, err, errWorkspaceTokenInDeployment)
	})
}

func TestFindTokenAsksThePicker(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	a := resultToken("t1", "same", astrov1.ApiTokenScopeDEPLOYMENT, "DEPLOYMENT_ADMIN")
	b := resultToken("t2", "same", astrov1.ApiTokenScopeDEPLOYMENT, "DEPLOYMENT_MEMBER")

	t.Run("naming nothing", func(t *testing.T) {
		var heading string
		var offered []apitoken.Token
		pick := func(h string, tokens []apitoken.Token) (int, error) {
			heading, offered = h, tokens
			return 1, nil
		}
		got, err := FindToken("", "", deploymentID, nil, pick, resultClient(t, a, b))
		require.NoError(t, err)
		assert.Equal(t, "t2", got.Id)
		assert.Empty(t, heading, "the picker asks the plain choice in its own words")
		require.Len(t, offered, 2)
		assert.Equal(t, "DEPLOYMENT_MEMBER", offered[1].Role)
	})

	t.Run("naming a name two tokens share", func(t *testing.T) {
		var heading string
		pick := func(h string, _ []apitoken.Token) (int, error) {
			heading = h
			return 0, nil
		}
		got, err := FindToken("", "same", deploymentID, nil, pick, resultClient(t, a, b))
		require.NoError(t, err)
		assert.Equal(t, "t1", got.Id)
		assert.Equal(t, "\nSeveral API tokens are named same. Select an API token:", heading)
	})

	t.Run("a picker's refusal is the error", func(t *testing.T) {
		refused := errors.New("cannot ask")
		pick := func(string, []apitoken.Token) (int, error) { return 0, refused }
		_, err := FindToken("", "", deploymentID, nil, pick, resultClient(t, a, b))
		assert.ErrorIs(t, err, refused)
	})

	t.Run("a pick out of range is an error, not a panic", func(t *testing.T) {
		pick := func(string, []apitoken.Token) (int, error) { return 5, nil }
		_, err := FindToken("", "", deploymentID, nil, pick, resultClient(t, a, b))
		assert.Error(t, err)
	})
}
