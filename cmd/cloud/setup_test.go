package cloud

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	astro "github.com/astronomer/astro-cli/astro-client"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	"github.com/astronomer/astro-cli/context"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestSetup(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	t.Run("login cmd", func(t *testing.T) {
		cmd := &cobra.Command{Use: "login"}
		cmd, err := cmd.ExecuteC()
		assert.NoError(t, err)
		err = Setup(cmd, []string{}, nil)
		assert.NoError(t, err)
	})

	t.Run("dev cmd", func(t *testing.T) {
		cmd := &cobra.Command{Use: "dev"}
		cmd, err := cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "astro"}
		rootCmd.AddCommand(cmd)

		err = Setup(cmd, []string{}, nil)
		assert.NoError(t, err)
	})

	t.Run("flow cmd", func(t *testing.T) {
		cmd := &cobra.Command{Use: "flow"}
		cmd, err := cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "astro"}
		rootCmd.AddCommand(cmd)

		err = Setup(cmd, []string{}, nil)
		assert.NoError(t, err)
	})

	t.Run("help cmd", func(t *testing.T) {
		cmd := &cobra.Command{Use: "help"}
		cmd, err := cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "astro"}
		rootCmd.AddCommand(cmd)

		err = Setup(cmd, []string{}, nil)
		assert.NoError(t, err)
	})

	t.Run("version cmd", func(t *testing.T) {
		cmd := &cobra.Command{Use: "version"}
		cmd, err := cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "astro"}
		rootCmd.AddCommand(cmd)

		err = Setup(cmd, []string{}, nil)
		assert.NoError(t, err)
	})

	t.Run("context cmd", func(t *testing.T) {
		cmd := &cobra.Command{Use: "list"}
		cmd, err := cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "context"}
		rootCmd.AddCommand(cmd)

		err = Setup(cmd, []string{}, nil)
		assert.NoError(t, err)
	})

	t.Run("completion cmd", func(t *testing.T) {
		cmd := &cobra.Command{Use: "generate"}
		cmd, err := cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "completion"}
		rootCmd.AddCommand(cmd)

		err = Setup(cmd, []string{}, nil)
		assert.NoError(t, err)
	})

	t.Run("deploy cmd", func(t *testing.T) {
		cmd := &cobra.Command{Use: "deploy"}
		cmd, err := cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "astro"}
		rootCmd.AddCommand(cmd)

		authLogin = func(domain, id, token string, client astro.Client, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}

		err = Setup(cmd, []string{}, nil)
		assert.NoError(t, err)
	})

	t.Run("use API token", func(t *testing.T) {
		mockDeplyResp := []astro.Deployment{
			{
				ID:        "test-id",
				Workspace: astro.Workspace{ID: "workspace-id"},
			},
		}

		mockOrgResp := []astro.Organization{
			{
				ID:   "test-org-id",
				Name: "test-org-name",
			},
		}

		mockClient := new(astro_mocks.Client)
		mockClient.On("GetOrganizations").Return(mockOrgResp, nil).Once()
		mockClient.On("ListDeployments", mockOrgResp[0].ID, "").Return(mockDeplyResp, nil).Once()

		cmd := &cobra.Command{Use: "deploy"}
		cmd, err := cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "astro"}
		rootCmd.AddCommand(cmd)

		authLogin = func(domain, id, token string, client astro.Client, out io.Writer, shouldDisplayLoginLink bool) error {
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

		err = Setup(cmd, []string{}, mockClient)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})

	t.Run("use API token for virtual runtime", func(t *testing.T) {
		mockOrgResp := []astro.Organization{
			{
				ID:   "test-org-id",
				Name: "test-org-name",
			},
		}

		mockClient := new(astro_mocks.Client)
		mockClient.On("GetOrganizations").Return(mockOrgResp, nil).Once()

		cmd := &cobra.Command{Use: "deploy"}
		cmd, err := cmd.ExecuteC()
		assert.NoError(t, err)

		rootCmd := &cobra.Command{Use: "astro"}
		rootCmd.AddCommand(cmd)

		authLogin = func(domain, id, token string, client astro.Client, out io.Writer, shouldDisplayLoginLink bool) error {
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

		err = Setup(cmd, []string{"vr-id"}, mockClient)
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

		mockOrgResp := []astro.Organization{
			{
				ID:   "test-org-id",
				Name: "test-org-name",
			},
		}

		mockClient := new(astro_mocks.Client)
		mockClient.On("GetOrganizations").Return(mockOrgResp, nil).Once()
		mockClient.On("ListDeployments", mockOrgResp[0].ID, "").Return(mockDeplyResp, nil).Once()

		authLogin = func(domain, id, token string, client astro.Client, out io.Writer, shouldDisplayLoginLink bool) error {
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
		_, err = checkAPIKeys(mockClient, []string{})
		assert.NoError(t, err)
	})
}
