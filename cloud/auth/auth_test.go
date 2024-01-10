package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/context"

	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astroplatformcore_mocks "github.com/astronomer/astro-cli/astro-client-platform-core/mocks"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	errMock = errors.New("mock-error")

	mockOrganizationID      = "test-org-id"
	mockOrganizationProduct = astroplatformcore.OrganizationProductHYBRID
	mockGetSelfResponse     = astrocore.GetSelfUserResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.Self{
			OrganizationId: &mockOrganizationID,
			Username:       "test@astronomer.io",
			FullName:       "jane",
			Id:             "user-id",
		},
	}
	mockGetSelfResponseNoOrg = astrocore.GetSelfUserResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.Self{
			Username: "test@astronomer.io",
			FullName: "jane",
			Id:       "user-id",
		},
	}
	mockGetSelfErrorBody, _ = json.Marshal(astrocore.Error{
		Message: "failed to fetch self user",
	})
	mockGetSelfErrorResponse = astrocore.GetSelfUserResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    mockGetSelfErrorBody,
		JSON200: nil,
	}
	mockOrganizationsResponse = astroplatformcore.ListOrganizationsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.OrganizationsPaginated{
			Organizations: []astroplatformcore.Organization{
				{Id: "org1", Name: "org1", Product: &mockOrganizationProduct},
				{Id: "org2", Name: "org2", Product: &mockOrganizationProduct},
			},
		},
	}
	mockOrganizationsResponseEmpty = astroplatformcore.ListOrganizationsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.OrganizationsPaginated{
			Organizations: []astroplatformcore.Organization{},
		},
	}
	errNetwork  = errors.New("network error")
	description = "test workspace"
	workspace1  = astrocore.Workspace{
		Name:                         "test-workspace",
		Description:                  &description,
		ApiKeyOnlyDeploymentsDefault: false,
		Id:                           "workspace-id",
	}

	workspaces = []astrocore.Workspace{
		workspace1,
	}

	ListWorkspacesResponseOK = astrocore.ListWorkspacesResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.WorkspacesPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
			Workspaces: workspaces,
		},
	}
)

func Test_FetchDomainAuthConfig(t *testing.T) {
	domain := "astronomer.io"
	actual, err := FetchDomainAuthConfig(domain)
	assert.NoError(t, err)
	assert.Equal(t, actual.ClientID, "5XYJZYf5xZ0eKALgBH3O08WzgfUfz7y9")
	assert.Equal(t, actual.Audience, "astronomer-ee")
	assert.Equal(t, actual.DomainURL, "https://auth.astronomer.io/")

	domain = "gcp0001.us-east4.astronomer.io" // Gen1 CLI domain
	actual, err = FetchDomainAuthConfig(domain)
	assert.Error(t, err)
	assert.Errorf(t, err, "Error! Invalid domain. "+
		"Are you trying to authenticate to Astronomer Software? If so, change your current context with 'astro context switch'. ")

	domain = "fail.astronomer.io"
	actual, err = FetchDomainAuthConfig(domain)
	assert.Error(t, err)
	assert.Errorf(t, err, "Error! Invalid domain. "+
		"Are you trying to authenticate to Astronomer Software? If so, change your current context with 'astro context switch'. ")

	domain = "astronomer-dev.io"
	actual, err = FetchDomainAuthConfig(domain)
	assert.NoError(t, err)
	assert.Equal(t, actual.ClientID, "PH3Nac2DtpSx1Tx3IGQmh2zaRbF5ubZG")
	assert.Equal(t, actual.Audience, "astronomer-ee")
	assert.Equal(t, actual.DomainURL, "https://auth.astronomer-dev.io/")

	domain = "fail.astronomer-dev.io"
	actual, err = FetchDomainAuthConfig(domain)
	assert.Error(t, err)
	assert.Errorf(t, err, "Error! Invalid domain. "+
		"Are you trying to authenticate to Astronomer Software? If so, change your current context with 'astro context switch'. ")

	domain = "astronomer-stage.io"
	actual, err = FetchDomainAuthConfig(domain)
	assert.NoError(t, err)
	assert.Equal(t, actual.ClientID, "jsarDat3BeDXZ1monEAeqJPOvRvterpm")
	assert.Equal(t, actual.Audience, "astronomer-ee")
	assert.Equal(t, actual.DomainURL, "https://auth.astronomer-stage.io/")

	domain = "fail.astronomer-stage.io"
	actual, err = FetchDomainAuthConfig(domain)
	assert.Error(t, err)
	assert.Errorf(t, err, "Error! Invalid domain. "+
		"Are you trying to authenticate to Astronomer Software? If so, change your current context with 'astro context switch'. ")

	domain = "fail.astronomer-perf.io"
	actual, err = FetchDomainAuthConfig(domain)
	assert.Error(t, err)
	assert.Errorf(t, err, "Error! Invalid domain. "+
		"Are you trying to authenticate to Astronomer Software? If so, change your current context with 'astro context switch'. ")

	t.Run("pr preview is a valid domain", func(t *testing.T) {
		// mocking this as once a PR closes, test would fail
		mockResponse := AuthConfig{
			ClientID:  "client-id",
			Audience:  "audience",
			DomainURL: "https://myURL.com/",
		}
		jsonResponse, err := json.Marshal(mockResponse)
		assert.NoError(t, err)
		httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		domain = "pr1234.astronomer-dev.io"
		actual, err = FetchDomainAuthConfig(domain)
		assert.NoError(t, err)
		assert.Equal(t, actual.ClientID, mockResponse.ClientID)
		assert.Equal(t, actual.Audience, mockResponse.Audience)
		assert.Equal(t, actual.DomainURL, mockResponse.DomainURL)
	})
}

func TestRequestUserInfo(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockUserInfo := UserInfo{
		Email:      "test@astronomer.test",
		Name:       "astro.crush",
		FamilyName: "Crush",
		GivenName:  "Astro",
	}
	emptyUserInfo := UserInfo{}
	mockAccessToken := "access-token"
	userInfoResponse, err := json.Marshal(mockUserInfo)
	assert.NoError(t, err)
	emptyResponse, err := json.Marshal(emptyUserInfo)
	assert.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(userInfoResponse)),
				Header:     make(http.Header),
			}
		})
		resp, err := requestUserInfo(AuthConfig{}, mockAccessToken)
		assert.NoError(t, err)
		assert.Equal(t, mockUserInfo, resp)
	})

	t.Run("failure", func(t *testing.T) {
		httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})

		_, err := requestUserInfo(AuthConfig{}, mockAccessToken)
		assert.Contains(t, err.Error(), "Internal Server Error")
	})

	t.Run("fail with no email", func(t *testing.T) {
		assert.NoError(t, err)
		httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(emptyResponse)),
				Header:     make(http.Header),
			}
		})
		_, err := requestUserInfo(AuthConfig{}, mockAccessToken)
		assert.Contains(t, err.Error(), "cannot retrieve email")
	})
}

func TestRequestToken(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockResponse := postTokenResponse{
		RefreshToken: "test-refresh-token",
		AccessToken:  "test-access-token",
		ExpiresIn:    300,
	}
	jsonResponse, err := json.Marshal(mockResponse)
	assert.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})

		resp, err := requestToken(AuthConfig{}, "", "")
		assert.NoError(t, err)
		assert.Equal(t, Result{RefreshToken: mockResponse.RefreshToken, AccessToken: mockResponse.AccessToken, ExpiresIn: mockResponse.ExpiresIn}, resp)
	})

	t.Run("failure", func(t *testing.T) {
		httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})

		_, err := requestToken(AuthConfig{}, "", "")
		assert.Contains(t, err.Error(), "Internal Server Error")
	})

	errMock := "test-error"
	mockResponse = postTokenResponse{
		ErrorDescription: errMock,
		Error:            &errMock,
	}
	jsonResponse, err = json.Marshal(mockResponse)
	assert.NoError(t, err)

	t.Run("token error", func(t *testing.T) {
		httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})

		_, err := requestToken(AuthConfig{}, "", "")
		assert.Contains(t, err.Error(), mockResponse.ErrorDescription)
	})
}

func TestAuthorizeCallbackHandler(t *testing.T) {
	httpClient = httputil.NewHTTPClient()
	t.Run("success", func(t *testing.T) {
		callbackServer = "localhost:12345"
		go func() {
			time.Sleep(2 * time.Second) // time to spinup the server in authorizeCallbackHandler

			opts := &httputil.DoOptions{
				Method: http.MethodGet,
				Path:   "http://localhost:12345/callback?code=test",
			}
			_, err = httpClient.Do(opts) //nolint
			assert.NoError(t, err)
		}()
		code, err := authorizeCallbackHandler()
		assert.Equal(t, "test", code)
		assert.NoError(t, err)
	})

	t.Run("error", func(t *testing.T) {
		callbackServer = "localhost:12346"
		go func() {
			time.Sleep(2 * time.Second) // time to spinup the server in authorizeCallbackHandler
			opts := &httputil.DoOptions{
				Method: http.MethodGet,
				Path:   "http://localhost:12346/callback?error=error&error_description=fatal_error",
			}
			_, err = httpClient.Do(opts) //nolint
			assert.NoError(t, err)
		}()
		_, err := authorizeCallbackHandler()
		assert.Contains(t, err.Error(), "fatal_error")
	})

	t.Run("timeout", func(t *testing.T) {
		callbackServer = "localhost:12347"
		callbackTimeout = 5 * time.Millisecond
		_, err := authorizeCallbackHandler()
		assert.Contains(t, err.Error(), "the operation has timed out")
	})
}

func TestAuthDeviceLogin(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("success without login link", func(t *testing.T) {
		mockResponse := Result{RefreshToken: "test-token", AccessToken: "test-token", ExpiresIn: 300}
		callbackHandler := func() (string, error) {
			return "test-code", nil
		}
		tokenRequester := func(authConfig AuthConfig, verifier, code string) (Result, error) {
			return mockResponse, nil
		}
		openURL = func(url string) error {
			return nil
		}
		mockAuthenticator := Authenticator{tokenRequester: tokenRequester, callbackHandler: callbackHandler}
		resp, err := mockAuthenticator.authDeviceLogin(AuthConfig{}, false)
		assert.NoError(t, err)
		assert.Equal(t, mockResponse, resp)
	})

	t.Run("openURL & callback failure", func(t *testing.T) {
		openURL = func(url string) error {
			return errMock
		}
		callbackHandler := func() (string, error) {
			return "", errMock
		}
		mockAuthenticator := Authenticator{callbackHandler: callbackHandler}
		_, err = mockAuthenticator.authDeviceLogin(AuthConfig{}, false)
		assert.ErrorIs(t, err, errMock)
	})

	t.Run("token requester failure", func(t *testing.T) {
		callbackHandler := func() (string, error) {
			return "test-code", nil
		}
		tokenRequester := func(authConfig AuthConfig, verifier, code string) (Result, error) {
			return Result{}, errMock
		}
		openURL = func(url string) error {
			return nil
		}
		mockAuthenticator := Authenticator{tokenRequester: tokenRequester, callbackHandler: callbackHandler}
		_, err = mockAuthenticator.authDeviceLogin(AuthConfig{}, false)
		assert.ErrorIs(t, err, errMock)
	})

	t.Run("success with login link", func(t *testing.T) {
		mockResponse := Result{RefreshToken: "test-token", AccessToken: "test-token", ExpiresIn: 300}
		callbackHandler := func() (string, error) {
			return "test-code", nil
		}
		tokenRequester := func(authConfig AuthConfig, verifier, code string) (Result, error) {
			return mockResponse, nil
		}
		mockAuthenticator := Authenticator{tokenRequester: tokenRequester, callbackHandler: callbackHandler}
		resp, err := mockAuthenticator.authDeviceLogin(AuthConfig{}, true)
		assert.NoError(t, err)
		assert.Equal(t, mockResponse, resp)
	})

	t.Run("callback failure with login link", func(t *testing.T) {
		callbackHandler := func() (string, error) {
			return "", errMock
		}
		mockAuthenticator := Authenticator{callbackHandler: callbackHandler}
		_, err = mockAuthenticator.authDeviceLogin(AuthConfig{}, true)
		assert.ErrorIs(t, err, errMock)
	})

	t.Run("token requester failure with login link", func(t *testing.T) {
		callbackHandler := func() (string, error) {
			return "test-code", nil
		}
		tokenRequester := func(authConfig AuthConfig, verifier, code string) (Result, error) {
			return Result{}, errMock
		}
		mockAuthenticator := Authenticator{tokenRequester: tokenRequester, callbackHandler: callbackHandler}
		_, err = mockAuthenticator.authDeviceLogin(AuthConfig{}, true)
		assert.ErrorIs(t, err, errMock)
	})
}

func TestSwitchToLastUsedWorkspace(t *testing.T) {
	t.Run("failure case", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		ctx := &config.Context{
			LastUsedWorkspace: "test-id",
		}
		resp, found, err := switchToLastUsedWorkspace(ctx, []astrocore.Workspace{{Id: "test-id"}})
		assert.ErrorIs(t, err, config.ErrCtxConfigErr)
		assert.False(t, found)
		assert.Equal(t, astrocore.Workspace{}, resp)
	})

	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("success", func(t *testing.T) {
		ctx := &config.Context{
			LastUsedWorkspace: "test-id",
			Domain:            "test-domain",
		}
		resp, found, err := switchToLastUsedWorkspace(ctx, []astrocore.Workspace{{Id: "test-id"}})
		assert.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, ctx.LastUsedWorkspace, resp.Id)
	})

	t.Run("failure, workspace not found", func(t *testing.T) {
		ctx := &config.Context{
			LastUsedWorkspace: "test-invalid-id",
		}
		resp, found, err := switchToLastUsedWorkspace(ctx, []astrocore.Workspace{{Id: "test-id"}})
		assert.NoError(t, err)
		assert.False(t, found)
		assert.Equal(t, astrocore.Workspace{}, resp)
	})
}

func TestCheckUserSession(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("success", func(t *testing.T) {
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockPlatformCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astroplatformcore.ListOrganizationsParams{}).Return(&mockOrganizationsResponse, nil).Once()
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockCoreClient, mockPlatformCoreClient, buf)
		assert.NoError(t, err)
	})

	t.Run("no organization found", func(t *testing.T) {
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockPlatformCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astroplatformcore.ListOrganizationsParams{}).Return(&mockOrganizationsResponseEmpty, nil).Once()
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockCoreClient, mockPlatformCoreClient, buf)
		assert.Contains(t, err.Error(), "Please contact your Astro Organization Owner to be invited to the organization")
	})

	t.Run("list organization network error", func(t *testing.T) {
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockPlatformCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astroplatformcore.ListOrganizationsParams{}).Return(nil, errNetwork).Once()
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockCoreClient, mockPlatformCoreClient, buf)
		assert.Contains(t, err.Error(), "network error")
	})

	t.Run("self user network error", func(t *testing.T) {
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(nil, errNetwork).Once()
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockCoreClient, mockPlatformCoreClient, buf)
		assert.Contains(t, err.Error(), "network error")
	})

	t.Run("self user failure", func(t *testing.T) {
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfErrorResponse, nil).Once()
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockCoreClient, mockPlatformCoreClient, buf)
		assert.Contains(t, err.Error(), "failed to fetch self user")
	})

	t.Run("set context failure", func(t *testing.T) {
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockPlatformCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astroplatformcore.ListOrganizationsParams{}).Return(&mockOrganizationsResponse, nil).Once()
		ctx := config.Context{}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockCoreClient, mockPlatformCoreClient, buf)
		assert.ErrorIs(t, err, config.ErrCtxConfigErr)
	})

	t.Run("list workspace failure", func(t *testing.T) {
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errMock).Once()
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockPlatformCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astroplatformcore.ListOrganizationsParams{}).Return(&mockOrganizationsResponse, nil).Once()
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockCoreClient, mockPlatformCoreClient, buf)
		assert.ErrorIs(t, err, errMock)
	})

	t.Run("success with more than one workspace", func(t *testing.T) {
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockPlatformCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astroplatformcore.ListOrganizationsParams{}).Return(&mockOrganizationsResponse, nil).Once()

		ctx := config.Context{Domain: "test-domain", LastUsedWorkspace: "workspace-id", Organization: "test-org-id"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockCoreClient, mockPlatformCoreClient, buf)
		assert.NoError(t, err)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("success with workspace switch", func(t *testing.T) {
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockPlatformCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astroplatformcore.ListOrganizationsParams{}).Return(&mockOrganizationsResponse, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		ctx := config.Context{Domain: "test-domain", Organization: "test-org-id"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockCoreClient, mockPlatformCoreClient, buf)
		assert.NoError(t, err)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("success but with workspace switch failure", func(t *testing.T) {
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		workspace2 := astrocore.Workspace{
			Name:                         "test-workspace-2",
			Description:                  &description,
			ApiKeyOnlyDeploymentsDefault: false,
			Id:                           "workspace-id-2",
		}

		workspaces = []astrocore.Workspace{
			workspace1,
			workspace2,
		}
		tempListWorkspacesResponseOK := astrocore.ListWorkspacesResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astrocore.WorkspacesPaginated{
				Limit:      1,
				Offset:     0,
				TotalCount: 1,
				Workspaces: workspaces,
			},
		}
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockPlatformCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astroplatformcore.ListOrganizationsParams{}).Return(&mockOrganizationsResponse, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&tempListWorkspacesResponseOK, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errMock).Once()
		ctx := config.Context{Domain: "test-domain", Organization: "test-org-id"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockCoreClient, mockPlatformCoreClient, buf)
		assert.NoError(t, err)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("success with identity first auth flow", func(t *testing.T) {
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockPlatformCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astroplatformcore.ListOrganizationsParams{}).Return(&mockOrganizationsResponse, nil).Once()
		// context org  "test-org-id-2" takes precedence over the getSelf org "test-org-id"
		ctx := config.Context{Domain: "test-domain", Organization: "test-org-id-2"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockCoreClient, mockPlatformCoreClient, buf)
		assert.NoError(t, err)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("set default org product", func(t *testing.T) {
		mockOrganizationsResponse = astroplatformcore.ListOrganizationsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astroplatformcore.OrganizationsPaginated{
				Limit:      1,
				Offset:     0,
				TotalCount: 1,
				Organizations: []astroplatformcore.Organization{
					{Id: "test-org-id", Name: "test-org"},
				},
			},
		}
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockPlatformCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astroplatformcore.ListOrganizationsParams{}).Return(&mockOrganizationsResponse, nil).Once()
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockCoreClient, mockPlatformCoreClient, buf)
		assert.NoError(t, err)
	})
}

func TestLogin(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("success", func(t *testing.T) {
		mockResponse := Result{RefreshToken: "test-token", AccessToken: "test-token", ExpiresIn: 300}
		mockUserInfo := UserInfo{Email: "test@astronomer.test"}
		callbackHandler := func() (string, error) {
			return "test-code", nil
		}
		tokenRequester := func(authConfig AuthConfig, verifier, code string) (Result, error) {
			return mockResponse, nil
		}
		userInfoRequester := func(authConfig AuthConfig, accessToken string) (UserInfo, error) {
			return mockUserInfo, nil
		}
		openURL = func(url string) error {
			return nil
		}
		authenticator = Authenticator{userInfoRequester, tokenRequester, callbackHandler}

		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockPlatformCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astroplatformcore.ListOrganizationsParams{}).Return(&mockOrganizationsResponse, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		err := Login("astronomer.io", "", mockCoreClient, mockPlatformCoreClient, os.Stdout, false)
		assert.NoError(t, err)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("can login to a pr preview environment successfully", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPrPreview)
		// mocking this as once a PR closes, test would fail
		mockAuthConfigResponse := AuthConfig{
			ClientID:  "client-id",
			Audience:  "audience",
			DomainURL: "https://myURL.com/",
		}
		jsonResponse, err := json.Marshal(mockAuthConfigResponse)
		assert.NoError(t, err)
		httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		mockResponse := Result{RefreshToken: "test-token", AccessToken: "test-token", ExpiresIn: 300}
		mockUserInfo := UserInfo{Email: "test@astronomer.test"}
		callbackHandler := func() (string, error) {
			return "test-code", nil
		}
		tokenRequester := func(authConfig AuthConfig, verifier, code string) (Result, error) {
			return mockResponse, nil
		}
		userInfoRequester := func(authConfig AuthConfig, accessToken string) (UserInfo, error) {
			return mockUserInfo, nil
		}
		openURL = func(url string) error {
			return nil
		}
		authenticator = Authenticator{userInfoRequester, tokenRequester, callbackHandler}
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockPlatformCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astroplatformcore.ListOrganizationsParams{}).Return(&mockOrganizationsResponse, nil).Once()

		err = Login("pr5723.cloud.astronomer-dev.io", "", mockCoreClient, mockPlatformCoreClient, os.Stdout, false)
		assert.NoError(t, err)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("oauth token success", func(t *testing.T) {
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockPlatformCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astroplatformcore.ListOrganizationsParams{}).Return(&mockOrganizationsResponse, nil).Once()

		err := Login("astronomer.io", "OAuth Token", mockCoreClient, mockPlatformCoreClient, os.Stdout, false)
		assert.NoError(t, err)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("invalid domain", func(t *testing.T) {
		err := Login("fail.astronomer.io", "", nil, nil, os.Stdout, false)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Invalid domain.")
	})

	t.Run("auth failure", func(t *testing.T) {
		callbackHandler := func() (string, error) {
			return "", errMock
		}
		authenticator = Authenticator{callbackHandler: callbackHandler}
		err := Login("cloud.astronomer.io", "", nil, nil, os.Stdout, false)
		assert.ErrorIs(t, err, errMock)
	})

	t.Run("check token failure", func(t *testing.T) {
		mockResponse := Result{RefreshToken: "test-token", AccessToken: "test-token", ExpiresIn: 300}
		mockUserInfo := UserInfo{Email: "test@astronomer.test"}
		callbackHandler := func() (string, error) {
			return "test-code", nil
		}
		tokenRequester := func(authConfig AuthConfig, verifier, code string) (Result, error) {
			return mockResponse, nil
		}
		userInfoRequester := func(authConfig AuthConfig, accessToken string) (UserInfo, error) {
			return mockUserInfo, nil
		}
		openURL = func(url string) error {
			return nil
		}
		authenticator = Authenticator{userInfoRequester, tokenRequester, callbackHandler}

		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfErrorResponse, nil).Once()
		err := Login("", "", mockCoreClient, mockPlatformCoreClient, os.Stdout, false)
		assert.Contains(t, err.Error(), "failed to fetch self user")
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("initial login with empty config file", func(t *testing.T) {
		// initialize empty config
		testUtil.InitTestConfig(testUtil.Initial)
		// initialize the mock client
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockPlatformCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astroplatformcore.ListOrganizationsParams{}).Return(&mockOrganizationsResponse, nil).Once()
		// initialize the test authenticator
		mockUserInfo := UserInfo{Email: "test@astronomer.test"}
		userInfoRequester := func(authConfig AuthConfig, accessToken string) (UserInfo, error) {
			return mockUserInfo, nil
		}
		authenticator = Authenticator{
			userInfoRequester: userInfoRequester,
			callbackHandler:   func() (string, error) { return "authorizationCode", nil },
			tokenRequester: func(authConfig AuthConfig, verifier, code string) (Result, error) {
				return Result{
					RefreshToken: "refresh_token",
					AccessToken:  "access_token",
					ExpiresIn:    1234,
				}, nil
			},
		}
		// initialize stdin with user email input
		defer testUtil.MockUserInput(t, "test.user@astronomer.io")()
		// do the test
		err = Login("astronomer.io", "", mockCoreClient, mockPlatformCoreClient, os.Stdout, true)
		assert.NoError(t, err)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("domain doesn't match current context", func(t *testing.T) {
		// initialize empty config
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		// initialize the mock client
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockPlatformCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astroplatformcore.ListOrganizationsParams{}).Return(&mockOrganizationsResponse, nil).Once()
		// initialize the test authenticator
		mockUserInfo := UserInfo{Email: "test@astronomer.test"}
		userInfoRequester := func(authConfig AuthConfig, accessToken string) (UserInfo, error) {
			return mockUserInfo, nil
		}
		authenticator = Authenticator{
			userInfoRequester: userInfoRequester,
			callbackHandler:   func() (string, error) { return "authorizationCode", nil },
			tokenRequester: func(authConfig AuthConfig, verifier, code string) (Result, error) {
				return Result{
					RefreshToken: "refresh_token",
					AccessToken:  "access_token",
					ExpiresIn:    1234,
				}, nil
			},
		}
		// initialize user input with email
		defer testUtil.MockUserInput(t, "test.user@astronomer.io")()
		err := Login("astronomer.io", "", mockCoreClient, mockPlatformCoreClient, os.Stdout, true)
		assert.NoError(t, err)
		// assert that everything got set in the right spot
		domainContext, err := context.GetContext("astronomer.io")
		assert.NoError(t, err)
		currentContext, err := context.GetContext("localhost")
		assert.NoError(t, err)
		assert.Equal(t, domainContext.Token, "Bearer access_token")
		assert.Equal(t, currentContext.Token, "token")
		mockCoreClient.AssertExpectations(t)
	})
}

func TestLogout(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("success", func(t *testing.T) {
		buf := new(bytes.Buffer)
		Logout("astronomer.io", buf)
		assert.Equal(t, "Successfully logged out of Astronomer\n", buf.String())
	})

	t.Run("success_with_email", func(t *testing.T) {
		assertions := func(expUserEmail string, expToken string) {
			contexts, err := config.GetContexts()
			assert.NoError(t, err)
			context := contexts.Contexts["localhost"]

			assert.NoError(t, err)
			assert.Equal(t, expUserEmail, context.UserEmail)
			assert.Equal(t, expToken, context.Token)
		}
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetContextKey("user_email", "test.user@astronomer.io")
		assert.NoError(t, err)
		err = c.SetContextKey("token", "Bearer some-token")
		assert.NoError(t, err)
		// test before
		assertions("test.user@astronomer.io", "Bearer some-token")

		// log out
		c, err = config.GetCurrentContext()
		assert.NoError(t, err)
		Logout(c.Domain, os.Stdout)

		// test after logout
		assertions("", "")
	})
}

func Test_writeResultToContext(t *testing.T) {
	assertConfigContents := func(expToken string, expRefresh string, expExpires time.Time, expUserEmail string) {
		context, err := config.GetCurrentContext()
		assert.NoError(t, err)
		// test the output on the config file
		assert.Equal(t, expToken, context.Token)
		assert.Equal(t, expRefresh, context.RefreshToken)
		expiresIn, err := context.GetExpiresIn()
		assert.NoError(t, err)
		assert.Equal(t, expExpires.Round(time.Second), expiresIn.Round(time.Second))
		assert.Equal(t, expUserEmail, context.UserEmail)
		assert.NoError(t, err)
	}
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	c, err := config.GetCurrentContext()
	assert.NoError(t, err)
	err = c.SetContextKey("token", "old_token")
	assert.NoError(t, err)
	// test input
	res := Result{
		AccessToken:  "new_token",
		RefreshToken: "new_refresh_token",
		ExpiresIn:    1234,
		UserEmail:    "test.user@astronomer.io",
	}
	// test before changes
	var timeZero time.Time
	assertConfigContents("old_token", "", timeZero, "")

	// apply function
	c, err = config.GetCurrentContext()
	assert.NoError(t, err)
	err = res.writeToContext(&c)
	assert.NoError(t, err)

	// test after changes
	assertConfigContents("Bearer new_token", "new_refresh_token", time.Now().Add(1234*time.Second), "test.user@astronomer.io")
}
