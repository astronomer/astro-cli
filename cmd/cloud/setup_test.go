package cloud

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	astro "github.com/astronomer/astro-cli/astro-client"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	"github.com/astronomer/astro-cli/context"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/astronomer/astro-cli/pkg/util"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	errorLogin              = errors.New("failed to login")
	mockOrganizationProduct = astrocore.OrganizationProductHYBRID
)

func TestSetup(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	t.Run("login cmd", func(t *testing.T) {
		testUtil.SetupOSArgsForGinkgo()
		cmd := &cobra.Command{Use: "login"}
		cmd, err := cmd.ExecuteC()
		assert.NoError(t, err)
		err = Setup(cmd, nil, nil)
		assert.NoError(t, err)
	})

	t.Run("dev cmd", func(t *testing.T) {
		testUtil.SetupOSArgsForGinkgo()
		cmd := &cobra.Command{Use: "dev"}
		cmd, err := cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "astro"}
		rootCmd.AddCommand(cmd)

		err = Setup(cmd, nil, nil)
		assert.NoError(t, err)
	})

	t.Run("flow cmd", func(t *testing.T) {
		testUtil.SetupOSArgsForGinkgo()
		cmd := &cobra.Command{Use: "flow"}
		cmd, err := cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "astro"}
		rootCmd.AddCommand(cmd)

		err = Setup(cmd, nil, nil)
		assert.NoError(t, err)
	})

	t.Run("help cmd", func(t *testing.T) {
		testUtil.SetupOSArgsForGinkgo()
		cmd := &cobra.Command{Use: "help"}
		cmd, err := cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "astro"}
		rootCmd.AddCommand(cmd)

		err = Setup(cmd, nil, nil)
		assert.NoError(t, err)
	})

	t.Run("version cmd", func(t *testing.T) {
		testUtil.SetupOSArgsForGinkgo()
		cmd := &cobra.Command{Use: "version"}
		cmd, err := cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "astro"}
		rootCmd.AddCommand(cmd)

		err = Setup(cmd, nil, nil)
		assert.NoError(t, err)
	})

	t.Run("context cmd", func(t *testing.T) {
		testUtil.SetupOSArgsForGinkgo()
		cmd := &cobra.Command{Use: "list"}
		cmd, err := cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "context"}
		rootCmd.AddCommand(cmd)

		err = Setup(cmd, nil, nil)
		assert.NoError(t, err)
	})

	t.Run("completion cmd", func(t *testing.T) {
		testUtil.SetupOSArgsForGinkgo()
		cmd := &cobra.Command{Use: "generate"}
		cmd, err := cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "completion"}
		rootCmd.AddCommand(cmd)

		err = Setup(cmd, nil, nil)
		assert.NoError(t, err)
	})

	t.Run("deployment cmd", func(t *testing.T) {
		testUtil.SetupOSArgsForGinkgo()
		cmd := &cobra.Command{Use: "inspect"}
		cmd, err := cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "deployment"}
		rootCmd.AddCommand(cmd)

		authLogin = func(domain, token string, client astro.Client, coreClient astrocore.CoreClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}

		err = Setup(cmd, nil, nil)
		assert.NoError(t, err)
	})

	t.Run("deploy cmd", func(t *testing.T) {
		testUtil.SetupOSArgsForGinkgo()
		cmd := &cobra.Command{Use: "deploy"}
		cmd, err := cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "astro"}
		rootCmd.AddCommand(cmd)

		authLogin = func(domain, token string, client astro.Client, coreClient astrocore.CoreClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}

		err = Setup(cmd, nil, nil)
		assert.NoError(t, err)
	})

	t.Run("use API token", func(t *testing.T) {
		mockDeplyResp := []astro.Deployment{
			{
				ID:        "test-id",
				Workspace: astro.Workspace{ID: "workspace-id"},
			},
		}
		mockOrgsResponse := astrocore.ListOrganizationsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &[]astrocore.Organization{
				{AuthServiceId: "auth-service-id", Id: "test-org-id", Name: "test-org-name", Product: &mockOrganizationProduct},
			},
		}
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", "test-org-id", "").Return(mockDeplyResp, nil).Once()
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOrgsResponse, nil).Once()

		cmd := &cobra.Command{Use: "deploy"}
		testUtil.SetupOSArgsForGinkgo()
		cmd, err := cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "astro"}
		rootCmd.AddCommand(cmd)

		authLogin = func(domain, token string, client astro.Client, coreClient astrocore.CoreClient, out io.Writer, shouldDisplayLoginLink bool) error {
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

		err = Setup(cmd, mockClient, mockCoreClient)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})
}

func TestCheckAPIKeys(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	t.Run("test context switch", func(t *testing.T) {
		mockDeplyResp := []astro.Deployment{
			{
				ID:        "test-id",
				Workspace: astro.Workspace{ID: "workspace-id"},
			},
		}

		mockOrgsResponse := astrocore.ListOrganizationsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &[]astrocore.Organization{
				{AuthServiceId: "auth-service-id", Id: "test-org-id", Name: "test-org-name", Product: &mockOrganizationProduct},
			},
		}
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", "test-org-id", "").Return(mockDeplyResp, nil).Once()
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOrgsResponse, nil).Once()

		authLogin = func(domain, token string, client astro.Client, coreClient astrocore.CoreClient, out io.Writer, shouldDisplayLoginLink bool) error {
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
		_, err = checkAPIKeys(mockClient, mockCoreClient, false)
		assert.NoError(t, err)
	})
}

func TestCheckToken(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	t.Run("test check token", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)

		authLogin = func(domain, token string, client astro.Client, coreClient astrocore.CoreClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}
		// run checkToken
		err := checkToken(mockClient, mockCoreClient, nil)
		assert.NoError(t, err)
	})

	t.Run("trigger login when no token is found", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)

		authLogin = func(domain, token string, client astro.Client, coreClient astrocore.CoreClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return errorLogin
		}

		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("token", "")
		// run checkToken
		err = checkToken(mockClient, mockCoreClient, nil)
		assert.Contains(t, err.Error(), "failed to login")
	})
}

func TestCheckAPIToken(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	mockOrgsResponse := astrocore.ListOrganizationsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &[]astrocore.Organization{
			{AuthServiceId: "auth-service-id", Id: "test-org-id", Name: "test-org-name", Product: &mockOrganizationProduct},
		},
	}

	t.Run("test context switch", func(t *testing.T) {
		permissions := []string{
			"",
			"workspaceId:workspace-id",
			"organizationId:org-ID",
			"orgShortName:org-short-name",
		}
		mockClaims := util.CustomClaims{
			Permissions: permissions,
		}

		authLogin = func(domain, token string, client astro.Client, coreClient astrocore.CoreClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}

		parseAPIToken = func(astroAPIToken string) (*util.CustomClaims, error) {
			return &mockClaims, nil
		}

		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOrgsResponse, nil).Once()

		t.Setenv("ASTRO_API_TOKEN", "token")

		// Switch context
		domain := "astronomer-dev.io"
		err := context.Switch(domain)
		assert.NoError(t, err)

		// run CheckAPIKeys
		_, err = checkAPIToken(true, mockCoreClient)
		assert.NoError(t, err)
	})

	t.Run("bad claims", func(t *testing.T) {
		permissions := []string{}
		mockClaims := util.CustomClaims{
			Permissions: permissions,
		}

		authLogin = func(domain, token string, client astro.Client, coreClient astrocore.CoreClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}

		parseAPIToken = func(astroAPIToken string) (*util.CustomClaims, error) {
			return &mockClaims, nil
		}

		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOrgsResponse, nil).Once()

		t.Setenv("ASTRO_API_TOKEN", "token")

		// Switch context
		domain := "astronomer-dev.io"
		err := context.Switch(domain)
		assert.NoError(t, err)

		// run CheckAPIKeys
		_, err = checkAPIToken(true, mockCoreClient)
		assert.ErrorIs(t, err, errNotAPIToken)
	})
}
