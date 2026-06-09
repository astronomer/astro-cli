package organization

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/astro-client-v1"
	astrov1_mocks "github.com/astronomer/astro-cli/astro-client-v1/mocks"
	"github.com/astronomer/astro-cli/cloud/user"
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
		Id:          "token1",
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
		err := AddOrgTokenToWorkspace(selectedTokenID, selectedTokenName, "INVALID_ROLE", workspace, nil, nil)
		s.Error(err)
		s.Equal(user.ErrInvalidWorkspaceRole, err)
	})

	s.Run("return error for failed to get current context", func() {
		testUtil.InitTestConfig(testUtil.Initial)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		err := AddOrgTokenToWorkspace(selectedTokenID, selectedTokenName, role, workspace, nil, mockClient)
		s.Error(err)
	})

	s.Run("return error for failed to list organization tokens", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseError, nil)

		err := AddOrgTokenToWorkspace("", selectedTokenName, role, workspace, nil, mockClient)
		s.Error(err)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("return error for failed to select organization token", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)

		err := AddOrgTokenToWorkspace("", "Invalid name", role, workspace, nil, mockClient)
		s.Error(err)
		s.Equal(errOrganizationTokenNotFound, err)
	})

	s.Run("return error for organization token already in workspace", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)

		err := AddOrgTokenToWorkspace("", selectedTokenName, "WORKSPACE_AUTHOR", workspace, nil, mockClient)
		s.Error(err)
		s.Equal(errOrgTokenInWorkspace, err)
	})

	s.Run("successfully add organization token to workspace using id", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateAPITokenRolesResponseOK, nil)

		err := AddOrgTokenToWorkspace(selectedTokenID, "", role, workspace, out, mockClient)
		s.NoError(err)
	})

	s.Run("error add organization token to workspace using id - wrong token type", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := AddOrgTokenToWorkspace(selectedTokenID, "", role, workspace, out, mockClient)
		s.ErrorContains(err, "the token selected is not of the type you are trying to modify")
	})

	s.Run("return error for failed to get organization token", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)

		err := AddOrgTokenToWorkspace(selectedTokenID, "", role, workspace, nil, mockClient)
		s.Error(err)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("successfully add organization token to workspace", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateAPITokenRolesResponseOK, nil)

		err := AddOrgTokenToWorkspace("", selectedTokenName2, role, workspace, out, mockClient)
		s.NoError(err)
	})

	s.Run("successfully select and add organization token to workspace", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateAPITokenRolesResponseOK, nil)
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
		err = AddOrgTokenToWorkspace("", "", role, workspace, out, mockClient)
		s.NoError(err)
	})

	s.Run("successfully select and add organization token to workspace 2", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponse2O0, nil)
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateAPITokenRolesResponseOK, nil)
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
		err = AddOrgTokenToWorkspace("", selectedTokenName, role, workspace, out, mockClient)
		s.NoError(err)
	})
}

func (s *Suite) TestListTokens() {
	s.Run("happy path", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		err := ListTokens(mockClient, out)
		s.NoError(err)
	})

	s.Run("error path when ListApiTokensWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseError, nil)
		err := ListTokens(mockClient, out)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error getting current context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		err := ListTokens(mockClient, out)

		s.Error(err)
	})
}

func (s *Suite) TestCreateToken() {
	s.Run("happy path", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&CreateOrganizationAPITokenResponseOK, nil)

		err := CreateToken("Token 1", "Description 1", "ORGANIZATION_MEMBER", 0, false, out, mockClient)

		s.NoError(err)
	})

	s.Run("error getting current context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)

		err := CreateToken("Token 1", "Description 1", "ORGANIZATION_MEMBER", 0, false, out, mockClient)

		s.Error(err)
	})

	s.Run("invalid role", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)

		err := CreateToken("Token 1", "Description 1", "InvalidRole", 0, false, out, mockClient)

		s.Error(err)
	})

	s.Run("empty name", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)

		err := CreateToken("", "Description 1", "ORGANIZATION_MEMBER", 0, true, out, mockClient)

		s.Equal(ErrInvalidName, err)
	})
}

func (s *Suite) TestUpdateToken() {
	s.Run("happy path", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
		err := UpdateToken("token1", "", "", "", "", out, mockClient)
		s.NoError(err)
	})

	s.Run("happy path no id", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
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
		err = UpdateToken("", "", "", "", "", out, mockClient)
		s.NoError(err)
	})

	s.Run("happy path multiple name", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponse2O0, nil)
		mockClient.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
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
		err = UpdateToken("", "Token 1", "", "", "", out, mockClient)
		s.NoError(err)
	})

	s.Run("happy path again", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
		err := UpdateToken("token1", "", "", "", "", out, mockClient)
		s.NoError(err)
	})

	s.Run("error path when listOrganizationTokens returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseError, nil)
		err := UpdateToken("", "", "", "", "", out, mockClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error path when listOrganizationToken returns an not found error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		err := UpdateToken("", "invalid name", "", "", "", out, mockClient)
		s.Equal(errOrganizationTokenNotFound, err)
	})

	s.Run("error path when getApiToken returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)

		err := UpdateToken("tokenId", "", "", "", "", out, mockClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error path when UpdateApiTokenWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseError, nil)
		err := UpdateToken("token3", "", "", "", "", out, mockClient)
		s.Equal("failed to update token", err.Error())
	})

	s.Run("error path when there is no context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		err := UpdateToken("token1", "", "", "", "", out, mockClient)
		s.Error(err)
	})

	s.Run("error path when organization role is invalid returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
		err := UpdateToken("token1", "", "", "", "Invalid Role", out, mockClient)
		s.Equal(user.ErrInvalidOrganizationRole.Error(), err.Error())
	})
	s.Run("Happy path when applying organization role", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateAPITokenRolesResponseOK, nil)
		err := UpdateToken("", apiToken1.Name, "", "", "ORGANIZATION_MEMBER", out, mockClient)
		s.NoError(err)
	})

	s.Run("Happy path when applying organization role using id", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil)
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateAPITokenRolesResponseOK, nil)
		err := UpdateToken("token1", "", "", "", "ORGANIZATION_MEMBER", out, mockClient)
		s.NoError(err)
	})

	s.Run("error path - wrong token type", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)
		err := UpdateToken("token1", "", "", "", "", out, mockClient)
		s.ErrorContains(err, "the token selected is not of the type you are trying to modify")
	})
}

func (s *Suite) TestRotateToken() {
	s.Run("happy path - id provided", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient.On("RotateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&RotateOrganizationAPITokenResponseOK, nil)
		err := RotateToken("token1", "", false, true, out, mockClient)
		s.NoError(err)
	})

	s.Run("happy path name provided", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("RotateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&RotateOrganizationAPITokenResponseOK, nil)
		err := RotateToken("", apiToken1.Name, false, true, out, mockClient)
		s.NoError(err)
	})

	s.Run("happy path with confirmation", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("RotateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&RotateOrganizationAPITokenResponseOK, nil)
		err := RotateToken("", apiToken1.Name, true, false, out, mockClient)
		s.NoError(err)
	})

	s.Run("error path when there is no context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		err := RotateToken("token1", "", false, false, out, mockClient)
		s.Error(err)
	})

	s.Run("error path when listOrganizationTokens returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseError, nil)
		err := RotateToken("", "", false, false, out, mockClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error path when listOrganizationToken returns an not found error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		err := RotateToken("", "invalid name", false, false, out, mockClient)
		s.Equal(errOrganizationTokenNotFound, err)
	})

	s.Run("error path when getApiToken returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)
		err := RotateToken("token1", "", false, false, out, mockClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error path when RotateApiTokenWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("RotateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&RotateOrganizationAPITokenResponseError, nil)
		err := RotateToken("", apiToken1.Name, false, true, out, mockClient)
		s.Equal("failed to update token", err.Error())
	})
}

func (s *Suite) TestDeleteToken() {
	s.Run("happy path - delete organization token - by name", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("DeleteApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationAPITokenResponseOK, nil)
		err := DeleteToken("", apiToken1.Name, false, out, mockClient)
		s.NoError(err)
	})

	s.Run("happy path - delete organization token - by id", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient.On("DeleteApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationAPITokenResponseOK, nil)
		err := DeleteToken("token1", "", false, out, mockClient)
		s.NoError(err)
	})

	s.Run("happy path - remove organization token", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("DeleteApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationAPITokenResponseOK, nil)
		err := DeleteToken("", apiToken1.Name, false, out, mockClient)
		s.NoError(err)
	})

	s.Run("error path when there is no context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		err := DeleteToken("token1", "", false, out, mockClient)
		s.Error(err)
	})

	s.Run("error path when getApiToken returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)
		err := DeleteToken("token1", "", false, out, mockClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error path when listOrganizationTokens returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseError, nil)
		err := DeleteToken("", apiToken1.Name, false, out, mockClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error path when listOrganizationToken returns a not found error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		err := DeleteToken("", "invalid name", false, out, mockClient)
		s.Equal(errOrganizationTokenNotFound, err)
	})

	s.Run("error path when DeleteApiTokenWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
		mockClient.On("DeleteApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationAPITokenResponseError, nil)
		err := DeleteToken("", apiToken1.Name, true, out, mockClient)
		s.Equal("failed to update token", err.Error())
	})
}

func (s *Suite) TestListTokenRoles() {
	s.Run("happy path - list token roles by id", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		err := ListTokenRoles("token1", mockClient, out)
		s.NoError(err)
	})

	s.Run("error path - list token roles by id - api error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)
		err := ListTokenRoles("token1", mockClient, out)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error path - list token roles by id - no context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		err := ListTokenRoles("token1", mockClient, out)
		s.Error(err)
	})

	s.Run("happy path - list token roles no id", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOK, nil)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil)
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
		err = ListTokenRoles("", mockClient, out)
		s.NoError(err)
	})

	s.Run("error path - list token roles no id - list tokens api error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseError, nil)
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
		err = ListTokenRoles("", mockClient, out)
		s.ErrorContains(err, "failed to list tokens")
	})
}

func (s *Suite) TestGetOrganizationToken() {
	s.Run("select token by id when name is empty", func() {
		token, err := getOrganizationToken("token1", "", "\nPlease select the workspace token you would like to delete or remove:", apiTokens)
		s.NoError(err)
		s.Equal(apiToken1, token)
	})

	s.Run("select token by name when id is empty and there is only one matching token", func() {
		token, err := getOrganizationToken("", "Token 2", "\nPlease select the workspace token you would like to delete or remove:", apiTokens)
		s.NoError(err)
		s.Equal(apiTokens[1], token)
	})

	s.Run("return error when token is not found by id", func() {
		token, err := getOrganizationToken("nonexistent", "", "\nPlease select the workspace token you would like to delete or remove:", apiTokens)
		s.Equal(errOrganizationTokenNotFound, err)
		s.Equal(astrov1.ApiToken{}, token)
	})

	s.Run("return error when token is not found by name", func() {
		token, err := getOrganizationToken("", "Nonexistent Token", "\nPlease select the workspace token you would like to delete or remove:", apiTokens)
		s.Equal(errOrganizationTokenNotFound, err)
		s.Equal(astrov1.ApiToken{}, token)
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
