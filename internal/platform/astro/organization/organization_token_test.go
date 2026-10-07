package organization

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/internal/platform/astro/apitoken"
	"github.com/astronomer/astro-cli/internal/platform/astro/apitoken/apitokentest"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/internal/platform/astro/user"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

var (
	description1 = "Description 1"
	description2 = "Description 2"
	fullName1    = "User 1"
	fullName2    = "User 2"
	token        = "token"
	workspaceID  = "ck05r3bor07h40d02y2hw4n4v"

	iamAPIToken = astrov1.ApiToken{
		Id:          "token1",
		Name:        "Token 1",
		Token:       &token,
		Description: description1,
		Scope:       astrov1.ApiTokenScope("ORGANIZATION"),
		Roles: &[]astrov1.ApiTokenRole{
			{EntityType: astrov1.ApiTokenRoleEntityTypeORGANIZATION, EntityId: "test-org-id", Role: "ORGANIZATION_MEMBER"},
			{EntityType: astrov1.ApiTokenRoleEntityTypeWORKSPACE, EntityId: workspaceID, Role: "WORKSPACE_AUTHOR"},
			{EntityType: astrov1.ApiTokenRoleEntityTypeWORKSPACE, EntityId: "WORKSPACE", Role: "WORKSPACE_AUTHOR"},
			{EntityType: astrov1.ApiTokenRoleEntityTypeDEPLOYMENT, EntityId: "DEPLOYMENT", Role: "DEPLOYMENT_ADMIN"},
		},
		CreatedAt: time.Now(),
		CreatedBy: &astrov1.BasicSubjectProfile{FullName: &fullName1},
	}
	GetAPITokensResponseOK = astrov1.GetApiTokenResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &iamAPIToken,
	}
	errorTokenGet, _ = json.Marshal(astrov1.Error{Message: "failed to get token"})

	iamAPIWorkspaceToken = astrov1.ApiToken{
		Id:          "token1",
		Name:        "token1",
		Token:       &token,
		Description: description1,
		Scope:       astrov1.ApiTokenScope("WORKSPACE"),
		Roles: &[]astrov1.ApiTokenRole{
			{EntityType: astrov1.ApiTokenRoleEntityTypeWORKSPACE, EntityId: workspaceID, Role: "WORKSPACE_AUTHOR"},
			{EntityType: astrov1.ApiTokenRoleEntityTypeDEPLOYMENT, EntityId: "DEPLOYMENT", Role: "DEPLOYMENT_ADMIN"},
		},
		CreatedAt: time.Now(),
		CreatedBy: &astrov1.BasicSubjectProfile{FullName: &fullName1},
	}

	GetAPITokensResponseOKWorkspaceToken = astrov1.GetApiTokenResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &iamAPIWorkspaceToken,
	}

	GetAPITokensResponseError = astrov1.GetApiTokenResponse{
		HTTPResponse: &http.Response{StatusCode: 500},
		Body:         errorTokenGet,
		JSON200:      nil,
	}

	apiToken1 = astrov1.ApiToken{
		Id:          "token1",
		Name:        "Token 1",
		Token:       &token,
		Description: description1,
		Scope:       astrov1.ApiTokenScope("ORGANIZATION"),
		Roles: &[]astrov1.ApiTokenRole{
			{EntityType: astrov1.ApiTokenRoleEntityTypeWORKSPACE, EntityId: workspaceID, Role: "ORGANIZATION_MEMBER"},
			{EntityType: astrov1.ApiTokenRoleEntityTypeORGANIZATION, EntityId: "ORGANIZATION", Role: "ORGANIZATION_MEMBER"},
		},
		CreatedAt: time.Now(),
		CreatedBy: &astrov1.BasicSubjectProfile{FullName: &fullName1},
	}
	apiToken2 = astrov1.ApiToken{
		Id:          "token1-twin", // shares Token 1's name, so naming it is a pick
		Name:        "Token 1",
		Token:       &token,
		Description: description1,
		Scope:       astrov1.ApiTokenScope("ORGANIZATION"),
		Roles: &[]astrov1.ApiTokenRole{
			{EntityType: astrov1.ApiTokenRoleEntityTypeORGANIZATION, EntityId: "ORGANIZATION", Role: "ORGANIZATION_MEMBER"},
			{EntityType: astrov1.ApiTokenRoleEntityTypeWORKSPACE, EntityId: "WORKSPACE", Role: "WORKSPACE_MEMBER"},
			{EntityType: astrov1.ApiTokenRoleEntityTypeDEPLOYMENT, EntityId: "DEPLOYMENT", Role: "DEPLOYMENT_ADMIN"},
		},
		CreatedAt: time.Now(),
		CreatedBy: &astrov1.BasicSubjectProfile{FullName: &fullName1},
	}

	apiTokens = []astrov1.ApiToken{
		apiToken1,
		{
			Id:          "token2",
			Name:        "Token 2",
			Description: description2,
			Scope:       astrov1.ApiTokenScope("ORGANIZATION"),
			Roles: &[]astrov1.ApiTokenRole{
				{EntityType: astrov1.ApiTokenRoleEntityTypeORGANIZATION, EntityId: "ORGANIZATION", Role: "ORGANIZATION_MEMBER"},
			},
			CreatedAt: time.Now(),
			CreatedBy: &astrov1.BasicSubjectProfile{FullName: &fullName2},
		},
	}
	apiTokens2 = []astrov1.ApiToken{
		apiToken1,
		apiToken2,
	}
	ListOrganizationAPITokensResponseOK = astrov1.ListApiTokensResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.ApiTokensPaginated{
			Tokens:     apiTokens,
			Limit:      1,
			Offset:     0,
			TotalCount: len(apiTokens),
		},
	}
	ListOrganizationAPITokensResponse2O0 = astrov1.ListApiTokensResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.ApiTokensPaginated{
			Tokens:     apiTokens2,
			Limit:      1,
			Offset:     0,
			TotalCount: len(apiTokens2),
		},
	}
	errorBodyList, _ = json.Marshal(astrov1.Error{Message: "failed to list tokens"})

	ListOrganizationAPITokensResponseError = astrov1.ListApiTokensResponse{
		HTTPResponse: &http.Response{StatusCode: 500},
		Body:         errorBodyList,
		JSON200:      nil,
	}

	CreateOrganizationAPITokenResponseOK = astrov1.CreateApiTokenResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &apiToken1,
	}

	errorBodyUpdate, _ = json.Marshal(astrov1.Error{Message: "failed to update token"})

	UpdateOrganizationAPITokenResponseOK = astrov1.UpdateApiTokenResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &apiToken1,
	}
	UpdateOrganizationAPITokenResponseError = astrov1.UpdateApiTokenResponse{
		HTTPResponse: &http.Response{StatusCode: 500},
		Body:         errorBodyUpdate,
		JSON200:      nil,
	}
	UpdateAPITokenRolesResponseOK = astrov1.UpdateApiTokenRolesResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
	}
	UpdateAPITokenRolesResponseError = astrov1.UpdateApiTokenRolesResponse{
		HTTPResponse: &http.Response{StatusCode: 500},
		Body:         errorBodyUpdate,
	}
	RotateOrganizationAPITokenResponseOK = astrov1.RotateApiTokenResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &apiToken1,
	}
	RotateOrganizationAPITokenResponseError = astrov1.RotateApiTokenResponse{
		HTTPResponse: &http.Response{StatusCode: 500},
		Body:         errorBodyUpdate,
		JSON200:      nil,
	}
	DeleteOrganizationAPITokenResponseOK = astrov1.DeleteApiTokenResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
	}
	DeleteOrganizationAPITokenResponseError = astrov1.DeleteApiTokenResponse{
		HTTPResponse: &http.Response{StatusCode: 500},
		Body:         errorBodyUpdate,
	}
)

// pickIndex is a picker that chooses tokens[i], recording what it was offered.
func pickIndex(i int, offered *[]apitoken.Token) apitoken.Picker {
	return func(_ string, tokens []apitoken.Token) (int, error) {
		if offered != nil {
			*offered = tokens
		}
		return i, nil
	}
}

// noPicking fails the test if the picker is asked.
func (s *Suite) noPicking() apitoken.Picker {
	return func(string, []apitoken.Token) (int, error) {
		s.Fail("the picker was asked, though the command named its token")
		return 0, nil
	}
}

func (s *Suite) TestAddOrgTokenToWorkspace() {
	var (
		workspace          = workspaceID
		role               = "WORKSPACE_MEMBER"
		selectedTokenID    = "token1"
		selectedTokenName  = "Token 1"
		selectedTokenName2 = "Token 2"
	)

	s.Run("return error for invalid workspace role", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		_, err := AddOrgTokenToWorkspace(selectedTokenID, selectedTokenName, "INVALID_ROLE", workspace, s.noPicking(), nil)
		s.Equal(user.ErrInvalidWorkspaceRole, err)
	})

	s.Run("return error for failed to get current context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		_, err := AddOrgTokenToWorkspace(selectedTokenID, selectedTokenName, role, workspace, s.noPicking(), mockClient)
		s.Error(err)
	})

	s.Run("return error for failed to list organization tokens", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseError, nil)
		_, err := AddOrgTokenToWorkspace("", selectedTokenName, role, workspace, s.noPicking(), mockClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("return error for a name no token has", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		_, err := AddOrgTokenToWorkspace("", "Invalid name", role, workspace, s.noPicking(), mockClient)
		s.Equal(errOrganizationTokenNotFound, err)
	})

	s.Run("return error for organization token already in workspace", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		_, err := AddOrgTokenToWorkspace("", selectedTokenName, "WORKSPACE_AUTHOR", workspace, s.noPicking(), mockClient)
		s.Equal(errOrgTokenInWorkspace, err)
	})

	s.Run("add by id returns the token with its new workspace role", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateAPITokenRolesResponseOK, nil)
		got, err := AddOrgTokenToWorkspace(selectedTokenID, "", role, workspace, s.noPicking(), mockClient)
		s.NoError(err)
		s.Equal("token1", got.ID)
		s.Equal(role, got.Role)
		s.Empty(got.Token, "an add carries no secret")
	})

	s.Run("error add by id - wrong token type", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)
		_, err := AddOrgTokenToWorkspace(selectedTokenID, "", role, workspace, s.noPicking(), mockClient)
		s.ErrorContains(err, "the token selected is not of the type you are trying to modify")
	})

	s.Run("return error for failed to get organization token", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)
		_, err := AddOrgTokenToWorkspace(selectedTokenID, "", role, workspace, s.noPicking(), mockClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("add by name", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateAPITokenRolesResponseOK, nil)
		_, err := AddOrgTokenToWorkspace("", selectedTokenName2, role, workspace, s.noPicking(), mockClient)
		s.NoError(err)
	})

	s.Run("picked, the picker is offered each token with its organization role", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, "token2").Return(&GetAPITokensResponseOK, nil)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateAPITokenRolesResponseOK, nil)
		var offered []apitoken.Token
		_, err := AddOrgTokenToWorkspace("", "", role, workspace, pickIndex(1, &offered), mockClient)
		s.NoError(err)
		s.Require().Len(offered, 2)
		s.Equal("token2", offered[1].ID)
		s.Equal("ORGANIZATION_MEMBER", offered[1].Role)
	})

	s.Run("a shared name is picked among the tokens that share it", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponse2O0, nil)
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateAPITokenRolesResponseOK, nil)
		var offered []apitoken.Token
		_, err := AddOrgTokenToWorkspace("", selectedTokenName, role, workspace, pickIndex(1, &offered), mockClient)
		s.NoError(err)
		s.Len(offered, 2)
	})

	s.Run("a picker that refuses stops the add", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		refused := errors.New("refused")
		_, err := AddOrgTokenToWorkspace("", "", role, workspace, func(string, []apitoken.Token) (int, error) { return 0, refused }, mockClient)
		s.ErrorIs(err, refused)
	})
}

func (s *Suite) TestListTokens() {
	s.Run("each organization token with its organization role", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		got, err := ListTokens(mockClient)
		s.NoError(err)
		s.Require().Len(got, 2)
		s.Equal("token1", got[0].ID)
		s.Equal("ORGANIZATION_MEMBER", got[0].Role)
		s.Equal(fullName1, got[0].CreatedBy)
		s.Empty(got[0].Token, "a list never carries a secret")
	})

	s.Run("none is empty, not nil", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.ListApiTokensResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200:      &astrov1.ApiTokensPaginated{Tokens: []astrov1.ApiToken{}},
		}, nil)
		got, err := ListTokens(mockClient)
		s.NoError(err)
		s.NotNil(got)
		s.Empty(got)
	})

	s.Run("error path when ListApiTokensWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseError, nil)
		_, err := ListTokens(mockClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error getting current context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		_, err := ListTokens(mockClient)
		s.Error(err)
	})
}

func (s *Suite) TestCreateToken() {
	s.Run("returns the token with its secret and the role asked for", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("CreateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&CreateOrganizationAPITokenResponseOK, nil)
		got, err := CreateToken("Token 1", "Description 1", "ORGANIZATION_OWNER", 0, mockClient)
		s.NoError(err)
		s.Equal("token1", got.ID)
		s.Equal("ORGANIZATION_OWNER", got.Role)
		s.Equal(token, got.Token)
	})

	s.Run("error getting current context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		_, err := CreateToken("Token 1", "Description 1", "ORGANIZATION_MEMBER", 0, mockClient)
		s.Error(err)
	})

	s.Run("invalid role", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		_, err := CreateToken("Token 1", "Description 1", "InvalidRole", 0, mockClient)
		s.Error(err)
	})

	s.Run("empty name", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		_, err := CreateToken("", "Description 1", "ORGANIZATION_MEMBER", 0, mockClient)
		s.Equal(ErrInvalidName, err)
	})

	s.Run("API error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("CreateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.CreateApiTokenResponse{
			HTTPResponse: &http.Response{StatusCode: 500}, Body: errorBodyUpdate,
		}, nil)
		_, err := CreateToken("Token 1", "Description 1", "ORGANIZATION_MEMBER", 0, mockClient)
		s.ErrorContains(err, "failed to update token")
	})
}

func (s *Suite) TestUpdateToken() {
	s.Run("by id, keeps the role it has", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, "token1", apitokentest.RenamesTo("renamed", "new description")).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
		got, err := UpdateToken("token1", "", "renamed", "new description", "", s.noPicking(), mockClient)
		s.NoError(err)
		s.Equal("Token 1", got.PreviousName)
		s.Equal("ORGANIZATION_MEMBER", got.Token.Role)
		mockClient.AssertNotCalled(s.T(), "UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	s.Run("picked", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, "token2").Return(apitokentest.GotAs(&GetAPITokensResponseOK, "token2"), nil).Once()
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Once()
		// A new name alone keeps the description the token has.
		mockClient.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, "token2", apitokentest.RenamesTo("renamed", iamAPIToken.Description)).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
		_, err := UpdateToken("", "", "renamed", "", "", pickIndex(1, nil), mockClient)
		s.NoError(err)
		mockClient.AssertCalled(s.T(), "GetApiTokenWithResponse", mock.Anything, mock.Anything, "token2")
	})

	s.Run("a shared name is picked", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, "token1-twin").Return(apitokentest.GotAs(&GetAPITokensResponseOK, "token1-twin"), nil).Once()
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponse2O0, nil).Once()
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, "token1-twin", mock.Anything).Return(&UpdateAPITokenRolesResponseOK, nil).Once()
		mockClient.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, "token1-twin", apitokentest.RenamesTo("renamed", iamAPIToken.Description)).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
		got, err := UpdateToken("", "Token 1", "renamed", "", "ORGANIZATION_OWNER", pickIndex(1, nil), mockClient)
		s.NoError(err)
		s.Equal("ORGANIZATION_OWNER", got.Token.Role)
	})

	// An update with nothing to change sends nothing, and reports the token
	// as it is.
	s.Run("nothing to change", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil).Once()
		got, err := UpdateToken("token1", "", "", "", "", s.noPicking(), mockClient)
		s.NoError(err)
		s.Equal("Token 1", got.Token.Name)
		s.Equal("Token 1", got.PreviousName)
		s.Equal("ORGANIZATION_MEMBER", got.Token.Role)
	})

	s.Run("error path when listOrganizationTokens returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseError, nil)
		_, err := UpdateToken("", "", "", "", "", s.noPicking(), mockClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error path when no token has the name", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		_, err := UpdateToken("", "invalid name", "", "", "", s.noPicking(), mockClient)
		s.Equal(errOrganizationTokenNotFound, err)
	})

	s.Run("error path when getApiToken returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)
		_, err := UpdateToken("tokenId", "", "", "", "", s.noPicking(), mockClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error path when UpdateApiTokenWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseError, nil)
		_, err := UpdateToken("token3", "", "renamed", "", "", s.noPicking(), mockClient)
		s.Equal("failed to update token", err.Error())
	})

	s.Run("error path when there is no context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		_, err := UpdateToken("token1", "", "", "", "", s.noPicking(), mockClient)
		s.Error(err)
	})

	s.Run("a role that is not an Organization role is refused before any call", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		// No call is registered: any request would panic the mock.
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		_, err := UpdateToken("token1", "", "renamed", "", "Invalid Role", s.noPicking(), mockClient)
		s.Equal(user.ErrInvalidOrganizationRole.Error(), err.Error())
	})

	s.Run("a role the token already holds is refused before any change", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		// Only the lookup is registered: a rename or a role write would
		// panic the mock.
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		_, err := UpdateToken("token1", "", "renamed", "", "ORGANIZATION_MEMBER", s.noPicking(), mockClient)
		s.Equal(errOrgTokenRoleSet, err)
		s.Equal("this Organization API token already has that role on the Organization", err.Error())
	})

	s.Run("applying an organization role reports it", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateAPITokenRolesResponseOK, nil)
		got, err := UpdateToken("", apiToken1.Name, "", "", "ORGANIZATION_OWNER", s.noPicking(), mockClient)
		s.NoError(err)
		s.Equal("ORGANIZATION_OWNER", got.Token.Role)
	})

	s.Run("error path when the roles update fails", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		// No rename is registered: the role goes first, so a refused role
		// leaves the name as it was.
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateAPITokenRolesResponseError, nil)
		_, err := UpdateToken("token1", "", "renamed", "", "ORGANIZATION_OWNER", s.noPicking(), mockClient)
		s.ErrorContains(err, "failed to update token")
		mockClient.AssertNotCalled(s.T(), "UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	s.Run("error path - wrong token type", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)
		_, err := UpdateToken("token1", "", "", "", "", s.noPicking(), mockClient)
		s.ErrorContains(err, "the token selected is not of the type you are trying to modify")
	})
}

func (s *Suite) TestRotateToken() {
	s.Run("returns the token with its new secret", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("RotateApiTokenWithResponse", mock.Anything, mock.Anything, "token1").Return(&RotateOrganizationAPITokenResponseOK, nil)
		got, err := RotateToken(iamAPIToken, mockClient)
		s.NoError(err)
		s.Equal("token1", got.ID)
		s.Equal(token, got.Token)
		s.Equal("ORGANIZATION_MEMBER", got.Role)
	})

	s.Run("error path when there is no context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		_, err := RotateToken(iamAPIToken, mockClient)
		s.Error(err)
	})

	s.Run("error path when RotateApiTokenWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("RotateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&RotateOrganizationAPITokenResponseError, nil)
		_, err := RotateToken(iamAPIToken, mockClient)
		s.Equal("failed to update token", err.Error())
	})
}

func (s *Suite) TestFindCurrentToken() {
	s.Run("by name", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		got, err := FindCurrentToken("", apiToken1.Name, s.noPicking(), mockClient)
		s.NoError(err)
		s.Equal("token1", got.Id)
	})

	s.Run("error path when there is no context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		_, err := FindCurrentToken("token1", "", s.noPicking(), mockClient)
		s.Error(err)
	})

	s.Run("error path when listOrganizationTokens returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseError, nil)
		_, err := FindCurrentToken("", "", s.noPicking(), mockClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error path when no token has the name", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		_, err := FindCurrentToken("", "invalid name", s.noPicking(), mockClient)
		s.Equal(errOrganizationTokenNotFound, err)
	})

	s.Run("error path when getApiToken returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)
		_, err := FindCurrentToken("token1", "", s.noPicking(), mockClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("a picker that returns a number out of range is an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		_, err := FindCurrentToken("", "", pickIndex(5, nil), mockClient)
		s.Error(err)
	})
}

func (s *Suite) TestDeleteToken() {
	s.Run("deletes the token and says so", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("DeleteApiTokenWithResponse", mock.Anything, mock.Anything, "token1").Return(&DeleteOrganizationAPITokenResponseOK, nil)
		got, err := DeleteToken(iamAPIToken, mockClient)
		s.NoError(err)
		s.Equal(apitoken.OrganizationRemoval{ID: "token1", Name: "Token 1", Scope: "ORGANIZATION", OrganizationID: "test-org-id", Action: apitoken.Deleted}, got)
	})

	s.Run("error path when there is no context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		_, err := DeleteToken(iamAPIToken, mockClient)
		s.Error(err)
	})

	s.Run("error path when DeleteApiTokenWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("DeleteApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationAPITokenResponseError, nil)
		_, err := DeleteToken(iamAPIToken, mockClient)
		s.Equal("failed to update token", err.Error())
	})
}

func (s *Suite) TestListTokenRoles() {
	s.Run("every role the token holds, by id", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		got, err := ListTokenRoles("token1", s.noPicking(), mockClient)
		s.NoError(err)
		s.Require().Len(got, 4)
		s.Equal(apitoken.Role{EntityType: "ORGANIZATION", EntityID: "test-org-id", Role: "ORGANIZATION_MEMBER"}, got[0])
		s.Equal(apitoken.Role{EntityType: "DEPLOYMENT", EntityID: "DEPLOYMENT", Role: "DEPLOYMENT_ADMIN"}, got[3])
	})

	s.Run("a token with no roles lists none, not nil", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		bare := iamAPIToken
		bare.Roles = nil
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.GetApiTokenResponse{
			HTTPResponse: &http.Response{StatusCode: 200}, JSON200: &bare,
		}, nil)
		got, err := ListTokenRoles("token1", s.noPicking(), mockClient)
		s.NoError(err)
		s.NotNil(got)
		s.Empty(got)
	})

	s.Run("error path - api error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)
		_, err := ListTokenRoles("token1", s.noPicking(), mockClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error path - no context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		_, err := ListTokenRoles("token1", s.noPicking(), mockClient)
		s.Error(err)
	})

	s.Run("picked when no id is given", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, "token1").Return(&GetAPITokensResponseOK, nil)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		_, err := ListTokenRoles("", pickIndex(0, nil), mockClient)
		s.NoError(err)
	})

	s.Run("error path - no id - list tokens api error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseError, nil)
		_, err := ListTokenRoles("", s.noPicking(), mockClient)
		s.ErrorContains(err, "failed to list tokens")
	})
}

func (s *Suite) TestGetOrganizationToken() {
	s.Run("select token by id when name is empty", func() {
		token, err := getOrganizationToken("token1", "", apiTokens, s.noPicking())
		s.NoError(err)
		s.Equal(apiToken1, token)
	})

	s.Run("select token by name when id is empty and there is only one matching token", func() {
		token, err := getOrganizationToken("", "Token 2", apiTokens, s.noPicking())
		s.NoError(err)
		s.Equal(apiTokens[1], token)
	})

	s.Run("return error when token is not found by id", func() {
		token, err := getOrganizationToken("nonexistent", "", apiTokens, s.noPicking())
		s.Equal(errOrganizationTokenNotFound, err)
		s.Equal(astrov1.ApiToken{}, token)
	})

	s.Run("return error when token is not found by name", func() {
		token, err := getOrganizationToken("", "Nonexistent Token", apiTokens, s.noPicking())
		s.Equal(errOrganizationTokenNotFound, err)
		s.Equal(astrov1.ApiToken{}, token)
	})

	s.Run("the plain choice leaves its question to the picker", func() {
		var heading string
		_, err := getOrganizationToken("", "", apiTokens, func(h string, _ []apitoken.Token) (int, error) {
			heading = h
			return 0, nil
		})
		s.NoError(err)
		s.Empty(heading)
	})
}
