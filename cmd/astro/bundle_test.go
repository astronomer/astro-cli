package astro

import (
	"bytes"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	astrov1alpha1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1"
	astrov1alpha1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1/mocks"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/pkg/input"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func execBundleCmd(args ...string) (string, error) {
	buf := new(bytes.Buffer)
	cmd := newBundleRootCmd(buf)
	cmd.SetOut(buf)
	cmd.SilenceUsage = true
	// The real CLI registers --verbosity on the root.
	var verbosity string
	cmd.PersistentFlags().StringVar(&verbosity, "verbosity", "", "")
	cmd.SetArgs(args)
	testUtil.SetupOSArgsForGinkgo()
	_, err := cmd.ExecuteC()
	return buf.String(), err
}

func TestBundleCreateCmd(t *testing.T) {
	t.Run("creates a Dag bundle that deploys write", func(t *testing.T) {
		mockAlpha := setupBundleCmdMocks(t)
		mockAlpha.On("CreateBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(func(req astrov1alpha1.CreateBundleRequest) bool {
			return req.Type == astrov1alpha1.CreateBundleRequestTypeDEPLOY && req.IsDagBundle != nil && *req.IsDagBundle && req.Name != nil && *req.Name == "my-dags"
		})).Return(&astrov1alpha1.CreateBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.DeploymentBundle{Id: "bundle-1"},
		}, nil).Once()

		out, err := execBundleCmd("create", "--deployment", "test-id-1", "--name", "my-dags")
		assert.NoError(t, err)
		assert.Contains(t, out, "Created bundle bundle-1")
		mockAlpha.AssertExpectations(t)
	})

	t.Run("creates a file bundle with the hidden --type file", func(t *testing.T) {
		mockAlpha := setupBundleCmdMocks(t)
		mockAlpha.On("CreateBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(func(req astrov1alpha1.CreateBundleRequest) bool {
			return req.Type == astrov1alpha1.CreateBundleRequestTypeFILE && req.Name != nil && *req.Name == "my-files"
		})).Return(&astrov1alpha1.CreateBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.DeploymentBundle{Id: "bundle-2"},
		}, nil).Once()

		out, err := execBundleCmd("create", "--deployment", "test-id-1", "--name", "my-files", "--type", "file")
		assert.NoError(t, err)
		assert.Contains(t, out, "Created bundle bundle-2")
		mockAlpha.AssertExpectations(t)
	})

	t.Run("rejects --name together with --mount-path", func(t *testing.T) {
		mockAlpha := setupBundleCmdMocks(t)

		_, err := execBundleCmd("create", "--deployment", "test-id-1", "--name", "a", "--mount-path", "/b")
		assert.ErrorContains(t, err, "exactly one of --name")
		mockAlpha.AssertNotCalled(t, "CreateBundleWithResponse")
	})

	// astro bundle is new in v2: no 1.x spelling of --deployment, and --type
	// is hidden from help.
	t.Run("takes no 1.x spellings, and hides --type", func(t *testing.T) {
		root := newBundleRootCmd(&bytes.Buffer{})
		create, _, err := root.Find([]string{"create"})
		require.NoError(t, err)
		assert.Nil(t, create.InheritedFlags().Lookup("deployment-id"))
		assert.Nil(t, create.InheritedFlags().Lookup("workspace-id"))
		assert.NotNil(t, create.InheritedFlags().Lookup("deployment"))
		assert.True(t, create.Flags().Lookup("type").Hidden)
	})
}

func TestBundleListCmd(t *testing.T) {
	t.Run("renders JSON when -o json is set", func(t *testing.T) {
		mockAlpha := setupBundleCmdMocks(t)
		mockAlpha.On("ListBundlesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1alpha1.ListBundlesResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &astrov1alpha1.BundlesPaginated{
				TotalCount: 1,
				Bundles:    []astrov1alpha1.DeploymentBundle{{Id: "bundle-1"}},
			},
		}, nil).Once()

		out, err := execBundleCmd("list", "--deployment", "test-id-1", "-o", "json")
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

		out, err := execBundleCmd("update", "bundle-1", "--deployment", "test-id-1", "--description", "new desc")
		assert.NoError(t, err)
		assert.Contains(t, out, "Updated bundle bundle-1")
		mockAlpha.AssertExpectations(t)
	})
}

func TestBundleDeleteCmd(t *testing.T) {
	t.Run("passes the bundle ID arg and skips confirmation with --yes", func(t *testing.T) {
		mockAlpha := setupBundleCmdMocks(t)
		mockAlpha.On("DeleteBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1").Return(&astrov1alpha1.DeleteBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		}, nil).Once()

		out, err := execBundleCmd("delete", "bundle-1", "--deployment", "test-id-1", "--yes")
		assert.NoError(t, err)
		assert.Contains(t, out, "Requested deletion of bundle bundle-1")
		mockAlpha.AssertExpectations(t)
	})

	t.Run("resolves the bundle ID from --name, under the rm alias", func(t *testing.T) {
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

		out, err := execBundleCmd("rm", "--deployment", "test-id-1", "--name", "my-dags", "--yes")
		assert.NoError(t, err)
		assert.Contains(t, out, "Requested deletion of bundle bundle-1")
		mockAlpha.AssertExpectations(t)
	})
}

func TestBundleRollbackCmd(t *testing.T) {
	rollbackMocks := func(t *testing.T) *astrov1alpha1_mocks.ClientWithResponsesInterface {
		mockAlpha := setupBundleCmdMocks(t)
		name, isDag := "my-files", true
		mockAlpha.On("ListBundlesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1alpha1.ListBundlesResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.BundlesPaginated{TotalCount: 1, Bundles: []astrov1alpha1.DeploymentBundle{{Id: "bundle-1", Name: &name, IsDagBundle: &isDag}}},
		}, nil)
		return mockAlpha
	}

	t.Run("rolls the bundle named by --name back to --version", func(t *testing.T) {
		mockAlpha := rollbackMocks(t)
		mockAlpha.On("RollbackBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(req astrov1alpha1.RollbackBundleRequest) bool {
			return req.Version == "old" && req.Description != nil && *req.Description == "undo"
		})).Return(&astrov1alpha1.RollbackBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.DeploymentBundle{Id: "bundle-1"},
		}, nil).Once()

		out, err := execBundleCmd("rollback", "--deployment", "test-id-1", "--name", "my-files", "--version", "old", "--description", "undo", "--yes")
		assert.NoError(t, err)
		assert.Contains(t, out, "Rolled back bundle bundle-1 on Deployment test-id-1 to version old")
		mockAlpha.AssertExpectations(t)
	})

	t.Run("asks first, and does nothing when declined", func(t *testing.T) {
		mockAlpha := rollbackMocks(t)
		defer testUtil.MockUserInput(t, "n\n")()

		out, err := execBundleCmd("rollback", "bundle-1", "--deployment", "test-id-1", "--version", "old")
		assert.ErrorIs(t, err, input.ErrAborted)
		assert.NotContains(t, out, "Rolled back")
		mockAlpha.AssertNotCalled(t, "RollbackBundleWithResponse")
	})

	t.Run("requires --version", func(t *testing.T) {
		mockAlpha := rollbackMocks(t)

		_, err := execBundleCmd("rollback", "bundle-1", "--deployment", "test-id-1")
		assert.ErrorContains(t, err, `required flag(s) "version" not set`)
		mockAlpha.AssertNotCalled(t, "RollbackBundleWithResponse")
	})

	t.Run("publishes the rollback under -o json, and refuses to ask", func(t *testing.T) {
		alpha := rollbackMocks(t)
		alpha.On("RollbackBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything).Return(&astrov1alpha1.RollbackBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		}, nil).Once()

		stdout, stderr, err := execUnboundRootWith(t, astroV1Client, alpha, "bundle", "rollback", "bundle-1", "--deployment", "test-id-1", "--version", "old", "--yes", "-o", "json")
		require.NoError(t, err, "stderr:\n%s", stderr)
		var got deployment.BundleRollback
		decodeOne(t, stdout, &got)
		assert.Equal(t, deployment.BundleRollback{ID: "bundle-1", DeploymentID: "test-id-1", Version: "old", Action: "rolled_back"}, got)

		refusedAlpha := rollbackMocks(t)
		stdout, _, err = execUnboundRootWith(t, astroV1Client, refusedAlpha, "bundle", "rollback", "bundle-1", "--deployment", "test-id-1", "--version", "old", "-o", "json")
		require.Error(t, err)
		var obj cliout.ErrorObject
		decodeOne(t, stdout, &obj)
		assert.Contains(t, obj.Error, "pass --yes")
		assert.Equal(t, "input_required", string(obj.Kind))
		refusedAlpha.AssertNotCalled(t, "RollbackBundleWithResponse")
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

		out, err := execBundleCmd("versions", "list", "bundle-1", "--deployment", "test-id-1", "--limit", "2")
		assert.NoError(t, err)
		assert.Contains(t, out, "VERSION")
		assert.Contains(t, out, "v5")
		assert.Contains(t, out, "dags/a.py")
		assert.Contains(t, out, "ROLLBACK")
		mockAlpha.AssertExpectations(t)
	})

	t.Run("asks for the newest 20 by default", func(t *testing.T) {
		mockAlpha := setupBundleCmdMocks(t)
		mockAlpha.On("ListBundleVersionsWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.ListBundleVersionsParams) bool {
			return *p.Limit == defaultBundleVersionLimit
		})).Return(&astrov1alpha1.ListBundleVersionsResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.BundleVersionsPaginated{TotalCount: 0},
		}, nil).Once()

		_, err := execBundleCmd("versions", "list", "bundle-1", "--deployment", "test-id-1")
		assert.NoError(t, err)
		mockAlpha.AssertExpectations(t)
	})

	t.Run("refuses a --limit below 1 before asking the API", func(t *testing.T) {
		mockAlpha := setupBundleCmdMocks(t)

		_, err := execBundleCmd("versions", "list", "bundle-1", "--deployment", "test-id-1", "--limit", "0")
		assert.ErrorContains(t, err, "--limit must be a whole number of 1 or more")
		assert.True(t, cliout.IsUsage(err))
		mockAlpha.AssertNotCalled(t, "ListBundleVersionsWithResponse")
	})

	t.Run("renders JSON when -o json is set", func(t *testing.T) {
		mockAlpha := setupBundleCmdMocks(t)
		mockAlpha.On("ListBundleVersionsWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything).Return(&astrov1alpha1.ListBundleVersionsResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &astrov1alpha1.BundleVersionsPaginated{
				TotalCount: 1,
				Versions:   []astrov1alpha1.BundleVersion{{Version: "v1", Action: astrov1alpha1.BundleVersionActionDEPLOY}},
			},
		}, nil).Once()

		out, err := execBundleCmd("version", "ls", "bundle-1", "--deployment", "test-id-1", "-o", "json")
		assert.NoError(t, err)
		assert.Contains(t, out, `"versions"`)
		assert.Contains(t, out, `"v1"`)
		mockAlpha.AssertExpectations(t)
	})
}
