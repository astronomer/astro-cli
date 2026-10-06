package deployment

import (
	"errors"
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

func resultToken(id, name string, scope astrov1.ApiTokenScope, role string) astrov1.ApiToken {
	return astrov1.ApiToken{
		Id: id, Name: name, Description: "about " + name, Scope: scope,
		Roles:     &[]astrov1.ApiTokenRole{{EntityType: astrov1.ApiTokenRoleEntityTypeDEPLOYMENT, EntityId: deploymentID, Role: role}},
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
		got, err := ListTokens(resultClient(dep, org), deploymentID, nil)
		require.NoError(t, err)
		assert.Equal(t, []apitoken.Token{
			{ID: "t1", Name: "one", Description: "about one", Scope: "DEPLOYMENT", Role: "DEPLOYMENT_ADMIN", CreatedAt: resultCreated, CreatedBy: fullName1},
			{ID: "t2", Name: "two", Description: "about two", Scope: "ORGANIZATION", Role: "DEPLOYMENT_MEMBER", CreatedAt: resultCreated, CreatedBy: fullName2},
		}, got)
	})

	t.Run("filtered by scope", func(t *testing.T) {
		dep := resultToken("t1", "one", astrov1.ApiTokenScopeDEPLOYMENT, "DEPLOYMENT_ADMIN")
		org := resultToken("t2", "two", astrov1.ApiTokenScopeORGANIZATION, "DEPLOYMENT_MEMBER")
		got, err := ListTokens(resultClient(dep, org), deploymentID, []DeploymentTokenType{DeploymentTokenTypeORGANIZATION})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "t2", got[0].ID)
	})

	t.Run("none is empty, not nil", func(t *testing.T) {
		got, err := ListTokens(resultClient(), deploymentID, nil)
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
	m := resultClient()
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
	m := resultClient(before)
	m.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, "t1", mock.Anything).Return(&astrov1.UpdateApiTokenResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &after,
	}, nil)
	m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "t1", mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)

	got, err := UpdateToken("t1", "", "uno", "", "DEPLOYMENT_MEMBER", deploymentID, noPicking(t), m)
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
	m := resultClient(tok)
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
		m := resultClient(tok)
		m.On("DeleteApiTokenWithResponse", mock.Anything, mock.Anything, "t1").Return(&DeleteDeploymentAPITokenResponseOK, nil)
		got, err := DeleteToken(tok, deploymentID, m)
		require.NoError(t, err)
		assert.Equal(t, apitoken.DeploymentRemoval{ID: "t1", Name: "one", Scope: "DEPLOYMENT", DeploymentID: deploymentID, Action: apitoken.Deleted}, got)
	})

	t.Run("any other token loses its Deployment role", func(t *testing.T) {
		tok := resultToken("t2", "two", astrov1.ApiTokenScopeWORKSPACE, "DEPLOYMENT_MEMBER")
		m := resultClient(tok)
		m.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "t2", mock.MatchedBy(func(r astrov1.UpdateApiTokenRolesRequest) bool {
			return len(r.Roles) == 0
		})).Return(&UpdateOrganizationAPITokenResponseOK, nil)
		got, err := DeleteToken(tok, deploymentID, m)
		require.NoError(t, err)
		assert.Equal(t, apitoken.DeploymentRemoval{ID: "t2", Name: "two", Scope: "WORKSPACE", DeploymentID: deploymentID, Action: apitoken.Removed}, got)
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
		got, err := FindToken("", "", deploymentID, nil, pick, resultClient(a, b))
		require.NoError(t, err)
		assert.Equal(t, "t2", got.Id)
		assert.Equal(t, "\nPlease select the Deployment API token:", heading)
		require.Len(t, offered, 2)
		assert.Equal(t, "DEPLOYMENT_MEMBER", offered[1].Role)
	})

	t.Run("naming a name two tokens share", func(t *testing.T) {
		var heading string
		pick := func(h string, _ []apitoken.Token) (int, error) {
			heading = h
			return 0, nil
		}
		got, err := FindToken("", "same", deploymentID, nil, pick, resultClient(a, b))
		require.NoError(t, err)
		assert.Equal(t, "t1", got.Id)
		assert.Equal(t, "\nThere are more than one API tokens with name same. Please select an API token:", heading)
	})

	t.Run("a picker's refusal is the error", func(t *testing.T) {
		refused := errors.New("cannot ask")
		pick := func(string, []apitoken.Token) (int, error) { return 0, refused }
		_, err := FindToken("", "", deploymentID, nil, pick, resultClient(a, b))
		assert.ErrorIs(t, err, refused)
	})

	t.Run("a pick out of range is an error, not a panic", func(t *testing.T) {
		pick := func(string, []apitoken.Token) (int, error) { return 5, nil }
		_, err := FindToken("", "", deploymentID, nil, pick, resultClient(a, b))
		assert.Error(t, err)
	})
}
