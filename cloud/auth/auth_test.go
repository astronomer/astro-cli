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

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/context"

	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/astronomer/astro-cli/astro-client"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

var (
	errMock = errors.New("mock-error")

	mockOrganizationID      = "test-org-id"
	mockOrganizationProduct = astrocore.OrganizationProductHYBRID
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
	mockOrganizationsResponse = astrocore.ListOrganizationsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &[]astrocore.Organization{
			{AuthServiceId: "auth-service-id", Id: "test-org-id", Name: "test-org", Product: &mockOrganizationProduct},
			{AuthServiceId: "auth-service-id", Id: "test-org-id-2", Name: "test-org-2", Product: &mockOrganizationProduct},
			{AuthServiceId: "auth-service-id", Id: "org1", Name: "org1", Product: &mockOrganizationProduct},
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

type Suite struct {
	suite.Suite
}

func TestCloudAuthSuite(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) Test_FetchDomainAuthConfig() {
	domain := "astronomer.io"
	actual, err := FetchDomainAuthConfig(domain)
	s.NoError(err)
	s.Equal(actual.ClientID, "5XYJZYf5xZ0eKALgBH3O08WzgfUfz7y9")
	s.Equal(actual.Audience, "astronomer-ee")
	s.Equal(actual.DomainURL, "https://auth.astronomer.io/")

	domain = "gcp0001.us-east4.astronomer.io" // Gen1 CLI domain
	actual, err = FetchDomainAuthConfig(domain)
	s.Error(err)
	s.Errorf(err, "Error! Invalid domain. "+
		"Are you trying to authenticate to Astronomer Software? If so, change your current context with 'astro context switch'. ")

	domain = "fail.astronomer.io"
	actual, err = FetchDomainAuthConfig(domain)
	s.Error(err)
	s.Errorf(err, "Error! Invalid domain. "+
		"Are you trying to authenticate to Astronomer Software? If so, change your current context with 'astro context switch'. ")

	domain = "astronomer-dev.io"
	actual, err = FetchDomainAuthConfig(domain)
	s.NoError(err)
	s.Equal(actual.ClientID, "PH3Nac2DtpSx1Tx3IGQmh2zaRbF5ubZG")
	s.Equal(actual.Audience, "astronomer-ee")
	s.Equal(actual.DomainURL, "https://auth.astronomer-dev.io/")

	domain = "fail.astronomer-dev.io"
	actual, err = FetchDomainAuthConfig(domain)
	s.Error(err)
	s.Errorf(err, "Error! Invalid domain. "+
		"Are you trying to authenticate to Astronomer Software? If so, change your current context with 'astro context switch'. ")

	domain = "astronomer-stage.io"
	actual, err = FetchDomainAuthConfig(domain)
	s.NoError(err)
	s.Equal(actual.ClientID, "jsarDat3BeDXZ1monEAeqJPOvRvterpm")
	s.Equal(actual.Audience, "astronomer-ee")
	s.Equal(actual.DomainURL, "https://auth.astronomer-stage.io/")

	domain = "fail.astronomer-stage.io"
	actual, err = FetchDomainAuthConfig(domain)
	s.Error(err)
	s.Errorf(err, "Error! Invalid domain. "+
		"Are you trying to authenticate to Astronomer Software? If so, change your current context with 'astro context switch'. ")

	domain = "fail.astronomer-perf.io"
	actual, err = FetchDomainAuthConfig(domain)
	s.Error(err)
	s.Errorf(err, "Error! Invalid domain. "+
		"Are you trying to authenticate to Astronomer Software? If so, change your current context with 'astro context switch'. ")

	s.Run("pr preview is a valid domain", func() {
		// mocking this as once a PR closes, test would fail
		mockResponse := astro.AuthConfig{
			ClientID:  "client-id",
			Audience:  "audience",
			DomainURL: "https://myURL.com/",
		}
		jsonResponse, err := json.Marshal(mockResponse)
		s.NoError(err)
		httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		domain = "pr1234.astronomer-dev.io"
		actual, err = FetchDomainAuthConfig(domain)
		s.NoError(err)
		s.Equal(actual.ClientID, mockResponse.ClientID)
		s.Equal(actual.Audience, mockResponse.Audience)
		s.Equal(actual.DomainURL, mockResponse.DomainURL)
	})
}

func (s *Suite) TestRequestUserInfo() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockUserInfo := UserInfo{
		Email:      "test@astronomer.test",
		Name:       "astro.crush",
		FamilyName: "Crush",
		GivenName:  "Astro",
	}
	emptyUserInfo := UserInfo{}
	mockAccessToken := "access-token"
	userInfoResponse, err := json.Marshal(mockUserInfo)
	s.NoError(err)
	emptyResponse, err := json.Marshal(emptyUserInfo)
	s.NoError(err)

	s.Run("success", func() {
		httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(userInfoResponse)),
				Header:     make(http.Header),
			}
		})
		resp, err := requestUserInfo(astro.AuthConfig{}, mockAccessToken)
		s.NoError(err)
		s.Equal(mockUserInfo, resp)
	})

	s.Run("failure", func() {
		httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})

		_, err := requestUserInfo(astro.AuthConfig{}, mockAccessToken)
		s.Contains(err.Error(), "Internal Server Error")
	})

	s.Run("fail with no email", func() {
		s.NoError(err)
		httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(emptyResponse)),
				Header:     make(http.Header),
			}
		})
		_, err := requestUserInfo(astro.AuthConfig{}, mockAccessToken)
		s.Contains(err.Error(), "cannot retrieve email")
	})
}

func (s *Suite) TestRequestToken() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockResponse := postTokenResponse{
		RefreshToken: "test-refresh-token",
		AccessToken:  "test-access-token",
		ExpiresIn:    300,
	}
	jsonResponse, err := json.Marshal(mockResponse)
	s.NoError(err)

	s.Run("success", func() {
		httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})

		resp, err := requestToken(astro.AuthConfig{}, "", "")
		s.NoError(err)
		s.Equal(Result{RefreshToken: mockResponse.RefreshToken, AccessToken: mockResponse.AccessToken, ExpiresIn: mockResponse.ExpiresIn}, resp)
	})

	s.Run("failure", func() {
		httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})

		_, err := requestToken(astro.AuthConfig{}, "", "")
		s.Contains(err.Error(), "Internal Server Error")
	})

	errMock := "test-error"
	mockResponse = postTokenResponse{
		ErrorDescription: errMock,
		Error:            &errMock,
	}
	jsonResponse, err = json.Marshal(mockResponse)
	s.NoError(err)

	s.Run("token error", func() {
		httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})

		_, err := requestToken(astro.AuthConfig{}, "", "")
		s.Contains(err.Error(), mockResponse.ErrorDescription)
	})
}

func (s *Suite) TestAuthorizeCallbackHandler() {
	httpClient = httputil.NewHTTPClient()
	s.Run("success", func() {
		callbackServer = "localhost:12345"
		go func() {
			time.Sleep(2 * time.Second) // time to spinup the server in authorizeCallbackHandler

			opts := &httputil.DoOptions{
				Method: http.MethodGet,
				Path:   "http://localhost:12345/callback?code=test",
			}
			_, err = httpClient.Do(opts) //nolint
			s.NoError(err)
		}()
		code, err := authorizeCallbackHandler()
		s.Equal("test", code)
		s.NoError(err)
	})

	s.Run("error", func() {
		callbackServer = "localhost:12346"
		go func() {
			time.Sleep(2 * time.Second) // time to spinup the server in authorizeCallbackHandler
			opts := &httputil.DoOptions{
				Method: http.MethodGet,
				Path:   "http://localhost:12346/callback?error=error&error_description=fatal_error",
			}
			_, err = httpClient.Do(opts) //nolint
			s.NoError(err)
		}()
		_, err := authorizeCallbackHandler()
		s.Contains(err.Error(), "fatal_error")
	})

	s.Run("timeout", func() {
		callbackServer = "localhost:12347"
		callbackTimeout = 5 * time.Millisecond
		_, err := authorizeCallbackHandler()
		s.Contains(err.Error(), "the operation has timed out")
	})
}

func (s *Suite) TestAuthDeviceLogin() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	s.Run("success without login link", func() {
		mockResponse := Result{RefreshToken: "test-token", AccessToken: "test-token", ExpiresIn: 300}
		callbackHandler := func() (string, error) {
			return "test-code", nil
		}
		tokenRequester := func(authConfig astro.AuthConfig, verifier, code string) (Result, error) {
			return mockResponse, nil
		}
		openURL = func(url string) error {
			return nil
		}
		mockAuthenticator := Authenticator{tokenRequester: tokenRequester, callbackHandler: callbackHandler}
		resp, err := mockAuthenticator.authDeviceLogin(astro.AuthConfig{}, false)
		s.NoError(err)
		s.Equal(mockResponse, resp)
	})

	s.Run("openURL & callback failure", func() {
		openURL = func(url string) error {
			return errMock
		}
		callbackHandler := func() (string, error) {
			return "", errMock
		}
		mockAuthenticator := Authenticator{callbackHandler: callbackHandler}
		_, err = mockAuthenticator.authDeviceLogin(astro.AuthConfig{}, false)
		s.ErrorIs(err, errMock)
	})

	s.Run("token requester failure", func() {
		callbackHandler := func() (string, error) {
			return "test-code", nil
		}
		tokenRequester := func(authConfig astro.AuthConfig, verifier, code string) (Result, error) {
			return Result{}, errMock
		}
		openURL = func(url string) error {
			return nil
		}
		mockAuthenticator := Authenticator{tokenRequester: tokenRequester, callbackHandler: callbackHandler}
		_, err = mockAuthenticator.authDeviceLogin(astro.AuthConfig{}, false)
		s.ErrorIs(err, errMock)
	})

	s.Run("success with login link", func() {
		mockResponse := Result{RefreshToken: "test-token", AccessToken: "test-token", ExpiresIn: 300}
		callbackHandler := func() (string, error) {
			return "test-code", nil
		}
		tokenRequester := func(authConfig astro.AuthConfig, verifier, code string) (Result, error) {
			return mockResponse, nil
		}
		mockAuthenticator := Authenticator{tokenRequester: tokenRequester, callbackHandler: callbackHandler}
		resp, err := mockAuthenticator.authDeviceLogin(astro.AuthConfig{}, true)
		s.NoError(err)
		s.Equal(mockResponse, resp)
	})

	s.Run("callback failure with login link", func() {
		callbackHandler := func() (string, error) {
			return "", errMock
		}
		mockAuthenticator := Authenticator{callbackHandler: callbackHandler}
		_, err = mockAuthenticator.authDeviceLogin(astro.AuthConfig{}, true)
		s.ErrorIs(err, errMock)
	})

	s.Run("token requester failure with login link", func() {
		callbackHandler := func() (string, error) {
			return "test-code", nil
		}
		tokenRequester := func(authConfig astro.AuthConfig, verifier, code string) (Result, error) {
			return Result{}, errMock
		}
		mockAuthenticator := Authenticator{tokenRequester: tokenRequester, callbackHandler: callbackHandler}
		_, err = mockAuthenticator.authDeviceLogin(astro.AuthConfig{}, true)
		s.ErrorIs(err, errMock)
	})
}

func (s *Suite) TestSwitchToLastUsedWorkspace() {
	s.Run("failure case", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		ctx := &config.Context{
			LastUsedWorkspace: "test-id",
		}
		resp, found, err := switchToLastUsedWorkspace(ctx, []astro.Workspace{{ID: "test-id"}})
		s.ErrorIs(err, config.ErrCtxConfigErr)
		s.False(found)
		s.Equal(astro.Workspace{}, resp)
	})

	testUtil.InitTestConfig(testUtil.CloudPlatform)
	s.Run("success", func() {
		ctx := &config.Context{
			LastUsedWorkspace: "test-id",
			Domain:            "test-domain",
		}
		resp, found, err := switchToLastUsedWorkspace(ctx, []astro.Workspace{{ID: "test-id"}})
		s.NoError(err)
		s.True(found)
		s.Equal(ctx.LastUsedWorkspace, resp.ID)
	})

	s.Run("failure, workspace not found", func() {
		ctx := &config.Context{
			LastUsedWorkspace: "test-invalid-id",
		}
		resp, found, err := switchToLastUsedWorkspace(ctx, []astro.Workspace{{ID: "test-id"}})
		s.NoError(err)
		s.False(found)
		s.Equal(astro.Workspace{}, resp)
	})
}

func (s *Suite) TestCheckUserSession() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	s.Run("success", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: "test-id"}}, nil).Once()
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOrganizationsResponse, nil).Once()
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockClient, mockCoreClient, buf)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("no organization found", func() {
		mockClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOrganizationsResponseEmpty, nil).Once()
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockClient, mockCoreClient, buf)
		s.Contains(err.Error(), "Please contact your Astro Organization Owner to be invited to the organization")
	})

	s.Run("list organization network error", func() {
		mockClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(nil, errNetwork).Once()
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockClient, mockCoreClient, buf)
		s.Contains(err.Error(), "network error")
	})

	s.Run("self user network error", func() {
		mockClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(nil, errNetwork).Once()
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockClient, mockCoreClient, buf)
		s.Contains(err.Error(), "network error")
	})

	s.Run("self user failure", func() {
		mockClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfErrorResponse, nil).Once()
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockClient, mockCoreClient, buf)
		s.Contains(err.Error(), "failed to fetch self user")
	})

	s.Run("set context failure", func() {
		mockClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOrganizationsResponse, nil).Once()
		ctx := config.Context{}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockClient, mockCoreClient, buf)
		s.ErrorIs(err, config.ErrCtxConfigErr)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("list workspace failure", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{}, errMock).Once()
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOrganizationsResponse, nil).Once()
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockClient, mockCoreClient, buf)
		s.ErrorIs(err, errMock)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("success with more than one workspace", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: "test-id-1"}, {ID: "test-id-2"}}, nil).Once()
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOrganizationsResponse, nil).Once()

		ctx := config.Context{Domain: "test-domain", LastUsedWorkspace: "test-id-1", Organization: "test-org-id"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockClient, mockCoreClient, buf)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
		mockCoreClient.AssertExpectations(s.T())
	})

	s.Run("success with workspace switch", func() {
		mockClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOrganizationsResponse, nil).Once()

		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: "test-id-1"}, {ID: "test-id-2"}}, nil)
		ctx := config.Context{Domain: "test-domain", Organization: "test-org-id"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockClient, mockCoreClient, buf)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
		mockCoreClient.AssertExpectations(s.T())
	})

	s.Run("success but with workspace switch failure", func() {
		mockClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOrganizationsResponse, nil).Once()
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: "test-id-1"}, {ID: "test-id-2"}}, nil).Once()
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{}, errMock).Once()
		ctx := config.Context{Domain: "test-domain", Organization: "test-org-id"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockClient, mockCoreClient, buf)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
		mockCoreClient.AssertExpectations(s.T())
	})

	s.Run("success with identity first auth flow", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListWorkspaces", "test-org-id-2").Return([]astro.Workspace{{ID: "test-id"}}, nil).Once()
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOrganizationsResponse, nil).Once()
		// context org  "test-org-id-2" takes precedence over the getSelf org "test-org-id"
		ctx := config.Context{Domain: "test-domain", Organization: "test-org-id-2"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockClient, mockCoreClient, buf)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
		mockCoreClient.AssertExpectations(s.T())
	})

	s.Run("set default org product", func() {
		mockOrganizationsResponse = astrocore.ListOrganizationsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &[]astrocore.Organization{
				{AuthServiceId: "auth-service-id", Id: "test-org-id", Name: "test-org"},
			},
		}
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: "test-id"}}, nil).Once()
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOrganizationsResponse, nil).Once()
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockClient, mockCoreClient, buf)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestLogin() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	s.Run("success", func() {
		mockResponse := Result{RefreshToken: "test-token", AccessToken: "test-token", ExpiresIn: 300}
		mockUserInfo := UserInfo{Email: "test@astronomer.test"}
		callbackHandler := func() (string, error) {
			return "test-code", nil
		}
		tokenRequester := func(authConfig astro.AuthConfig, verifier, code string) (Result, error) {
			return mockResponse, nil
		}
		userInfoRequester := func(authConfig astro.AuthConfig, accessToken string) (UserInfo, error) {
			return mockUserInfo, nil
		}
		openURL = func(url string) error {
			return nil
		}
		authenticator = Authenticator{userInfoRequester, tokenRequester, callbackHandler}

		mockClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOrganizationsResponse, nil).Once()
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: "test-id"}}, nil).Once()
		err := Login("astronomer.io", "", mockClient, mockCoreClient, os.Stdout, false)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("can login to a pr preview environment successfully", func() {
		testUtil.InitTestConfig(testUtil.CloudPrPreview)
		// mocking this as once a PR closes, test would fail
		mockAuthConfigResponse := astro.AuthConfig{
			ClientID:  "client-id",
			Audience:  "audience",
			DomainURL: "https://myURL.com/",
		}
		jsonResponse, err := json.Marshal(mockAuthConfigResponse)
		s.NoError(err)
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
		tokenRequester := func(authConfig astro.AuthConfig, verifier, code string) (Result, error) {
			return mockResponse, nil
		}
		userInfoRequester := func(authConfig astro.AuthConfig, accessToken string) (UserInfo, error) {
			return mockUserInfo, nil
		}
		openURL = func(url string) error {
			return nil
		}
		authenticator = Authenticator{userInfoRequester, tokenRequester, callbackHandler}
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: "test-id"}}, nil).Once()
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOrganizationsResponse, nil).Once()

		err = Login("pr5723.cloud.astronomer-dev.io", "", mockClient, mockCoreClient, os.Stdout, false)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
		mockCoreClient.AssertExpectations(s.T())
	})

	s.Run("oauth token success", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: "test-id"}}, nil).Once()
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOrganizationsResponse, nil).Once()

		err := Login("astronomer.io", "OAuth Token", mockClient, mockCoreClient, os.Stdout, false)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
		mockCoreClient.AssertExpectations(s.T())
	})

	s.Run("invalid domain", func() {
		err := Login("fail.astronomer.io", "", nil, nil, os.Stdout, false)
		s.Error(err)
		s.Contains(err.Error(), "Invalid domain.")
	})

	s.Run("auth failure", func() {
		callbackHandler := func() (string, error) {
			return "", errMock
		}
		authenticator = Authenticator{callbackHandler: callbackHandler}
		err := Login("cloud.astronomer.io", "", nil, nil, os.Stdout, false)
		s.ErrorIs(err, errMock)
	})

	s.Run("check token failure", func() {
		mockResponse := Result{RefreshToken: "test-token", AccessToken: "test-token", ExpiresIn: 300}
		mockUserInfo := UserInfo{Email: "test@astronomer.test"}
		callbackHandler := func() (string, error) {
			return "test-code", nil
		}
		tokenRequester := func(authConfig astro.AuthConfig, verifier, code string) (Result, error) {
			return mockResponse, nil
		}
		userInfoRequester := func(authConfig astro.AuthConfig, accessToken string) (UserInfo, error) {
			return mockUserInfo, nil
		}
		openURL = func(url string) error {
			return nil
		}
		authenticator = Authenticator{userInfoRequester, tokenRequester, callbackHandler}

		mockClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfErrorResponse, nil).Once()
		err := Login("", "", mockClient, mockCoreClient, os.Stdout, false)
		s.Contains(err.Error(), "failed to fetch self user")
		mockClient.AssertExpectations(s.T())
		mockCoreClient.AssertExpectations(s.T())
	})

	s.Run("initial login with empty config file", func() {
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
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOrganizationsResponse, nil).Once()
		// initialize the test authenticator
		mockUserInfo := UserInfo{Email: "test@astronomer.test"}
		userInfoRequester := func(authConfig astro.AuthConfig, accessToken string) (UserInfo, error) {
			return mockUserInfo, nil
		}
		authenticator = Authenticator{
			userInfoRequester: userInfoRequester,
			callbackHandler:   func() (string, error) { return "authorizationCode", nil },
			tokenRequester: func(authConfig astro.AuthConfig, verifier, code string) (Result, error) {
				return Result{
					RefreshToken: "refresh_token",
					AccessToken:  "access_token",
					ExpiresIn:    1234,
				}, nil
			},
		}
		// initialize stdin with user email input
		defer testUtil.MockUserInput(s.T(), "test.user@astronomer.io")()
		// do the test
		err = Login("astronomer.io", "", mockClient, mockCoreClient, os.Stdout, true)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
		mockCoreClient.AssertExpectations(s.T())
	})

	s.Run("domain doesn't match current context", func() {
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
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOrganizationsResponse, nil).Once()
		// initialize the test authenticator
		mockUserInfo := UserInfo{Email: "test@astronomer.test"}
		userInfoRequester := func(authConfig astro.AuthConfig, accessToken string) (UserInfo, error) {
			return mockUserInfo, nil
		}
		authenticator = Authenticator{
			userInfoRequester: userInfoRequester,
			callbackHandler:   func() (string, error) { return "authorizationCode", nil },
			tokenRequester: func(authConfig astro.AuthConfig, verifier, code string) (Result, error) {
				return Result{
					RefreshToken: "refresh_token",
					AccessToken:  "access_token",
					ExpiresIn:    1234,
				}, nil
			},
		}
		// initialize user input with email
		defer testUtil.MockUserInput(s.T(), "test.user@astronomer.io")()
		err := Login("astronomer.io", "", mockClient, mockCoreClient, os.Stdout, true)
		s.NoError(err)
		// assert that everything got set in the right spot
		domainContext, err := context.GetContext("astronomer.io")
		s.NoError(err)
		currentContext, err := context.GetContext("localhost")
		s.NoError(err)
		s.Equal(domainContext.Token, "Bearer access_token")
		s.Equal(currentContext.Token, "token")
		mockClient.AssertExpectations(s.T())
		mockCoreClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestLogout() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	s.Run("success", func() {
		buf := new(bytes.Buffer)
		Logout("astronomer.io", buf)
		s.Equal("Successfully logged out of Astronomer\n", buf.String())
	})

	s.Run("success_with_email", func() {
		assertions := func(expUserEmail string, expToken string) {
			contexts, err := config.GetContexts()
			s.NoError(err)
			context := contexts.Contexts["localhost"]

			s.NoError(err)
			s.Equal(expUserEmail, context.UserEmail)
			s.Equal(expToken, context.Token)
		}
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		c, err := config.GetCurrentContext()
		s.NoError(err)
		err = c.SetContextKey("user_email", "test.user@astronomer.io")
		s.NoError(err)
		err = c.SetContextKey("token", "Bearer some-token")
		s.NoError(err)
		// test before
		assertions("test.user@astronomer.io", "Bearer some-token")

		// log out
		c, err = config.GetCurrentContext()
		s.NoError(err)
		Logout(c.Domain, os.Stdout)

		// test after logout
		assertions("", "")
	})
}

func (s *Suite) Test_writeResultToContext() {
	assertConfigContents := func(expToken string, expRefresh string, expExpires time.Time, expUserEmail string) {
		context, err := config.GetCurrentContext()
		s.NoError(err)
		// test the output on the config file
		s.Equal(expToken, context.Token)
		s.Equal(expRefresh, context.RefreshToken)
		expiresIn, err := context.GetExpiresIn()
		s.NoError(err)
		s.Equal(expExpires.Round(time.Second), expiresIn.Round(time.Second))
		s.Equal(expUserEmail, context.UserEmail)
		s.NoError(err)
	}
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	c, err := config.GetCurrentContext()
	s.NoError(err)
	err = c.SetContextKey("token", "old_token")
	s.NoError(err)
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
	s.NoError(err)
	err = res.writeToContext(&c)
	s.NoError(err)

	// test after changes
	assertConfigContents("Bearer new_token", "new_refresh_token", time.Now().Add(1234*time.Second), "test.user@astronomer.io")
}
