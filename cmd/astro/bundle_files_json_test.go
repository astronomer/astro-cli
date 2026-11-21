package astro

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	astrov1alpha1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
)

// Under --output json the files commands publish one document on stdout and
// nothing else, in the tree production builds.
func TestBundleFilesJSON(t *testing.T) {
	t.Run("ls", func(t *testing.T) {
		alpha := setupBundleFilesCmdMocks(t)
		alpha.On("ListBundleFilesWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything).Return(&astrov1alpha1.ListBundleFilesResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.BundleFileList{Path: "dags", Files: []astrov1alpha1.BundleFile{{Path: "dags/a.py", Digest: "sha256:aaa", Size: 1}}},
		}, nil).Once()

		stdout, stderr, err := execUnboundRootWith(t, astroV1Client, alpha, "bundle", "files", "ls", "my-files:dags", "--deployment", "test-id-1", "-o", "json")
		require.NoError(t, err, "stderr:\n%s", stderr)
		var got deployment.BundleFileListing
		decodeOne(t, stdout, &got)
		assert.Equal(t, "dags", got.Path)
		assert.Equal(t, []string{}, got.Directories)
		assert.Len(t, got.Files, 1)
	})

	t.Run("rm without --yes is refused under json, with nothing asked", func(t *testing.T) {
		alpha := setupBundleFilesCmdMocks(t)

		stdout, stderr, err := execUnboundRootWith(t, astroV1Client, alpha, "bundle", "files", "rm", "my-files:dags", "--recursive", "--deployment", "test-id-1", "-o", "json")
		require.Error(t, err)
		var got cliout.ErrorObject
		decodeOne(t, stdout, &got)
		assert.Contains(t, got.Error, "pass --yes")
		assert.Equal(t, "input_required", string(got.Kind))
		assert.NotContains(t, stderr, "Remove my-files:dags?")
		alpha.AssertNotCalled(t, "DeleteBundleDirectoryWithResponse")
	})

	t.Run("rm --yes publishes the removal", func(t *testing.T) {
		alpha := setupBundleFilesCmdMocks(t)
		alpha.On("DeleteBundleDirectoryWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything).Return(&astrov1alpha1.DeleteBundleDirectoryResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.BundleVersion{Version: "new"},
		}, nil).Once()

		stdout, stderr, err := execUnboundRootWith(t, astroV1Client, alpha, "bundle", "files", "rm", "my-files:dags", "--recursive", "--yes", "--deployment", "test-id-1", "-o", "json")
		require.NoError(t, err, "stderr:\n%s", stderr)
		var got deployment.BundleFileChange
		decodeOne(t, stdout, &got)
		assert.Equal(t, deployment.BundleFileChange{Action: "removed", Source: "my-files:dags", Version: "new"}, got)
	})

	// stdout is a file's own bytes for these, so json has nowhere to go.
	t.Run("cat and a copy to - refuse json", func(t *testing.T) {
		alpha := setupBundleFilesCmdMocks(t)

		for _, args := range [][]string{
			{"bundle", "files", "cat", "my-files:dags/a.py", "--deployment", "test-id-1", "-o", "json"},
			{"bundle", "files", "cp", "my-files:dags/a.py", "-", "--deployment", "test-id-1", "-o", "json"},
		} {
			stdout, _, err := execUnboundRootWith(t, astroV1Client, alpha, args...)
			require.Error(t, err, args)
			assert.True(t, cliout.IsUsage(err), args)
			var got cliout.ErrorObject
			decodeOne(t, stdout, &got)
			assert.Contains(t, got.Error, "no --output json", args)
		}
		alpha.AssertNotCalled(t, "ListBundlesWithResponse")
	})
}

// A path that names its Deployment looks no Workspace up: the run needs none.
func TestBundleFilesPathNamingItsDeploymentNeedsNoWorkspace(t *testing.T) {
	alpha := setupBundleFilesCmdMocks(t)
	alpha.On("DeleteBundleFileWithResponse", mock.Anything, mock.Anything, "test-id-1", "bundle-1", mock.Anything).Return(&astrov1alpha1.DeleteBundleFileResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &astrov1alpha1.BundleVersion{Version: "new"},
	}, nil).Once()
	workspaceID = ""

	out, err := execBundleCmd("files", "rm", "test-id-1:my-files:dags/a.py")
	assert.NoError(t, err)
	assert.Contains(t, out, "Removed test-id-1:my-files:dags/a.py at version new")
	alpha.AssertExpectations(t)
}
