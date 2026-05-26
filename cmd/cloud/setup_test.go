package cloud

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/astro-client-v1"
	astrov1_mocks "github.com/astronomer/astro-cli/astro-client-v1/mocks"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/astronomer/astro-cli/pkg/util"
)

var (
	errorLogin              = errors.New("failed to login")
	mockOrganizationProduct = astrov1.OrganizationProductHYBRID
)

func TestSetup(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
	t.Run("login cmd", func(t *testing.T) {
		cmd := &cobra.Command{Use: "login"}
		cmd, err := cmd.ExecuteC()
		assert.NoError(t, err)
		err = Setup(cmd, nil)
		assert.NoError(t, err)
	})

	t.Run("dev cmd", func(t *testing.T) {
		cmd := &cobra.Command{Use: "dev"}
		cmd, err := cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "astro"}
		rootCmd.AddCommand(cmd)

		err = Setup(cmd, nil)
		assert.NoError(t, err)
	})

	t.Run("dev cmd with workspace flag set", func(t *testing.T) {
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetContextKey("domain", "astronomer.io")
		assert.NoError(t, err)
		cmd := &cobra.Command{Use: "dev"}
		cmd.Flags().StringVarP(&workspaceID, "workspace-id", "w", "test-workspace-id", "")
		cmd, err = cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "astro"}
		rootCmd.AddCommand(cmd)

		authLogin = func(domain, token string, astroV1Client astrov1.APIClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}

		err = Setup(cmd, nil)
		assert.NoError(t, err)
	})

	t.Run("dev cmd with deployment flag set", func(t *testing.T) {
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetContextKey("domain", "astronomer.io")
		assert.NoError(t, err)
		cmd := &cobra.Command{Use: "dev"}
		cmd.Flags().StringVarP(&workspaceID, "deployment-id", "w", "test-deployment-id", "")
		cmd, err = cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "astro"}
		rootCmd.AddCommand(cmd)

		authLogin = func(domain, token string, astroV1Client astrov1.APIClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}

		err = Setup(cmd, nil)
		assert.NoError(t, err)
	})

	t.Run("flow cmd", func(t *testing.T) {
		cmd := &cobra.Command{Use: "flow"}
		cmd, err := cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "astro"}
		rootCmd.AddCommand(cmd)

		err = Setup(cmd, nil)
		assert.NoError(t, err)
	})

	t.Run("help cmd", func(t *testing.T) {
		cmd := &cobra.Command{Use: "help"}
		cmd, err := cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "astro"}
		rootCmd.AddCommand(cmd)

		err = Setup(cmd, nil)
		assert.NoError(t, err)
	})

	t.Run("version cmd", func(t *testing.T) {
		cmd := &cobra.Command{Use: "version"}
		cmd, err := cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "astro"}
		rootCmd.AddCommand(cmd)

		err = Setup(cmd, nil)
		assert.NoError(t, err)
	})

	t.Run("context cmd", func(t *testing.T) {
		cmd := &cobra.Command{Use: "list"}
		cmd, err := cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "context"}
		rootCmd.AddCommand(cmd)

		err = Setup(cmd, nil)
		assert.NoError(t, err)
	})

	t.Run("completion cmd", func(t *testing.T) {
		cmd := &cobra.Command{Use: "generate"}
		cmd, err := cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "completion"}
		rootCmd.AddCommand(cmd)

		err = Setup(cmd, nil)
		assert.NoError(t, err)
	})

	t.Run("deployment cmd", func(t *testing.T) {
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetContextKey("domain", "astronomer.io")
		assert.NoError(t, err)
		cmd := &cobra.Command{Use: "inspect"}
		cmd, err = cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "deployment"}
		rootCmd.AddCommand(cmd)

		authLogin = func(domain, token string, astroV1Client astrov1.APIClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}

		err = Setup(cmd, nil)
		assert.NoError(t, err)
	})

	t.Run("deploy cmd", func(t *testing.T) {
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetContextKey("domain", "astronomer.io")
		assert.NoError(t, err)
		cmd := &cobra.Command{Use: "deploy"}
		cmd, err = cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "astro"}
		rootCmd.AddCommand(cmd)

		authLogin = func(domain, token string, astroV1Client astrov1.APIClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}

		err = Setup(cmd, nil)
		assert.NoError(t, err)
	})

	t.Run("use API token", func(t *testing.T) {
		mockOrgsResponse := astrov1.ListOrganizationsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astrov1.OrganizationsPaginated{
				Organizations: []astrov1.Organization{
					{Name: "test-org", Id: "test-org-id", Product: &mockOrganizationProduct},
				},
			},
		}
		mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOrgsResponse, nil).Once()
		mockClaims := util.CustomClaims{
			Permissions: []string{
				"workspaceId:workspace-id",
				"organizationId:org-ID",
			},
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer:    "test-issuer",
				Subject:   "test-subject",
				Audience:  jwt.ClaimStrings{"audience1", "audience2"},         // Audience can be a single string or an array of strings
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)), // Set expiration date 24 hours from now
				NotBefore: jwt.NewNumericDate(time.Now()),                     // Set not before to current time
				IssuedAt:  jwt.NewNumericDate(time.Now()),                     // Set issued at to current time
				ID:        "test-id",
			},
		}
		parseAPIToken = func(astroAPIToken string) (*util.CustomClaims, error) {
			return &mockClaims, nil
		}

		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetContextKey("domain", "astronomer.io")
		assert.NoError(t, err)

		cmd := &cobra.Command{Use: "deploy"}
		cmd, err = cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "astro"}
		rootCmd.AddCommand(cmd)

		t.Setenv("ASTRO_API_TOKEN", "token")

		err = Setup(cmd, mockV1Client)
		assert.NoError(t, err)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("using a bad API token will throw error", func(t *testing.T) {
		parseAPIToken = func(astroAPIToken string) (*util.CustomClaims, error) {
			return nil, errors.New("bad token")
		}

		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetContextKey("domain", "astronomer.io")
		assert.NoError(t, err)

		cmd := &cobra.Command{Use: "deploy"}
		cmd, err = cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "astro"}
		rootCmd.AddCommand(cmd)

		t.Setenv("ASTRO_API_TOKEN", "bad token")

		err = Setup(cmd, mockV1Client)
		assert.Error(t, err)
	})

	t.Run("using a empty API token will skip api token check and go to auth login", func(t *testing.T) {
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetContextKey("domain", "astronomer.io")
		assert.NoError(t, err)

		cmd := &cobra.Command{Use: "deploy"}
		cmd, err = cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "astro"}
		rootCmd.AddCommand(cmd)

		authLogin = func(domain, token string, astroV1Client astrov1.APIClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}

		t.Setenv("ASTRO_API_TOKEN", "")

		err = Setup(cmd, mockV1Client)
		assert.NoError(t, err)
	})

	t.Run("use API key", func(t *testing.T) {
		mockOrgsResponse := astrov1.ListOrganizationsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astrov1.OrganizationsPaginated{
				Organizations: []astrov1.Organization{
					{Name: "test-org", Id: "test-org-id", Product: &mockOrganizationProduct},
				},
			},
		}
		mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOrgsResponse, nil).Once()
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()

		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetContextKey("domain", "astronomer.io")
		assert.NoError(t, err)

		cmd := &cobra.Command{Use: "deploy"}
		cmd, err = cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "astro"}
		rootCmd.AddCommand(cmd)

		authLogin = func(domain, token string, astroV1Client astrov1.APIClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}

		t.Setenv("ASTRONOMER_KEY_ID", "key")
		t.Setenv("ASTRONOMER_KEY_SECRET", "secret")

		mockResp := TokenResponse{
			AccessToken: "test-token",
			IDToken:     "test-id",
		}
		jsonResponse, err := json.Marshal(mockResp)
		assert.NoError(t, err)

		client = testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		err = Setup(cmd, mockV1Client)
		assert.NoError(t, err)
		mockV1Client.AssertExpectations(t)
		mockV1Client.AssertExpectations(t)
	})
}

func TestCheckAPIKeys(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
	t.Run("test context switch", func(t *testing.T) {
		mockOrgsResponse := astrov1.ListOrganizationsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astrov1.OrganizationsPaginated{
				Organizations: []astrov1.Organization{
					{Name: "test-org", Id: "test-org-id", Product: &mockOrganizationProduct},
				},
			},
		}
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetContextKey("domain", "astronomer.io")
		assert.NoError(t, err)

		mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOrgsResponse, nil).Once()
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()

		authLogin = func(domain, token string, astroV1Client astrov1.APIClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}

		t.Setenv("ASTRONOMER_KEY_ID", "key")
		t.Setenv("ASTRONOMER_KEY_SECRET", "secret")

		mockResp := TokenResponse{
			AccessToken: "test-token",
			IDToken:     "test-id",
		}
		jsonResponse, err := json.Marshal(mockResp)
		assert.NoError(t, err)

		client = testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})

		// Switch context
		domain := "astronomer-dev.io"
		err = context.Switch(domain)
		assert.NoError(t, err)

		// run CheckAPIKeys
		_, err = checkAPIKeys(mockV1Client, false)
		assert.NoError(t, err)
		mockV1Client.AssertExpectations(t)
		mockV1Client.AssertExpectations(t)
	})
}

func TestCheckToken(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	t.Run("test check token", func(t *testing.T) {
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		authLogin = func(domain, token string, astroV1Client astrov1.APIClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}
		// run checkToken
		err := checkToken(mockV1Client, nil)
		assert.NoError(t, err)
	})
	t.Run("trigger login when no token is found", func(t *testing.T) {
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		authLogin = func(domain, token string, astroV1Client astrov1.APIClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return errorLogin
		}

		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("token", "")
		// run checkToken
		err = checkToken(mockV1Client, nil)
		assert.Contains(t, err.Error(), "failed to login")
	})
}

func TestCheckAPIToken(t *testing.T) {
	var mockClaims util.CustomClaims
	var permissions []string
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockOrgsResponse := astrov1.ListOrganizationsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.OrganizationsPaginated{
			Organizations: []astrov1.Organization{
				{Name: "test-org", Id: "test-org-id", Product: &mockOrganizationProduct},
			},
			TotalCount: 1,
			Limit:      100,
			Offset:     0,
		},
	}
	permissions = []string{
		"workspaceId:workspace-id",
		"organizationId:org-ID",
	}
	mockClaims = util.CustomClaims{
		Permissions: permissions,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "test-issuer",
			Subject:   "test-subject",
			Audience:  jwt.ClaimStrings{"audience1", "audience2"},         // Audience can be a single string or an array of strings
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)), // Set expiration date 24 hours from now
			NotBefore: jwt.NewNumericDate(time.Now()),                     // Set not before to current time
			IssuedAt:  jwt.NewNumericDate(time.Now()),                     // Set issued at to current time
			ID:        "test-id",
		},
	}
	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)

	t.Run("test context switch", func(t *testing.T) {
		authLogin = func(domain, token string, astroV1Client astrov1.APIClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}

		parseAPIToken = func(astroAPIToken string) (*util.CustomClaims, error) {
			return &mockClaims, nil
		}

		mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOrgsResponse, nil).Once()

		t.Setenv("ASTRO_API_TOKEN", "token")

		// Switch context
		domain := "astronomer-dev.io"
		err := context.Switch(domain)
		assert.NoError(t, err)

		// run checkAPIToken
		_, err = checkAPIToken(true, mockV1Client)
		assert.NoError(t, err)
	})

	t.Run("failed to parse api token", func(t *testing.T) {
		authLogin = func(domain, token string, astroV1Client astrov1.APIClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}

		parseAPIToken = func(astroAPIToken string) (*util.CustomClaims, error) {
			return nil, errors.New("Failed to parse token")
		}

		mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOrgsResponse, nil).Once()

		t.Setenv("ASTRO_API_TOKEN", "token")

		// Switch context
		domain := "astronomer-dev.io"
		err := context.Switch(domain)
		assert.NoError(t, err)

		// run checkAPIToken
		_, err = checkAPIToken(true, mockV1Client)
		assert.Error(t, err)
	})
	t.Run("unable to fetch current context", func(t *testing.T) {
		authLogin = func(domain, token string, astroV1Client astrov1.APIClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}

		parseAPIToken = func(astroAPIToken string) (*util.CustomClaims, error) {
			return &mockClaims, nil
		}

		mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOrgsResponse, nil).Once()

		t.Setenv("ASTRO_API_TOKEN", "token")
		err := config.ResetCurrentContext()
		assert.NoError(t, err)

		// run checkAPIToken
		_, err = checkAPIToken(true, mockV1Client)
		assert.NoError(t, err)
	})

	t.Run("bad claims", func(t *testing.T) {
		permissions = []string{}
		mockClaims = util.CustomClaims{
			Permissions: permissions,
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer:    "test-issuer",
				Subject:   "test-subject",
				Audience:  jwt.ClaimStrings{"audience1", "audience2"},
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
				NotBefore: jwt.NewNumericDate(time.Now()),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				ID:        "test-id",
			},
		}

		authLogin = func(domain, token string, astroV1Client astrov1.APIClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}

		parseAPIToken = func(astroAPIToken string) (*util.CustomClaims, error) {
			return &mockClaims, nil
		}

		mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOrgsResponse, nil).Once()

		t.Setenv("ASTRO_API_TOKEN", "token")

		// Switch context
		domain := "astronomer-dev.io"
		err := context.Switch(domain)
		assert.NoError(t, err)

		// run checkAPIToken
		_, err = checkAPIToken(false, mockV1Client)
		assert.ErrorIs(t, err, errNotAPIToken)
	})

	t.Run("expired token", func(t *testing.T) {
		permissions = []string{
			"workspaceId:workspace-id",
			"organizationId:org-ID",
		}
		mockClaims = util.CustomClaims{
			Permissions: permissions,
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer:    "test-issuer",
				Subject:   "test-subject",
				Audience:  jwt.ClaimStrings{"audience1", "audience2"},
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
				NotBefore: jwt.NewNumericDate(time.Now()),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				ID:        "test-id",
			},
		}

		authLogin = func(domain, token string, astroV1Client astrov1.APIClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}

		parseAPIToken = func(astroAPIToken string) (*util.CustomClaims, error) {
			return &mockClaims, nil
		}

		mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOrgsResponse, nil).Once()

		t.Setenv("ASTRO_API_TOKEN", "token")

		// Switch context
		domain := "astronomer-dev.io"
		err := context.Switch(domain)
		assert.NoError(t, err)

		// run checkAPIToken
		_, err = checkAPIToken(true, mockV1Client)
		assert.ErrorIs(t, err, errExpiredAPIToken)
	})

	t.Run("non-expiry token", func(t *testing.T) {
		permissions = []string{
			"workspaceId:workspace-id",
			"organizationId:org-ID",
		}
		mockClaims = util.CustomClaims{
			Permissions: permissions,
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer:    "test-issuer",
				Subject:   "test-subject",
				Audience:  jwt.ClaimStrings{"audience1", "audience2"},
				NotBefore: jwt.NewNumericDate(time.Now()),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				ID:        "test-id",
			},
		}

		authLogin = func(domain, token string, astroV1Client astrov1.APIClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}

		parseAPIToken = func(astroAPIToken string) (*util.CustomClaims, error) {
			return &mockClaims, nil
		}

		mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOrgsResponse, nil).Once()

		t.Setenv("ASTRO_API_TOKEN", "token")

		// Switch context
		domain := "astronomer-dev.io"
		err := context.Switch(domain)
		assert.NoError(t, err)

		// run checkAPIToken
		_, err = checkAPIToken(true, mockV1Client)
		assert.NoError(t, err)
	})
}
