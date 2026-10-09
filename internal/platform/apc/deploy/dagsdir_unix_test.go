//go:build !windows

package deploy

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	houston_mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
	"github.com/astronomer/astro-cli/pkg/input"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// takesUploads is a DAG-only Deployment on a cluster that takes DAG-only
// deploys, so DagsOnlyDeploy gets past its refusals to the dags directory.
// It returns the upload URL and the bundles uploaded to it, each the
// gzipped tar's entry names.
func takesUploads(t *testing.T) (client *houston_mocks.ClientInterface, uploadURL *string, uploads *[][]string) {
	t.Helper()
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	prev := getDeploymentIDForCurrentCommandVar
	t.Cleanup(func() { getDeploymentIDForCurrentCommandVar = prev })
	getDeploymentIDForCurrentCommandVar = func(houston.ClientInterface, string, string, bool) (string, []houston.Deployment, error) {
		return "dep", nil, nil
	}
	client = new(houston_mocks.ClientInterface)
	client.On("GetDeployment", "dep").Return(&houston.Deployment{ID: "dep", DagDeployment: houston.DagDeploymentConfig{Type: houston.DagOnlyDeploymentType}}, nil)
	client.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{Flags: houston.FeatureFlags{DagOnlyDeployment: true}}, nil)
	uploads = new([][]string)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		file, _, err := r.FormFile("file")
		if !assert.NoError(t, err) {
			return
		}
		*uploads = append(*uploads, tarNames(t, file))
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	return client, &server.URL, uploads
}

// tarNames is the sorted entry names of a gzipped tar.
func tarNames(t *testing.T, r io.Reader) []string {
	t.Helper()
	gz, err := gzip.NewReader(r)
	require.NoError(t, err)
	tr := tar.NewReader(gz)
	names := []string{}
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		names = append(names, h.Name)
	}
	sort.Strings(names)
	return names
}

// A dags directory that cannot be looked at is not one that is missing: the
// deploy fails on it, rather than skipping the upload as if there were none.
func TestDagsOnlyDeployFailsOnADagsDirectoryItCannotRead(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores the directory modes this failure is injected with")
	}
	parent := filepath.Join(t.TempDir(), "project")
	require.NoError(t, os.MkdirAll(filepath.Join(parent, "dags"), 0o755))
	require.NoError(t, os.Chmod(parent, 0o000))
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

	client, url, uploads := takesUploads(t)
	got, err := DagsOnlyDeploy(client, "ws", "dep", parent, url, true, "", Options{Yes: true})
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNoDagsDirectory)
	assert.ErrorIs(t, err, syscall.EACCES)
	assert.ErrorContains(t, err, "reading the dags directory")
	assert.Equal(t, "dep", got)
	assert.Empty(t, *uploads)
}

// A path through a file is not a directory, as a file named dags is not.
func TestDagsOnlyDeployRefusesADagsPathThroughAFile(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(parent, nil, 0o600))
	client, url, uploads := takesUploads(t)
	_, err := DagsOnlyDeploy(client, "ws", "dep", parent, url, true, "", Options{Yes: true})
	assert.ErrorIs(t, err, ErrNoDagsDirectory)
	assert.Empty(t, *uploads)
}

// A dags symlink is uploaded as the directory it points at, under dags/ as
// any dags directory is: archived as the link it is, the bundle would hold
// no DAGs, and replace the Deployment's with none.
func TestDagsOnlyDeployUploadsTheDirectoryADagsSymlinkPointsAt(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, "shared_dags")
	require.NoError(t, os.MkdirAll(filepath.Join(shared, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(shared, "a.py"), []byte("a"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(shared, "sub", "b.py"), []byte("b"), 0o600))
	parent := filepath.Join(root, "project")
	require.NoError(t, os.Mkdir(parent, 0o755))
	require.NoError(t, os.Symlink(filepath.Join("..", "shared_dags"), filepath.Join(parent, "dags")))

	// The DAGs are counted in the directory pointed at: not asked whether
	// to deploy none.
	prevConfirm := confirmEmptyDags
	t.Cleanup(func() { confirmEmptyDags = prevConfirm })
	confirmEmptyDags = func(string, ...input.Option) (bool, error) {
		t.Error("asked whether to deploy no DAGs, of a dags symlink to DAGs")
		return false, nil
	}
	client, url, uploads := takesUploads(t)
	_, err := DagsOnlyDeploy(client, "ws", "dep", parent, url, true, "", Options{})
	require.NoError(t, err)
	assert.Equal(t, [][]string{{"dags/a.py", "dags/sub/b.py"}}, *uploads)
}

// A dags symlink to nothing, or to a file, is no dags directory.
func TestDagsOnlyDeployRefusesADagsSymlinkToNoDirectory(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	for name, target := range map[string]string{"dangling": filepath.Join(root, "gone"), "to a file": file} {
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			require.NoError(t, os.Symlink(target, filepath.Join(parent, "dags")))
			client, url, uploads := takesUploads(t)
			_, err := DagsOnlyDeploy(client, "ws", "dep", parent, url, true, "", Options{Yes: true})
			assert.ErrorIs(t, err, ErrNoDagsDirectory)
			assert.ErrorContains(t, err, filepath.Join(parent, "dags")+" is not a directory")
			assert.Empty(t, *uploads)
		})
	}
}
