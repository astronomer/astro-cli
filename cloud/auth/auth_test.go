package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/astro-client-v1"
	astrov1_mocks "github.com/astronomer/astro-cli/astro-client-v1/mocks"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/httputil"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

var (
	errMock = errors.New("mock-error")

	mockOrganizationID      = "test-org-id"
	mockOrganizationProduct = astrov1.OrganizationProductHYBRID
	mockGetSelfResponse     = astrov1.GetSelfUserResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.SelfUser{
			OrganizationId: &mockOrganizationID,
			Username:       "test@astronomer.io",
			FullName:       "jane",
			Id:             "user-id",
		},
	}
	mockGetSelfResponseNoOrg = astrov1.GetSelfUserResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.SelfUser{
			Username: "test@astronomer.io",
			FullName: "jane",
			Id:       "user-id",
		},
	}
	mockGetSelfErrorBody, _ = json.Marshal(astrov1.Error{
		Message: "failed to fetch self user",
	})
	mockGetSelfErrorResponse = astrov1.GetSelfUserResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    mockGetSelfErrorBody,
		JSON200: nil,
	}
	mockOrganizationsResponse = astrov1.ListOrganizationsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.OrganizationsPaginated{
			Organizations: []astrov1.Organization{
				{Id: "org1", Name: "org1", Product: &mockOrganizationProduct},
				{Id: "org2", Name: "org2", Product: &mockOrganizationProduct},
			},
		},
	}
	mockOrganizationsResponseEmpty = astrov1.ListOrganizationsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.OrganizationsPaginated{
			Organizations: []astrov1.Organization{},
		},
	}
	mockGetOrganizationResponse = astrov1.GetOrganizationResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.Organization{
			Id: "test-org-id", Name: "test-org", Product: &mockOrganizationProduct,
		},
	}
	mockGetOrganizationResponse2 = astrov1.GetOrganizationResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.Organization{
			Id: "test-org-id-2", Name: "test-org-2", Product: &mockOrganizationProduct,
		},
	}
	errNetwork  = errors.New("network error")
	description = "test workspace"
	workspace1  = astrov1.Workspace{
		Name:        "test-workspace",
		Description: &description,
		Id:          "workspace-id",
	}

	workspaces = []astrov1.Workspace{
		workspace1,
	}

	ListWorkspacesResponseOK = astrov1.ListWorkspacesResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.WorkspacesPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
			Workspaces: workspaces,
		},
	}
)

func Test_FetchDomainAuthConfig(t *testing.T) {
	// Stub the package http client so the test builds the right URL per
	// domain and parses the response without hitting live environments.
	originalHTTPClient := httpClient
	t.Cleanup(func() { httpClient = originalHTTPClient })

	responses := map[string]Config{
		"https://api.astronomer.io/private/v1alpha1/cli/auth-config": {
			ClientID:  "5XYJZYf5xZ0eKALgBH3O08WzgfUfz7y9",
			Audience:  "astronomer-ee",
			DomainURL: "https://auth.astronomer.io/",
		},
		"https://api.astronomer-dev.io/private/v1alpha1/cli/auth-config": {
			ClientID:  "PH3Nac2DtpSx1Tx3IGQmh2zaRbF5ubZG",
			Audience:  "astronomer-ee",
			DomainURL: "https://auth.astronomer-dev.io/",
		},
		"https://api.astronomer-stage.io/private/v1alpha1/cli/auth-config": {
			ClientID:  "jsarDat3BeDXZ1monEAeqJPOvRvterpm",
			Audience:  "astronomer-ee",
			DomainURL: "https://auth.astronomer-stage.io/",
		},
		"https://pr1234.api.astronomer-dev.io/private/v1alpha1/cli/auth-config": {
			ClientID:  "client-id",
			Audience:  "audience",
			DomainURL: "https://myURL.com/",
		},
	}

	httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
		assert.Equal(t, "cli", req.Header.Get("X-Astro-Client-Identifier"))
		cfg, ok := responses[req.URL.String()]
		if !ok {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(bytes.NewBufferString("unexpected URL: " + req.URL.String())),
				Header:     make(http.Header),
			}
		}
		body, _ := json.Marshal(cfg)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBuffer(body)),
			Header:     make(http.Header),
		}
	})

	domain := "astronomer.io"
	actual, err := FetchDomainAuthConfig(domain)
	assert.NoError(t, err)
	assert.Equal(t, actual.ClientID, "5XYJZYf5xZ0eKALgBH3O08WzgfUfz7y9")
	assert.Equal(t, actual.Audience, "astronomer-ee")
	assert.Equal(t, actual.DomainURL, "https://auth.astronomer.io/")

	domain = "gcp0001.us-east4.astronomer.io" // Gen1 CLI domain
	_, err = FetchDomainAuthConfig(domain)
	assert.Error(t, err)
	assert.Errorf(t, err, "Error! Invalid domain. "+
		"Are you trying to authenticate to APC? If so, change your current context with 'astro context switch'. ")

	domain = "fail.astronomer.io"
	_, err = FetchDomainAuthConfig(domain)
	assert.Error(t, err)
	assert.Errorf(t, err, "Error! Invalid domain. "+
		"Are you trying to authenticate to APC? If so, change your current context with 'astro context switch'. ")

	domain = "astronomer-dev.io"
	actual, err = FetchDomainAuthConfig(domain)
	assert.NoError(t, err)
	assert.Equal(t, actual.ClientID, "PH3Nac2DtpSx1Tx3IGQmh2zaRbF5ubZG")
	assert.Equal(t, actual.Audience, "astronomer-ee")
	assert.Equal(t, actual.DomainURL, "https://auth.astronomer-dev.io/")

	domain = "fail.astronomer-dev.io"
	_, err = FetchDomainAuthConfig(domain)
	assert.Error(t, err)
	assert.Errorf(t, err, "Error! Invalid domain. "+
		"Are you trying to authenticate to APC? If so, change your current context with 'astro context switch'. ")

	domain = "astronomer-stage.io"
	actual, err = FetchDomainAuthConfig(domain)
	assert.NoError(t, err)
	assert.Equal(t, actual.ClientID, "jsarDat3BeDXZ1monEAeqJPOvRvterpm")
	assert.Equal(t, actual.Audience, "astronomer-ee")
	assert.Equal(t, actual.DomainURL, "https://auth.astronomer-stage.io/")

	domain = "fail.astronomer-stage.io"
	_, err = FetchDomainAuthConfig(domain)
	assert.Error(t, err)
	assert.Errorf(t, err, "Error! Invalid domain. "+
		"Are you trying to authenticate to APC? If so, change your current context with 'astro context switch'. ")

	domain = "fail.astronomer-perf.io"
	_, err = FetchDomainAuthConfig(domain)
	assert.Error(t, err)
	assert.Errorf(t, err, "Error! Invalid domain. "+
		"Are you trying to authenticate to APC? If so, change your current context with 'astro context switch'. ")

	t.Run("pr preview is a valid domain", func(t *testing.T) {
		domain = "pr1234.astronomer-dev.io"
		actual, err = FetchDomainAuthConfig(domain)
		assert.NoError(t, err)
		assert.Equal(t, actual.ClientID, "client-id")
		assert.Equal(t, actual.Audience, "audience")
		assert.Equal(t, actual.DomainURL, "https://myURL.com/")
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
		resp, err := requestUserInfo(Config{}, mockAccessToken)
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

		_, err := requestUserInfo(Config{}, mockAccessToken)
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
		_, err := requestUserInfo(Config{}, mockAccessToken)
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

		resp, err := requestToken(Config{}, "", "")
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

		_, err := requestToken(Config{}, "", "")
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

		_, err := requestToken(Config{}, "", "")
		assert.Contains(t, err.Error(), mockResponse.ErrorDescription)
	})
}

func TestAuthorizeError(t *testing.T) {
	t.Run("a denial that names an unverified email address is a pending sign-up", func(t *testing.T) {
		err := authorizeError(
			[]string{"access_denied"},
			[]string{"Thanks for signing up. Please check your inbox for a verification email to get started."},
		)
		assert.ErrorIs(t, err, ErrEmailVerificationPending)
	})

	t.Run("any other denial keeps the device message", func(t *testing.T) {
		err := authorizeError([]string{"access_denied"}, []string{"user is blocked"})
		assert.NotErrorIs(t, err, ErrEmailVerificationPending)
		assert.Contains(t, err.Error(), "Could not authorize your device")
		assert.Contains(t, err.Error(), "user is blocked")
	})

	t.Run("another error code keeps the device message", func(t *testing.T) {
		err := authorizeError([]string{"invalid_request"}, []string{"please verify your email address"})
		assert.NotErrorIs(t, err, ErrEmailVerificationPending)
		assert.Contains(t, err.Error(), "Could not authorize your device")
	})
}

func TestAuthorizeCallbackHandler(t *testing.T) {
	client := httputil.NewHTTPClient()
	httpClient = client
	t.Run("success", func(t *testing.T) {
		callbackServer = "localhost:12345"
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			time.Sleep(2 * time.Second) // time to spin up the server in authorizeCallbackHandler

			opts := &httputil.DoOptions{
				Method: http.MethodGet,
				Path:   "http://localhost:12345/callback?code=test",
			}
			_, cbErr := client.Do(opts)
			assert.NoError(t, cbErr)
		}()
		code, err := authorizeCallbackHandler()
		assert.Equal(t, "test", code)
		assert.NoError(t, err)
		wg.Wait()
	})

	t.Run("error", func(t *testing.T) {
		callbackServer = "localhost:12346"
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			time.Sleep(2 * time.Second) // time to spin up the server in authorizeCallbackHandler
			opts := &httputil.DoOptions{
				Method: http.MethodGet,
				Path:   "http://localhost:12346/callback?error=error&error_description=fatal_error",
			}
			_, cbErr := client.Do(opts)
			assert.NoError(t, cbErr)
		}()
		_, err := authorizeCallbackHandler()
		assert.Contains(t, err.Error(), "fatal_error")
		wg.Wait()
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
		tokenRequester := func(authConfig Config, verifier, code string) (Result, error) {
			return mockResponse, nil
		}
		openURL = func(url string) error {
			return nil
		}
		mockAuthenticator := Authenticator{tokenRequester: tokenRequester, callbackHandler: callbackHandler}
		resp, err := mockAuthenticator.authDeviceLogin(Config{}, false, false)
		assert.NoError(t, err)
		assert.Equal(t, mockResponse, resp)
	})

	t.Run("signup adds the sign-up parameters to the authorize URL", func(t *testing.T) {
		var authorizeURL string
		openURL = func(url string) error {
			authorizeURL = url
			return nil
		}
		callbackHandler := func() (string, error) {
			return "test-code", nil
		}
		tokenRequester := func(authConfig Config, verifier, code string) (Result, error) {
			return Result{}, nil
		}
		mockAuthenticator := Authenticator{tokenRequester: tokenRequester, callbackHandler: callbackHandler}

		_, err := mockAuthenticator.authDeviceLogin(Config{}, false, false)
		assert.NoError(t, err)
		assert.NotContains(t, authorizeURL, "screen_hint=signup")
		assert.NotContains(t, authorizeURL, "ext-signup-source=cli")

		_, err = mockAuthenticator.authDeviceLogin(Config{}, false, true)
		assert.NoError(t, err)
		assert.Contains(t, authorizeURL, "screen_hint=signup")
		assert.Contains(t, authorizeURL, "ext-signup-source=cli")
	})

	t.Run("openURL & callback failure", func(t *testing.T) {
		openURL = func(url string) error {
			return errMock
		}
		callbackHandler := func() (string, error) {
			return "", errMock
		}
		mockAuthenticator := Authenticator{callbackHandler: callbackHandler}
		_, err := mockAuthenticator.authDeviceLogin(Config{}, false, false)
		assert.ErrorIs(t, err, errMock)
	})

	t.Run("token requester failure", func(t *testing.T) {
		callbackHandler := func() (string, error) {
			return "test-code", nil
		}
		tokenRequester := func(authConfig Config, verifier, code string) (Result, error) {
			return Result{}, errMock
		}
		openURL = func(url string) error {
			return nil
		}
		mockAuthenticator := Authenticator{tokenRequester: tokenRequester, callbackHandler: callbackHandler}
		_, err := mockAuthenticator.authDeviceLogin(Config{}, false, false)
		assert.ErrorIs(t, err, errMock)
	})

	t.Run("success with login link", func(t *testing.T) {
		mockResponse := Result{RefreshToken: "test-token", AccessToken: "test-token", ExpiresIn: 300}
		callbackHandler := func() (string, error) {
			return "test-code", nil
		}
		tokenRequester := func(authConfig Config, verifier, code string) (Result, error) {
			return mockResponse, nil
		}
		mockAuthenticator := Authenticator{tokenRequester: tokenRequester, callbackHandler: callbackHandler}
		resp, err := mockAuthenticator.authDeviceLogin(Config{}, true, false)
		assert.NoError(t, err)
		assert.Equal(t, mockResponse, resp)
	})

	t.Run("callback failure with login link", func(t *testing.T) {
		callbackHandler := func() (string, error) {
			return "", errMock
		}
		mockAuthenticator := Authenticator{callbackHandler: callbackHandler}
		_, err := mockAuthenticator.authDeviceLogin(Config{}, true, false)
		assert.ErrorIs(t, err, errMock)
	})

	t.Run("token requester failure with login link", func(t *testing.T) {
		callbackHandler := func() (string, error) {
			return "test-code", nil
		}
		tokenRequester := func(authConfig Config, verifier, code string) (Result, error) {
			return Result{}, errMock
		}
		mockAuthenticator := Authenticator{tokenRequester: tokenRequester, callbackHandler: callbackHandler}
		_, err := mockAuthenticator.authDeviceLogin(Config{}, true, false)
		assert.ErrorIs(t, err, errMock)
	})
}

// stubCreateOrganization answers the create-organization call with one status
// and body, and puts the real client back when the test ends.
func stubCreateOrganization(t *testing.T, status int, body string) *http.Request {
	t.Helper()
	previous := httpClient
	t.Cleanup(func() { httpClient = previous })
	seen := &http.Request{}
	httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
		*seen = *req
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Header:     make(http.Header),
		}
	})
	return seen
}

func TestBootstrapOrganization(t *testing.T) {
	// The account has nothing else to name the organization after, so the name
	// comes from the email's local part.
	for _, tc := range []struct {
		name    string
		email   string
		orgName string
	}{
		{"the local part becomes the name", "af2-signup-test@astronomer.test", "af2-signup-test"},
		{"anything but a letter, a digit or a dash becomes a dash", "user+astro.signup@example.com", "user-astro-signup"},
		{"leading and trailing dashes come off", "_jane.doe_@astronomer.io", "jane-doe"},
		{"a name under three characters gets a suffix", "ab@example.com", "ab-org"},
		{"a name over fifty characters is cut", strings.Repeat("a", 60) + "@astronomer.io", strings.Repeat("a", 50)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			c, err := config.GetCurrentContext()
			assert.NoError(t, err)
			assert.NoError(t, c.SetContextKey("user_email", tc.email))

			seen := stubCreateOrganization(t, http.StatusOK, "{}")

			assert.NoError(t, bootstrapOrganization())
			assert.Equal(t, "/private/v1alpha1/create-organization-and-workspace", seen.URL.Path)
			sent, err := io.ReadAll(seen.Body)
			assert.NoError(t, err)
			assert.JSONEq(t, `{"organization":{"name":"`+tc.orgName+`"},"workspace":{"name":"Default"}}`, string(sent))
		})
	}

	t.Run("a refused call reports the status and the body", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		assert.NoError(t, c.SetContextKey("user_email", "jane@astronomer.io"))

		stubCreateOrganization(t, http.StatusForbidden, "not allowed")
		err = bootstrapOrganization()
		assert.ErrorContains(t, err, "403")
		assert.ErrorContains(t, err, "not allowed")
	})
}

func TestSwitchToLastUsedWorkspace(t *testing.T) {
	t.Run("failure case", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		ctx := &config.Context{
			LastUsedWorkspace: "test-id",
		}
		resp, found, err := switchToLastUsedWorkspace(ctx, []astrov1.Workspace{{Id: "test-id"}})
		assert.ErrorIs(t, err, config.ErrCtxConfigErr)
		assert.False(t, found)
		assert.Equal(t, astrov1.Workspace{}, resp)
	})

	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("success", func(t *testing.T) {
		ctx := &config.Context{
			LastUsedWorkspace: "test-id",
			Domain:            "test-domain",
		}
		resp, found, err := switchToLastUsedWorkspace(ctx, []astrov1.Workspace{{Id: "test-id"}})
		assert.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, ctx.LastUsedWorkspace, resp.Id)
	})

	t.Run("failure, workspace not found", func(t *testing.T) {
		ctx := &config.Context{
			LastUsedWorkspace: "test-invalid-id",
		}
		resp, found, err := switchToLastUsedWorkspace(ctx, []astrov1.Workspace{{Id: "test-id"}})
		assert.NoError(t, err)
		assert.False(t, found)
		assert.Equal(t, astrov1.Workspace{}, resp)
	})
}

func TestCheckUserSession(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("success", func(t *testing.T) {
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockV1Client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOrganizationsResponse, nil).Once()
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockV1Client, buf)
		mockV1Client.AssertExpectations(t)
		assert.NoError(t, err)
	})

	t.Run("no organization found", func(t *testing.T) {
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOrganizationsResponseEmpty, nil).Once()
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockV1Client, buf)
		assert.Contains(t, err.Error(), "Please contact your Astro Organization Owner to be invited to the organization")
		mockV1Client.AssertExpectations(t)
	})

	t.Run("list organization network error", func(t *testing.T) {
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(nil, errNetwork).Once()
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockV1Client, buf)
		assert.Contains(t, err.Error(), "network error")
		mockV1Client.AssertExpectations(t)
	})

	t.Run("self user network error", func(t *testing.T) {
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(nil, errNetwork).Once()
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockV1Client, buf)
		assert.Contains(t, err.Error(), "network error")
		mockV1Client.AssertExpectations(t)
	})

	t.Run("self user failure", func(t *testing.T) {
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfErrorResponse, nil).Once()
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockV1Client, buf)
		assert.Contains(t, err.Error(), "failed to fetch self user")
		mockV1Client.AssertExpectations(t)
	})

	t.Run("set context failure", func(t *testing.T) {
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOrganizationsResponse, nil).Once()
		ctx := config.Context{}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockV1Client, buf)
		assert.ErrorIs(t, err, config.ErrCtxConfigErr)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("list workspace failure", func(t *testing.T) {
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errMock).Once()
		mockV1Client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOrganizationsResponse, nil).Once()
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockV1Client, buf)
		assert.ErrorIs(t, err, errMock)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("success with more than one workspace", func(t *testing.T) {
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockV1Client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockV1Client.On("GetOrganizationWithResponse", mock.Anything, "test-org-id", mock.Anything).Return(&mockGetOrganizationResponse, nil).Once()

		ctx := config.Context{Domain: "test-domain", LastUsedWorkspace: "workspace-id", Organization: "test-org-id"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockV1Client, buf)
		assert.NoError(t, err)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("success with workspace switch", func(t *testing.T) {
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockV1Client.On("GetOrganizationWithResponse", mock.Anything, "test-org-id", mock.Anything).Return(&mockGetOrganizationResponse, nil).Once()
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		ctx := config.Context{Domain: "test-domain", Organization: "test-org-id"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockV1Client, buf)
		assert.NoError(t, err)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("success but with workspace switch failure", func(t *testing.T) {
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		workspace2 := astrov1.Workspace{
			Name:        "test-workspace-2",
			Description: &description,
			Id:          "workspace-id-2",
		}

		workspaces = []astrov1.Workspace{
			workspace1,
			workspace2,
		}
		tempListWorkspacesResponseOK := astrov1.ListWorkspacesResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astrov1.WorkspacesPaginated{
				Limit:      1,
				Offset:     0,
				TotalCount: 1,
				Workspaces: workspaces,
			},
		}
		mockV1Client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockV1Client.On("GetOrganizationWithResponse", mock.Anything, "test-org-id", mock.Anything).Return(&mockGetOrganizationResponse, nil).Once()
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&tempListWorkspacesResponseOK, nil).Once()
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errMock).Once()
		ctx := config.Context{Domain: "test-domain", Organization: "test-org-id"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockV1Client, buf)
		assert.NoError(t, err)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("success with identity first auth flow", func(t *testing.T) {
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockV1Client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		// context org "test-org-id-2" is fetched directly by ID, not via org list
		mockV1Client.On("GetOrganizationWithResponse", mock.Anything, "test-org-id-2", mock.Anything).Return(&mockGetOrganizationResponse2, nil).Once()
		// context org  "test-org-id-2" takes precedence over the getSelf org "test-org-id"
		ctx := config.Context{Domain: "test-domain", Organization: "test-org-id-2"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockV1Client, buf)
		assert.NoError(t, err)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("set default org product", func(t *testing.T) {
		mockOrganizationsResponse = astrov1.ListOrganizationsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astrov1.OrganizationsPaginated{
				Limit:      1,
				Offset:     0,
				TotalCount: 1,
				Organizations: []astrov1.Organization{
					{Id: "test-org-id", Name: "test-org"},
				},
			},
		}
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockV1Client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOrganizationsResponse, nil).Once()
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockV1Client, buf)
		mockV1Client.AssertExpectations(t)
		assert.NoError(t, err)
	})

	// Regression test for #2083: the user belongs to more orgs than the API's
	// default page size and their context org is not in the first page. The old
	// code listed orgs with no limit and silently fell back to orgs[0]; the new
	// code resolves by ID. We prove the fix by not mocking ListOrganizations at
	// all — any fallthrough would blow up the mock framework.
	t.Run("success when context org is beyond default page size", func(t *testing.T) {
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockV1Client.On("GetOrganizationWithResponse", mock.Anything, "org-beyond-page", mock.Anything).Return(&astrov1.GetOrganizationResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200:      &astrov1.Organization{Id: "org-beyond-page", Name: "Org Beyond Page", Product: &mockOrganizationProduct},
		}, nil).Once()
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		ctx := config.Context{Domain: "test-domain", Organization: "org-beyond-page"}
		buf := new(bytes.Buffer)
		err := CheckUserSession(&ctx, mockV1Client, buf)
		assert.NoError(t, err)
		mockV1Client.AssertExpectations(t)
	})

	// Stale context org (membership revoked, org deleted, or state left over from a
	// prior account on the same domain): GetOrganization returns 403/404. Rather
	// than failing the whole session, we fall through to list-and-pick-first so
	// the user lands somewhere usable. See #2097 for the lifecycle cleanup that
	// would make this fallback unnecessary.
	for _, tc := range []struct {
		name   string
		status int
	}{
		{"forbidden", http.StatusForbidden},
		{"not found", http.StatusNotFound},
	} {
		t.Run("stale context org ("+tc.name+") falls through to first available", func(t *testing.T) {
			mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
			mockV1Client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
			mockV1Client.On("GetOrganizationWithResponse", mock.Anything, "stale-org", mock.Anything).Return(&astrov1.GetOrganizationResponse{
				HTTPResponse: &http.Response{StatusCode: tc.status},
			}, nil).Once()
			mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOrganizationsResponse, nil).Once()
			mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
			ctx := config.Context{Domain: "test-domain", Organization: "stale-org"}
			buf := new(bytes.Buffer)
			err := CheckUserSession(&ctx, mockV1Client, buf)
			assert.NoError(t, err)
			mockV1Client.AssertExpectations(t)
		})
	}
}

func TestCheckUserSessionNoOrganization(t *testing.T) {
	// A zero-organization login is either a brand-new account or someone
	// waiting on an invite. --signup answers that question up front; without
	// it the user is asked.
	orgCreated := func() *astrov1.ListOrganizationsResponse {
		return &astrov1.ListOrganizationsResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200: &astrov1.OrganizationsPaginated{
				Limit:         1,
				TotalCount:    1,
				Organizations: []astrov1.Organization{{Id: "new-org-id", Name: "jane", Product: &mockOrganizationProduct}},
			},
		}
	}
	noOrgs := func() *astrov1.ListOrganizationsResponse {
		return &astrov1.ListOrganizationsResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200:      &astrov1.OrganizationsPaginated{Limit: 1},
		}
	}
	// answer feeds input.Confirm through a stand-in for stdin.
	answer := func(t *testing.T, text string) {
		t.Helper()
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.WriteString(text + "\n")
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		t.Cleanup(func() { os.Stdin = stdin })
		os.Stdin = r
	}
	t.Run("signup creates the organization without asking", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		stubCreateOrganization(t, http.StatusOK, "{}")
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(noOrgs(), nil).Once()
		mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(orgCreated(), nil).Once()
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()

		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := checkUserSession(&ctx, mockV1Client, buf, true)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "Creating your organization")
		mockV1Client.AssertExpectations(t)
	})

	t.Run("without signup the user is asked, and yes creates the organization", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		stubCreateOrganization(t, http.StatusOK, "{}")
		answer(t, "y")
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(noOrgs(), nil).Once()
		mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(orgCreated(), nil).Once()
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()

		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := checkUserSession(&ctx, mockV1Client, buf, false)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "Creating your organization")
		mockV1Client.AssertExpectations(t)
	})

	t.Run("without signup, no leaves the account alone", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		answer(t, "n")
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(noOrgs(), nil).Once()

		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := checkUserSession(&ctx, mockV1Client, buf, false)
		assert.ErrorIs(t, err, ErrorNoOrganization)
		assert.NotContains(t, buf.String(), "Creating your organization")
		mockV1Client.AssertExpectations(t)
	})

	t.Run("a failed creation stops the login", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		stubCreateOrganization(t, http.StatusForbidden, "not allowed")
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(noOrgs(), nil).Once()

		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := checkUserSession(&ctx, mockV1Client, buf, true)
		assert.ErrorContains(t, err, "not allowed")
		mockV1Client.AssertExpectations(t)
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
		tokenRequester := func(authConfig Config, verifier, code string) (Result, error) {
			return mockResponse, nil
		}
		userInfoRequester := func(authConfig Config, accessToken string) (UserInfo, error) {
			return mockUserInfo, nil
		}
		openURL = func(url string) error {
			return nil
		}
		authenticator = Authenticator{userInfoRequester, tokenRequester, callbackHandler}

		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOrganizationsResponse, nil).Once()
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		err := Login("astronomer.io", "", mockV1Client, os.Stdout, false, false)
		assert.NoError(t, err)
		mockV1Client.AssertExpectations(t)
	})
	t.Run("can login to a pr preview environment successfully", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPrPreview)
		// mocking this as once a PR closes, test would fail
		mockAuthConfigResponse := Config{
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
		tokenRequester := func(authConfig Config, verifier, code string) (Result, error) {
			return mockResponse, nil
		}
		userInfoRequester := func(authConfig Config, accessToken string) (UserInfo, error) {
			return mockUserInfo, nil
		}
		openURL = func(url string) error {
			return nil
		}
		authenticator = Authenticator{userInfoRequester, tokenRequester, callbackHandler}
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockV1Client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOrganizationsResponse, nil).Once()

		err = Login("pr5723.cloud.astronomer-dev.io", "", mockV1Client, os.Stdout, false, false)
		assert.NoError(t, err)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("oauth token success", func(t *testing.T) {
		mockResponse := Result{RefreshToken: "test-token", AccessToken: "test-token", ExpiresIn: 300}
		mockUserInfo := UserInfo{Email: "test@astronomer.test"}
		callbackHandler := func() (string, error) {
			return "test-code", nil
		}
		tokenRequester := func(authConfig Config, verifier, code string) (Result, error) {
			return mockResponse, nil
		}
		userInfoRequester := func(authConfig Config, accessToken string) (UserInfo, error) {
			return mockUserInfo, nil
		}
		openURL = func(url string) error {
			return nil
		}
		authenticator = Authenticator{userInfoRequester, tokenRequester, callbackHandler}

		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockV1Client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOrganizationsResponse, nil).Once()

		err := Login("astronomer.io", "OAuth Token", mockV1Client, os.Stdout, false, false)
		assert.NoError(t, err)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("invalid domain", func(t *testing.T) {
		err := Login("fail.astronomer.io", "", nil, os.Stdout, false, false)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Invalid domain.")
	})

	t.Run("auth failure", func(t *testing.T) {
		callbackHandler := func() (string, error) {
			return "", errMock
		}
		authenticator = Authenticator{callbackHandler: callbackHandler}
		err := Login("cloud.astronomer.io", "", nil, os.Stdout, false, false)
		assert.ErrorIs(t, err, errMock)
	})

	t.Run("check token failure", func(t *testing.T) {
		mockResponse := Result{RefreshToken: "test-token", AccessToken: "test-token", ExpiresIn: 300}
		mockUserInfo := UserInfo{Email: "test@astronomer.test"}
		callbackHandler := func() (string, error) {
			return "test-code", nil
		}
		tokenRequester := func(authConfig Config, verifier, code string) (Result, error) {
			return mockResponse, nil
		}
		userInfoRequester := func(authConfig Config, accessToken string) (UserInfo, error) {
			return mockUserInfo, nil
		}
		openURL = func(url string) error {
			return nil
		}
		authenticator = Authenticator{userInfoRequester, tokenRequester, callbackHandler}

		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfErrorResponse, nil).Once()
		err := Login("", "", mockV1Client, os.Stdout, false, false)
		assert.Contains(t, err.Error(), "failed to fetch self user")
		mockV1Client.AssertExpectations(t)
	})

	t.Run("initial login with empty config file", func(t *testing.T) {
		// initialize empty config
		testUtil.InitTestConfig(testUtil.Initial)
		// initialize the mock client
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockV1Client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOrganizationsResponse, nil).Once()
		// initialize the test authenticator
		mockUserInfo := UserInfo{Email: "test@astronomer.test"}
		userInfoRequester := func(authConfig Config, accessToken string) (UserInfo, error) {
			return mockUserInfo, nil
		}
		authenticator = Authenticator{
			userInfoRequester: userInfoRequester,
			callbackHandler:   func() (string, error) { return "authorizationCode", nil },
			tokenRequester: func(authConfig Config, verifier, code string) (Result, error) {
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
		err := Login("astronomer.io", "", mockV1Client, os.Stdout, true, false)
		assert.NoError(t, err)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("domain doesn't match current context", func(t *testing.T) {
		// initialize empty config
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		// initialize the mock client
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockV1Client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOrganizationsResponse, nil).Once()
		// initialize the test authenticator
		mockUserInfo := UserInfo{Email: "test@astronomer.test"}
		userInfoRequester := func(authConfig Config, accessToken string) (UserInfo, error) {
			return mockUserInfo, nil
		}
		authenticator = Authenticator{
			userInfoRequester: userInfoRequester,
			callbackHandler:   func() (string, error) { return "authorizationCode", nil },
			tokenRequester: func(authConfig Config, verifier, code string) (Result, error) {
				return Result{
					RefreshToken: "refresh_token",
					AccessToken:  "access_token",
					ExpiresIn:    1234,
				}, nil
			},
		}
		// initialize user input with email
		defer testUtil.MockUserInput(t, "test.user@astronomer.io")()
		err := Login("astronomer.io", "", mockV1Client, os.Stdout, true, false)
		assert.NoError(t, err)
		// assert that everything got set in the right spot
		domainContext, err := context.GetContext("astronomer.io")
		assert.NoError(t, err)
		currentContext, err := context.GetContext("localhost")
		assert.NoError(t, err)
		assert.Equal(t, domainContext.Token, "Bearer access_token")
		assert.Equal(t, currentContext.Token, "token")
		mockV1Client.AssertExpectations(t)
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
