package organization

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"time"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astroiamcore "github.com/astronomer/astro-cli/astro-client-iam-core"
	astroiamcore_mocks "github.com/astronomer/astro-cli/astro-client-iam-core/mocks"
	"github.com/astronomer/astro-cli/cloud/user"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/mock"
)

var (
	description1           = "Description 1"
	description2           = "Description 2"
	fullName1              = "User 1"
	fullName2              = "User 2"
	token                  = "token"
	workspaceID            = "ck05r3bor07h40d02y2hw4n4v"
	iamAPIToken            = astroiamcore.ApiToken{Id: "token1", Name: "Token 1", Token: &token, Description: description1, Type: "ORGANIZATION", Roles: &[]astroiamcore.ApiTokenRole{{EntityType: "ORGANIZATION", EntityId: "test-org-id", Role: "ORGANIZATION_MEMBER"}, {EntityType: "WORKSPACE", EntityId: workspaceID, Role: "WORKSPACE_AUTHOR"}, {EntityType: "WORKSPACE", EntityId: "WORKSPACE", Role: "WORKSPACE_AUTHOR"}, {EntityType: "DEPLOYMENT", EntityId: "DEPLOYMENT", Role: "DEPLOYMENT_ADMIN"}}, CreatedAt: time.Now(), CreatedBy: &astroiamcore.BasicSubjectProfile{FullName: &fullName1}}
	GetAPITokensResponseOK = astroiamcore.GetApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &iamAPIToken,
	}
	errorTokenGet, _ = json.Marshal(astroiamcore.Error{
		Message: "failed to get token",
	})

	iamAPIWorkspaceToken = astroiamcore.ApiToken{Id: "token1", Name: "token1", Token: &token, Description: description1, Type: "WORKSPACE", Roles: &[]astroiamcore.ApiTokenRole{{EntityType: "WORKSPACE", EntityId: workspaceID, Role: "WORKSPACE_AUTHOR"}, {EntityType: "DEPLOYMENT", EntityId: "DEPLOYMENT", Role: "DEPLOYMENT_ADMIN"}}, CreatedAt: time.Now(), CreatedBy: &astroiamcore.BasicSubjectProfile{FullName: &fullName1}}

	GetAPITokensResponseOKWorkspaceToken = astroiamcore.GetApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &iamAPIWorkspaceToken,
	}

	GetAPITokensResponseError = astroiamcore.GetApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorTokenGet,
		JSON200: nil,
	}

	apiToken1 = astrocore.ApiToken{Id: "token1", Name: "Token 1", Token: &token, Description: description1, Type: "Type 1", Roles: []astrocore.ApiTokenRole{{EntityType: "WORKSPACE", EntityId: workspaceID, Role: "ORGANIZATION_MEMBER"}, {EntityId: "ORGANIZATION", Role: "ORGANIZATION_MEMBER"}}, CreatedAt: time.Now(), CreatedBy: &astrocore.BasicSubjectProfile{FullName: &fullName1}}
	apiToken2 = astrocore.ApiToken{Id: "token1", Name: "Token 1", Token: &token, Description: description1, Type: "Type 1", Roles: []astrocore.ApiTokenRole{{EntityType: "ORGANIZATION", EntityId: "ORGANIZATION", Role: "ORGANIZATION_MEMBER"}, {EntityType: "WORKSPACE", EntityId: "WORKSPACE", Role: "WORKSPACE_MEMBER"}, {EntityType: "DEPLOYMENT", EntityId: "DEPLOYMENT", Role: "DEPLOYMENT_ADMIN"}}, CreatedAt: time.Now(), CreatedBy: &astrocore.BasicSubjectProfile{FullName: &fullName1}}

	apiTokens = []astrocore.ApiToken{
		apiToken1,
		{Id: "token2", Name: "Token 2", Description: description2, Type: "Type 2", Roles: []astrocore.ApiTokenRole{{EntityType: "ORGANIZATION", EntityId: "ORGANIZATION", Role: "ORGANIZATION_MEMBER"}}, CreatedAt: time.Now(), CreatedBy: &astrocore.BasicSubjectProfile{FullName: &fullName2}},
	}
	apiTokens2 = []astrocore.ApiToken{
		apiToken1,
		apiToken2,
	}
	ListOrganizationAPITokensResponseOK = astrocore.ListOrganizationApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.ListApiTokensPaginated{
			ApiTokens: apiTokens,
			Limit:     1,
			Offset:    0,
		},
	}
	ListOrganizationAPITokensResponse2O0 = astrocore.ListOrganizationApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.ListApiTokensPaginated{
			ApiTokens: apiTokens2,
			Limit:     1,
			Offset:    0,
		},
	}
	errorBodyList, _ = json.Marshal(astrocore.Error{
		Message: "failed to list tokens",
	})
	ListOrganizationAPITokensResponseError = astrocore.ListOrganizationApiTokensResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyList,
		JSON200: nil,
	}

	GetOrganizationAPITokenResponseOK = astrocore.GetOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &apiToken2,
	}
	getTokenErrorBodyList, _ = json.Marshal(astrocore.Error{
		Message: "failed to get token",
	})
	GetOrganizationAPITokenResponseError = astrocore.GetOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    getTokenErrorBodyList,
		JSON200: nil,
	}

	CreateOrganizationAPITokenResponseOK = astrocore.CreateOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &apiToken1,
	}

	errorBodyCreate, _ = json.Marshal(astrocore.Error{
		Message: "failed to create token",
	})
	CreateOrganizationAPITokenResponseError = astrocore.CreateOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyCreate,
		JSON200: nil,
	}
	UpdateOrganizationAPITokenResponseOK = astrocore.UpdateOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &apiToken1,
	}

	errorBodyUpdate, _ = json.Marshal(astrocore.Error{
		Message: "failed to update token",
	})
	UpdateOrganizationAPITokenResponseError = astrocore.UpdateOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyUpdate,
		JSON200: nil,
	}
	RotateOrganizationAPITokenResponseOK = astrocore.RotateOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &apiToken1,
	}
	RotateOrganizationAPITokenResponseError = astrocore.RotateOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyUpdate,
		JSON200: nil,
	}
	DeleteOrganizationAPITokenResponseOK = astrocore.DeleteOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	DeleteOrganizationAPITokenResponseError = astrocore.DeleteOrganizationApiTokenResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorBodyUpdate,
	}
)

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
		err := AddOrgTokenToWorkspace(selectedTokenID, selectedTokenName, "INVALID_ROLE", workspace, nil, nil, nil)
		s.Error(err)
		s.Equal(user.ErrInvalidWorkspaceRole, err)
	})

	s.Run("return error for failed to get current context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := AddOrgTokenToWorkspace(selectedTokenID, selectedTokenName, role, workspace, nil, mockClient, mockIamClient)
		s.Error(err)
	})

	s.Run("return error for failed to list organization tokens", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseError, nil)

		err := AddOrgTokenToWorkspace("", selectedTokenName, role, workspace, nil, mockClient, mockIamClient)
		s.Error(err)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("return error for failed to select organization token", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)

		err := AddOrgTokenToWorkspace("", "Invalid name", role, workspace, nil, mockClient, mockIamClient)
		s.Error(err)
		s.Equal(errOrganizationTokenNotFound, err)
	})

	s.Run("return error for organization token already in workspace", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)

		err := AddOrgTokenToWorkspace("", selectedTokenName, "WORKSPACE_AUTHOR", workspace, nil, mockClient, mockIamClient)
		s.Error(err)
		s.Equal(errOrgTokenInWorkspace, err)
	})

	s.Run("successfully add organization token to workspace using id", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)

		err := AddOrgTokenToWorkspace(selectedTokenID, "", role, workspace, out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("error add organization token to workspace using id - wrong token type", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)

		err := AddOrgTokenToWorkspace(selectedTokenID, "", role, workspace, out, mockClient, mockIamClient)
		s.ErrorContains(err, "the token selected is not of the type you are trying to modify")
	})

	s.Run("return error for failed to get organization token", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)

		err := AddOrgTokenToWorkspace(selectedTokenID, "", role, workspace, nil, mockClient, mockIamClient)
		s.Error(err)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("successfully add organization token to workspace", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)

		err := AddOrgTokenToWorkspace("", selectedTokenName2, role, workspace, out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("successfully select and add organization token to workspace", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
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
		err = AddOrgTokenToWorkspace("", "", role, workspace, out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("successfully select and add organization token to workspace 2", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponse2O0, nil)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
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
		err = AddOrgTokenToWorkspace("", selectedTokenName, role, workspace, out, mockClient, mockIamClient)
		s.NoError(err)
	})
}

func (s *Suite) TestListTokens() {
	s.Run("happy path", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Twice()
		err := ListTokens(mockClient, out)
		s.NoError(err)
	})

	s.Run("error path when ListOrganizationApiTokensWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseError, nil).Twice()
		err := ListTokens(mockClient, out)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error getting current context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := ListTokens(mockClient, out)

		s.Error(err)
	})
}

func (s *Suite) TestCreateToken() {
	s.Run("happy path", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateOrganizationAPITokenResponseOK, nil)

		err := CreateToken("Token 1", "Description 1", "ORGANIZATION_MEMBER", 0, false, out, mockClient)

		s.NoError(err)
	})

	s.Run("error getting current context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)

		err := CreateToken("Token 1", "Description 1", "ORGANIZATION_MEMBER", 0, false, out, mockClient)

		s.Error(err)
	})

	s.Run("invalid role", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)

		err := CreateToken("Token 1", "Description 1", "InvalidRole", 0, false, out, mockClient)

		s.Error(err)
	})

	s.Run("empty name", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)

		err := CreateToken("", "Description 1", "ORGANIZATION_MEMBER", 0, true, out, mockClient)

		s.Equal(ErrInvalidName, err)
	})
}

func (s *Suite) TestUpdateToken() {
	s.Run("happy path", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
		err := UpdateToken("token1", "", "", "", "", out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("happy path no id", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Twice()
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
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
		err = UpdateToken("", "", "", "", "", out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("happy path multiple name", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponse2O0, nil).Twice()
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
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
		err = UpdateToken("", "Token 1", "", "", "", out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("happy path", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
		err := UpdateToken("token1", "", "", "", "", out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("error path when listOrganizationTokens returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseError, nil)
		err := UpdateToken("", "", "", "", "", out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error path when listOrganizationToken returns an not found error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		err := UpdateToken("", "invalid name", "", "", "", out, mockClient, mockIamClient)
		s.Equal(errOrganizationTokenNotFound, err)
	})

	s.Run("error path when getApiToken returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)

		err := UpdateToken("tokenId", "", "", "", "", out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error path when UpdateOrganizationApiTokenWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseError, nil)
		err := UpdateToken("token3", "", "", "", "", out, mockClient, mockIamClient)
		s.Equal("failed to update token", err.Error())
	})

	s.Run("error path when there is no context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := UpdateToken("token1", "", "", "", "", out, mockClient, mockIamClient)
		s.Error(err)
	})

	s.Run("error path when workspace role is invalid returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
		err := UpdateToken("token1", "", "", "", "Invalid Role", out, mockClient, mockIamClient)
		s.Equal(user.ErrInvalidOrganizationRole.Error(), err.Error())
	})
	s.Run("Happy path when applying workspace role", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
		err := UpdateToken("", apiToken1.Name, "", "", "ORGANIZATION_MEMBER", out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("Happy path when applying organization role using id", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
		err := UpdateToken("token1", "", "", "", "ORGANIZATION_MEMBER", out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("error path - wrong token type", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
		err := UpdateToken("token1", "", "", "", "", out, mockClient, mockIamClient)
		s.ErrorContains(err, "the token selected is not of the type you are trying to modify")
	})
}

func (s *Suite) TestRotateToken() {
	s.Run("happy path - id provided", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("RotateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateOrganizationAPITokenResponseOK, nil)
		err := RotateToken("token1", "", false, true, out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("happy path name provided", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("RotateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateOrganizationAPITokenResponseOK, nil)
		err := RotateToken("", apiToken1.Name, false, true, out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("happy path with confirmation", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("RotateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateOrganizationAPITokenResponseOK, nil)
		err := RotateToken("", apiToken1.Name, true, false, out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("error path when there is no context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := RotateToken("token1", "", false, false, out, mockClient, mockIamClient)
		s.Error(err)
	})

	s.Run("error path when listOrganizationTokens returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseError, nil)
		err := RotateToken("", "", false, false, out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error path when listOrganizationToken returns an not found error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		err := RotateToken("", "invalid name", false, false, out, mockClient, mockIamClient)
		s.Equal(errOrganizationTokenNotFound, err)
	})

	s.Run("error path when getApiToken returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)
		err := RotateToken("token1", "", false, false, out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error path when RotateOrganizationApiTokenWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("RotateOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RotateOrganizationAPITokenResponseError, nil)
		err := RotateToken("", apiToken1.Name, false, true, out, mockClient, mockIamClient)
		s.Equal("failed to update token", err.Error())
	})
}

func (s *Suite) TestDeleteToken() {
	s.Run("happy path - delete workspace token - by name", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Twice()
		mockClient.On("DeleteOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationAPITokenResponseOK, nil)
		err := DeleteToken("", apiToken1.Name, false, out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("happy path - delete workspace token - by id", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("DeleteOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationAPITokenResponseOK, nil)
		err := DeleteToken("token1", "", false, out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("happy path - remove organization token", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Twice()
		mockClient.On("DeleteOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationAPITokenResponseOK, nil)
		err := DeleteToken("", apiToken1.Name, false, out, mockClient, mockIamClient)
		s.NoError(err)
	})

	s.Run("error path when there is no context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := DeleteToken("token1", "", false, out, mockClient, mockIamClient)
		s.Error(err)
	})

	s.Run("error path when getApiToken returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)
		err := DeleteToken("token1", "", false, out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error path when listOrganizationTokens returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseError, nil)
		err := DeleteToken("", apiToken1.Name, false, out, mockClient, mockIamClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error path when listOrganizationToken returns a not found error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		err := DeleteToken("", "invalid name", false, out, mockClient, mockIamClient)
		s.Equal(errOrganizationTokenNotFound, err)
	})

	s.Run("error path when DeleteOrganizationApiTokenWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("DeleteOrganizationApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationAPITokenResponseError, nil)
		err := DeleteToken("", apiToken1.Name, true, out, mockClient, mockIamClient)
		s.Equal("failed to update token", err.Error())
	})
}

func (s *Suite) TestListTokenRoles() {
	s.Run("happy path - list token roles by id", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		err := ListTokenRoles("token1", mockClient, mockIamClient, out)
		s.NoError(err)
	})

	s.Run("error path - list token roles by id - api error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)
		err := ListTokenRoles("token1", mockClient, mockIamClient, out)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error path - list token roles by id - no context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := ListTokenRoles("token1", mockClient, mockIamClient, out)
		s.Error(err)
	})

	s.Run("happy path - list token roles no id", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockIamClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Twice()
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = ListTokenRoles("", mockClient, mockIamClient, out)
		s.NoError(err)
	})

	s.Run("error path - list token roles no id - list tokens api error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockIamClient := new(astroiamcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseError, nil).Twice()
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		err = ListTokenRoles("", mockClient, mockIamClient, out)
		s.ErrorContains(err, "failed to list tokens")
	})
}

func (s *Suite) TestGetOrganizationToken() {
	s.Run("select token by id when name is empty", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Twice()
		token, err := getOrganizationToken("token1", "", "\nPlease select the workspace token you would like to delete or remove:", apiTokens)
		s.NoError(err)
		s.Equal(apiToken1, token)
	})

	s.Run("select token by name when id is empty and there is only one matching token", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Twice()
		token, err := getOrganizationToken("", "Token 2", "\nPlease select the workspace token you would like to delete or remove:", apiTokens)
		s.NoError(err)
		s.Equal(apiTokens[1], token)
	})

	s.Run("return error when token is not found by id", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Twice()
		token, err := getOrganizationToken("nonexistent", "", "\nPlease select the workspace token you would like to delete or remove:", apiTokens)
		s.Equal(errOrganizationTokenNotFound, err)
		s.Equal(astrocore.ApiToken{}, token)
	})

	s.Run("return error when token is not found by name", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Twice()
		token, err := getOrganizationToken("", "Nonexistent Token", "\nPlease select the workspace token you would like to delete or remove:", apiTokens)
		s.Equal(errOrganizationTokenNotFound, err)
		s.Equal(astrocore.ApiToken{}, token)
	})
}

func (s *Suite) TestTimeAgo() {
	currentTime := time.Now()

	s.Run("return 'Just now' for current time", func() {
		result := TimeAgo(currentTime)
		s.Equal("Just now", result)
	})

	s.Run("return '30 minutes ago' for 30 minutes ago", func() {
		pastTime := currentTime.Add(-30 * time.Minute)
		result := TimeAgo(pastTime)
		s.Equal("30 minutes ago", result)
	})

	s.Run("return '5 hours ago' for 5 hours ago", func() {
		pastTime := currentTime.Add(-5 * time.Hour)
		result := TimeAgo(pastTime)
		s.Equal("5 hours ago", result)
	})

	s.Run("return '10 days ago' for 10 days ago", func() {
		pastTime := currentTime.Add(-10 * 24 * time.Hour)
		result := TimeAgo(pastTime)
		s.Equal("10 days ago", result)
	})
}
