package cloud

import (
	"bytes"
	"net/http"
	"testing"
	"time"

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

func execBundleCmd(args ...string) (string, error) {
	buf := new(bytes.Buffer)
	cmd := newBundleRootCmd(buf)
	cmd.SetOut(buf)
	cmd.SetArgs(args)
	testUtil.SetupOSArgsForGinkgo()
	_, err := cmd.ExecuteC()
	return buf.String(), err
}

func TestBundleCreateCmd(t *testing.T) {
	t.Run("creates a DAG bundle and resolves the workspace deployment", func(t *testing.T) {
		mockAlpha := setupBundleCmdMocks(t)
		mockAlpha.On("CreateBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(func(req astrov1alpha1.CreateBundleRequest) bool {
			return req.Type == astrov1alpha1.CreateBundleRequestTypeDEPLOY && req.IsDagBundle != nil && *req.IsDagBundle && req.Name != nil && *req.Name == "my-dags"
		})).Return(&astrov1alpha1.CreateBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.DeploymentBundle{Id: "bundle-1"},
		}, nil).Once()

		out, err := execBundleCmd("create", "--deployment-id", "test-id-1", "--name", "my-dags")
		assert.NoError(t, err)
		assert.Contains(t, out, "Created bundle bundle-1")
		mockAlpha.AssertExpectations(t)
	})

	t.Run("creates a file bundle with --type file", func(t *testing.T) {
		mockAlpha := setupBundleCmdMocks(t)
		mockAlpha.On("CreateBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(func(req astrov1alpha1.CreateBundleRequest) bool {
			return req.Type == astrov1alpha1.CreateBundleRequestTypeFILE && req.Name != nil && *req.Name == "my-files"
		})).Return(&astrov1alpha1.CreateBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.DeploymentBundle{Id: "bundle-2"},
		}, nil).Once()

		out, err := execBundleCmd("create", "--deployment-id", "test-id-1", "--name", "my-files", "--type", "file")
		assert.NoError(t, err)
		assert.Contains(t, out, "Created bundle bundle-2")
		mockAlpha.AssertExpectations(t)
	})

	t.Run("rejects --name together with --mount-path", func(t *testing.T) {
		mockAlpha := setupBundleCmdMocks(t)

		_, err := execBundleCmd("create", "--deployment-id", "test-id-1", "--name", "a", "--mount-path", "/b")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exactly one of --name")
		mockAlpha.AssertNotCalled(t, "CreateBundleWithResponse")
	})
}

func TestBundleListCmd(t *testing.T) {
	t.Run("renders JSON when --json is set", func(t *testing.T) {
		mockAlpha := setupBundleCmdMocks(t)
		mockAlpha.On("ListBundlesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1alpha1.ListBundlesResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &astrov1alpha1.BundlesPaginated{
				TotalCount: 1,
				Bundles:    []astrov1alpha1.DeploymentBundle{{Id: "bundle-1"}},
			},
		}, nil).Once()

		out, err := execBundleCmd("list", "--deployment-id", "test-id-1", "--json")
		assert.NoError(t, err)
		assert.Contains(t, out, "bundles")
		assert.Contains(t, out, "bundle-1")
		mockAlpha.AssertExpectations(t)
	})
}

func TestBundleUpdateCmd(t *testing.T) {
	t.Run("passes the bundle ID arg and description to the API", func(t *testing.T) {
		mockAlpha := setupBundleCmdMocks(t)
		mockAlpha.On("UpdateBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(req astrov1alpha1.UpdateBundleRequest) bool {
			return req.Description != nil && *req.Description == "new desc"
		})).Return(&astrov1alpha1.UpdateBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.DeploymentBundle{Id: "bundle-1"},
		}, nil).Once()

		out, err := execBundleCmd("update", "bundle-1", "--deployment-id", "test-id-1", "--description", "new desc")
		assert.NoError(t, err)
		assert.Contains(t, out, "Updated bundle bundle-1")
		mockAlpha.AssertExpectations(t)
	})
}

func TestBundleDeleteCmd(t *testing.T) {
	t.Run("passes the bundle ID arg and skips confirmation with --force", func(t *testing.T) {
		mockAlpha := setupBundleCmdMocks(t)
		mockAlpha.On("DeleteBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1").Return(&astrov1alpha1.DeleteBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		}, nil).Once()

		out, err := execBundleCmd("delete", "bundle-1", "--deployment-id", "test-id-1", "--force")
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

		out, err := execBundleCmd("rm", "--deployment-id", "test-id-1", "--name", "my-dags", "--force")
		assert.NoError(t, err)
		assert.Contains(t, out, "Deleted bundle bundle-1")
		mockAlpha.AssertExpectations(t)
	})
}

func TestBundleRollbackCmd(t *testing.T) {
	t.Run("rolls the bundle named by --name back to --version", func(t *testing.T) {
		mockAlpha := setupBundleCmdMocks(t)
		name, isDag := "my-files", true
		mockAlpha.On("ListBundlesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1alpha1.ListBundlesResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.BundlesPaginated{TotalCount: 1, Bundles: []astrov1alpha1.DeploymentBundle{{Id: "bundle-1", Name: &name, IsDagBundle: &isDag}}},
		}, nil)
		mockAlpha.On("RollbackBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(req astrov1alpha1.RollbackBundleRequest) bool {
			return req.Version == "old" && req.Description != nil && *req.Description == "undo"
		})).Return(&astrov1alpha1.RollbackBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.DeploymentBundle{Id: "bundle-1"},
		}, nil).Once()

		out, err := execBundleCmd("rollback", "--deployment-id", "test-id-1", "--name", "my-files", "--version", "old", "--description", "undo")
		assert.NoError(t, err)
		assert.Contains(t, out, "Rolled back bundle bundle-1 on deployment test-id-1 to version old")
		mockAlpha.AssertExpectations(t)
	})

	t.Run("requires --version", func(t *testing.T) {
		mockAlpha := setupBundleCmdMocks(t)

		_, err := execBundleCmd("rollback", "bundle-1", "--deployment-id", "test-id-1")
		assert.ErrorContains(t, err, `required flag(s) "version" not set`)
		mockAlpha.AssertNotCalled(t, "RollbackBundleWithResponse")
	})
}

func TestBundleVersionsListCmd(t *testing.T) {
	t.Run("lists the bundle's versions newest first, up to --limit", func(t *testing.T) {
		mockAlpha := setupBundleCmdMocks(t)
		path := "dags/a.py"
		mockAlpha.On("ListBundleVersionsWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.ListBundleVersionsParams) bool {
			return *p.Limit == 2 && *p.Offset == 0
		})).Return(&astrov1alpha1.ListBundleVersionsResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &astrov1alpha1.BundleVersionsPaginated{
				TotalCount: 5,
				Versions: []astrov1alpha1.BundleVersion{
					{Version: "v5", Action: astrov1alpha1.BundleVersionActionWRITEFILE, ActionPath: &path, CreatedAt: time.Now()},
					{Version: "v4", Action: astrov1alpha1.BundleVersionActionROLLBACK, CreatedAt: time.Now()},
				},
			},
		}, nil).Once()

		out, err := execBundleCmd("versions", "list", "bundle-1", "--deployment-id", "test-id-1", "--limit", "2")
		assert.NoError(t, err)
		assert.Contains(t, out, "VERSION")
		assert.Contains(t, out, "v5")
		assert.Contains(t, out, "dags/a.py")
		assert.Contains(t, out, "ROLLBACK")
		mockAlpha.AssertExpectations(t)
	})

	t.Run("renders JSON when --json is set", func(t *testing.T) {
		mockAlpha := setupBundleCmdMocks(t)
		mockAlpha.On("ListBundleVersionsWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything).Return(&astrov1alpha1.ListBundleVersionsResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &astrov1alpha1.BundleVersionsPaginated{
				TotalCount: 1,
				Versions:   []astrov1alpha1.BundleVersion{{Version: "v1", Action: astrov1alpha1.BundleVersionActionDEPLOY}},
			},
		}, nil).Once()

		out, err := execBundleCmd("version", "ls", "bundle-1", "--deployment-id", "test-id-1", "--json")
		assert.NoError(t, err)
		assert.Contains(t, out, `"versions"`)
		assert.Contains(t, out, `"v1"`)
		mockAlpha.AssertExpectations(t)
	})
}
