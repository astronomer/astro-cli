package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/context"

	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/astronomer/astro-cli/astro-client"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	errMock = errors.New("mock-error")

	mockOrganizationID  = "test-org-id"
	mockGetSelfResponse = astrocore.GetSelfUserResponse{
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
	mockOrganizationsResponse = astrocore.ListOrganizationsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &[]astrocore.Organization{
			{AuthServiceId: "auth-service-id", Id: "test-org-id", Name: "test-org"},
			{AuthServiceId: "auth-service-id", Id: "test-org-id-2", Name: "test-org-2"},
			{AuthServiceId: "auth-service-id", Id: "org1", Name: "org1"},
		},
	}
	mockOrganizationsResponseEmpty = astrocore.ListOrganizationsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &[]astrocore.Organization{},
	}
	errNetwork = errors.New("network error")
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
		mockResponse := astro.AuthConfig{
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

func TestOrgLookup(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockResponse := orgLookupResults{
		OrganizationIds: []string{"test-org-id"},
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

		resp, err := orgLookup("cloud.test-org.io")
		assert.NoError(t, err)
		assert.Equal(t, mockResponse.OrganizationIds[0], resp)
	})

	t.Run("failure", func(t *testing.T) {
		httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})

		_, err := orgLookup("cloud.test-org.io")
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestRequestToken(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
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

		resp, err := requestToken(astro.AuthConfig{}, "", "")
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

		_, err := requestToken(astro.AuthConfig{}, "", "")
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

		_, err := requestToken(astro.AuthConfig{}, "", "")
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
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	t.Run("success without login link", func(t *testing.T) {
		mockResponse := Result{RefreshToken: "test-token", AccessToken: "test-token", ExpiresIn: 300}
		orgChecker := func(domain string) (string, error) {
			return "test-org-id", nil
		}
		callbackHandler := func() (string, error) {
			return "test-code", nil
		}
		tokenRequester := func(authConfig astro.AuthConfig, verifier, code string) (Result, error) {
			return mockResponse, nil
		}
		openURL = func(url string) error {
			return nil
		}
		mockAuthenticator := Authenticator{orgChecker, tokenRequester, callbackHandler}
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		resp, err := mockAuthenticator.authDeviceLogin(c, astro.AuthConfig{}, false)
		assert.NoError(t, err)
		assert.Equal(t, mockResponse, resp)
	})

	t.Run("openURL & callback failure", func(t *testing.T) {
		orgChecker := func(domain string) (string, error) {
			return "test-org-id", nil
		}
		openURL = func(url string) error {
			return errMock
		}
		callbackHandler := func() (string, error) {
			return "", errMock
		}
		mockAuthenticator := Authenticator{orgChecker: orgChecker, callbackHandler: callbackHandler}
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		_, err = mockAuthenticator.authDeviceLogin(c, astro.AuthConfig{}, false)
		assert.ErrorIs(t, err, errMock)
	})

	t.Run("token requester failure", func(t *testing.T) {
		orgChecker := func(domain string) (string, error) {
			return "test-org-id", nil
		}
		callbackHandler := func() (string, error) {
			return "test-code", nil
		}
		tokenRequester := func(authConfig astro.AuthConfig, verifier, code string) (Result, error) {
			return Result{}, errMock
		}
		openURL = func(url string) error {
			return nil
		}
		mockAuthenticator := Authenticator{orgChecker, tokenRequester, callbackHandler}
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		_, err = mockAuthenticator.authDeviceLogin(c, astro.AuthConfig{}, false)
		assert.ErrorIs(t, err, errMock)
	})

	t.Run("success with login link", func(t *testing.T) {
		mockResponse := Result{RefreshToken: "test-token", AccessToken: "test-token", ExpiresIn: 300}
		orgChecker := func(domain string) (string, error) {
			return "test-org-id", nil
		}
		callbackHandler := func() (string, error) {
			return "test-code", nil
		}
		tokenRequester := func(authConfig astro.AuthConfig, verifier, code string) (Result, error) {
			return mockResponse, nil
		}
		mockAuthenticator := Authenticator{orgChecker, tokenRequester, callbackHandler}
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		resp, err := mockAuthenticator.authDeviceLogin(c, astro.AuthConfig{}, true)
		assert.NoError(t, err)
		assert.Equal(t, mockResponse, resp)
	})

	t.Run("callback failure with login link", func(t *testing.T) {
		orgChecker := func(domain string) (string, error) {
			return "test-org-id", nil
		}
		callbackHandler := func() (string, error) {
			return "", errMock
		}
		mockAuthenticator := Authenticator{orgChecker: orgChecker, callbackHandler: callbackHandler}
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		_, err = mockAuthenticator.authDeviceLogin(c, astro.AuthConfig{}, true)
		assert.ErrorIs(t, err, errMock)
	})

	t.Run("token requester failure with login link", func(t *testing.T) {
		orgChecker := func(domain string) (string, error) {
			return "test-org-id", nil
		}
		callbackHandler := func() (string, error) {
			return "test-code", nil
		}
		tokenRequester := func(authConfig astro.AuthConfig, verifier, code string) (Result, error) {
			return Result{}, errMock
		}
		mockAuthenticator := Authenticator{orgChecker, tokenRequester, callbackHandler}
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		_, err = mockAuthenticator.authDeviceLogin(c, astro.AuthConfig{}, true)
		assert.ErrorIs(t, err, errMock)
	})

	t.Run("do not call org lookup in identity first flow", func(t *testing.T) {
		mockResponse := Result{RefreshToken: "test-token", AccessToken: "test-token", ExpiresIn: 300}
		orgChecker := func(domain string) (string, error) {
			return "", errMock
		}
		callbackHandler := func() (string, error) {
			return "test-code", nil
		}
		tokenRequester := func(authConfig astro.AuthConfig, verifier, code string) (Result, error) {
			return mockResponse, nil
		}
		openURL = func(url string) error {
			return nil
		}
		mockAuthenticator := Authenticator{orgChecker, tokenRequester, callbackHandler}
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		resp, err := mockAuthenticator.authDeviceLogin(c, astro.AuthConfig{}, false)
		assert.NoError(t, err)
		assert.Equal(t, mockResponse, resp)
	})
}

func TestSwitchToLastUsedWorkspace(t *testing.T) {
	t.Run("failure case", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		ctx := &config.Context{
			LastUsedWorkspace: "test-id",
		}
		resp, found, err := switchToLastUsedWorkspace(ctx, []astro.Workspace{{ID: "test-id"}})
		assert.ErrorIs(t, err, config.ErrCtxConfigErr)
		assert.False(t, found)
		assert.Equal(t, astro.Workspace{}, resp)
	})

	testUtil.InitTestConfig(testUtil.CloudPlatform)
	t.Run("success", func(t *testing.T) {
		ctx := &config.Context{
			LastUsedWorkspace: "test-id",
			Domain:            "test-domain",
		}
		resp, found, err := switchToLastUsedWorkspace(ctx, []astro.Workspace{{ID: "test-id"}})
		assert.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, ctx.LastUsedWorkspace, resp.ID)
	})

	t.Run("failure, workspace not found", func(t *testing.T) {
		ctx := &config.Context{
			LastUsedWorkspace: "test-invalid-id",
		}
		resp, found, err := switchToLastUsedWorkspace(ctx, []astro.Workspace{{ID: "test-id"}})
		assert.NoError(t, err)
		assert.False(t, found)
		assert.Equal(t, astro.Workspace{}, resp)
	})
}

func TestCheckUserSession(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	t.Run("success", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: "test-id"}}, nil).Once()
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything).Return(&mockOrganizationsResponse, nil).Once()
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockClient, mockCoreClient, buf)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})

	t.Run("no organization found", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything).Return(&mockOrganizationsResponseEmpty, nil).Once()
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockClient, mockCoreClient, buf)
		assert.Contains(t, err.Error(), "Please contact your Astro Organization Owner to be invited to the organization")
	})

	t.Run("list organization network error", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything).Return(nil, errNetwork).Once()
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockClient, mockCoreClient, buf)
		assert.Contains(t, err.Error(), "network error")
	})

	t.Run("self user network error", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(nil, errNetwork).Once()
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockClient, mockCoreClient, buf)
		assert.Contains(t, err.Error(), "network error")
	})

	t.Run("self user failure", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfErrorResponse, nil).Once()
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockClient, mockCoreClient, buf)
		assert.Contains(t, err.Error(), "failed to fetch self user")
	})

	t.Run("set context failure", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything).Return(&mockOrganizationsResponse, nil).Once()
		ctx := config.Context{}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockClient, mockCoreClient, buf)
		assert.ErrorIs(t, err, config.ErrCtxConfigErr)
		mockClient.AssertExpectations(t)
	})

	t.Run("list workspace failure", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{}, errMock).Once()
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything).Return(&mockOrganizationsResponse, nil).Once()
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockClient, mockCoreClient, buf)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})

	t.Run("success with more than one workspace", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: "test-id-1"}, {ID: "test-id-2"}}, nil).Once()
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything).Return(&mockOrganizationsResponse, nil).Once()

		ctx := config.Context{Domain: "test-domain", LastUsedWorkspace: "test-id-1", Organization: "test-org-id"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockClient, mockCoreClient, buf)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("success with workspace switch", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything).Return(&mockOrganizationsResponse, nil).Once()

		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: "test-id-1"}, {ID: "test-id-2"}}, nil)
		ctx := config.Context{Domain: "test-domain", Organization: "test-org-id"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockClient, mockCoreClient, buf)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("success but with workspace switch failure", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything).Return(&mockOrganizationsResponse, nil).Once()
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: "test-id-1"}, {ID: "test-id-2"}}, nil).Once()
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{}, errMock).Once()
		ctx := config.Context{Domain: "test-domain", Organization: "test-org-id"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockClient, mockCoreClient, buf)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("success with identity first auth flow", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListWorkspaces", "test-org-id-2").Return([]astro.Workspace{{ID: "test-id"}}, nil).Once()
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything).Return(&mockOrganizationsResponse, nil).Once()
		// context org  "test-org-id-2" takes precedence over the getSelf org "test-org-id"
		ctx := config.Context{Domain: "test-domain", Organization: "test-org-id-2"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockClient, mockCoreClient, buf)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
}

func TestLogin(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	t.Run("success", func(t *testing.T) {
		mockResponse := Result{RefreshToken: "test-token", AccessToken: "test-token", ExpiresIn: 300}
		orgChecker := func(domain string) (string, error) {
			return "test-org-id", nil
		}
		callbackHandler := func() (string, error) {
			return "test-code", nil
		}
		tokenRequester := func(authConfig astro.AuthConfig, verifier, code string) (Result, error) {
			return mockResponse, nil
		}
		openURL = func(url string) error {
			return nil
		}
		authenticator = Authenticator{orgChecker, tokenRequester, callbackHandler}

		mockClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything).Return(&mockOrganizationsResponse, nil).Once()
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: "test-id"}}, nil).Once()
		err := Login("astronomer.io", "", mockClient, mockCoreClient, os.Stdout, false)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("can login to a pr preview environment successfully", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPrPreview)
		// mocking this as once a PR closes, test would fail
		mockAuthConfigResponse := astro.AuthConfig{
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
		orgChecker := func(domain string) (string, error) {
			return "test-org-id", nil
		}
		callbackHandler := func() (string, error) {
			return "test-code", nil
		}
		tokenRequester := func(authConfig astro.AuthConfig, verifier, code string) (Result, error) {
			return mockResponse, nil
		}
		openURL = func(url string) error {
			return nil
		}
		authenticator = Authenticator{orgChecker, tokenRequester, callbackHandler}

		mockClient := new(astro_mocks.Client)
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: "test-id"}}, nil).Once()
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything).Return(&mockOrganizationsResponse, nil).Once()

		err = Login("pr5723.cloud.astronomer-dev.io", "", mockClient, mockCoreClient, os.Stdout, false)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("oauth token success", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: "test-id"}}, nil).Once()
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything).Return(&mockOrganizationsResponse, nil).Once()

		err := Login("astronomer.io", "OAuth Token", mockClient, mockCoreClient, os.Stdout, false)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("invalid domain", func(t *testing.T) {
		err := Login("fail.astronomer.io", "", nil, nil, os.Stdout, false)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Invalid domain.")
	})

	t.Run("auth failure", func(t *testing.T) {
		orgChecker := func(domain string) (string, error) {
			return "test-org-id", nil
		}
		callbackHandler := func() (string, error) {
			return "", errMock
		}
		authenticator = Authenticator{orgChecker: orgChecker, callbackHandler: callbackHandler}
		err := Login("cloud.astronomer.io", "", nil, nil, os.Stdout, false)
		assert.ErrorIs(t, err, errMock)
	})

	t.Run("check token failure", func(t *testing.T) {
		mockResponse := Result{RefreshToken: "test-token", AccessToken: "test-token", ExpiresIn: 300}
		orgChecker := func(domain string) (string, error) {
			return "test-org-id", nil
		}
		callbackHandler := func() (string, error) {
			return "test-code", nil
		}
		tokenRequester := func(authConfig astro.AuthConfig, verifier, code string) (Result, error) {
			return mockResponse, nil
		}
		openURL = func(url string) error {
			return nil
		}
		authenticator = Authenticator{orgChecker, tokenRequester, callbackHandler}

		mockClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfErrorResponse, nil).Once()
		err := Login("", "", mockClient, mockCoreClient, os.Stdout, false)
		assert.Contains(t, err.Error(), "failed to fetch self user")
		mockClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("initial login with empty config file", func(t *testing.T) {
		// initialize empty config
		testUtil.InitTestConfig(testUtil.Initial)
		// initialize the mock client
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{
			ID:    "test-workspace",
			Label: "something-label",
		}}, nil).Once()
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything).Return(&mockOrganizationsResponse, nil).Once()
		// initialize the test authenticator
		orgChecker := func(domain string) (string, error) {
			return "test-org-id", nil
		}
		authenticator = Authenticator{
			orgChecker:      orgChecker,
			callbackHandler: func() (string, error) { return "authorizationCode", nil },
			tokenRequester: func(authConfig astro.AuthConfig, verifier, code string) (Result, error) {
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
		err = Login("astronomer.io", "", mockClient, mockCoreClient, os.Stdout, true)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("domain doesn't match current context", func(t *testing.T) {
		// initialize empty config
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		// initialize the mock client
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{
			ID:    "test-workspace",
			Label: "something-label",
		}}, nil).Once()
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything).Return(&mockOrganizationsResponse, nil).Once()
		// initialize the test authenticator
		orgChecker := func(domain string) (string, error) {
			return "test-org-id", nil
		}
		authenticator = Authenticator{
			orgChecker:      orgChecker,
			callbackHandler: func() (string, error) { return "authorizationCode", nil },
			tokenRequester: func(authConfig astro.AuthConfig, verifier, code string) (Result, error) {
				return Result{
					RefreshToken: "refresh_token",
					AccessToken:  "access_token",
					ExpiresIn:    1234,
				}, nil
			},
		}
		// initialize user input with email
		defer testUtil.MockUserInput(t, "test.user@astronomer.io")()
		err := Login("astronomer.io", "", mockClient, mockCoreClient, os.Stdout, true)
		assert.NoError(t, err)
		// assert that everything got set in the right spot
		domainContext, err := context.GetContext("astronomer.io")
		assert.NoError(t, err)
		currentContext, err := context.GetContext("localhost")
		assert.NoError(t, err)
		assert.Equal(t, domainContext.Token, "Bearer access_token")
		assert.Equal(t, currentContext.Token, "token")
		mockClient.AssertExpectations(t)
		mockCoreClient.AssertExpectations(t)
	})
}

func TestLogout(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
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

func Test_getUserEmail_FromConfig(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	c, err := config.GetCurrentContext()
	assert.NoError(t, err)
	err = c.SetContextKey("user_email", "test.user@astronomer.io")
	assert.NoError(t, err)
	c, err = config.GetCurrentContext()
	assert.NoError(t, err)
	userEmail, err := getUserEmail(c)
	assert.NoError(t, err)
	assert.Equal(t, "test.user@astronomer.io", userEmail)
}

func testGetsInputHelper(t *testing.T, c config.Context) { //nolint:gocritic
	defer testUtil.MockUserInput(t, "test.user@astronomer.io")()
	userEmail, err := getUserEmail(c)
	assert.NoError(t, err)
	// print a new line so that goland recognizes the test
	fmt.Println("")
	assert.Equal(t, "test.user@astronomer.io", userEmail)
}

func Test_getUserEmail_FromStdin(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	c, err := config.GetCurrentContext()
	assert.NoError(t, err)
	err = c.SetContextKey("user_email", "")
	assert.NoError(t, err)
	// TODO: remove set user email and get user email
	testGetsInputHelper(t, c)
}

func Test_getUserEmail_OldStyleConfig(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	c, err := config.GetCurrentContext()
	assert.NoError(t, err)
	testGetsInputHelper(t, c)
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
