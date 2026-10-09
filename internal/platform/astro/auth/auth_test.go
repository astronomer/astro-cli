package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/pkg/astroauth"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/input"
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
	restoreHTTPClient(t)

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
	restoreHTTPClient(t)

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
	// The callback answers the browser with a redirect to auth.astronomer.io.
	// The test checks where it points without following it there.
	client := httputil.NewHTTPClient()
	client.HTTPClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	previous := httpClient
	t.Cleanup(func() { httpClient = previous })
	httpClient = client
	t.Run("success", func(t *testing.T) {
		callbackServer = "localhost:12345"
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			awaitCallbackServer(t, callbackServer)

			opts := &httputil.DoOptions{
				Method: http.MethodGet,
				Path:   "http://localhost:12345/callback?code=test",
			}
			res, cbErr := client.Do(opts)
			if assert.NoError(t, cbErr) {
				defer res.Body.Close()
				assert.Equal(t, http.StatusFound, res.StatusCode)
				assert.Equal(t, "https://auth.astronomer.io/device/success", res.Header.Get("Location"))
			}
		}()
		code, err := authorizeCallbackHandler()
		assert.Equal(t, "test", code)
		assert.NoError(t, err)
		wg.Wait()
	})

	t.Run("an address already taken returns an error", func(t *testing.T) {
		held, err := net.Listen("tcp", "localhost:0")
		assert.NoError(t, err)
		defer held.Close()
		previous := callbackServer
		t.Cleanup(func() { callbackServer = previous })
		callbackServer = held.Addr().String()

		code, err := authorizeCallbackHandler()
		assert.Error(t, err)
		assert.Empty(t, code)
		assert.Contains(t, err.Error(), "cannot open the login callback on "+held.Addr().String())
		assert.Contains(t, err.Error(), "Close any other astro login")
	})

	t.Run("error", func(t *testing.T) {
		callbackServer = "localhost:12346"
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			awaitCallbackServer(t, callbackServer)
			opts := &httputil.DoOptions{
				Method: http.MethodGet,
				Path:   "http://localhost:12346/callback?error=error&error_description=fatal_error",
			}
			res, cbErr := client.Do(opts)
			if assert.NoError(t, cbErr) {
				defer res.Body.Close()
				assert.Equal(t, http.StatusFound, res.StatusCode)
				assert.Equal(t, "https://auth.astronomer.io/device/denied", res.Header.Get("Location"))
			}
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

// awaitCallbackServer waits for authorizeCallbackHandler, which runs in the
// test's own goroutine, to open its listener. A fixed sleep would be both
// slower and still a race.
func awaitCallbackServer(t *testing.T, addr string) {
	t.Helper()
	for range 400 {
		conn, err := net.Dial("tcp", addr)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Errorf("the login callback never opened %s", addr)
}

func TestShouldSignup(t *testing.T) {
	const domain = "astronomer.io"
	tests := []struct {
		name     string
		contexts map[string]config.Context
		want     bool
	}{
		{"no context at all", nil, true},
		{"a context for another domain only", map[string]config.Context{
			"astronomer-dev.io": {Domain: "astronomer-dev.io", Token: "Bearer token"},
		}, true},
		{"a context for this domain with nothing in it", map[string]config.Context{
			domain: {Domain: domain},
		}, true},
		{"an access token for this domain", map[string]config.Context{
			domain: {Domain: domain, Token: "Bearer token"},
		}, false},
		{"a refresh token for this domain", map[string]config.Context{
			domain: {Domain: domain, RefreshToken: "refresh-token"},
		}, false},
		{"a workspace for this domain", map[string]config.Context{
			domain: {Domain: domain, Workspace: "test-workspace-id"},
		}, false},
		{"an organization for this domain", map[string]config.Context{
			domain: {Domain: domain, Organization: "test-org-id"},
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getContext := func(d string) (config.Context, error) {
				c, ok := tt.contexts[d]
				if !ok {
					return config.Context{}, errMock
				}
				return c, nil
			}
			assert.Equal(t, tt.want, shouldSignup(getContext, domain))
		})
	}

	// The expiry lives beside the context rather than in it, so read it through
	// the real store: a stale token is still proof that the account exists.
	t.Run("an expired token for this domain", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		c := config.Context{Domain: domain}
		assert.NoError(t, c.SetExpiresIn(-3600))
		assert.False(t, ShouldSignup(domain))
		assert.False(t, ShouldSignup("cloud.astronomer.io"))
	})

	t.Run("only production opens the sign-up screen", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		assert.True(t, ShouldSignup("astronomer.io"))
		assert.False(t, ShouldSignup("astronomer-dev.io"))
		assert.False(t, ShouldSignup("astronomer-stage.io"))
		assert.False(t, ShouldSignup("pr12345.astronomer-dev.io"))
	})
}

func TestAuthDeviceLogin(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	// The prompt path is the one these tests were written for, and a test binary
	// has no terminal, so say it has one unless a test says otherwise.
	stubStdinIsTerminal(t, true)
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
		resp, err := mockAuthenticator.authDeviceLogin(Config{}, false, false, false)
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

		_, err := mockAuthenticator.authDeviceLogin(Config{}, false, false, false)
		assert.NoError(t, err)
		assert.NotContains(t, authorizeURL, "screen_hint=signup")
		assert.NotContains(t, authorizeURL, "ext-signup-source=cli")

		_, err = mockAuthenticator.authDeviceLogin(Config{}, false, true, false)
		assert.NoError(t, err)
		assert.Contains(t, authorizeURL, "screen_hint=signup")
		assert.Contains(t, authorizeURL, "ext-signup-source=cli")
	})

	t.Run("force and signup make the identity provider ask for the password again", func(t *testing.T) {
		var authorizeURL string
		openURL = func(url string) error {
			authorizeURL = url
			return nil
		}
		mockAuthenticator := Authenticator{
			tokenRequester:  func(Config, string, string) (Result, error) { return Result{}, nil },
			callbackHandler: func() (string, error) { return "test-code", nil },
		}

		_, err := mockAuthenticator.authDeviceLogin(Config{}, false, false, false)
		assert.NoError(t, err)
		assert.NotContains(t, authorizeURL, "prompt=login")

		_, err = mockAuthenticator.authDeviceLogin(Config{}, false, false, true)
		assert.NoError(t, err)
		assert.Contains(t, authorizeURL, "prompt=login")

		_, err = mockAuthenticator.authDeviceLogin(Config{}, false, true, false)
		assert.NoError(t, err)
		assert.Contains(t, authorizeURL, "prompt=login")
	})

	t.Run("the prompt names the screen the browser opens", func(t *testing.T) {
		openURL = func(url string) error {
			return nil
		}
		callbackHandler := func() (string, error) {
			return "test-code", nil
		}
		tokenRequester := func(authConfig Config, verifier, code string) (Result, error) {
			return Result{}, nil
		}
		mockAuthenticator := Authenticator{tokenRequester: tokenRequester, callbackHandler: callbackHandler}

		out := captureStderr(t, func() {
			_, err := mockAuthenticator.authDeviceLogin(Config{}, false, true, false)
			assert.NoError(t, err)
		})
		assert.Contains(t, out, "to open the browser to create your Astro account")
		assert.Contains(t, out, "astro login --signin")

		out = captureStderr(t, func() {
			_, err := mockAuthenticator.authDeviceLogin(Config{}, false, false, false)
			assert.NoError(t, err)
		})
		assert.Contains(t, out, "to open the browser to log in")
		assert.NotContains(t, out, "create your Astro account")
	})

	t.Run("openURL & callback failure", func(t *testing.T) {
		openURL = func(url string) error {
			return errMock
		}
		callbackHandler := func() (string, error) {
			return "", errMock
		}
		mockAuthenticator := Authenticator{callbackHandler: callbackHandler}
		_, err := mockAuthenticator.authDeviceLogin(Config{}, false, false, false)
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
		_, err := mockAuthenticator.authDeviceLogin(Config{}, false, false, false)
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
		resp, err := mockAuthenticator.authDeviceLogin(Config{}, true, false, false)
		assert.NoError(t, err)
		assert.Equal(t, mockResponse, resp)
	})

	t.Run("callback failure with login link", func(t *testing.T) {
		callbackHandler := func() (string, error) {
			return "", errMock
		}
		mockAuthenticator := Authenticator{callbackHandler: callbackHandler}
		_, err := mockAuthenticator.authDeviceLogin(Config{}, true, false, false)
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
		_, err := mockAuthenticator.authDeviceLogin(Config{}, true, false, false)
		assert.ErrorIs(t, err, errMock)
	})

	t.Run("a terminal gets the Enter prompt", func(t *testing.T) {
		stubStdinIsTerminal(t, true)
		stdin := stubStdin(t, "\n")
		browserOpened := false
		openURL = func(url string) error {
			browserOpened = true
			return nil
		}
		callbackHandler := func() (string, error) { return "test-code", nil }
		tokenRequester := func(authConfig Config, verifier, code string) (Result, error) {
			return Result{}, nil
		}
		mockAuthenticator := Authenticator{tokenRequester: tokenRequester, callbackHandler: callbackHandler}

		var err error
		out := captureStderr(t, func() {
			_, err = mockAuthenticator.authDeviceLogin(Config{}, false, false, false)
		})
		assert.NoError(t, err)
		assert.Contains(t, out, "to open the browser to log in")
		assert.True(t, browserOpened)
		assert.Equal(t, "", readAll(t, stdin))
	})

	t.Run("no terminal takes the login link without reading stdin", func(t *testing.T) {
		stubStdinIsTerminal(t, false)
		stdin := stubStdin(t, "unread\n")
		openURL = func(url string) error {
			t.Error("the login-link path must not open a browser")
			return nil
		}
		mockResponse := Result{RefreshToken: "test-token", AccessToken: "test-token", ExpiresIn: 300}
		callbackHandler := func() (string, error) { return "test-code", nil }
		tokenRequester := func(authConfig Config, verifier, code string) (Result, error) {
			return mockResponse, nil
		}
		mockAuthenticator := Authenticator{tokenRequester: tokenRequester, callbackHandler: callbackHandler}

		var resp Result
		var err error
		out := captureStderr(t, func() {
			resp, err = mockAuthenticator.authDeviceLogin(Config{}, false, false, false)
		})
		assert.NoError(t, err)
		assert.Equal(t, mockResponse, resp)
		assert.Contains(t, out, "Please visit the following link")
		assert.NotContains(t, out, "to open the browser to log in")
		// Nothing consumed stdin, so the line is still there to read.
		assert.Equal(t, "unread\n", readAll(t, stdin))
	})
}

// stubStdinIsTerminal answers the terminal check with one value, and puts the
// real check back when the test ends.
func stubStdinIsTerminal(t *testing.T, isTerminal bool) {
	t.Helper()
	previous := stdinIsTerminal
	t.Cleanup(func() { stdinIsTerminal = previous })
	stdinIsTerminal = func() bool { return isTerminal }
}

// stubStdin makes stdin hold text, and returns it so a test can read what the
// code under test left behind.
func stubStdin(t *testing.T, text string) *os.File {
	t.Helper()
	r, w, err := os.Pipe()
	assert.NoError(t, err)
	_, err = w.WriteString(text)
	assert.NoError(t, err)
	w.Close()
	previous := os.Stdin
	t.Cleanup(func() {
		os.Stdin = previous
		r.Close()
	})
	os.Stdin = r
	return r
}

func readAll(t *testing.T, f *os.File) string {
	t.Helper()
	left, err := io.ReadAll(f)
	assert.NoError(t, err)
	return string(left)
}

// captureStdout collects what f prints to stdout.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	return captureFile(t, &os.Stdout, f)
}

func captureStderr(t *testing.T, f func()) string {
	t.Helper()
	return captureFile(t, &os.Stderr, f)
}

// captureFile returns what f writes to *stream, which it swaps for a pipe
// while f runs.
func captureFile(t *testing.T, stream **os.File, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	assert.NoError(t, err)
	previous := *stream
	*stream = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		done <- buf.String()
	}()
	f()
	*stream = previous
	w.Close()
	out := <-done
	r.Close()
	return out
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

// A login that has to ask which Workspace to use asks on stderr, picker and
// all, and stdout stays empty; the context the switch leaves is the result,
// on the command's writer.
func TestCheckUserSessionAsksForTheWorkspaceOnStderr(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	two := ListWorkspacesResponseOK
	two.JSON200 = &astrov1.WorkspacesPaginated{TotalCount: 2, Workspaces: []astrov1.Workspace{
		{Name: "first-workspace", Id: "ws-first"},
		{Name: "second-workspace", Id: "ws-second"},
	}}
	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&two, nil)
	mockV1Client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Maybe()
	mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOrganizationsResponse, nil).Maybe()
	mockV1Client.On("GetOrganizationWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetOrganizationResponse, nil).Maybe()
	ctx, err := config.GetCurrentContext()
	assert.NoError(t, err)

	r, w, err := os.Pipe()
	assert.NoError(t, err)
	_, _ = w.WriteString("2\n")
	w.Close()
	previous := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = previous })

	out := new(bytes.Buffer)
	var stdout string
	errOut := captureStderr(t, func() {
		stdout = captureStdout(t, func() {
			assert.NoError(t, checkUserSession(&ctx, mockV1Client, out, false))
		})
	})

	assert.Empty(t, stdout)
	assert.Contains(t, errOut, cliChooseWorkspace)
	assert.Contains(t, errOut, "first-workspace", "the picker's table")
	assert.Contains(t, errOut, "\n> ", "the picker's prompt")
	assert.Contains(t, out.String(), "ws-second", "the context the switch left")
	assert.NotContains(t, out.String(), "first-workspace", "the question is not on the command's writer")
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

	// The question can come from the login inside any command, and only
	// `astro login` has --signup, so the refusal names the command too.
	t.Run("a run that may not ask is refused, naming astro login", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		t.Cleanup(input.SetGuard(func() string { return "with --output json it cannot" }))
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
		mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(noOrgs(), nil).Once()

		ctx := config.Context{Domain: "test-domain"}
		err := checkUserSession(&ctx, mockV1Client, new(bytes.Buffer), false)
		assert.True(t, input.IsRequired(err), "err: %v", err)
		assert.ErrorContains(t, err, "pass --signup to astro login")
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

// A browser login prints to stdout and then waits, for Enter or for the
// browser's callback. A run that may not ask (a command under --output json
// whose login check finds no usable login) is refused before either, printing
// nothing, as unauthenticated rather than a question a flag could answer.
func TestLoginRefusedWhenTheRunMayNotAsk(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	stubAuthConfig(t, testAuthConfig)
	stubRefresh(t, astroauth.TokenResponse{}, errMock)
	t.Cleanup(input.SetGuard(func() string { return "with --output json it cannot" }))
	previous := authenticator
	t.Cleanup(func() { authenticator = previous })
	authenticator = Authenticator{
		callbackHandler: func() (string, error) {
			t.Error("the browser login started")
			return "", errMock
		},
	}

	r, w, err := os.Pipe()
	assert.NoError(t, err)
	stdout := os.Stdout
	os.Stdout = w
	err = Login("astronomer.io", "", new(astrov1_mocks.ClientWithResponsesInterface), io.Discard, false, false, false)
	os.Stdout = stdout
	w.Close()
	printed, readErr := io.ReadAll(r)
	assert.NoError(t, readErr)

	assert.ErrorIs(t, err, ErrLoginNeeded)
	assert.False(t, input.IsRequired(err), "no flag answers a login, so it is not input_required")
	assert.ErrorContains(t, err, "with --output json it cannot — run astro login first, or set ASTRO_API_TOKEN")
	assert.Empty(t, string(printed), "nothing reaches stdout")
}

func TestLogin(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	// These cases log in through the browser, so the logins they save must not
	// be reused by the cases after them.
	stubRefresh(t, astroauth.TokenResponse{}, errMock)
	restoreHTTPClient(t)
	t.Run("success", func(t *testing.T) {
		stubAuthConfig(t, testAuthConfig)
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
		err := Login("astronomer.io", "", mockV1Client, os.Stdout, false, false, false)
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

		err = Login("pr5723.cloud.astronomer-dev.io", "", mockV1Client, os.Stdout, false, false, false)
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

		err := Login("astronomer.io", "OAuth Token", mockV1Client, os.Stdout, false, false, false)
		assert.NoError(t, err)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("invalid domain", func(t *testing.T) {
		err := Login("fail.astronomer.io", "", nil, os.Stdout, false, false, false)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Invalid domain.")
	})

	t.Run("auth failure", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		callbackHandler := func() (string, error) {
			return "", errMock
		}
		authenticator = Authenticator{callbackHandler: callbackHandler}
		err := Login("cloud.astronomer.io", "", nil, os.Stdout, false, false, false)
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
		err := Login("", "", mockV1Client, os.Stdout, false, false, false)
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
		err := Login("astronomer.io", "", mockV1Client, os.Stdout, true, false, false)
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
		err := Login("astronomer.io", "", mockV1Client, os.Stdout, true, false, false)
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

// testAuthConfig is what a stubbed auth-config request answers with.
var testAuthConfig = Config{ClientID: "client-id", Audience: "audience", DomainURL: "https://auth.astronomer.test/"}

// restoreHTTPClient puts the package's client back when the test ends, for a
// test whose cases replace it without doing so themselves. A client left
// behind would answer the next test's requests, or, if it is the real one,
// send them to the network.
func restoreHTTPClient(t *testing.T) {
	t.Helper()
	previous := httpClient
	t.Cleanup(func() { httpClient = previous })
}

// stubAuthConfig answers every auth-config request with authConfig, and puts
// the real client back when the test ends.
func stubAuthConfig(t *testing.T, authConfig Config) {
	t.Helper()
	restoreHTTPClient(t)
	body, err := json.Marshal(authConfig)
	assert.NoError(t, err)
	httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBuffer(body)),
			Header:     make(http.Header),
		}
	})
}

// stubRefresh answers every refresh with tok, or with err when it is set, and
// counts the calls.
func stubRefresh(t *testing.T, tok astroauth.TokenResponse, err error) *int {
	t.Helper()
	previous := refreshAccessToken
	t.Cleanup(func() { refreshAccessToken = previous })
	calls := new(int)
	refreshAccessToken = func(_ astroauth.AuthConfig, _ string, _ ...astroauth.RequestOption) (*astroauth.TokenResponse, error) {
		*calls++
		if err != nil {
			return nil, err
		}
		return &tok, nil
	}
	return calls
}

// browserAuthenticator stands in for the identity provider. It counts the
// browser logins, and its userinfo call rejects the tokens in rejected.
func browserAuthenticator(t *testing.T, rejected ...string) *int {
	t.Helper()
	previous := authenticator
	t.Cleanup(func() { authenticator = previous })
	browserLogins := new(int)
	authenticator = Authenticator{
		userInfoRequester: func(_ Config, accessToken string) (UserInfo, error) {
			if slices.Contains(rejected, accessToken) {
				return UserInfo{}, errMock
			}
			return UserInfo{Email: "test@astronomer.test"}, nil
		},
		tokenRequester: func(Config, string, string) (Result, error) {
			return Result{AccessToken: "browser-token", RefreshToken: "browser-refresh", ExpiresIn: 3600}, nil
		},
		callbackHandler: func() (string, error) {
			*browserLogins++
			return "test-code", nil
		},
	}
	openURL = func(string) error { return nil }
	stubStdinIsTerminal(t, false)
	return browserLogins
}

func checkUserSessionMocks() *astrov1_mocks.ClientWithResponsesInterface {
	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockV1Client.On("GetSelfUserWithResponse", mock.Anything, mock.Anything).Return(&mockGetSelfResponse, nil).Once()
	mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOrganizationsResponse, nil).Once()
	mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
	return mockV1Client
}

func saveLogin(t *testing.T, domain, token, refreshToken string, expiresIn int64) {
	t.Helper()
	c := config.Context{Domain: domain}
	assert.NoError(t, c.SetContextKey("token", token))
	assert.NoError(t, c.SetContextKey("refreshtoken", refreshToken))
	assert.NoError(t, c.SetExpiresIn(expiresIn))
}

func TestLoginReusesSavedLogin(t *testing.T) {
	const domain = "astronomer.io"
	stubAuthConfig(t, Config{ClientID: "client-id", Audience: "audience", DomainURL: "https://auth.example.com/"})

	t.Run("a saved refresh token logs in without a browser", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		saveLogin(t, domain, "Bearer expired", "saved-refresh", -60)
		browserLogins := browserAuthenticator(t)
		refreshes := stubRefresh(t, astroauth.TokenResponse{AccessToken: "refreshed", ExpiresIn: 3600}, nil)
		mockV1Client := checkUserSessionMocks()

		var out string
		errOut := captureStderr(t, func() {
			out = captureStdout(t, func() {
				assert.NoError(t, Login(domain, "", mockV1Client, io.Discard, false, false, false))
			})
		})

		assert.Equal(t, 0, *browserLogins)
		assert.Equal(t, 1, *refreshes)
		// All on stderr: inside another command's login check, stdout is that
		// command's own output.
		assert.Contains(t, errOut, "Using your saved login for astronomer.io")
		assert.Contains(t, errOut, "Logging in as")
		assert.Contains(t, errOut, "test@astronomer.test")
		assert.Contains(t, errOut, "Successfully authenticated to Astronomer")
		assert.Empty(t, out, "a reused login prints nothing on stdout")
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		assert.Equal(t, domain, c.Domain)
		assert.Equal(t, "Bearer refreshed", c.Token)
		assert.Equal(t, "saved-refresh", c.RefreshToken)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("a saved access token with time left is used without a refresh", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		saveLogin(t, domain, "Bearer still-good", "saved-refresh", 3600)
		browserLogins := browserAuthenticator(t)
		refreshes := stubRefresh(t, astroauth.TokenResponse{}, errMock)
		mockV1Client := checkUserSessionMocks()

		captureStdout(t, func() {
			assert.NoError(t, Login(domain, "", mockV1Client, io.Discard, false, false, false))
		})

		assert.Equal(t, 0, *browserLogins)
		assert.Equal(t, 0, *refreshes)
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		assert.Equal(t, "Bearer still-good", c.Token)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("a failed refresh falls back to the browser", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		saveLogin(t, domain, "Bearer expired", "revoked-refresh", -60)
		browserLogins := browserAuthenticator(t)
		stubRefresh(t, astroauth.TokenResponse{}, errMock)
		mockV1Client := checkUserSessionMocks()

		captureStdout(t, func() {
			assert.NoError(t, Login(domain, "", mockV1Client, io.Discard, false, false, false))
		})

		assert.Equal(t, 1, *browserLogins)
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		assert.Equal(t, "Bearer browser-token", c.Token)
		assert.Equal(t, "browser-refresh", c.RefreshToken)
	})

	// A login runs inside other commands' login checks too, where stdout is
	// the command's own output, and with stdout redirected the person still
	// has to see the question. So the whole flow (the welcome, the Enter
	// prompt or the link, the progress) is on stderr, and stdout stays empty.
	t.Run("a browser login asks on stderr and leaves stdout empty", func(t *testing.T) {
		for _, terminal := range []bool{false, true} {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			saveLogin(t, domain, "", "", 0)
			browserLogins := browserAuthenticator(t)
			stubStdinIsTerminal(t, terminal)
			mockV1Client := checkUserSessionMocks()
			if terminal {
				// The Enter the prompt waits for.
				r, w, err := os.Pipe()
				assert.NoError(t, err)
				_, _ = w.WriteString("\n")
				w.Close()
				previous := os.Stdin
				os.Stdin = r
				t.Cleanup(func() { os.Stdin = previous })
			}

			var out string
			errOut := captureStderr(t, func() {
				out = captureStdout(t, func() {
					assert.NoError(t, Login(domain, "", mockV1Client, io.Discard, false, false, false))
				})
			})

			assert.Equal(t, 1, *browserLogins, "terminal=%v", terminal)
			assert.Empty(t, out, "terminal=%v", terminal)
			assert.Contains(t, errOut, "Welcome to the Astro CLI")
			if terminal {
				assert.Contains(t, errOut, "Press Enter")
			} else {
				assert.Contains(t, errOut, "Please visit the following link")
			}
			assert.Contains(t, errOut, "Logging in as")
		}
	})

	t.Run("a token userinfo rejects falls back to the browser", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		saveLogin(t, domain, "Bearer rejected", "", 3600)
		browserLogins := browserAuthenticator(t, "rejected")
		mockV1Client := checkUserSessionMocks()

		captureStdout(t, func() {
			assert.NoError(t, Login(domain, "", mockV1Client, io.Discard, false, false, false))
		})

		assert.Equal(t, 1, *browserLogins)
	})

	t.Run("an access token userinfo rejects falls back to the refresh token", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		saveLogin(t, domain, "Bearer api-token", "saved-refresh", 3600)
		browserLogins := browserAuthenticator(t, "api-token")
		refreshes := stubRefresh(t, astroauth.TokenResponse{AccessToken: "refreshed", ExpiresIn: 3600}, nil)
		mockV1Client := checkUserSessionMocks()

		captureStdout(t, func() {
			assert.NoError(t, Login(domain, "", mockV1Client, io.Discard, false, false, false))
		})

		assert.Equal(t, 0, *browserLogins)
		assert.Equal(t, 1, *refreshes)
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		assert.Equal(t, "Bearer refreshed", c.Token)
		assert.Equal(t, "test@astronomer.test", c.UserEmail)
	})

	t.Run("a login to another host on the same tenant logs in a new host", func(t *testing.T) {
		const loggedIn, newHost = "pr1111.astronomer-dev.io", "pr2222.astronomer-dev.io"
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		saveLogin(t, loggedIn, "Bearer expired", "sandbox-refresh", -60)
		assert.NoError(t, (&config.Context{Domain: loggedIn}).SetAuthTenant("https://auth.example.com/", "client-id"))
		browserLogins := browserAuthenticator(t)
		stubRefresh(t, astroauth.TokenResponse{AccessToken: "refreshed", ExpiresIn: 3600}, nil)
		mockV1Client := checkUserSessionMocks()

		captureStdout(t, func() {
			assert.NoError(t, Login(newHost, "", mockV1Client, io.Discard, false, false, false))
		})

		assert.Equal(t, 0, *browserLogins)
		for _, host := range []string{newHost, loggedIn} {
			c, err := context.GetContext(host)
			assert.NoError(t, err)
			assert.Equal(t, "Bearer refreshed", c.Token, host)
			assert.Equal(t, "sandbox-refresh", c.RefreshToken, host)
			assert.Equal(t, "https://auth.example.com/", c.AuthDomain, host)
			assert.Equal(t, "client-id", c.AuthClientID, host)
		}
		current, err := config.GetCurrentDomain()
		assert.NoError(t, err)
		assert.Equal(t, newHost, current)
	})

	t.Run("a login on another tenant is not reused", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		saveLogin(t, "astronomer-stage.io", "Bearer still-good", "stage-refresh", 3600)
		assert.NoError(t, (&config.Context{Domain: "astronomer-stage.io"}).SetAuthTenant("https://auth.astronomer-stage.io/", "stage-client"))
		browserLogins := browserAuthenticator(t)
		stubRefresh(t, astroauth.TokenResponse{}, errMock)
		mockV1Client := checkUserSessionMocks()

		captureStdout(t, func() {
			assert.NoError(t, Login(domain, "", mockV1Client, io.Discard, false, false, false))
		})

		assert.Equal(t, 1, *browserLogins)
		c, err := context.GetContext("astronomer-stage.io")
		assert.NoError(t, err)
		assert.Equal(t, "Bearer still-good", c.Token)
	})

	for _, tc := range []struct {
		name          string
		token         string
		signup, force bool
		browserLogins int
		savedToken    string
	}{
		{name: "force opens the browser", force: true, browserLogins: 1, savedToken: "Bearer browser-token"},
		{name: "signup opens the browser", signup: true, browserLogins: 1, savedToken: "Bearer browser-token"},
		{name: "a given token is used as it is", token: "given-token", savedToken: "Bearer given-token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			saveLogin(t, domain, "Bearer still-good", "saved-refresh", 3600)
			browserLogins := browserAuthenticator(t)
			mockV1Client := checkUserSessionMocks()

			captureStdout(t, func() {
				assert.NoError(t, Login(domain, tc.token, mockV1Client, io.Discard, false, tc.signup, tc.force))
			})

			assert.Equal(t, tc.browserLogins, *browserLogins)
			c, err := config.GetCurrentContext()
			assert.NoError(t, err)
			assert.Equal(t, tc.savedToken, c.Token)
		})
	}
}

func TestSwitch(t *testing.T) {
	const domain = "astronomer-dev.io"
	stubAuthConfig(t, Config{ClientID: "client-id", Audience: "audience", DomainURL: "https://auth.example.com/"})

	t.Run("a saved login is refreshed and the context switched", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		saveLogin(t, domain, "Bearer expired", "saved-refresh", -60)
		browserLogins := browserAuthenticator(t)
		stubRefresh(t, astroauth.TokenResponse{AccessToken: "refreshed", ExpiresIn: 3600}, nil)
		mockV1Client := checkUserSessionMocks()

		captureStdout(t, func() {
			assert.NoError(t, Switch("cloud.astronomer-dev.io", mockV1Client, io.Discard))
		})

		assert.Equal(t, 0, *browserLogins)
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		assert.Equal(t, domain, c.Domain)
		assert.Equal(t, "Bearer refreshed", c.Token)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("without a working login it switches and says to log in", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		browserLogins := browserAuthenticator(t)
		out := new(bytes.Buffer)

		assert.NoError(t, Switch(domain, nil, out))

		assert.Equal(t, 0, *browserLogins)
		assert.Contains(t, out.String(), "Run 'astro login astronomer-dev.io' to log in")
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		assert.Equal(t, domain, c.Domain)
	})

	t.Run("when the auth config cannot be read it switches and says why", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		previous := httpClient
		t.Cleanup(func() { httpClient = previous })
		httpClient = testUtil.NewTestClient(func(*http.Request) *http.Response {
			return &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(bytes.NewBufferString("")), Header: make(http.Header)}
		})
		out := new(bytes.Buffer)

		assert.NoError(t, Switch(domain, nil, out))

		assert.Contains(t, out.String(), "Switched to astronomer-dev.io, but could not check its login")
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		assert.Equal(t, domain, c.Domain)
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
			context, err := (&config.Context{Domain: "localhost"}).GetContext()

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

	t.Run("clears the refresh token so the next login cannot reuse it", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		assert.NoError(t, c.SetContextKey("refreshtoken", "saved-refresh"))

		Logout(c.Domain, io.Discard)

		c, err = context.GetContext(c.Domain)
		assert.NoError(t, err)
		assert.Empty(t, c.RefreshToken)
	})

	t.Run("keeps the current context when logging out of another domain", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		current, err := config.GetCurrentDomain()
		assert.NoError(t, err)

		Logout("astronomer-dev.io", io.Discard)

		after, err := config.GetCurrentDomain()
		assert.NoError(t, err)
		assert.Equal(t, current, after)
	})

	t.Run("resets the current context when logging out of it", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		current, err := config.GetCurrentDomain()
		assert.NoError(t, err)

		Logout(current, io.Discard)

		_, err = config.GetCurrentDomain()
		assert.ErrorIs(t, err, config.ErrGetHomeString)
	})

	t.Run("logs out of every host on the same tenant", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		for _, host := range []string{"pr1111.astronomer-dev.io", "pr2222.astronomer-dev.io"} {
			assert.NoError(t, context.SetContext(host))
			assert.NoError(t, (&config.Context{Domain: host}).SetAuthTenant("https://auth.example.com/", "client-id"))
		}
		for _, host := range []string{"pr1111.astronomer-dev.io", "pr2222.astronomer-dev.io"} {
			saveLogin(t, host, "Bearer shared", "shared-refresh", 3600)
		}

		Logout("pr1111.astronomer-dev.io", io.Discard)

		c, err := context.GetContext("pr2222.astronomer-dev.io")
		assert.NoError(t, err)
		assert.Empty(t, c.Token)
		assert.Empty(t, c.RefreshToken)
	})
}

func Test_writeResultToContext(t *testing.T) {
	// The expiry is bracketed rather than compared for equality. SetExpiresIn
	// reads the clock itself, so the stored value can only be pinned down to
	// the window the write happened in — asserting it against a second reading
	// taken afterwards compared two different clocks, and rounding both to the
	// second only hid that until a `.5` boundary fell between them. It failed
	// roughly as often as the write was slow, which is why CI saw it and a
	// laptop did not.
	//
	// The bracket keeps the assertion exact: the stored time must be the
	// duration under test past the clock, to the millisecond, and a write that
	// stored 1233 seconds instead of 1234 still fails.
	assertConfigContents := func(expToken string, expRefresh string, notBefore, notAfter time.Time, expUserEmail string) {
		context, err := config.GetCurrentContext()
		assert.NoError(t, err)
		// test the output on the config file
		assert.Equal(t, expToken, context.Token)
		assert.Equal(t, expRefresh, context.RefreshToken)
		expiresIn, err := context.GetExpiresIn()
		assert.NoError(t, err)
		assert.False(t, expiresIn.Before(notBefore), "expiry %s is before the window opened at %s", expiresIn, notBefore)
		assert.False(t, expiresIn.After(notAfter), "expiry %s is after the window closed at %s", expiresIn, notAfter)
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
	assertConfigContents("old_token", "", timeZero, timeZero, "")

	// apply function
	c, err = config.GetCurrentContext()
	assert.NoError(t, err)
	before := time.Now()
	err = res.writeToContext(&c)
	after := time.Now()
	assert.NoError(t, err)

	// test after changes
	expiry := time.Duration(res.ExpiresIn) * time.Second
	assertConfigContents("Bearer new_token", "new_refresh_token",
		before.Add(expiry), after.Add(expiry), "test.user@astronomer.io")
}

func TestNoOrganizationPrompt(t *testing.T) {
	t.Run("offers a free organization on a real Astro host", func(t *testing.T) {
		out := new(bytes.Buffer)
		prompt := noOrganizationPrompt("astronomer.io", out)
		assert.Equal(t, "No organization found. Create your own free organization now", prompt)
		assert.Empty(t, out.String())
	})

	t.Run("points a preview environment at its existing data and offers a temporary org", func(t *testing.T) {
		out := new(bytes.Buffer)
		prompt := noOrganizationPrompt("pr41517.astronomer-dev.io", out)
		assert.Contains(t, out.String(), "This is a preview environment")
		assert.Contains(t, out.String(), "astro login pr41517 --force")
		assert.Contains(t, out.String(), "the account it was set up with")
		assert.NotContains(t, out.String(), "@", "the message names no account")
		assert.Contains(t, prompt, "temporary organization in pr41517")
	})
}

func TestWriteToContextKeepsTokenOnlyLoginOnItsOwnHost(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	for _, host := range []string{"pr1111.astronomer-dev.io", "pr3333.astronomer-dev.io"} {
		assert.NoError(t, context.SetContext(host))
		assert.NoError(t, (&config.Context{Domain: host}).SetAuthTenant("https://auth.example.com/", "client-id"))
	}
	saveLogin(t, "pr1111.astronomer-dev.io", "Bearer browser", "browser-refresh", 3600)

	c := config.Context{Domain: "pr3333.astronomer-dev.io"}
	assert.NoError(t, Result{AccessToken: "pasted", ExpiresIn: 3600, UserEmail: "user@astronomer.test"}.writeToContext(&c))

	sibling, err := context.GetContext("pr1111.astronomer-dev.io")
	assert.NoError(t, err)
	assert.Equal(t, "Bearer browser", sibling.Token)
	assert.Equal(t, "browser-refresh", sibling.RefreshToken)
	own, err := context.GetContext("pr3333.astronomer-dev.io")
	assert.NoError(t, err)
	assert.Equal(t, "Bearer pasted", own.Token)
	assert.Empty(t, own.RefreshToken)
	assert.Equal(t, "user@astronomer.test", own.UserEmail)
}
