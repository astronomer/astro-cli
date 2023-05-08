package cloud

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	astro "github.com/astronomer/astro-cli/astro-client"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	"github.com/astronomer/astro-cli/context"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/astronomer/astro-cli/pkg/util"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/mock"
)

var (
	errorLogin              = errors.New("failed to login")
	mockOrganizationProduct = astrocore.OrganizationProductHYBRID
)

func (s *Suite) TestSetup() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	s.Run("login cmd", func() {
		testUtil.SetupOSArgsForGinkgo()
		cmd := &cobra.Command{Use: "login"}
		cmd, err := cmd.ExecuteC()
		s.NoError(err)
		err = Setup(cmd, nil, nil)
		s.NoError(err)
	})

	s.Run("dev cmd", func() {
		testUtil.SetupOSArgsForGinkgo()
		cmd := &cobra.Command{Use: "dev"}
		cmd, err := cmd.ExecuteC()
		s.NoError(err)

		rootCmd := &cobra.Command{Use: "astro"}
		rootCmd.AddCommand(cmd)

		err = Setup(cmd, nil, nil)
		s.NoError(err)
	})

	s.Run("flow cmd", func() {
		testUtil.SetupOSArgsForGinkgo()
		cmd := &cobra.Command{Use: "flow"}
		cmd, err := cmd.ExecuteC()
		s.NoError(err)

		rootCmd := &cobra.Command{Use: "astro"}
		rootCmd.AddCommand(cmd)

		err = Setup(cmd, nil, nil)
		s.NoError(err)
	})

	s.Run("help cmd", func() {
		testUtil.SetupOSArgsForGinkgo()
		cmd := &cobra.Command{Use: "help"}
		cmd, err := cmd.ExecuteC()
		s.NoError(err)

		rootCmd := &cobra.Command{Use: "astro"}
		rootCmd.AddCommand(cmd)

		err = Setup(cmd, nil, nil)
		s.NoError(err)
	})

	s.Run("version cmd", func() {
		testUtil.SetupOSArgsForGinkgo()
		cmd := &cobra.Command{Use: "version"}
		cmd, err := cmd.ExecuteC()
		s.NoError(err)

		rootCmd := &cobra.Command{Use: "astro"}
		rootCmd.AddCommand(cmd)

		err = Setup(cmd, nil, nil)
		s.NoError(err)
	})

	s.Run("context cmd", func() {
		testUtil.SetupOSArgsForGinkgo()
		cmd := &cobra.Command{Use: "list"}
		cmd, err := cmd.ExecuteC()
		s.NoError(err)

		rootCmd := &cobra.Command{Use: "context"}
		rootCmd.AddCommand(cmd)

		err = Setup(cmd, nil, nil)
		s.NoError(err)
	})

	s.Run("completion cmd", func() {
		testUtil.SetupOSArgsForGinkgo()
		cmd := &cobra.Command{Use: "generate"}
		cmd, err := cmd.ExecuteC()
		s.NoError(err)

		rootCmd := &cobra.Command{Use: "completion"}
		rootCmd.AddCommand(cmd)

		err = Setup(cmd, nil, nil)
		s.NoError(err)
	})

	s.Run("deployment cmd", func() {
		testUtil.SetupOSArgsForGinkgo()
		cmd := &cobra.Command{Use: "inspect"}
		cmd, err := cmd.ExecuteC()
		s.NoError(err)

		rootCmd := &cobra.Command{Use: "deployment"}
		rootCmd.AddCommand(cmd)

		authLogin = func(domain, token string, client astro.Client, coreClient astrocore.CoreClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}

		err = Setup(cmd, nil, nil)
		s.NoError(err)
	})

	s.Run("deploy cmd", func() {
		testUtil.SetupOSArgsForGinkgo()
		cmd := &cobra.Command{Use: "deploy"}
		cmd, err := cmd.ExecuteC()
		s.NoError(err)

		rootCmd := &cobra.Command{Use: "astro"}
		rootCmd.AddCommand(cmd)

		authLogin = func(domain, token string, client astro.Client, coreClient astrocore.CoreClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}

		err = Setup(cmd, nil, nil)
		s.NoError(err)
	})

	s.Run("use API token", func() {
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
		s.NoError(err)

		rootCmd := &cobra.Command{Use: "astro"}
		rootCmd.AddCommand(cmd)

		authLogin = func(domain, token string, client astro.Client, coreClient astrocore.CoreClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}

		s.T().Setenv("ASTRONOMER_KEY_ID", "key")
		s.T().Setenv("ASTRONOMER_KEY_SECRET", "secret")

		mockResp := TokenResponse{
			AccessToken: "test-token",
			IDToken:     "test-id",
		}
		jsonResponse, err := json.Marshal(mockResp)
		s.NoError(err)

		client = testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})

		err = Setup(cmd, mockClient, mockCoreClient)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestCheckAPIKeys() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	s.Run("test context switch", func() {
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

		s.T().Setenv("ASTRONOMER_KEY_ID", "key")
		s.T().Setenv("ASTRONOMER_KEY_SECRET", "secret")

		mockResp := TokenResponse{
			AccessToken: "test-token",
			IDToken:     "test-id",
		}
		jsonResponse, err := json.Marshal(mockResp)
		s.NoError(err)

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
		s.NoError(err)

		// run CheckAPIKeys
		_, err = checkAPIKeys(mockClient, mockCoreClient, false)
		s.NoError(err)
	})
}

func (s *Suite) TestCheckToken() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	s.Run("test check token", func() {
		mockClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)

		authLogin = func(domain, token string, client astro.Client, coreClient astrocore.CoreClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}
		// run checkToken
		err := checkToken(mockClient, mockCoreClient, nil)
		s.NoError(err)
	})

	s.Run("trigger login when no token is found", func() {
		mockClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)

		authLogin = func(domain, token string, client astro.Client, coreClient astrocore.CoreClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return errorLogin
		}

		ctx, err := context.GetCurrentContext()
		s.NoError(err)
		ctx.SetContextKey("token", "")
		// run checkToken
		err = checkToken(mockClient, mockCoreClient, nil)
		s.Contains(err.Error(), "failed to login")
	})
}

func (s *Suite) TestCheckAPIToken() {
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

	s.Run("test context switch", func() {
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

		s.T().Setenv("ASTRO_API_TOKEN", "token")

		// Switch context
		domain := "astronomer-dev.io"
		err := context.Switch(domain)
		s.NoError(err)

		// run CheckAPIKeys
		_, err = checkAPIToken(true, mockCoreClient)
		s.NoError(err)
	})

	s.Run("bad claims", func() {
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

		s.T().Setenv("ASTRO_API_TOKEN", "token")

		// Switch context
		domain := "astronomer-dev.io"
		err := context.Switch(domain)
		s.NoError(err)

		// run CheckAPIKeys
		_, err = checkAPIToken(true, mockCoreClient)
		s.ErrorIs(err, errNotAPIToken)
	})
}
