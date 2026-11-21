package astro

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	astrov1alpha1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1"
	astrov1alpha1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1/mocks"
)

// setupBundleFilesCmdMocks sets up the mocks that resolve the bundle "my-files", whose ID
// is "bundle-1".
func setupBundleFilesCmdMocks(t *testing.T) *astrov1alpha1_mocks.ClientWithResponsesInterface {
	t.Helper()
	mockAlpha := setupBundleCmdMocks(t)
	name := "my-files"
	isDag := true
	mockAlpha.On("ListBundlesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1alpha1.ListBundlesResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &astrov1alpha1.BundlesPaginated{
			TotalCount: 1,
			Bundles:    []astrov1alpha1.DeploymentBundle{{Id: "bundle-1", Name: &name, IsDagBundle: &isDag}},
		},
	}, nil)
	astroV1Alpha1StreamClient = new(astrov1alpha1_mocks.ClientInterface)
	return mockAlpha
}

// streamedResponse is a response of a bundle read whose body the CLI streams.
func streamedResponse(body []byte) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"X-Astro-Bundle-Version": {"v1"}}, Body: io.NopCloser(bytes.NewReader(body))}
}

func TestBundleFilesListCmd(t *testing.T) {
	t.Run("passes the bundle path and --recursive", func(t *testing.T) {
		mockAlpha := setupBundleFilesCmdMocks(t)
		mockAlpha.On("ListBundleFilesWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.ListBundleFilesParams) bool {
			return p.Path != nil && *p.Path == "dags" && p.Recursive != nil && *p.Recursive
		})).Return(&astrov1alpha1.ListBundleFilesResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &astrov1alpha1.BundleFileList{
				Path:        "dags",
				Directories: []string{},
				Files:       []astrov1alpha1.BundleFile{{Path: "dags/a.py", Digest: "sha256:aaa", Size: 1}},
			},
		}, nil).Once()

		out, err := execBundleCmd("files", "ls", "my-files:dags", "--recursive", "--deployment", "test-id-1")
		assert.NoError(t, err)
		assert.Contains(t, out, "dags/a.py")
		mockAlpha.AssertExpectations(t)
	})

	t.Run("requires a bundle", func(t *testing.T) {
		setupBundleFilesCmdMocks(t)

		_, err := execBundleCmd("files", "ls", "--deployment", "test-id-1")
		assert.Error(t, err)
	})
}

func TestBundleFilesCatCmd(t *testing.T) {
	t.Run("prints the file content", func(t *testing.T) {
		setupBundleFilesCmdMocks(t)
		mockStream := astroV1Alpha1StreamClient.(*astrov1alpha1_mocks.ClientInterface)
		mockStream.On("ReadBundleFile", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.ReadBundleFileParams) bool {
			return p.Path == "dags/a.py" && p.Version != nil && *p.Version == "old"
		})).Return(streamedResponse([]byte("print('hi')\n")), nil).Once()

		out, err := execBundleCmd("files", "cat", "my-files:dags/a.py", "--version", "old", "--deployment", "test-id-1")
		assert.NoError(t, err)
		assert.Equal(t, "print('hi')\n", out)
		mockStream.AssertExpectations(t)
	})
}

func TestBundleFilesCopyCmd(t *testing.T) {
	t.Run("copies a directory into the bundle with --recursive, --if-exists and --expected-version", func(t *testing.T) {
		setupBundleFilesCmdMocks(t)
		mockStream := astroV1Alpha1StreamClient.(*astrov1alpha1_mocks.ClientInterface)
		fromDir := t.TempDir()
		assert.NoError(t, os.WriteFile(filepath.Join(fromDir, "a.py"), []byte("x"), 0o600))
		mockStream.On("WriteBundleDirectoryWithBody", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.WriteBundleDirectoryParams) bool {
			return p.Path != nil && *p.Path == "dags" && p.IfExists != nil && *p.IfExists == astrov1alpha1.WriteBundleDirectoryParamsIfExistsMERGE && p.ExpectedVersion != nil && *p.ExpectedVersion == "old"
		}), "application/octet-stream", mock.Anything).Return(&http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"version":"new"}`)),
		}, nil).Once()

		out, err := execBundleCmd("files", "cp", "--recursive", fromDir, "my-files:dags", "--if-exists", "merge", "--expected-version", "old", "--deployment", "test-id-1")
		assert.NoError(t, err)
		assert.Contains(t, out, "Copied 1 files from "+fromDir+" to my-files:dags at version new")
		mockStream.AssertExpectations(t)
	})

	t.Run("copies a file out of the bundle", func(t *testing.T) {
		setupBundleFilesCmdMocks(t)
		mockStream := astroV1Alpha1StreamClient.(*astrov1alpha1_mocks.ClientInterface)
		localFile := filepath.Join(t.TempDir(), "a.py")
		mockStream.On("ReadBundleFile", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.ReadBundleFileParams) bool {
			return p.Path == "dags/a.py"
		})).Return(streamedResponse([]byte("x")), nil).Once()

		out, err := execBundleCmd("files", "cp", "my-files:dags/a.py", localFile, "--deployment", "test-id-1")
		assert.NoError(t, err)
		assert.Contains(t, out, "Copied my-files:dags/a.py at version v1 to "+localFile)
		mockStream.AssertExpectations(t)
	})

	t.Run("streams a file from an earlier version into a path named with its Deployment ID", func(t *testing.T) {
		mockAlpha := setupBundleFilesCmdMocks(t)
		mockStream := astroV1Alpha1StreamClient.(*astrov1alpha1_mocks.ClientInterface)
		mockStream.On("ReadBundleFile", mock.Anything, mock.Anything, "test-id-1", "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.ReadBundleFileParams) bool {
			return p.Path == "a.py" && *p.Version == "old"
		})).Return(streamedResponse([]byte("x")), nil).Once()
		mockAlpha.On("WriteBundleFileWithBodyWithResponse", mock.Anything, mock.Anything, "test-id-1", "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.WriteBundleFileParams) bool {
			return p.Path == "dags/a.py"
		}), "application/octet-stream", mock.Anything).Return(&astrov1alpha1.WriteBundleFileResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.BundleVersion{Version: "new"},
		}, nil).Once()

		out, err := execBundleCmd("files", "cp", "my-files:a.py", "test-id-1:my-files:dags/", "--version", "old", "--deployment", "test-id-1")
		assert.NoError(t, err)
		assert.Contains(t, out, "Copied my-files:a.py at version v1 to test-id-1:my-files:dags/a.py at version new")
		mockAlpha.AssertExpectations(t)
		mockStream.AssertExpectations(t)
	})

	t.Run("requires SRC and DST", func(t *testing.T) {
		setupBundleFilesCmdMocks(t)

		_, err := execBundleCmd("files", "cp", "my-files:dags/a.py", "--deployment", "test-id-1")
		assert.Error(t, err)
	})
}

func TestBundleFilesSyncCmd(t *testing.T) {
	t.Run("deletes by default, and passes --dry-run and the filter flags", func(t *testing.T) {
		mockAlpha := setupBundleFilesCmdMocks(t)
		fromDir := t.TempDir()
		assert.NoError(t, os.WriteFile(filepath.Join(fromDir, "a.py"), []byte("x"), 0o600))
		assert.NoError(t, os.WriteFile(filepath.Join(fromDir, "b.py"), []byte("x"), 0o600))
		mockAlpha.On("ListBundleFilesWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything).Return(&astrov1alpha1.ListBundleFilesResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &astrov1alpha1.BundleFileList{
				Path:        "dags",
				Directories: []string{},
				Files:       []astrov1alpha1.BundleFile{{Path: "dags/old.py", Digest: "sha256:aaa", Size: 1}},
				Version:     new("v1"),
			},
		}, nil).Once()

		out, err := execBundleCmd("files", "sync", fromDir, "my-files:dags", "--dry-run", "--exclude", "b.py", "--deployment", "test-id-1")
		assert.NoError(t, err)
		assert.Contains(t, out, "add     my-files:dags/a.py")
		assert.Contains(t, out, "delete  my-files:dags/old.py")
		assert.NotContains(t, out, "b.py")
		assert.Contains(t, out, "1 added, 0 updated, 1 deleted; nothing was changed")
		mockAlpha.AssertExpectations(t)
	})

	t.Run("deletes local files without asking with --yes", func(t *testing.T) {
		mockAlpha := setupBundleFilesCmdMocks(t)
		toDir := t.TempDir()
		assert.NoError(t, os.WriteFile(filepath.Join(toDir, "a.py"), []byte("x"), 0o600))
		assert.NoError(t, os.WriteFile(filepath.Join(toDir, "extra.py"), []byte("x"), 0o600))
		digest := sha256.Sum256([]byte("x"))
		mockAlpha.On("ListBundleFilesWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything).Return(&astrov1alpha1.ListBundleFilesResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &astrov1alpha1.BundleFileList{
				Path:        "dags",
				Directories: []string{},
				Files:       []astrov1alpha1.BundleFile{{Path: "dags/a.py", Digest: "sha256:" + hex.EncodeToString(digest[:]), Size: 1}},
				Version:     new("v1"),
			},
		}, nil).Once()

		out, err := execBundleCmd("files", "sync", "my-files:dags", toDir, "--yes", "--deployment", "test-id-1")
		assert.NoError(t, err)
		assert.Contains(t, out, "0 added, 0 updated, 1 deleted")
		assert.NoFileExists(t, filepath.Join(toDir, "extra.py"))
	})

	t.Run("refuses --yes for a sync into a bundle", func(t *testing.T) {
		setupBundleFilesCmdMocks(t)

		_, err := execBundleCmd("files", "sync", t.TempDir(), "my-files:dags", "--yes", "--deployment", "test-id-1")
		assert.EqualError(t, err, "--yes applies only when syncing from a bundle")
	})
}

func TestBundleFilesMoveCmd(t *testing.T) {
	t.Run("moves a directory with --if-exists", func(t *testing.T) {
		mockAlpha := setupBundleFilesCmdMocks(t)
		mockAlpha.On("ListBundleFilesWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything).Return(&astrov1alpha1.ListBundleFilesResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.BundleFileList{Directories: []string{"staging"}, Files: []astrov1alpha1.BundleFile{}},
		}, nil).Once()
		mockAlpha.On("MoveBundleDirectoryWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(r astrov1alpha1.MoveBundlePathRequest) bool {
			return r.FromPath == "staging" && *r.ToPath == "dags" && *r.IfExists == astrov1alpha1.MoveBundlePathRequestIfExistsREPLACE
		})).Return(&astrov1alpha1.MoveBundleDirectoryResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.BundleVersion{Version: "new"},
		}, nil).Once()

		out, err := execBundleCmd("files", "mv", "my-files:staging", "my-files:dags", "--if-exists", "replace", "--deployment", "test-id-1")
		assert.NoError(t, err)
		assert.Contains(t, out, "Moved my-files:staging to my-files:dags at version new")
		mockAlpha.AssertExpectations(t)
	})
}

func TestBundleFilesRemoveCmd(t *testing.T) {
	t.Run("removes a directory with --recursive", func(t *testing.T) {
		mockAlpha := setupBundleFilesCmdMocks(t)
		mockAlpha.On("DeleteBundleDirectoryWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.DeleteBundleDirectoryParams) bool {
			return p.Path != nil && *p.Path == "dags"
		})).Return(&astrov1alpha1.DeleteBundleDirectoryResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.BundleVersion{Version: "new"},
		}, nil).Once()

		out, err := execBundleCmd("files", "rm", "--recursive", "my-files:dags", "--yes", "--deployment", "test-id-1")
		assert.NoError(t, err)
		assert.Contains(t, out, "Removed my-files:dags at version new")
		mockAlpha.AssertExpectations(t)
	})

	t.Run("removes with --yes and --expected-version", func(t *testing.T) {
		mockAlpha := setupBundleFilesCmdMocks(t)
		mockAlpha.On("DeleteBundleFileWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.DeleteBundleFileParams) bool {
			return p.Path == "dags/a.py" && p.ExpectedVersion != nil && *p.ExpectedVersion == "old"
		})).Return(&astrov1alpha1.DeleteBundleFileResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.BundleVersion{Version: "new"},
		}, nil).Once()

		out, err := execBundleCmd("files", "rm", "my-files:dags/a.py", "--yes", "--expected-version", "old", "--deployment", "test-id-1")
		assert.NoError(t, err)
		assert.Contains(t, out, "Removed my-files:dags/a.py at version new")
		mockAlpha.AssertExpectations(t)
	})

	t.Run("refuses a bundle without a path", func(t *testing.T) {
		mockAlpha := setupBundleFilesCmdMocks(t)

		_, err := execBundleCmd("files", "rm", "--recursive", "--yes", "my-files", "--deployment", "test-id-1")
		assert.EqualError(t, err, "my-files names no path in a bundle: write BUNDLE:PATH, or BUNDLE: for every file in the bundle")
		mockAlpha.AssertNotCalled(t, "DeleteBundleDirectoryWithResponse")
	})
}
