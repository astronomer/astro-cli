package deployment

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/stretchr/testify/mock"

	astrov1alpha1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1"
	astrov1alpha1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// jsonBundleOut is a BundleOutput in json mode: the result goes to result, the
// notes to notes, and nothing else is written.
func jsonBundleOut(result, notes *bytes.Buffer) BundleOutput {
	return BundleOutput{Result: testUtil.Renderer{JSON: true, Out: result}, Content: result, Notes: notes}
}

func (s *Suite) TestBundleFilesPublishTheirResults() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("ls publishes the listing, with [] for no directories", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := s.mockBundleLookup()
		version := testBundleVersion
		mockV1Alpha1Client.On("ListBundleFilesWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything).Return(&astrov1alpha1.ListBundleFilesResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &astrov1alpha1.BundleFileList{
				Version: &version,
				Path:    "dags",
				Files:   []astrov1alpha1.BundleFile{{Path: "dags/a.py", Digest: "sha256:aaa", Size: 12}},
			},
		}, nil).Once()

		err := ListBundleFilesWithFormat(context.Background(), BundlePath{Bundle: "my-files", Path: "dags"}, false, "", ws, testBundleDeploymentID, testUtil.Renderer{JSON: true, Out: out}, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		var got struct {
			Path        string           `json:"path"`
			Version     string           `json:"version"`
			Directories []string         `json:"directories"`
			Files       []BundleFileInfo `json:"files"`
		}
		s.NoError(json.Unmarshal(out.Bytes(), &got))
		s.Equal("dags", got.Path)
		s.Equal(version, got.Version)
		s.NotNil(got.Directories)
		s.Empty(got.Directories)
		s.Equal([]BundleFileInfo{{Path: "dags/a.py", Size: 12, Digest: "sha256:aaa"}}, got.Files)
		s.Contains(out.String(), `"directories":[]`)
	})

	s.Run("rm publishes the removal", func() {
		out, notes := &bytes.Buffer{}, &bytes.Buffer{}
		mockV1Alpha1Client := s.mockBundleLookup()
		mockV1Alpha1Client.On("DeleteBundleFileWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything).Return(&astrov1alpha1.DeleteBundleFileResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      bundleVersionResponse(),
		}, nil).Once()

		err := RemoveBundlePath(context.Background(), BundlePath{Bundle: "my-files", Path: "dags/a.py"}, false, "", ws, testBundleDeploymentID, false, jsonBundleOut(out, notes), mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		var got BundleFileChange
		s.NoError(json.Unmarshal(out.Bytes(), &got))
		s.Equal(BundleFileChange{Action: "removed", Source: "my-files:dags/a.py", Version: testBundleVersion}, got)
		s.Empty(notes.String())
	})

	s.Run("cp publishes the version a copy created", func() {
		out, notes := &bytes.Buffer{}, &bytes.Buffer{}
		localFile := filepath.Join(s.T().TempDir(), "a.py")
		s.NoError(os.WriteFile(localFile, []byte("x"), 0o600))
		mockV1Alpha1Client := s.mockBundleLookup()
		mockV1Alpha1Client.On("WriteBundleFileWithBodyWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything, bundleFileContentType, mock.Anything).Return(&astrov1alpha1.WriteBundleFileResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      bundleVersionResponse(),
		}, nil).Once()

		err := CopyBundleFiles(context.Background(), localFile, "my-files:dags/a.py", &BundleCopyOptions{}, ws, testBundleDeploymentID, jsonBundleOut(out, notes), mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.NoError(err)
		var got BundleFileChange
		s.NoError(json.Unmarshal(out.Bytes(), &got))
		s.Equal(BundleFileChange{Action: "copied", Source: localFile, Destination: "my-files:dags/a.py", Version: testBundleVersion}, got)
	})

	s.Run("cp -r puts the skipped note on stderr and the count in the result", func() {
		out, notes := &bytes.Buffer{}, &bytes.Buffer{}
		fromDir := s.T().TempDir()
		writeLocalFiles(s, fromDir, map[string]string{"a.py": "a", ".venv/pyvenv.cfg": "venv"})
		mockV1Alpha1Client := s.mockBundleLookup()
		mockStreamClient := s.streamClient()
		mockStreamClient.On("WriteBundleDirectoryWithBody", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything, bundleFileContentType, mock.Anything).Return(directoryWritten(testBundleVersion), nil).Once()

		err := CopyBundleFiles(context.Background(), fromDir, "my-files:dags", &BundleCopyOptions{Recursive: true}, ws, testBundleDeploymentID, jsonBundleOut(out, notes), mockV1Client, mockV1Alpha1Client, mockStreamClient)
		s.NoError(err)
		var got BundleFileChange
		s.NoError(json.Unmarshal(out.Bytes(), &got))
		s.Equal(1, *got.Files)
		s.Equal(1, got.Skipped)
		s.Contains(notes.String(), "Skipped 1 paths")
		s.NotContains(out.String(), "Skipped 1 paths")
	})

	s.Run("sync publishes what a dry run would change", func() {
		out, notes := &bytes.Buffer{}, &bytes.Buffer{}
		fromDir := s.T().TempDir()
		writeLocalFiles(s, fromDir, map[string]string{"same.py": "same", "changed.py": "new", "added.py": "added"})
		mockV1Alpha1Client := s.mockBundleLookup()
		mockSyncListing(mockV1Alpha1Client, "", map[string]string{"same.py": "same", "changed.py": "old", "extra.py": "extra"})

		err := SyncBundleFiles(context.Background(), fromDir, "my-files:dags", &BundleSyncOptions{DryRun: true}, ws, testBundleDeploymentID, jsonBundleOut(out, notes), mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.NoError(err)
		var got BundleSyncResult
		s.NoError(json.Unmarshal(out.Bytes(), &got))
		s.Equal(BundleSyncResult{
			Source: fromDir, Destination: "my-files:dags", Direction: "upload", DryRun: true,
			Version: "v1", Added: []string{"added.py"}, Updated: []string{"changed.py"}, Deleted: []string{"extra.py"},
		}, got)
		s.False(got.Changed)
	})

	s.Run("sync publishes [] for the changes it has none of", func() {
		out, notes := &bytes.Buffer{}, &bytes.Buffer{}
		fromDir := s.T().TempDir()
		writeLocalFiles(s, fromDir, map[string]string{"same.py": "same"})
		mockV1Alpha1Client := s.mockBundleLookup()
		mockSyncListing(mockV1Alpha1Client, "", map[string]string{"same.py": "same"})

		err := SyncBundleFiles(context.Background(), fromDir, "my-files:dags", &BundleSyncOptions{}, ws, testBundleDeploymentID, jsonBundleOut(out, notes), mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.NoError(err)
		s.Contains(out.String(), `"added":[],"updated":[],"deleted":[]`)
		s.Contains(out.String(), `"changed":false`)
	})
}

func (s *Suite) TestBundleFilesStopWithTheContext() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	s.Run("a canceled run looks up nothing and sends nothing", func() {
		out := &bytes.Buffer{}
		fromDir := s.T().TempDir()
		writeLocalFiles(s, fromDir, map[string]string{"a.py": "a"})
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		err := CopyBundleFiles(canceled, fromDir, "my-files:dags", &BundleCopyOptions{Recursive: true}, ws, testBundleDeploymentID, bundleOut(out), mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.ErrorIs(err, context.Canceled)
		err = SyncBundleFiles(canceled, fromDir, "my-files:dags", &BundleSyncOptions{}, ws, testBundleDeploymentID, bundleOut(out), mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.ErrorIs(err, context.Canceled)
		mockV1Alpha1Client.AssertNotCalled(s.T(), "ListBundleFilesWithResponse")
	})

	s.Run("a download stops reading the archive", func() {
		archive := bytes.NewReader(bundleDirectoryArchive(s, map[string]string{"a.py": "a", "b.py": "b"}))

		_, err := unpackBundleDirectory(canceled, archive, s.T().TempDir(), nil)
		s.ErrorIs(err, context.Canceled)
	})

	s.Run("an upload stops reading local files", func() {
		fromDir := s.T().TempDir()
		writeLocalFiles(s, fromDir, map[string]string{"a.py": "a"})

		err := packBundleDirectory(canceled, &bytes.Buffer{}, fromDir, &bundleSyncPlan{added: []string{"a.py"}})
		s.ErrorIs(err, context.Canceled)
	})

	s.Run("a local copy stops", func() {
		root, err := os.OpenRoot(s.T().TempDir())
		s.Require().NoError(err)
		defer root.Close()

		err = writeLocalFile(canceled, root, "a.py", strings.NewReader("a"))
		s.ErrorIs(err, context.Canceled)
		_, statErr := root.Stat("a.py")
		s.Error(statErr, "no half-written file is left")
	})
}

func (s *Suite) TestNeedsDeploymentScope() {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"a bundle path without a Deployment", []string{"my-files:dags"}, true},
		{"a bundle path with one", []string{"cm2deploy0001x8kq0000000a:my-files:dags"}, false},
		{"a copy between two Deployments", []string{"dep-1:my-files:a.py", "dep-2:my-files:a.py"}, false},
		{"a local file to a bundle without a Deployment", []string{"./a.py", "my-files:a.py"}, true},
		{"a local file to a bundle with one", []string{"./a.py", "dep-2:my-files:a.py"}, false},
		{"one of two bundle paths names none", []string{"dep-1:my-files:a.py", "my-files:b.py"}, true},
		{"no bundle path, which the operation refuses", []string{"./a.py", "./b.py"}, true},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			s.Equal(tc.want, NeedsDeploymentScope(tc.args...))
		})
	}
}
