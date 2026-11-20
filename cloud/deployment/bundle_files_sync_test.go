package deployment

import (
	"bytes"
	httpContext "context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/stretchr/testify/mock"

	astrov1alpha1 "github.com/astronomer/astro-cli/astro-client-v1alpha1"
	astrov1alpha1_mocks "github.com/astronomer/astro-cli/astro-client-v1alpha1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func testDigest(content string) string {
	sum := sha256.Sum256([]byte(content))
	return bundleDigestPrefix + hex.EncodeToString(sum[:])
}

// mockSyncListing mocks the recursive listing of dags at version, or at the
// current version when version is empty, which the response says is v1.
func mockSyncListing(mockV1Alpha1Client *astrov1alpha1_mocks.ClientWithResponsesInterface, version string, files map[string]string) {
	listed := []astrov1alpha1.BundleFile{}
	for name, content := range files {
		listed = append(listed, astrov1alpha1.BundleFile{Path: "dags/" + name, Digest: testDigest(content), Size: len(content)})
	}
	mockV1Alpha1Client.On("ListBundleFilesWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.ListBundleFilesParams) bool {
		return *p.Path == "dags" && *p.Recursive && (version == "" && p.Version == nil || p.Version != nil && *p.Version == version)
	})).Return(&astrov1alpha1.ListBundleFilesResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &astrov1alpha1.BundleFileList{Path: "dags", Directories: []string{}, Files: listed, Version: new("v1")},
	}, nil).Once()
}

func (s *Suite) TestSyncBundleFilesToBundle() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	remote := map[string]string{"same.py": "same", "changed.py": "old", "extra.py": "extra", ".env": "secret"}
	local := map[string]string{"same.py": "same", "changed.py": "new", "added.py": "added", ".venv/pyvenv.cfg": "venv"}

	s.Run("writes only what changed, as one merging write at the listed version", func() {
		out := &bytes.Buffer{}
		fromDir := s.T().TempDir()
		writeLocalFiles(s, fromDir, local)
		mockV1Alpha1Client := s.mockBundleLookup()
		mockSyncListing(mockV1Alpha1Client, "", remote)
		mockStreamClient := s.streamClient()
		mockStreamClient.On("WriteBundleDirectoryWithBody", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.WriteBundleDirectoryParams) bool {
			return *p.Path == "dags" && *p.IfExists == astrov1alpha1.WriteBundleDirectoryParamsIfExistsMERGE && *p.ExpectedVersion == "v1"
		}), bundleFileContentType, mock.MatchedBy(func(body io.Reader) bool {
			files, err := bundleDirectoryFiles(body)
			return err == nil && len(files) == 3 && files["added.py"] == "added" && files["changed.py"] == "new" && files["__DEL__extra.py"] == ""
		})).Return(directoryWritten(testBundleVersion), nil).Once()

		err := SyncBundleFiles(httpContext.Background(), fromDir, "my-files:dags", &BundleSyncOptions{}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, mockStreamClient)
		s.NoError(err)
		s.Contains(out.String(), "add     my-files:dags/added.py\n")
		s.Contains(out.String(), "update  my-files:dags/changed.py\n")
		s.Contains(out.String(), "delete  my-files:dags/extra.py\n")
		s.NotContains(out.String(), ".env")
		s.Contains(out.String(), "Synced "+fromDir+" to my-files:dags at version "+testBundleVersion+": 1 added, 1 updated, 1 deleted")
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("leaves extras with --no-delete", func() {
		out := &bytes.Buffer{}
		fromDir := s.T().TempDir()
		writeLocalFiles(s, fromDir, local)
		mockV1Alpha1Client := s.mockBundleLookup()
		mockSyncListing(mockV1Alpha1Client, "", remote)
		mockStreamClient := s.streamClient()
		mockStreamClient.On("WriteBundleDirectoryWithBody", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything, bundleFileContentType, mock.MatchedBy(func(body io.Reader) bool {
			files, err := bundleDirectoryFiles(body)
			return err == nil && len(files) == 2
		})).Return(directoryWritten(testBundleVersion), nil).Once()

		err := SyncBundleFiles(httpContext.Background(), fromDir, "my-files:dags", &BundleSyncOptions{NoDelete: true}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, mockStreamClient)
		s.NoError(err)
		s.Contains(out.String(), "1 added, 1 updated, 0 deleted")
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("prints the plan without writing for a dry run at the expected version", func() {
		out := &bytes.Buffer{}
		fromDir := s.T().TempDir()
		writeLocalFiles(s, fromDir, local)
		mockV1Alpha1Client := s.mockBundleLookup()
		mockSyncListing(mockV1Alpha1Client, "old", remote)

		err := SyncBundleFiles(httpContext.Background(), fromDir, "my-files:dags", &BundleSyncOptions{DryRun: true, ExpectedVersion: "old"}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.NoError(err)
		s.Contains(out.String(), "delete  my-files:dags/extra.py\n")
		s.Contains(out.String(), "Dry run at bundle version v1: 1 added, 1 updated, 1 deleted; nothing was changed")
	})

	s.Run("writes nothing when up to date", func() {
		out := &bytes.Buffer{}
		fromDir := s.T().TempDir()
		writeLocalFiles(s, fromDir, map[string]string{"same.py": "same"})
		mockV1Alpha1Client := s.mockBundleLookup()
		mockSyncListing(mockV1Alpha1Client, "", map[string]string{"same.py": "same"})

		err := SyncBundleFiles(httpContext.Background(), fromDir, "my-files:dags", &BundleSyncOptions{}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.NoError(err)
		s.Contains(out.String(), "my-files:dags is up to date with "+fromDir+" at bundle version v1")
	})

	s.Run("reads a local directory reached through a symbolic link", func() {
		out := &bytes.Buffer{}
		realDir := s.T().TempDir()
		writeLocalFiles(s, realDir, map[string]string{"same.py": "same"})
		link := filepath.Join(s.T().TempDir(), "link")
		s.NoError(os.Symlink(realDir, link))
		mockV1Alpha1Client := s.mockBundleLookup()
		mockSyncListing(mockV1Alpha1Client, "", map[string]string{"same.py": "same"})

		err := SyncBundleFiles(httpContext.Background(), link, "my-files:dags", &BundleSyncOptions{}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.NoError(err)
		s.Contains(out.String(), "my-files:dags is up to date with ")
	})

	s.Run("refuses a local file", func() {
		out := &bytes.Buffer{}
		localFile := filepath.Join(s.T().TempDir(), "a.py")
		s.NoError(os.WriteFile(localFile, []byte("x"), 0o600))
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		err := SyncBundleFiles(httpContext.Background(), localFile, "my-files:dags", &BundleSyncOptions{}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.ErrorContains(err, "is not a directory")
	})
}

func (s *Suite) TestSyncBundleFilesFromBundle() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	remote := map[string]string{"same.py": "same", "changed.py": "new", "new/added.py": "added"}
	local := map[string]string{"same.py": "same", "changed.py": "old", "gone/extra.py": "extra", ".venv/pyvenv.cfg": "venv"}

	s.Run("writes only what changed, read at the listed version, and deletes extras once confirmed", func() {
		defer testUtil.MockUserInput(s.T(), "y")()
		out := &bytes.Buffer{}
		toDir := s.T().TempDir()
		writeLocalFiles(s, toDir, local)
		mockV1Alpha1Client := s.mockBundleLookup()
		mockSyncListing(mockV1Alpha1Client, "", remote)
		mockStreamClient := s.streamClient()
		mockStreamClient.On("ReadBundleDirectory", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.ReadBundleDirectoryParams) bool {
			return *p.Path == "dags" && *p.Version == "v1"
		})).Return(streamedResponse(http.StatusOK, bundleDirectoryArchive(s, map[string]string{"same.py": "overwritten", "changed.py": "new", "new/added.py": "added"})), nil).Once()

		err := SyncBundleFiles(httpContext.Background(), "my-files:dags", toDir, &BundleSyncOptions{}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, mockStreamClient)
		s.NoError(err)
		for name, want := range map[string]string{"same.py": "same", "changed.py": "new", "new/added.py": "added", ".venv/pyvenv.cfg": "venv"} {
			content, err := os.ReadFile(filepath.Join(toDir, name))
			s.NoError(err, name)
			s.Equal(want, string(content), name)
		}
		s.NoDirExists(filepath.Join(toDir, "gone"))
		s.Contains(out.String(), "Synced my-files:dags at version v1 to "+toDir+": 1 added, 1 updated, 1 deleted")
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("changes nothing when the deletes are not confirmed", func() {
		defer testUtil.MockUserInput(s.T(), "n")()
		out := &bytes.Buffer{}
		toDir := s.T().TempDir()
		writeLocalFiles(s, toDir, local)
		mockV1Alpha1Client := s.mockBundleLookup()
		mockSyncListing(mockV1Alpha1Client, "", remote)

		err := SyncBundleFiles(httpContext.Background(), "my-files:dags", toDir, &BundleSyncOptions{}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.NoError(err)
		s.Contains(out.String(), "Canceling sync")
		s.FileExists(filepath.Join(toDir, "gone", "extra.py"))
		s.NoFileExists(filepath.Join(toDir, "new", "added.py"))
	})

	s.Run("replaces a file with a directory, and a directory with a file", func() {
		out := &bytes.Buffer{}
		toDir := s.T().TempDir()
		writeLocalFiles(s, toDir, map[string]string{"a": "file", "c/d.py": "d"})
		swapped := map[string]string{"a/b.py": "b", "c": "file"}
		mockV1Alpha1Client := s.mockBundleLookup()
		mockSyncListing(mockV1Alpha1Client, "", swapped)
		mockStreamClient := s.streamClient()
		mockStreamClient.On("ReadBundleDirectory", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything).Return(streamedResponse(http.StatusOK, bundleDirectoryArchive(s, swapped)), nil).Once()

		err := SyncBundleFiles(httpContext.Background(), "my-files:dags", toDir, &BundleSyncOptions{Force: true}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, mockStreamClient)
		s.NoError(err)
		for name, want := range swapped {
			content, err := os.ReadFile(filepath.Join(toDir, filepath.FromSlash(name)))
			s.NoError(err, name)
			s.Equal(want, string(content), name)
		}
	})

	s.Run("keeps a file the bundle renamed only in case", func() {
		out := &bytes.Buffer{}
		toDir := s.T().TempDir()
		writeLocalFiles(s, toDir, map[string]string{"Foo.py": "old"})
		renamed := map[string]string{"foo.py": "new"}
		mockV1Alpha1Client := s.mockBundleLookup()
		mockSyncListing(mockV1Alpha1Client, "", renamed)
		mockStreamClient := s.streamClient()
		mockStreamClient.On("ReadBundleDirectory", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything).Return(streamedResponse(http.StatusOK, bundleDirectoryArchive(s, renamed)), nil).Once()

		err := SyncBundleFiles(httpContext.Background(), "my-files:dags", toDir, &BundleSyncOptions{Force: true}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, mockStreamClient)
		s.NoError(err)
		entries, err := os.ReadDir(toDir)
		s.Require().NoError(err)
		s.Len(entries, 1)
		s.Equal("foo.py", entries[0].Name())
		content, err := os.ReadFile(filepath.Join(toDir, "foo.py"))
		s.NoError(err)
		s.Equal("new", string(content))
	})

	s.Run("changes nothing when an excluded file is in the way of a transfer", func() {
		out := &bytes.Buffer{}
		toDir := s.T().TempDir()
		writeLocalFiles(s, toDir, map[string]string{"c/d.py": "d", "c/.DS_Store": "x"})
		mockV1Alpha1Client := s.mockBundleLookup()
		mockSyncListing(mockV1Alpha1Client, "", map[string]string{"c": "file"})

		err := SyncBundleFiles(httpContext.Background(), "my-files:dags", toDir, &BundleSyncOptions{Force: true}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.EqualError(err, filepath.Join(toDir, "c", ".DS_Store")+" must be removed to write "+filepath.Join(toDir, "c")+", but the sync keeps it, because it is excluded or --no-delete is set")
		s.FileExists(filepath.Join(toDir, "c", "d.py"))
	})

	s.Run("deletes nothing when the download fails", func() {
		out := &bytes.Buffer{}
		toDir := s.T().TempDir()
		writeLocalFiles(s, toDir, local)
		mockV1Alpha1Client := s.mockBundleLookup()
		mockSyncListing(mockV1Alpha1Client, "", remote)
		mockStreamClient := s.streamClient()
		mockStreamClient.On("ReadBundleDirectory", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything).Return(streamedResponse(http.StatusInternalServerError, []byte(`{"message":"internal error"}`)), nil).Once()

		err := SyncBundleFiles(httpContext.Background(), "my-files:dags", toDir, &BundleSyncOptions{Force: true}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, mockStreamClient)
		s.EqualError(err, "internal error")
		s.FileExists(filepath.Join(toDir, "gone", "extra.py"))
	})

	s.Run("fails, deleting nothing, for a bundle path that holds no files", func() {
		out := &bytes.Buffer{}
		toDir := s.T().TempDir()
		writeLocalFiles(s, toDir, local)
		mockV1Alpha1Client := s.mockBundleLookup()
		mockSyncListing(mockV1Alpha1Client, "", map[string]string{})

		err := SyncBundleFiles(httpContext.Background(), "my-files:dags", toDir, &BundleSyncOptions{Force: true}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.EqualError(err, "my-files:dags holds no files")
		s.FileExists(filepath.Join(toDir, "gone", "extra.py"))
	})

	s.Run("refuses a bundle path that is a file", func() {
		out := &bytes.Buffer{}
		toDir := s.T().TempDir()
		mockV1Alpha1Client := s.mockBundleLookup()
		mockV1Alpha1Client.On("ListBundleFilesWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything).Return(&astrov1alpha1.ListBundleFilesResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.BundleFileList{Path: "dags/a.py", Directories: []string{}, Files: []astrov1alpha1.BundleFile{{Path: "dags/a.py", Digest: testDigest("a")}}, Version: new("v1")},
		}, nil).Once()

		err := SyncBundleFiles(httpContext.Background(), "my-files:dags/a.py", toDir, &BundleSyncOptions{}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.EqualError(err, "my-files:dags/a.py is a file: sync takes a directory")
	})

	s.Run("syncs into a local directory reached through a symbolic link", func() {
		out := &bytes.Buffer{}
		realDir := s.T().TempDir()
		writeLocalFiles(s, realDir, map[string]string{"same.py": "same"})
		link := filepath.Join(s.T().TempDir(), "link")
		s.NoError(os.Symlink(realDir, link))
		mockV1Alpha1Client := s.mockBundleLookup()
		mockSyncListing(mockV1Alpha1Client, "", map[string]string{"same.py": "same"})

		err := SyncBundleFiles(httpContext.Background(), "my-files:dags", link, &BundleSyncOptions{}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.NoError(err)
		s.Contains(out.String(), "is up to date with my-files:dags")
	})

	s.Run("creates nothing for a dry run into a missing directory", func() {
		out := &bytes.Buffer{}
		toDir := filepath.Join(s.T().TempDir(), "out")
		mockV1Alpha1Client := s.mockBundleLookup()
		mockSyncListing(mockV1Alpha1Client, "old", remote)
		mockStreamClient := s.streamClient()

		err := SyncBundleFiles(httpContext.Background(), "my-files:dags", toDir, &BundleSyncOptions{DryRun: true, Version: "old"}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, mockStreamClient)
		s.NoError(err)
		s.Contains(out.String(), "Dry run at bundle version v1: 3 added, 0 updated, 0 deleted; nothing was changed")
		s.NoDirExists(toDir)
		mockStreamClient.AssertNotCalled(s.T(), "ReadBundleDirectory")
	})
}

func (s *Suite) TestSyncBundleFilesRefusesArguments() {
	out := &bytes.Buffer{}
	mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

	err := SyncBundleFiles(httpContext.Background(), "./a", "./b", &BundleSyncOptions{}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
	s.ErrorIs(err, errSyncNoBundle)

	err = SyncBundleFiles(httpContext.Background(), "my-files:a", "my-files:b", &BundleSyncOptions{}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
	s.ErrorIs(err, errSyncNoBundle)

	err = SyncBundleFiles(httpContext.Background(), "./a", "my-files:b", &BundleSyncOptions{Version: "old"}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
	s.ErrorIs(err, errSyncVersion)

	err = SyncBundleFiles(httpContext.Background(), "my-files:a", "./b", &BundleSyncOptions{ExpectedVersion: "old"}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
	s.ErrorIs(err, errSyncExpectedVersion)
}
