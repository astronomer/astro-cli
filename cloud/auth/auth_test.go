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

	"github.com/astronomer/astro-cli/context"

	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/astronomer/astro-cli/astro-client"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/stretchr/testify/assert"
)

var errMock = errors.New("mock-error")

func Test_validateDomain(t *testing.T) {
	domain := "astronomer.io"
	actual, err := ValidateDomain(domain)
	assert.NoError(t, err)
	assert.Equal(t, actual.ClientID, "5XYJZYf5xZ0eKALgBH3O08WzgfUfz7y9")
	assert.Equal(t, actual.Audience, "astronomer-ee")
	assert.Equal(t, actual.DomainURL, "https://auth.astronomer.io/")

	domain = "gcp0001.us-east4.astronomer.io" // Gen1 CLI domain
	actual, err = ValidateDomain(domain)
	assert.Error(t, err)
	assert.Errorf(t, err, "Error! Invalid domain. "+
		"Are you trying to authenticate to Astronomer Software? If so, change your current context with 'astro context switch'. ")

	domain = "fail.astronomer.io"
	actual, err = ValidateDomain(domain)
	assert.Error(t, err)
	assert.Errorf(t, err, "Error! Invalid domain. "+
		"Are you trying to authenticate to Astronomer Software? If so, change your current context with 'astro context switch'. ")

	domain = "astronomer-dev.io"
	actual, err = ValidateDomain(domain)
	assert.NoError(t, err)
	assert.Equal(t, actual.ClientID, "PH3Nac2DtpSx1Tx3IGQmh2zaRbF5ubZG")
	assert.Equal(t, actual.Audience, "astronomer-ee")
	assert.Equal(t, actual.DomainURL, "https://auth.astronomer-dev.io/")

	domain = "fail.astronomer-dev.io"
	actual, err = ValidateDomain(domain)
	assert.Error(t, err)
	assert.Errorf(t, err, "Error! Invalid domain. "+
		"Are you trying to authenticate to Astronomer Software? If so, change your current context with 'astro context switch'. ")

	domain = "astronomer-stage.io"
	actual, err = ValidateDomain(domain)
	assert.NoError(t, err)
	assert.Equal(t, actual.ClientID, "jsarDat3BeDXZ1monEAeqJPOvRvterpm")
	assert.Equal(t, actual.Audience, "astronomer-ee")
	assert.Equal(t, actual.DomainURL, "https://auth.astronomer-stage.io/")

	domain = "fail.astronomer-stage.io"
	actual, err = ValidateDomain(domain)
	assert.Error(t, err)
	assert.Errorf(t, err, "Error! Invalid domain. "+
		"Are you trying to authenticate to Astronomer Software? If so, change your current context with 'astro context switch'. ")

	domain = "astronomer-perf.io"
	actual, err = ValidateDomain(domain)
	assert.NoError(t, err)
	assert.Equal(t, actual.ClientID, "3PKxm3e1ldZYP1xWh5rXbOgFzIWbxnTN")
	assert.Equal(t, actual.Audience, "astronomer-ee")
	assert.Equal(t, actual.DomainURL, "https://auth.astronomer-perf.io/")

	domain = "fail.astronomer-perf.io"
	actual, err = ValidateDomain(domain)
	assert.Error(t, err)
	assert.Errorf(t, err, "Error! Invalid domain. "+
		"Are you trying to authenticate to Astronomer Software? If so, change your current context with 'astro context switch'. ")
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
		go func() {
			time.Sleep(2 * time.Second) // time to spinup the server in authorizeCallbackHandler

			_, err = httpClient.Do("GET", "http://localhost:12345/callback?code=test", &httputil.DoOptions{}) //nolint
			assert.NoError(t, err)
		}()
		code, err := authorizeCallbackHandler()
		assert.Equal(t, "test", code)
		assert.NoError(t, err)
	})

	t.Run("timeout", func(t *testing.T) {
		callbackTimeout = 5 * time.Millisecond
		_, err := authorizeCallbackHandler()
		assert.Contains(t, err.Error(), "The operation has timed out")
	})
}

func TestAuthDeviceLogin(t *testing.T) {
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
		resp, err := mockAuthenticator.authDeviceLogin(c, astro.AuthConfig{}, false, "test-domain")
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
		_, err = mockAuthenticator.authDeviceLogin(c, astro.AuthConfig{}, false, "test-domain")
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
		_, err = mockAuthenticator.authDeviceLogin(c, astro.AuthConfig{}, false, "test-domain")
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
		resp, err := mockAuthenticator.authDeviceLogin(c, astro.AuthConfig{}, true, "test-domain")
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
		_, err = mockAuthenticator.authDeviceLogin(c, astro.AuthConfig{}, true, "test-domain")
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
		_, err = mockAuthenticator.authDeviceLogin(c, astro.AuthConfig{}, true, "test-domain")
		assert.ErrorIs(t, err, errMock)
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

func TestCheckToken(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	t.Run("success", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListUserRoleBindings").Return([]astro.RoleBinding{{Role: "SYSTEM_ADMIN"}}, nil).Once()
		mockClient.On("ListWorkspaces").Return([]astro.Workspace{{ID: "test-id"}}, nil).Once()
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := checkToken(&ctx, mockClient, buf)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})

	t.Run("list user role bindings failure", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListUserRoleBindings").Return([]astro.RoleBinding{}, errMock).Once()
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := checkToken(&ctx, mockClient, buf)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})

	t.Run("set context failure", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListUserRoleBindings").Return([]astro.RoleBinding{{Role: "SYSTEM_ADMIN"}}, nil).Once()
		ctx := config.Context{}
		buf := new(bytes.Buffer)
		err := checkToken(&ctx, mockClient, buf)
		assert.ErrorIs(t, err, config.ErrCtxConfigErr)
		mockClient.AssertExpectations(t)
	})

	t.Run("list workspace failure", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListUserRoleBindings").Return([]astro.RoleBinding{{Role: "SYSTEM_ADMIN"}}, nil).Once()
		mockClient.On("ListWorkspaces").Return([]astro.Workspace{}, errMock).Once()
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := checkToken(&ctx, mockClient, buf)
		assert.ErrorIs(t, err, errMock)
		mockClient.AssertExpectations(t)
	})

	t.Run("success with more than one workspace", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListUserRoleBindings").Return([]astro.RoleBinding{{Role: "SYSTEM_ADMIN"}}, nil).Once()
		mockClient.On("ListWorkspaces").Return([]astro.Workspace{{ID: "test-id-1"}, {ID: "test-id-2"}}, nil).Once()
		ctx := config.Context{Domain: "test-domain", LastUsedWorkspace: "test-id-1"}
		buf := new(bytes.Buffer)
		err := checkToken(&ctx, mockClient, buf)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})

	t.Run("success with workspace switch", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListUserRoleBindings").Return([]astro.RoleBinding{{Role: "SYSTEM_ADMIN"}}, nil).Once()
		mockClient.On("ListWorkspaces").Return([]astro.Workspace{{ID: "test-id-1"}, {ID: "test-id-2"}}, nil)
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := checkToken(&ctx, mockClient, buf)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})

	t.Run("success but with workspace switch failure", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListUserRoleBindings").Return([]astro.RoleBinding{{Role: "SYSTEM_ADMIN"}}, nil).Once()
		mockClient.On("ListWorkspaces").Return([]astro.Workspace{{ID: "test-id-1"}, {ID: "test-id-2"}}, nil).Once()
		mockClient.On("ListWorkspaces").Return([]astro.Workspace{}, errMock).Once()
		ctx := config.Context{Domain: "test-domain"}
		buf := new(bytes.Buffer)
		err := checkToken(&ctx, mockClient, buf)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
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
		mockClient.On("ListUserRoleBindings").Return([]astro.RoleBinding{{Role: "SYSTEM_ADMIN"}}, nil).Once()
		mockClient.On("ListWorkspaces").Return([]astro.Workspace{{ID: "test-id"}}, nil).Once()

		err := Login("astronomer.io", mockClient, os.Stdout, false)
		assert.NoError(t, err)
	})

	t.Run("invalid domain", func(t *testing.T) {
		err := Login("fail.astronomer.io", nil, os.Stdout, false)
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
		err := Login("cloud.astronomer.io", nil, os.Stdout, false)
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
		mockClient.On("ListUserRoleBindings").Return([]astro.RoleBinding{}, errMock).Once()

		err := Login("", mockClient, os.Stdout, false)
		assert.ErrorIs(t, err, errMock)
	})

	t.Run("initial login with empty config file", func(t *testing.T) {
		// initialize empty config
		testUtil.InitTestConfig(testUtil.Initial)
		// initialize the mock client
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListUserRoleBindings").Return([]astro.RoleBinding{}, nil).Once()
		mockClient.On("ListWorkspaces").Return([]astro.Workspace{{
			ID:    "test-workspace",
			Label: "something-label",
		}}, nil).Once()
		// initialize the test authenticator
		authenticator = Authenticator{
			orgChecker:      orgLookup,
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
		err = Login("astronomer.io", mockClient, os.Stdout, true)
		assert.NoError(t, err)
	})

	t.Run("domain doesn't match current context", func(t *testing.T) {
		// initialize empty config
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		// initialize the mock client
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListUserRoleBindings").Return([]astro.RoleBinding{}, nil).Once()
		mockClient.On("ListWorkspaces").Return([]astro.Workspace{{
			ID:    "test-workspace",
			Label: "something-label",
		}}, nil).Once()
		// initialize the test authenticator
		authenticator = Authenticator{
			orgChecker:      orgLookup,
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
		err := Login("astronomer.io", mockClient, os.Stdout, true)
		assert.NoError(t, err)
		// assert that everything got set in the right spot
		domainContext, err := context.GetContext("astronomer.io")
		assert.NoError(t, err)
		currentContext, err := context.GetContext("localhost")
		assert.NoError(t, err)
		assert.Equal(t, domainContext.Token, "Bearer access_token")
		assert.Equal(t, currentContext.Token, "token")
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
		assertions := func(expIsSystemAdmin bool, expUserEmail string, expToken string) {
			contexts, err := config.GetContexts()
			assert.NoError(t, err)
			context := contexts.Contexts["localhost"]

			isSystemAdmin, err := context.GetSystemAdmin()
			assert.NoError(t, err)
			assert.Equal(t, expIsSystemAdmin, isSystemAdmin)
			assert.Equal(t, expUserEmail, context.UserEmail)
			assert.Equal(t, expToken, context.Token)
		}
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetSystemAdmin(true)
		assert.NoError(t, err)
		err = c.SetContextKey("user_email", "test.user@astronomer.io")
		assert.NoError(t, err)
		err = c.SetContextKey("token", "Bearer some-token")
		assert.NoError(t, err)
		// test before
		assertions(true, "test.user@astronomer.io", "Bearer some-token")

		// log out
		c, err = config.GetCurrentContext()
		assert.NoError(t, err)
		Logout(c.Domain, os.Stdout)

		// test after logout
		assertions(false, "", "")
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
