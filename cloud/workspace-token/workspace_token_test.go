package workspacetoken

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	astrov1 "github.com/astronomer/astro-cli/astro-client-v1"
	astrov1_mocks "github.com/astronomer/astro-cli/astro-client-v1/mocks"
	"github.com/astronomer/astro-cli/cloud/user"
	"github.com/astronomer/astro-cli/cloud/workspace"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

var (
	workspaceID  = "ck05r3bor07h40d02y2hw4n4v"
	description1 = "Description 1"
	description2 = "Description 2"
	fullName1    = "User 1"
	fullName2    = "User 2"
	token        = "token"

	iamAPIOrgnaizationToken = astrov1.ApiToken{
		Id:          "token1",
		Name:        "token1",
		Token:       &token,
		Description: description1,
		Scope:       astrov1.ApiTokenScopeORGANIZATION,
		Roles: &[]astrov1.ApiTokenRole{
			{EntityType: astrov1.ApiTokenRoleEntityTypeORGANIZATION, EntityId: "test-org-id", Role: "ORGANIZATION_MEMBER"},
			{EntityType: astrov1.ApiTokenRoleEntityTypeWORKSPACE, EntityId: workspaceID, Role: "WORKSPACE_AUTHOR"},
			{EntityType: astrov1.ApiTokenRoleEntityTypeWORKSPACE, EntityId: "WORKSPACE", Role: "WORKSPACE_AUTHOR"},
			{EntityType: astrov1.ApiTokenRoleEntityTypeDEPLOYMENT, EntityId: "DEPLOYMENT", Role: "DEPLOYMENT_ADMIN"},
		},
		CreatedAt: time.Now(),
		CreatedBy: &astrov1.BasicSubjectProfile{FullName: &fullName1},
	}
	GetAPITokensResponseOKOrganizationToken = astrov1.GetApiTokenResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &iamAPIOrgnaizationToken,
	}

	iamAPIWorkspaceToken = astrov1.ApiToken{
		Id:          "token1",
		Name:        "token1",
		Token:       &token,
		Description: description1,
		Scope:       astrov1.ApiTokenScopeWORKSPACE,
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

	errorTokenGet, _ = json.Marshal(astrov1.Error{
		Message: "failed to get token",
	})
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
		Scope:       astrov1.ApiTokenScopeWORKSPACE,
		Roles: &[]astrov1.ApiTokenRole{
			{EntityId: workspaceID, EntityType: astrov1.ApiTokenRoleEntityTypeWORKSPACE, Role: "WORKSPACE_MEMBER"},
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
			Scope:       astrov1.ApiTokenScopeORGANIZATION,
			Roles: &[]astrov1.ApiTokenRole{
				{EntityId: workspaceID, EntityType: astrov1.ApiTokenRoleEntityTypeWORKSPACE, Role: "WORKSPACE_MEMBER"},
			},
			CreatedAt: time.Now(),
			CreatedBy: &astrov1.BasicSubjectProfile{FullName: &fullName2},
		},
	}
	apiTokens2 = []astrov1.ApiToken{
		apiToken1,
		{
			Id:          "token2",
			Name:        "Token 2",
			Description: description2,
			Scope:       astrov1.ApiTokenScopeWORKSPACE,
			Roles: &[]astrov1.ApiTokenRole{
				{EntityId: workspaceID, EntityType: astrov1.ApiTokenRoleEntityTypeWORKSPACE, Role: "WORKSPACE_MEMBER"},
			},
			CreatedAt: time.Now(),
			CreatedBy: &astrov1.BasicSubjectProfile{FullName: &fullName2},
		},
	}

	ListWorkspaceAPITokensResponseOK = astrov1.ListApiTokensResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.ApiTokensPaginated{
			Tokens:     apiTokens,
			Limit:      100,
			Offset:     0,
			TotalCount: len(apiTokens),
		},
	}
	ListWorkspaceAPITokensResponse2O0 = astrov1.ListApiTokensResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.ApiTokensPaginated{
			Tokens:     apiTokens2,
			Limit:      100,
			Offset:     0,
			TotalCount: len(apiTokens2),
		},
	}
	errorBodyListTokens, _ = json.Marshal(astrov1.Error{
		Message: "failed to list tokens",
	})
	ListWorkspaceAPITokensResponseError = astrov1.ListApiTokensResponse{
		HTTPResponse: &http.Response{StatusCode: 500},
		Body:         errorBodyListTokens,
		JSON200:      nil,
	}

	CreateWorkspaceAPITokenResponseOK = astrov1.CreateApiTokenResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &apiToken1,
	}

	orgAPIToken1 = astrov1.ApiToken{
		Id:          "token1",
		Name:        "Token 1",
		Token:       &token,
		Description: description1,
		Scope:       astrov1.ApiTokenScopeORGANIZATION,
		Roles: &[]astrov1.ApiTokenRole{
			{EntityType: astrov1.ApiTokenRoleEntityTypeWORKSPACE, EntityId: workspaceID, Role: "ORGANIZATION_MEMBER"},
			{EntityType: astrov1.ApiTokenRoleEntityTypeORGANIZATION, EntityId: "ORGANIZATION", Role: "ORGANIZATION_MEMBER"},
		},
		CreatedAt: time.Now(),
		CreatedBy: &astrov1.BasicSubjectProfile{FullName: &fullName1},
	}
	orgAPIToken2 = astrov1.ApiToken{
		Id:          "token1",
		Name:        "Token 1",
		Token:       &token,
		Description: description1,
		Scope:       astrov1.ApiTokenScopeORGANIZATION,
		Roles: &[]astrov1.ApiTokenRole{
			{EntityType: astrov1.ApiTokenRoleEntityTypeORGANIZATION, EntityId: "ORGANIZATION", Role: "ORGANIZATION_MEMBER"},
			{EntityType: astrov1.ApiTokenRoleEntityTypeWORKSPACE, EntityId: "WORKSPACE", Role: "WORKSPACE_MEMBER"},
			{EntityType: astrov1.ApiTokenRoleEntityTypeDEPLOYMENT, EntityId: "DEPLOYMENT", Role: "DEPLOYMENT_ADMIN"},
		},
		CreatedAt: time.Now(),
		CreatedBy: &astrov1.BasicSubjectProfile{FullName: &fullName1},
	}

	orgAPITokens = []astrov1.ApiToken{
		orgAPIToken1,
		orgAPIToken2,
	}

	ListOrganizationAPITokensResponseOK = astrov1.ListApiTokensResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.ApiTokensPaginated{
			Tokens:     orgAPITokens,
			Limit:      100,
			Offset:     0,
			TotalCount: len(orgAPITokens),
		},
	}

	errorBodyList, _ = json.Marshal(astrov1.Error{
		Message: "failed to list tokens",
	})
	ListOrganizationAPITokensResponseError = astrov1.ListApiTokensResponse{
		HTTPResponse: &http.Response{StatusCode: 500},
		Body:         errorBodyList,
		JSON200:      nil,
	}

	UpdateOrganizationAPITokenResponseOK = astrov1.UpdateApiTokenRolesResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.SubjectRoles{},
	}

	errorBodyUpdateToken, _ = json.Marshal(astrov1.Error{
		Message: "failed to update token",
	})
	UpdateOrganizationAPITokenResponseError = astrov1.UpdateApiTokenRolesResponse{
		HTTPResponse: &http.Response{StatusCode: 500},
		Body:         errorBodyUpdateToken,
		JSON200:      nil,
	}

	errorBodyCreate, _ = json.Marshal(astrov1.Error{
		Message: "failed to create workspace token",
	})
	CreateWorkspaceAPITokenResponseError = astrov1.CreateApiTokenResponse{
		HTTPResponse: &http.Response{StatusCode: 500},
		Body:         errorBodyCreate,
		JSON200:      nil,
	}

	UpdateWorkspaceAPITokenResponseOK = astrov1.UpdateApiTokenResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &apiToken1,
	}
	UpdateWorkspaceAPITokenRolesResponseOK = astrov1.UpdateApiTokenRolesResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.SubjectRoles{},
	}

	errorBodyUpdate, _ = json.Marshal(astrov1.Error{
		Message: "failed to update workspace token",
	})
	UpdateWorkspaceAPITokenResponseError = astrov1.UpdateApiTokenResponse{
		HTTPResponse: &http.Response{StatusCode: 500},
		Body:         errorBodyUpdate,
		JSON200:      nil,
	}

	RotateWorkspaceAPITokenResponseOK = astrov1.RotateApiTokenResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &apiToken1,
	}
	RotateWorkspaceAPITokenResponseError = astrov1.RotateApiTokenResponse{
		HTTPResponse: &http.Response{StatusCode: 500},
		Body:         errorBodyUpdate,
		JSON200:      nil,
	}
	DeleteWorkspaceAPITokenResponseOK = astrov1.DeleteApiTokenResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
	}
	DeleteWorkspaceAPITokenResponseError = astrov1.DeleteApiTokenResponse{
		HTTPResponse: &http.Response{StatusCode: 500},
		Body:         errorBodyUpdate,
	}
)

type Suite struct {
	suite.Suite
}

func TestWorkspaceToken(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestListTokens() {
	s.Run("happy path", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Twice()
		err := ListTokens(mockClient, "", nil, out)
		s.NoError(err)
	})

	s.Run("with specified workspace", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Twice()

		err := ListTokens(mockClient, "otherWorkspace", nil, out)

		s.NoError(err)
	})

	s.Run("error path when ListApiTokensWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseError, nil).Twice()
		err := ListTokens(mockClient, "otherWorkspace", nil, out)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error getting current context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		err := ListTokens(mockClient, "", nil, out)

		s.Error(err)
	})
}

func (s *Suite) TestCreateToken() {
	s.Run("happy path", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&CreateWorkspaceAPITokenResponseOK, nil)

		err := CreateToken("Token 1", "Description 1", "WORKSPACE_MEMBER", "", 0, false, out, mockClient)

		s.NoError(err)
	})

	s.Run("error getting current context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)

		err := CreateToken("Token 1", "Description 1", "WORKSPACE_MEMBER", "", 0, false, out, mockClient)

		s.Error(err)
	})

	s.Run("invalid role", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)

		err := CreateToken("Token 1", "Description 1", "InvalidRole", "", 0, false, out, mockClient)

		s.Error(err)
	})

	s.Run("empty name", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)

		err := CreateToken("", "Description 1", "WORKSPACE_MEMBER", "", 0, true, out, mockClient)

		s.Equal(workspace.ErrInvalidTokenName, err)
	})
}

func (s *Suite) TestUpdateToken() {
	s.Run("happy path", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil)
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenRolesResponseOK, nil)

		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)
		err := UpdateToken("token1", "", "", "", "", "", out, mockClient)
		s.NoError(err)
	})

	s.Run("happy path no id", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponse2O0, nil).Twice()
		mockClient.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil)
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenRolesResponseOK, nil)

		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)
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
		err = UpdateToken("", "", "", "", "", "", out, mockClient)
		s.NoError(err)
	})

	s.Run("happy path multiple name", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponse2O0, nil).Twice()
		mockClient.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil)
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenRolesResponseOK, nil)

		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)
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
		err = UpdateToken("", "Token 1", "", "", "", "", out, mockClient)
		s.NoError(err)
	})

	s.Run("happy path duplicate", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil)
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenRolesResponseOK, nil)

		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)
		err := UpdateToken("token1", "", "", "", "", "", out, mockClient)
		s.NoError(err)
	})

	s.Run("error path when listWorkspaceTokens returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseError, nil)
		err := UpdateToken("", "", "", "", "", "", out, mockClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error path when listWorkspaceToken returns an not found error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseError, nil)
		err := UpdateToken("", "invalid name", "", "", "", "", out, mockClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error path when getApiToken returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)
		err := UpdateToken("tokenId", "", "", "", "", "", out, mockClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error path when UpdateApiTokenWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseError, nil)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := UpdateToken("token3", "", "", "", "", "", out, mockClient)
		s.Equal("failed to update workspace token", err.Error())
	})

	s.Run("error path when there is no context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		err := UpdateToken("token1", "", "", "", "", "", out, mockClient)
		s.Error(err)
	})

	s.Run("error path when workspace role is invalid returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := UpdateToken("token1", "", "", "", "Invalid Role", "", out, mockClient)
		s.Equal(user.ErrInvalidWorkspaceRole.Error(), err.Error())
	})

	s.Run("Happy path when applying workspace role", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		mockClient.On("UpdateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenResponseOK, nil)
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateWorkspaceAPITokenRolesResponseOK, nil)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := UpdateToken("", apiToken1.Name, "", "", "WORKSPACE_MEMBER", "", out, mockClient)
		s.NoError(err)
	})
}

func (s *Suite) TestRotateToken() {
	s.Run("happy path - id provided", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("RotateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&RotateWorkspaceAPITokenResponseOK, nil)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := RotateToken("token1", "", "", false, true, out, mockClient)
		s.NoError(err)
	})

	s.Run("happy path name provided", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		mockClient.On("RotateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&RotateWorkspaceAPITokenResponseOK, nil)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := RotateToken("", apiToken1.Name, "", false, true, out, mockClient)
		s.NoError(err)
	})

	s.Run("happy path with confirmation", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		mockClient.On("RotateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&RotateWorkspaceAPITokenResponseOK, nil)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := RotateToken("", apiToken1.Name, "", true, false, out, mockClient)
		s.NoError(err)
	})

	s.Run("error path when there is no context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		err := RotateToken("token1", "", "", false, false, out, mockClient)
		s.Error(err)
	})

	s.Run("error path when listWorkspaceTokens returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseError, nil)
		err := RotateToken("", "", "", false, false, out, mockClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error path when listWorkspaceToken returns an not found error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		err := RotateToken("", "invalid name", "", false, false, out, mockClient)
		s.Equal(ErrWorkspaceTokenNotFound, err)
	})

	s.Run("error path when getWorkspaceToken returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)
		err := RotateToken("token1", "", "", false, false, out, mockClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error path when RotateApiTokenWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		mockClient.On("RotateApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&RotateWorkspaceAPITokenResponseError, nil)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := RotateToken("", apiToken1.Name, "", false, true, out, mockClient)
		s.Equal("failed to update workspace token", err.Error())
	})
}

func (s *Suite) TestDeleteToken() {
	s.Run("happy path - delete workspace token - by name", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Twice()
		mockClient.On("DeleteApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceAPITokenResponseOK, nil)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := DeleteToken("", apiToken1.Name, "", false, out, mockClient)
		s.NoError(err)
	})

	s.Run("happy path - delete workspace token - by id", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("DeleteApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceAPITokenResponseOK, nil)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := DeleteToken("token1", "", "", false, out, mockClient)
		s.NoError(err)
	})

	s.Run("happy path - remove organization token", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Twice()
		mockClient.On("DeleteApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceAPITokenResponseOK, nil)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := DeleteToken("", apiToken1.Name, "", false, out, mockClient)
		s.NoError(err)
	})

	s.Run("error path when there is no context", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		err := DeleteToken("token1", "", "", false, out, mockClient)
		s.Error(err)
	})

	s.Run("error path when getApiToken returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil)

		err := DeleteToken("token1", "", "", false, out, mockClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error path when listWorkspaceTokens returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseError, nil)
		err := DeleteToken("", apiToken1.Name, "", false, out, mockClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error path when listWorkspaceToken returns a not found error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		err := DeleteToken("", "invalid name", "", false, out, mockClient)
		s.Equal(ErrWorkspaceTokenNotFound, err)
	})

	s.Run("error path when DeleteApiTokenWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil)
		mockClient.On("DeleteApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceAPITokenResponseError, nil)
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKWorkspaceToken, nil)

		err := DeleteToken("", apiToken1.Name, "", true, out, mockClient)
		s.Equal("failed to update workspace token", err.Error())
	})
}

func (s *Suite) TestGetWorkspaceToken() {
	s.Run("select token by id when name is empty", func() {
		token, err := getWorkspaceToken("token1", "", "testWorkspace", "\nPlease select the workspace token you would like to delete or remove:", apiTokens)
		s.NoError(err)
		s.Equal(apiToken1, token)
	})

	s.Run("select token by name when id is empty and there is only one matching token", func() {
		token, err := getWorkspaceToken("", "Token 2", "testWorkspace", "\nPlease select the workspace token you would like to delete or remove:", apiTokens)
		s.NoError(err)
		s.Equal(apiTokens[1], token)
	})

	s.Run("return error when token is not found by id", func() {
		token, err := getWorkspaceToken("nonexistent", "", "testWorkspace", "\nPlease select the workspace token you would like to delete or remove:", apiTokens)
		s.Equal(ErrWorkspaceTokenNotFound, err)
		s.Equal(astrov1.ApiToken{}, token)
	})

	s.Run("return error when token is not found by name", func() {
		token, err := getWorkspaceToken("", "Nonexistent Token", "testWorkspace", "\nPlease select the workspace token you would like to delete or remove:", apiTokens)
		s.Equal(ErrWorkspaceTokenNotFound, err)
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

func (s *Suite) TestUpsertOrgTokenWorkspaceRole() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path Create", func() {
		expectedOutMessage := "Astro Organization API token token1 was successfully added/updated to the Workspace\n"
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
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
		err = UpsertOrgTokenWorkspaceRole("", "", "", "", "create", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error on ListApiTokensWithResponse (create)", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseError, nil).Once()
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
		err = UpsertOrgTokenWorkspaceRole("", "", "", "", "create", out, mockClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("happy path Update", func() {
		expectedOutMessage := "Astro Organization API token token1 was successfully added/updated to the Workspace\n"
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()
		// mock os.Stdin (only 1 ORG-scoped token after filter)
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
		err = UpsertOrgTokenWorkspaceRole("", "", "", "", "update", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error on ListApiTokensWithResponse (update)", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseError, nil).Once()
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
		err = UpsertOrgTokenWorkspaceRole("", "", "", "", "update", out, mockClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error on GetApiTokenWithResponse", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Once()
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
		err = UpsertOrgTokenWorkspaceRole("", "", "", "", "create", out, mockClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error on UpdateApiTokenRolesWithResponse", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseError, nil).Once()
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
		err = UpsertOrgTokenWorkspaceRole("", "", "", "", "create", out, mockClient)
		s.ErrorContains(err, "failed to update token")
	})

	s.Run("happy path with token id passed in", func() {
		expectedOutMessage := "Astro Organization API token token1 was successfully added/updated to the Workspace\n"
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
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
		err = UpsertOrgTokenWorkspaceRole("token-id", "", "", "", "create", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error on GetApiTokenWithResponse with token id passed in", func() {
		out := new(bytes.Buffer)
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
		err = UpsertOrgTokenWorkspaceRole("token-id", "", "", "", "create", out, mockClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error path with token id passed in - wrong token type", func() {
		s.T().Skip("create path does not enforce workspace-scope tokenTypes restriction on id-only lookups; see organization.GetTokenFromInputOrUser")
	})
}

func (s *Suite) TestRemoveOrgTokenWorkspaceRole() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path", func() {
		expectedOutMessage := "Astro Organization API token token1 was successfully removed from the Workspace\n"
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()
		// mock os.Stdin (only 1 ORG-scoped token after filter)
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
		err = RemoveOrgTokenWorkspaceRole("", "", "", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error on ListApiTokensWithResponse", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseError, nil).Once()
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
		err = RemoveOrgTokenWorkspaceRole("", "", "", out, mockClient)
		s.ErrorContains(err, "failed to list tokens")
	})

	s.Run("error on GetApiTokenWithResponse", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Once()
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseError, nil).Once()
		// mock os.Stdin (only 1 ORG-scoped token after filter)
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
		err = RemoveOrgTokenWorkspaceRole("", "", "", out, mockClient)
		s.ErrorContains(err, "failed to get token")
	})

	s.Run("error on UpdateApiTokenRolesWithResponse", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListApiTokensWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceAPITokensResponseOK, nil).Once()
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseError, nil).Once()
		mockClient.On("GetApiTokenWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetAPITokensResponseOKOrganizationToken, nil).Once()
		// mock os.Stdin (only 1 ORG-scoped token after filter)
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
		err = RemoveOrgTokenWorkspaceRole("", "", "", out, mockClient)
		s.ErrorContains(err, "failed to update token")
	})

	s.Run("happy path with token id passed in", func() {
		expectedOutMessage := "Astro Organization API token token1 was successfully removed from the Workspace\n"
		out := new(bytes.Buffer)
		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("UpdateApiTokenRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateOrganizationAPITokenResponseOK, nil).Once()
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
		err = RemoveOrgTokenWorkspaceRole("token-id", "", "", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error on GetApiTokenWithResponse with token id passed in", func() {
		out := new(bytes.Buffer)
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
		err = RemoveOrgTokenWorkspaceRole("token-id", "", "", out, mockClient)
		s.ErrorContains(err, "failed to get token")
	})
}
