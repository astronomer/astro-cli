package cloud

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	astrov1_mocks "github.com/astronomer/astro-cli/astro-client-v1/mocks"
	astrov1alpha1 "github.com/astronomer/astro-cli/astro-client-v1alpha1"
	astrov1alpha1_mocks "github.com/astronomer/astro-cli/astro-client-v1alpha1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// setupBundleCmdMocks wires both clients and the deployment lookup (ListDeployments
// + GetDeploymentByID) that every bundle subcommand performs to resolve --deployment-id.
func setupBundleCmdMocks(t *testing.T) *astrov1alpha1_mocks.ClientWithResponsesInterface {
	t.Helper()
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockV1 := new(astrov1_mocks.ClientWithResponsesInterface)
	mockV1.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil)
	mockV1.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil)
	astroV1Client = mockV1

	mockAlpha := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
	astroV1Alpha1Client = mockAlpha
	return mockAlpha
}

func TestDeploymentBundleCreateCmd(t *testing.T) {
	t.Run("creates a DAG bundle and resolves the workspace deployment", func(t *testing.T) {
		mockAlpha := setupBundleCmdMocks(t)
		mockAlpha.On("CreateBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(func(req astrov1alpha1.CreateBundleRequest) bool {
			return req.IsDagBundle != nil && *req.IsDagBundle && req.Name != nil && *req.Name == "my-dags"
		})).Return(&astrov1alpha1.CreateBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.DeploymentBundle{Id: "bundle-1"},
		}, nil).Once()

		out, err := execDeploymentCmd("bundle", "create", "--deployment-id", "test-id-1", "--name", "my-dags")
		assert.NoError(t, err)
		assert.Contains(t, out, "Created bundle bundle-1")
		mockAlpha.AssertExpectations(t)
	})

	t.Run("rejects --name together with --mount-path", func(t *testing.T) {
		mockAlpha := setupBundleCmdMocks(t)

		_, err := execDeploymentCmd("bundle", "create", "--deployment-id", "test-id-1", "--name", "a", "--mount-path", "/b")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exactly one of --name")
		mockAlpha.AssertNotCalled(t, "CreateBundleWithResponse")
	})
}

func TestDeploymentBundleListCmd(t *testing.T) {
	t.Run("renders JSON when --json is set", func(t *testing.T) {
		mockAlpha := setupBundleCmdMocks(t)
		mockAlpha.On("ListBundlesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1alpha1.ListBundlesResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &astrov1alpha1.BundlesPaginated{
				TotalCount: 1,
				Bundles:    []astrov1alpha1.DeploymentBundle{{Id: "bundle-1"}},
			},
		}, nil).Once()

		out, err := execDeploymentCmd("bundle", "list", "--deployment-id", "test-id-1", "--json")
		assert.NoError(t, err)
		assert.Contains(t, out, "bundles")
		assert.Contains(t, out, "bundle-1")
		mockAlpha.AssertExpectations(t)
	})
}

func TestDeploymentBundleUpdateCmd(t *testing.T) {
	t.Run("passes the bundle ID arg and description to the API", func(t *testing.T) {
		mockAlpha := setupBundleCmdMocks(t)
		mockAlpha.On("UpdateBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(req astrov1alpha1.UpdateBundleRequest) bool {
			return req.Description != nil && *req.Description == "new desc"
		})).Return(&astrov1alpha1.UpdateBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.DeploymentBundle{Id: "bundle-1"},
		}, nil).Once()

		out, err := execDeploymentCmd("bundle", "update", "bundle-1", "--deployment-id", "test-id-1", "--description", "new desc")
		assert.NoError(t, err)
		assert.Contains(t, out, "Updated bundle bundle-1")
		mockAlpha.AssertExpectations(t)
	})
}

func TestDeploymentBundleDeleteCmd(t *testing.T) {
	t.Run("passes the bundle ID arg and skips confirmation with --force", func(t *testing.T) {
		mockAlpha := setupBundleCmdMocks(t)
		mockAlpha.On("DeleteBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1").Return(&astrov1alpha1.DeleteBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		}, nil).Once()

		out, err := execDeploymentCmd("bundle", "delete", "bundle-1", "--deployment-id", "test-id-1", "--force")
		assert.NoError(t, err)
		assert.Contains(t, out, "Deleted bundle bundle-1")
		mockAlpha.AssertExpectations(t)
	})
}
