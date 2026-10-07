package deployment

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/internal/platform/astro/apitoken"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

var (
	deploymentID = "ck05r3bor07h40d02y2hw4n4v"
	workspaceID  = "ck05r3bor07h40d02y2hw4n4w"
	description1 = "Description 1"
	description2 = "Description 2"
	fullName1    = "User 1"
	fullName2    = "User 2"
	token        = "token"

	iamAPIOrgnaizationToken = astrov1.ApiToken{Id: "token1", Name: "Token 1", Token: &token, Description: description1, Scope: astrov1.ApiTokenScopeORGANIZATION, Roles: &[]astrov1.ApiTokenRole{{EntityType: "ORGANIZATION", EntityId: "test-org-id", Role: "ORGANIZATION_MEMBER"}, {EntityType: "WORKSPACE", EntityId: workspaceID, Role: "WORKSPACE_AUTHOR"}, {EntityType: "WORKSPACE", EntityId: "WORKSPACE", Role: "WORKSPACE_AUTHOR"}, {EntityType: "DEPLOYMENT", EntityId: "DEPLOYMENT", Role: "DEPLOYMENT_ADMIN"}, {EntityType: "DEPLOYMENT", EntityId: deploymentID, Role: "DEPLOYMENT_ADMIN"}}, CreatedAt: time.Now(), CreatedBy: &astrov1.BasicSubjectProfile{FullName: &fullName1}}

	GetAPITokensResponseOKOrganizationToken = astrov1.GetApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &iamAPIOrgnaizationToken,
	}
	iamAPIWorkspaceToken = astrov1.ApiToken{Id: "token1", Name: "Token 1", Token: &token, Description: description1, Scope: astrov1.ApiTokenScopeWORKSPACE, Roles: &[]astrov1.ApiTokenRole{{EntityType: "WORKSPACE", EntityId: workspaceID, Role: "WORKSPACE_AUTHOR"}, {EntityType: "DEPLOYMENT", EntityId: "DEPLOYMENT", Role: "DEPLOYMENT_ADMIN"}, {EntityType: "DEPLOYMENT", EntityId: deploymentID, Role: "DEPLOYMENT_ADMIN"}}, CreatedAt: time.Now(), CreatedBy: &astrov1.BasicSubjectProfile{FullName: &fullName1}}

	GetAPITokensResponseOKWorkspaceToken = astrov1.GetApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &iamAPIWorkspaceToken,
	}

	iamAPIDeploymentToken = astrov1.ApiToken{Id: "token1", Name: "Token 1", Token: &token, Description: description1, Scope: astrov1.ApiTokenScopeDEPLOYMENT, Roles: &[]astrov1.ApiTokenRole{{EntityType: "DEPLOYMENT", EntityId: "DEPLOYMENT", Role: "DEPLOYMENT_ADMIN"}}, CreatedAt: time.Now(), CreatedBy: &astrov1.BasicSubjectProfile{FullName: &fullName1}}

	GetAPITokensResponseOKDeploymentToken = astrov1.GetApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &iamAPIDeploymentToken,
	}

	errorTokenGet, _ = json.Marshal(astrov1.Error{
		Message: "failed to get token",
	})
	GetAPITokensResponseError = astrov1.GetApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorTokenGet,
		JSON200: nil,
	}

	apiToken1               = astrov1.ApiToken{Id: "token1", Name: "Token 1", Token: &token, Description: description1, Scope: astrov1.ApiTokenScopeDEPLOYMENT, Roles: &[]astrov1.ApiTokenRole{{EntityId: deploymentID, EntityType: "DEPLOYMENT", Role: "DEPLOYMENT_MEMBER"}}, CreatedAt: time.Now(), CreatedBy: &astrov1.BasicSubjectProfile{FullName: &fullName1}}
	apiTokenDeploymentOther = astrov1.ApiToken{Id: "token2", Name: "Token 2", Description: description2, Scope: astrov1.ApiTokenScopeDEPLOYMENT, Roles: &[]astrov1.ApiTokenRole{{EntityId: deploymentID, EntityType: "DEPLOYMENT", Role: "DEPLOYMENT_MEMBER"}}, CreatedAt: time.Now(), CreatedBy: &astrov1.BasicSubjectProfile{ApiTokenName: &fullName2}}
	apiTokenOrg             = astrov1.ApiToken{Id: "token-org-1", Name: "Org Token 1", Description: description2, Scope: astrov1.ApiTokenScopeORGANIZATION, Roles: &[]astrov1.ApiTokenRole{{EntityId: "otherDeployment", EntityType: "DEPLOYMENT", Role: "DEPLOYMENT_MEMBER"}, {EntityType: "WORKSPACE", EntityId: "WORKSPACE", Role: "WORKSPACE_MEMBER"}, {EntityType: "ORGANIZATION", EntityId: "ORGANIZATION", Role: "ORGANIZATION_MEMBER"}}, CreatedAt: time.Now(), CreatedBy: &astrov1.BasicSubjectProfile{ApiTokenName: &fullName2}}
	apiTokenOrg2            = astrov1.ApiToken{Id: "token-org-2", Name: "Org Token 2", Description: description2, Scope: astrov1.ApiTokenScopeORGANIZATION, Roles: &[]astrov1.ApiTokenRole{{EntityId: deploymentID, EntityType: "DEPLOYMENT", Role: "DEPLOYMENT_MEMBER"}, {EntityType: "ORGANIZATION", EntityId: "ORGANIZATION", Role: "ORGANIZATION_MEMBER"}}, CreatedAt: time.Now(), CreatedBy: &astrov1.BasicSubjectProfile{ApiTokenName: &fullName2}}
	apiTokenWorkspace       = astrov1.ApiToken{Id: "token-ws-1", Name: "WS Token 1", Description: description2, Scope: astrov1.ApiTokenScopeWORKSPACE, Roles: &[]astrov1.ApiTokenRole{{EntityType: "WORKSPACE", EntityId: "WORKSPACE", Role: "WORKSPACE_MEMBER"}}, CreatedAt: time.Now(), CreatedBy: &astrov1.BasicSubjectProfile{ApiTokenName: &fullName2}}
	apiTokenWorkspace2      = astrov1.ApiToken{Id: "token-ws-2", Name: "WS Token 2", Description: description2, Scope: astrov1.ApiTokenScopeWORKSPACE, Roles: &[]astrov1.ApiTokenRole{{EntityId: deploymentID, EntityType: "DEPLOYMENT", Role: "DEPLOYMENT_MEMBER"}, {EntityType: "WORKSPACE", EntityId: "WORKSPACE", Role: "WORKSPACE_MEMBER"}}, CreatedAt: time.Now(), CreatedBy: &astrov1.BasicSubjectProfile{ApiTokenName: &fullName2}}
	apiTokens               = []astrov1.ApiToken{
		apiToken1,
		apiTokenDeploymentOther,
		apiTokenOrg,
		apiTokenOrg2,
		apiTokenWorkspace,
		apiTokenWorkspace2,
	}
	apiTokens2 = []astrov1.ApiToken{
		apiToken1,
		{Id: "token2", Name: "Token 2", Description: description2, Scope: astrov1.ApiTokenScopeDEPLOYMENT, Roles: &[]astrov1.ApiTokenRole{{EntityId: deploymentID, EntityType: "DEPLOYMENT", Role: "DEPLOYMENT_MEMBER"}}, CreatedAt: time.Now(), CreatedBy: &astrov1.BasicSubjectProfile{FullName: &fullName2}},
	}
	ListDeploymentAPITokensResponseOK = astrov1.ListApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.ApiTokensPaginated{
			Tokens:     apiTokens,
			Limit:      1,
			Offset:     0,
			TotalCount: len(apiTokens),
		},
	}
	ListDeploymentAPITokensResponse2O0 = astrov1.ListApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.ApiTokensPaginated{
			Tokens:     apiTokens2,
			Limit:      1,
			Offset:     0,
			TotalCount: len(apiTokens2),
		},
	}
	errorBodyList, _ = json.Marshal(astrov1.Error{
		Message: "failed to list tokens",
	})
	ListDeploymentAPITokensResponseError = astrov1.ListApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyList,
		JSON200: nil,
	}
	CreateDeploymentAPITokenResponseOK = astrov1.CreateApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &apiToken1,
	}
	errorBodyCreate, _ = json.Marshal(astrov1.Error{
		Message: "failed to create token",
	})
	_ = astrov1.CreateApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyCreate,
		JSON200: nil,
	}
	UpdateDeploymentAPITokenResponseOK = astrov1.UpdateApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &apiToken1,
	}

	errorBodyUpdate, _ = json.Marshal(astrov1.Error{
		Message: "failed to update token",
	})
	UpdateDeploymentAPITokenResponseError = astrov1.UpdateApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyUpdate,
		JSON200: nil,
	}
	RotateDeploymentAPITokenResponseOK = astrov1.RotateApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &apiToken1,
	}
	RotateDeploymentAPITokenResponseError = astrov1.RotateApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyUpdate,
		JSON200: nil,
	}
	DeleteDeploymentAPITokenResponseOK = astrov1.DeleteApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	DeleteDeploymentAPITokenResponseError = astrov1.DeleteApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorBodyUpdate,
	}
	UpdateOrganizationAPITokenResponseOK = astrov1.UpdateApiTokenRolesResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.SubjectRoles{},
	}

	UpdateOrganizationAPITokenResponseError = astrov1.UpdateApiTokenRolesResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyUpdate,
		JSON200: nil,
	}

	UpdateWorkspaceAPITokenResponseOK = astrov1.UpdateApiTokenRolesResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.SubjectRoles{},
	}

	UpdateWorkspaceAPITokenResponseError = astrov1.UpdateApiTokenRolesResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyUpdate,
		JSON200: nil,
	}

	ListOrganizationAPITokensResponseOK = astrov1.ListApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.ApiTokensPaginated{
			Tokens:     apiTokens,
			Limit:      1,
			Offset:     0,
			TotalCount: len(apiTokens),
		},
	}
	ListOrganizationAPITokensResponseError = astrov1.ListApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyList,
		JSON200: nil,
	}

	ListWorkspaceAPITokensResponseOK = astrov1.ListApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.ApiTokensPaginated{
			Tokens:     apiTokens,
			Limit:      1,
			Offset:     0,
			TotalCount: len(apiTokens),
		},
	}

	ListWorkspaceAPITokensResponseError = astrov1.ListApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyList,
		JSON200: nil,
	}
)

// pickSecond answers a picker the way a person typing "2" would.
func pickSecond(string, []apitoken.Token) (int, error) { return 1, nil }

// findAndRotate and findAndDelete are what the commands do: find the token,
// then act on it.
func findAndRotate(id, name string, client astrov1.APIClient) (apitoken.Token, error) {
	tokenTypes := []DeploymentTokenType{DeploymentTokenTypeDEPLOYMENT}
	token, err := FindToken(id, name, deploymentID, tokenTypes, pickSecond, client)
	if err != nil {
		return apitoken.Token{}, err
	}
	return RotateToken(token, deploymentID, client)
}

func findAndDelete(id, name string, client astrov1.APIClient) (apitoken.DeploymentRemoval, error) {
	token, err := FindToken(id, name, deploymentID, nil, pickSecond, client)
	if err != nil {
		return apitoken.DeploymentRemoval{}, err
	}
	return DeleteToken(token, deploymentID, client)
}

func (s *Suite) TestListTokens() {
	s.Run("happy path", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Twice()
		_, err := ListTokens(mockClient, "", nil)
		s.NoError(err)
	})

	s.Run("with specified deployment", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Twice()

		_, err := ListTokens(mockClient, "otherDeployment", nil)

		s.NoError(err)
	})

	s.Run("error path when ListDeploymentApiTokensWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseError, nil).Twice()
		_, err := ListTokens(mockClient, "otherDeployment", nil)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error getting current context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		_, err := ListTokens(mockClient, "", nil)

		s.Error(err)
	})
}

func (s *Suite) TestCreateToken() {
	s.Run("happy path", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateDeploymentAPITokenResponseOK, nil)

		_, err := CreateToken("Token 1", "Description 1", "DEPLOYMENT_MEMBER", "", 100, mockClient)

		s.NoError(err)
	})

	s.Run("error getting current context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)

		_, err := CreateToken("Token 1", "Description 1", "DEPLOYMENT_MEMBER", "", 0, mockClient)

		s.Error(err)
	})

	s.Run("empty name", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)

		_, err := CreateToken("", "Description 1", "DEPLOYMENT_MEMBER", "", 0, mockClient)

		s.Equal(errInvalidTokenName, err)
	})
}

func (s *Suite) TestUpdateToken() {
	s.Run("happy path", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateDeploymentAPITokenResponseOK, nil)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKDeploymentToken, nil)

		_, err := UpdateToken("token1", "", "", "", "", "", pickSecond, mockClient)
		s.NoError(err)
	})

	s.Run("happy path no id", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKDeploymentToken, nil)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Twice()
		mockClient.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateDeploymentAPITokenResponseOK, nil)
		_, err := UpdateToken("", "", "", "", "", "", pickSecond, mockClient)
		s.NoError(err)
	})

	s.Run("happy path multiple name", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKDeploymentToken, nil)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponse2O0, nil).Twice()
		mockClient.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateDeploymentAPITokenResponseOK, nil)
		_, err := UpdateToken("", "Token 1", "", "", "", "", pickSecond, mockClient)
		s.NoError(err)
	})

	s.Run("happy path", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKDeploymentToken, nil)
		mockClient.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateDeploymentAPITokenResponseOK, nil)
		_, err := UpdateToken("token1", "", "mockNewName", "mockDescription", "", "", pickSecond, mockClient)
		s.NoError(err)
	})

	s.Run("error path when listDeploymentTokens returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseError, nil)
		_, err := UpdateToken("", "", "", "", "", "", pickSecond, mockClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error path when listDeploymentToken returns an not found error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		_, err := UpdateToken("", "invalid name", "", "", "", "", pickSecond, mockClient)
		s.Equal(errDeploymentTokenNotFound, err)
	})

	s.Run("error path when getDeploymentToken returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)
		_, err := UpdateToken("tokenId", "", "", "", "", "", pickSecond, mockClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error path when UpdateDeploymentApiTokenWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKDeploymentToken, nil)
		mockClient.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateDeploymentAPITokenResponseError, nil)
		_, err := UpdateToken("token3", "", "", "", "", "", pickSecond, mockClient)
		s.Equal("failed to update token", err.Error())
	})

	s.Run("error path when there is no context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		_, err := UpdateToken("token1", "", "", "", "", "", pickSecond, mockClient)
		s.Error(err)
	})

	s.Run("Happy path when applying deployment role", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKDeploymentToken, nil)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		mockClient.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateDeploymentAPITokenResponseOK, nil)
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
		// Use a role different from the token's current DEPLOYMENT_ADMIN to avoid short-circuit.
		_, err := UpdateToken("", apiToken1.Name, "", "", "DEPLOYMENT_MEMBER", "", pickSecond, mockClient)
		s.NoError(err)
	})

	s.Run("error path wrong token type provided", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateDeploymentAPITokenResponseOK, nil)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil)

		_, err := UpdateToken("token1", "", "", "", "", "", pickSecond, mockClient)
		s.ErrorContains(err, "the token selected is not of the type you are trying to modify")
	})
}

func (s *Suite) TestRotateToken() {
	s.Run("happy path - id provided", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKDeploymentToken, nil)
		mockClient.On("RotateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateDeploymentAPITokenResponseOK, nil)
		res, err := findAndRotate("token1", "", mockClient)
		s.NoError(err)
		s.Equal(token, res.Token)
	})

	s.Run("happy path name provided", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKDeploymentToken, nil)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		mockClient.On("RotateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateDeploymentAPITokenResponseOK, nil)
		_, err := findAndRotate("", apiToken1.Name, mockClient)
		s.NoError(err)
	})

	s.Run("error path when there is no context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		_, err := findAndRotate("token1", "", mockClient)
		s.Error(err)
	})

	s.Run("error path when listDeploymentTokens returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseError, nil)
		_, err := findAndRotate("", "", mockClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error path when listDeploymentToken returns an not found error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		_, err := findAndRotate("", "invalid name", mockClient)
		s.Equal(errDeploymentTokenNotFound, err)
	})

	s.Run("error path when getApiToken returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)
		_, err := findAndRotate("token1", "", mockClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error path when RotateDeploymentApiTokenWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKDeploymentToken, nil)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		mockClient.On("RotateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateDeploymentAPITokenResponseError, nil)
		_, err := findAndRotate("", apiToken1.Name, mockClient)
		s.Equal("failed to update token", err.Error())
	})
}

func (s *Suite) TestDeleteToken() {
	s.Run("happy path - delete deployment token - by name", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKDeploymentToken, nil)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Twice()
		mockClient.On("DeleteApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentAPITokenResponseOK, nil)
		_, err := findAndDelete("", apiToken1.Name, mockClient)
		s.NoError(err)
	})

	s.Run("happy path - delete deployment token - by id", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKDeploymentToken, nil)
		mockClient.On("DeleteApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentAPITokenResponseOK, nil)
		res, err := findAndDelete("token1", "", mockClient)
		s.NoError(err)
		s.Equal(apitoken.Deleted, res.Action)
	})

	s.Run("error path when there is no context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		_, err := findAndDelete("token1", "", mockClient)
		s.Error(err)
	})

	s.Run("error path when getApiToken returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)
		_, err := findAndDelete("token1", "", mockClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error path when listDeploymentTokens returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseError, nil)
		_, err := findAndDelete("", apiToken1.Name, mockClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error path when listDeploymentToken returns a not found error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		_, err := findAndDelete("", "invalid name", mockClient)
		s.Equal(errDeploymentTokenNotFound, err)
	})

	s.Run("error path when DeleteDeploymentApiTokenWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKDeploymentToken, nil)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil)
		mockClient.On("DeleteApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentAPITokenResponseError, nil)
		_, err := findAndDelete("", apiToken1.Name, mockClient)
		s.Equal("failed to update token", err.Error())
	})
}

func (s *Suite) TestGetDeploymentToken() {
	s.Run("select token by id when name is empty", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Twice()
		token, err := getDeploymentToken("token1", "", "testDeployment", apiTokens, pickSecond)
		s.NoError(err)
		s.Equal(apiToken1, token)
	})

	s.Run("select token by name when id is empty and there is only one matching token", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Twice()
		token, err := getDeploymentToken("", "Token 2", "testDeployment", apiTokens, pickSecond)
		s.NoError(err)
		s.Equal(apiTokens[1], token)
	})

	s.Run("return error when token is not found by id", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Twice()
		token, err := getDeploymentToken("nonexistent", "", "testDeployment", apiTokens, pickSecond)
		s.Equal(errDeploymentTokenNotFound, err)
		s.Equal(astrov1.ApiToken{}, token)
	})

	s.Run("return error when token is not found by name", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Twice()
		token, err := getDeploymentToken("", "Nonexistent Token", "testDeployment", apiTokens, pickSecond)
		s.Equal(errDeploymentTokenNotFound, err)
		s.Equal(astrov1.ApiToken{}, token)
	})
}

func (s *Suite) TestRemoveOrgTokenDeploymentRole() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()
		res, err := RemoveOrgTokenDeploymentRole("", "", deploymentID, pickSecond, mockClient)
		s.NoError(err)
		s.Equal("Token 1", res.Name)
	})

	s.Run("error on ListDeploymentApiTokensWithResponse", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseError, nil).Once()
		_, err := RemoveOrgTokenDeploymentRole("", "", deploymentID, pickSecond, mockClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error on GetApiTokenWithResponse", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Once()
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil).Once()
		_, err := RemoveOrgTokenDeploymentRole("", "", deploymentID, pickSecond, mockClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error on UpdateOrganizationApiTokenWithResponse", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseError, nil).Once()
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()
		_, err := RemoveOrgTokenDeploymentRole("", "", deploymentID, pickSecond, mockClient)
		s.ErrorContains(err, "failed to update token")
	})

	s.Run("happy path with token id passed in", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()
		res, err := RemoveOrgTokenDeploymentRole("token-id", "", deploymentID, pickSecond, mockClient)
		s.NoError(err)
		s.Equal("Token 1", res.Name)
	})

	s.Run("error on GetApiTokenWithResponse with token id passed in", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		_, err = RemoveOrgTokenDeploymentRole("token-id", "", deploymentID, pickSecond, mockClient)
		s.ErrorContains(err, "failed to get token")
	})
}

func (s *Suite) TestRemoveWorkspaceTokenDeploymentRole() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil).Once()
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		res, err := RemoveWorkspaceTokenDeploymentRole("", "", "", deploymentID, pickSecond, mockClient)
		s.NoError(err)
		s.Equal("Token 1", res.Name)
	})

	s.Run("error on ListDeploymentApiTokensWithResponse", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseError, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		_, err = RemoveWorkspaceTokenDeploymentRole("", "", "", deploymentID, pickSecond, mockClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error on GetApiTokenWithResponse", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Once()
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		_, err = RemoveWorkspaceTokenDeploymentRole("", "", "", deploymentID, pickSecond, mockClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error on UpdateWorkspaceApiTokenWithResponse", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseError, nil).Once()
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		_, err = RemoveWorkspaceTokenDeploymentRole("", "", "", deploymentID, pickSecond, mockClient)
		s.ErrorContains(err, "failed to update token")
	})

	s.Run("happy path with token id passed in", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil).Once()
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		res, err := RemoveWorkspaceTokenDeploymentRole("token-id", "", "", deploymentID, pickSecond, mockClient)
		s.NoError(err)
		s.Equal("Token 1", res.Name)
	})

	s.Run("error on GetApiTokenWithResponse with token id passed in", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		_, err = RemoveWorkspaceTokenDeploymentRole("token-id", "", "", deploymentID, pickSecond, mockClient)
		s.ErrorContains(err, "failed to get token")
	})
}

func (s *Suite) TestUpsertOrgTokenDeploymentRole() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path Create", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		res, err := UpsertOrgTokenDeploymentRole("", "", "", deploymentID, "create", pickSecond, mockClient)
		s.NoError(err)
		s.Equal("Token 1", res.Name)
	})

	s.Run("error on ListOrganizationApiTokensWithResponse", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseError, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		_, err = UpsertOrgTokenDeploymentRole("", "", "", deploymentID, "create", pickSecond, mockClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("happy path Update", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()
		res, err := UpsertOrgTokenDeploymentRole("", "", "", deploymentID, "update", pickSecond, mockClient)
		s.NoError(err)
		s.Equal("Token 1", res.Name)
	})

	s.Run("error on ListDeploymentApiTokensWithResponse", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseError, nil).Once()
		_, err := UpsertOrgTokenDeploymentRole("", "", "", deploymentID, "update", pickSecond, mockClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error on GetApiTokenWithResponse", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Once()
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		_, err = UpsertOrgTokenDeploymentRole("", "", "", deploymentID, "create", pickSecond, mockClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error on UpdateOrganizationApiTokenWithResponse", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseError, nil).Once()
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		_, err = UpsertOrgTokenDeploymentRole("", "", "", deploymentID, "create", pickSecond, mockClient)
		s.ErrorContains(err, "failed to update token")
	})

	s.Run("happy path with token id passed in", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		res, err := UpsertOrgTokenDeploymentRole("token-id", "", "", deploymentID, "create", pickSecond, mockClient)
		s.NoError(err)
		s.Equal("Token 1", res.Name)
	})

	s.Run("error on GetApiTokenWithResponse with token id passed in", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		_, err = UpsertOrgTokenDeploymentRole("token-id", "", "", deploymentID, "create", pickSecond, mockClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error path with token id passed in - wrong token type", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		_, err = UpsertOrgTokenDeploymentRole("token-id", "", "", deploymentID, "create", pickSecond, mockClient)
		s.ErrorContains(err, "the token selected is not of the type you are trying to modify")
	})
}

func (s *Suite) TestUpsertWorkspaceTokenDeploymentRole() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path Create", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil).Once()
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		res, err := UpsertWorkspaceTokenDeploymentRole("", "", "", "", deploymentID, "create", pickSecond, mockClient)
		s.NoError(err)
		s.Equal("Token 1", res.Name)
	})

	s.Run("error on ListWorkspaceApiTokensWithResponse", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseError, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		_, err = UpsertWorkspaceTokenDeploymentRole("", "", "", "", deploymentID, "create", pickSecond, mockClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("happy path Update", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil).Once()
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil).Once()
		res, err := UpsertWorkspaceTokenDeploymentRole("", "", "", "", deploymentID, "update", pickSecond, mockClient)
		s.NoError(err)
		s.Equal("Token 1", res.Name)
	})

	s.Run("error on ListDeploymentApiTokensWithResponse", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentAPITokensResponseError, nil).Once()
		_, err := UpsertWorkspaceTokenDeploymentRole("", "", "", "", deploymentID, "update", pickSecond, mockClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error on GetApiTokenWithResponse", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Once()
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		_, err = UpsertWorkspaceTokenDeploymentRole("", "", "", "", deploymentID, "create", pickSecond, mockClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error on UpdateWorkspaceApiTokenWithResponse", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseError, nil).Once()
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		_, err = UpsertWorkspaceTokenDeploymentRole("", "", "", "", deploymentID, "create", pickSecond, mockClient)
		s.ErrorContains(err, "failed to update token")
	})

	s.Run("happy path with token id passed in", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil).Once()
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		res, err := UpsertWorkspaceTokenDeploymentRole("token-id", "", "", "", deploymentID, "create", pickSecond, mockClient)
		s.NoError(err)
		s.Equal("Token 1", res.Name)
	})

	s.Run("error on GetApiTokenWithResponse with token id passed in", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		_, err = UpsertWorkspaceTokenDeploymentRole("token-id", "", "", "", deploymentID, "create", pickSecond, mockClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error path with token id passed in - wrong token type", func() {
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil).Once()
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()
		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		_, err = UpsertWorkspaceTokenDeploymentRole("token-id", "", "", "", deploymentID, "create", pickSecond, mockClient)
		s.ErrorContains(err, "the token selected is not of the type you are trying to modify")
	})
}
