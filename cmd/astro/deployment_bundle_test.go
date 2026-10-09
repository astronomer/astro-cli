package astro

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	astrov1alpha1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1"
	astrov1alpha1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1/mocks"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
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
	t.Run("renders JSON when -o json is set", func(t *testing.T) {
		mockAlpha := setupBundleCmdMocks(t)
		mockAlpha.On("ListBundlesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1alpha1.ListBundlesResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &astrov1alpha1.BundlesPaginated{
				TotalCount: 1,
				Bundles:    []astrov1alpha1.DeploymentBundle{{Id: "bundle-1"}},
			},
		}, nil).Once()

		out, err := execDeploymentCmd("bundle", "list", "--deployment-id", "test-id-1", "-o", "json")
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
	t.Run("passes the bundle ID arg and skips confirmation with --yes", func(t *testing.T) {
		mockAlpha := setupBundleCmdMocks(t)
		mockAlpha.On("DeleteBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1").Return(&astrov1alpha1.DeleteBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		}, nil).Once()

		out, err := execDeploymentCmd("bundle", "delete", "bundle-1", "--deployment-id", "test-id-1", "--yes")
		assert.NoError(t, err)
		assert.Contains(t, out, "Deleted bundle bundle-1")
		mockAlpha.AssertExpectations(t)
	})

	t.Run("resolves the bundle ID from --name", func(t *testing.T) {
		mockAlpha := setupBundleCmdMocks(t)
		isDag := true
		name := "my-dags"
		mockAlpha.On("ListBundlesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1alpha1.ListBundlesResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &astrov1alpha1.BundlesPaginated{
				TotalCount: 1,
				Bundles:    []astrov1alpha1.DeploymentBundle{{Id: "bundle-1", Name: &name, IsDagBundle: &isDag}},
			},
		}, nil).Once()
		mockAlpha.On("DeleteBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1").Return(&astrov1alpha1.DeleteBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		}, nil).Once()

		out, err := execDeploymentCmd("bundle", "delete", "--deployment-id", "test-id-1", "--name", "my-dags", "--yes")
		assert.NoError(t, err)
		assert.Contains(t, out, "Deleted bundle bundle-1")
		mockAlpha.AssertExpectations(t)
	})
}

// Under --output json, create and update publish the bundle as bundle list
// gives each one, and delete what it deleted, on stdout and in the tree
// production builds, where nothing binds the root's out.
func TestDeploymentBundleJSON(t *testing.T) {
	name := "my-dags"
	isDag := true
	bundle := &astrov1alpha1.DeploymentBundle{Id: "bundle-1", Name: &name, IsDagBundle: &isDag, Type: "DEPLOY"}

	t.Run("create", func(t *testing.T) {
		alpha := setupBundleCmdMocks(t)
		alpha.On("CreateBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1alpha1.CreateBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      bundle,
		}, nil).Once()

		stdout, stderr, err := execUnboundRootWith(t, astroV1Client, alpha, "deployment", "bundle", "create", "--deployment-id", "test-id-1", "--name", name, "-o", "json")
		require.NoError(t, err, "stderr:\n%s", stderr)
		var got deployment.BundleInfo
		decodeOne(t, stdout, &got)
		assert.Equal(t, "bundle-1", got.ID)
		assert.Equal(t, &name, got.Name)
		assert.NotContains(t, stderr, "bundle-1", "the result went to stderr")
		alpha.AssertExpectations(t)
	})

	t.Run("update", func(t *testing.T) {
		alpha := setupBundleCmdMocks(t)
		alpha.On("UpdateBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything).Return(&astrov1alpha1.UpdateBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      bundle,
		}, nil).Once()

		stdout, stderr, err := execUnboundRootWith(t, astroV1Client, alpha, "deployment", "bundle", "update", "bundle-1", "--deployment-id", "test-id-1", "--description", "d", "-o", "json")
		require.NoError(t, err, "stderr:\n%s", stderr)
		var got deployment.BundleInfo
		decodeOne(t, stdout, &got)
		assert.Equal(t, "bundle-1", got.ID)
		assert.NotContains(t, stderr, "bundle-1", "the result went to stderr")
		alpha.AssertExpectations(t)
	})

	t.Run("delete", func(t *testing.T) {
		alpha := setupBundleCmdMocks(t)
		alpha.On("DeleteBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1").Return(&astrov1alpha1.DeleteBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		}, nil).Once()

		stdout, stderr, err := execUnboundRootWith(t, astroV1Client, alpha, "deployment", "bundle", "delete", "bundle-1", "--deployment-id", "test-id-1", "--yes", "-o", "json")
		require.NoError(t, err, "stderr:\n%s", stderr)
		var got deployment.BundleRemoval
		decodeOne(t, stdout, &got)
		assert.Equal(t, deployment.BundleRemoval{ID: "bundle-1", DeploymentID: "test-id-1", Action: "deletion_requested"}, got)
		assert.NotContains(t, stderr, "bundle-1", "the result went to stderr")
		alpha.AssertExpectations(t)
	})

	// Without --yes there is nobody to answer under json: the question is
	// refused rather than asked, and stdout holds only the error object.
	t.Run("delete without --yes", func(t *testing.T) {
		alpha := setupBundleCmdMocks(t)

		stdout, stderr, err := execUnboundRootWith(t, astroV1Client, alpha, "deployment", "bundle", "delete", "bundle-1", "--deployment-id", "test-id-1", "-o", "json")
		require.Error(t, err)
		var got cliout.ErrorObject
		decodeOne(t, stdout, &got)
		assert.Contains(t, got.Error, "pass --yes")
		assert.Equal(t, "input_required", string(got.Kind))
		assert.NotContains(t, stderr, "Are you sure", "the question was asked")
		alpha.AssertNotCalled(t, "DeleteBundleWithResponse")
	})

	// Text is what it always was.
	t.Run("text", func(t *testing.T) {
		alpha := setupBundleCmdMocks(t)
		alpha.On("DeleteBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1").Return(&astrov1alpha1.DeleteBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		}, nil).Once()

		stdout, stderr, err := execUnboundRootWith(t, astroV1Client, alpha, "deployment", "bundle", "delete", "bundle-1", "--deployment-id", "test-id-1", "--yes")
		require.NoError(t, err, "stderr:\n%s", stderr)
		assert.Equal(t, "Deleted bundle bundle-1 from deployment test-id-1\n", stdout)
	})
}
