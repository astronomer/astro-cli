package deployment

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing/iotest"

	"github.com/stretchr/testify/mock"

	astrov1 "github.com/astronomer/astro-cli/astro-client-v1"
	astrov1alpha1 "github.com/astronomer/astro-cli/astro-client-v1alpha1"
	astrov1alpha1_mocks "github.com/astronomer/astro-cli/astro-client-v1alpha1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

const testBundleVersion = "4f2a9c"

func bundleVersionResponse() *astrov1alpha1.BundleVersion {
	return &astrov1alpha1.BundleVersion{Version: testBundleVersion}
}

func bundleVersionHeaders() http.Header {
	return http.Header{"X-Astro-Bundle-Version": {testBundleVersion}}
}

// streamedResponse is a response of a bundle read whose body the CLI streams.
func streamedResponse(status int, body []byte) *http.Response {
	return &http.Response{StatusCode: status, Header: bundleVersionHeaders(), Body: io.NopCloser(bytes.NewReader(body))}
}

// directoryWritten is the API's answer to a directory write that created version.
func directoryWritten(version string) *http.Response {
	body := `{"version":"` + version + `"}`
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}

// streamClient returns a mock of the client whose responses are streamed, asserting its expectations when the test ends.
func (s *Suite) streamClient() *astrov1alpha1_mocks.ClientInterface {
	mockStreamClient := new(astrov1alpha1_mocks.ClientInterface)
	t := s.T()
	t.Cleanup(func() { mockStreamClient.AssertExpectations(t) })
	return mockStreamClient
}

// mockBundleLookup sets up the deployment and bundle listing that resolve the
// bundle "my-files", whose ID is "bundle-1".
func (s *Suite) mockBundleLookup() *astrov1alpha1_mocks.ClientWithResponsesInterface {
	s.mockGetDeployment()
	name := "my-files"
	isDag := true
	mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
	mockV1Alpha1Client.On("ListBundlesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1alpha1.ListBundlesResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &astrov1alpha1.BundlesPaginated{
			TotalCount: 1,
			Bundles:    []astrov1alpha1.DeploymentBundle{{Id: "bundle-1", Name: &name, IsDagBundle: &isDag}},
		},
	}, nil).Once()
	return mockV1Alpha1Client
}

// mockTwoBundleLookup sets up the deployment and bundle listing that resolve the
// bundles "my-files" and "other-files", whose IDs are "bundle-1" and "bundle-2".
func (s *Suite) mockTwoBundleLookup() *astrov1alpha1_mocks.ClientWithResponsesInterface {
	s.mockGetDeployment()
	mine, other := "my-files", "other-files"
	mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
	mockV1Alpha1Client.On("ListBundlesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1alpha1.ListBundlesResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &astrov1alpha1.BundlesPaginated{
			TotalCount: 2,
			Bundles:    []astrov1alpha1.DeploymentBundle{{Id: "bundle-1", Name: &mine}, {Id: "bundle-2", Name: &other}},
		},
	}, nil).Once()
	return mockV1Alpha1Client
}

func bundleDirectoryArchive(s *Suite, files map[string]string) []byte {
	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, content := range files {
		s.NoError(tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}))
		_, err := tarWriter.Write([]byte(content))
		s.NoError(err)
	}
	s.NoError(tarWriter.Close())
	s.NoError(gzipWriter.Close())
	return buf.Bytes()
}

func bundleDirectoryFiles(archive io.Reader) (map[string]string, error) {
	gzipReader, err := gzip.NewReader(archive)
	if err != nil {
		return nil, err
	}
	tarReader := tar.NewReader(gzipReader)
	files := map[string]string{}
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return files, nil
		}
		if err != nil {
			return nil, err
		}
		content, err := io.ReadAll(tarReader)
		if err != nil {
			return nil, err
		}
		files[header.Name] = string(content)
	}
}

func (s *Suite) TestParseBundlePath() {
	tests := []struct {
		arg  string
		want BundlePath
	}{
		{arg: "my-files", want: BundlePath{Bundle: "my-files"}},
		{arg: "my-files:", want: BundlePath{Bundle: "my-files"}},
		{arg: "my-files:dags", want: BundlePath{Bundle: "my-files", Path: "dags"}},
		{arg: "my-files:dags/", want: BundlePath{Bundle: "my-files", Path: "dags", TrailingSlash: true}},
		{arg: "my-files:/dags/a.py", want: BundlePath{Bundle: "my-files", Path: "dags/a.py"}},
		{arg: "dep-2:my-files:", want: BundlePath{Deployment: "dep-2", Bundle: "my-files"}},
		{arg: "dep-2:my-files:dags/", want: BundlePath{Deployment: "dep-2", Bundle: "my-files", Path: "dags", TrailingSlash: true}},
	}
	for i := range tests {
		got, err := ParseBundlePath(tests[i].arg)
		s.NoError(err, tests[i].arg)
		s.Equal(tests[i].want, got, tests[i].arg)
	}
	s.Equal("dep-2:my-files:dags/", BundlePath{Deployment: "dep-2", Bundle: "my-files", Path: "dags", TrailingSlash: true}.String())

	_, err := ParseBundlePath(":dags")
	s.ErrorIs(err, errBundleRequired)

	_, err = ParseBundlePath("dep-2::dags")
	s.ErrorIs(err, errBundleRequired)

	_, err = ParseBundlePath(":my-files:dags")
	s.ErrorIs(err, errDeploymentRequired)
}

func (s *Suite) TestIsBundlePath() {
	s.True(isBundlePath("my-files:dags/a.py"))
	s.True(isBundlePath("my-files:"))
	s.False(isBundlePath("my-files"))
	s.False(isBundlePath("-"))
	s.False(isBundlePath("./odd:name.py"))
	// A single letter before the colon is a drive only on Windows, and a one-letter bundle name elsewhere.
	s.Equal(runtime.GOOS != "windows", isBundlePath(`C:\dags`))
	s.Equal(runtime.GOOS != "windows", isBundlePath("a:dags/"))
}

func (s *Suite) TestListBundleFilesWithFormat() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	version := testBundleVersion
	listResponse := &astrov1alpha1.ListBundleFilesResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &astrov1alpha1.BundleFileList{
			Version:     &version,
			Path:        "dags",
			Directories: []string{"finance"},
			Files:       []astrov1alpha1.BundleFile{{Path: "dags/a.py", Digest: "sha256:aaa", Size: 12}},
		},
	}

	s.Run("resolves the bundle by name and lists the directory", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := s.mockBundleLookup()
		mockV1Alpha1Client.On("ListBundleFilesWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.ListBundleFilesParams) bool {
			return p.Path != nil && *p.Path == "dags" && p.Recursive != nil && *p.Recursive && p.Version != nil && *p.Version == "old"
		})).Return(listResponse, nil).Once()

		err := ListBundleFilesWithFormat(BundlePath{Bundle: "my-files", Path: "dags"}, true, "old", ws, testBundleDeploymentID, "table", "", out, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Contains(out.String(), "dags/finance/")
		s.Contains(out.String(), "dags/a.py")
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("resolves the bundle by ID", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := s.mockBundleLookup()
		mockV1Alpha1Client.On("ListBundleFilesWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.ListBundleFilesParams) bool {
			return p.Path == nil && p.Recursive == nil && p.Version == nil
		})).Return(listResponse, nil).Once()

		err := ListBundleFilesWithFormat(BundlePath{Bundle: "bundle-1"}, false, "", ws, testBundleDeploymentID, "table", "", out, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("fails for an unknown bundle", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := s.mockBundleLookup()

		err := ListBundleFilesWithFormat(BundlePath{Bundle: "nope"}, false, "", ws, testBundleDeploymentID, "table", "", out, mockV1Client, mockV1Alpha1Client)
		s.ErrorContains(err, `no bundle with the ID or name "nope"`)
		mockV1Alpha1Client.AssertNotCalled(s.T(), "ListBundleFilesWithResponse")
	})

	s.Run("suggests the Deployment form for a Deployment ID read as a bundle", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := s.mockBundleLookup()

		err := ListBundleFilesWithFormat(BundlePath{Bundle: "cm2deploy0001x8kq0000000a", Path: "my-files"}, false, "", ws, testBundleDeploymentID, "table", "", out, mockV1Client, mockV1Alpha1Client)
		s.ErrorContains(err, `did you mean "cm2deploy0001x8kq0000000a:my-files:"?`)
	})
}

func (s *Suite) TestCatBundleFile() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("prints the file's content", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := s.mockBundleLookup()
		mockStreamClient := s.streamClient()
		mockStreamClient.On("ReadBundleFile", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.ReadBundleFileParams) bool {
			return p.Path == "dags/a.py" && p.Version != nil && *p.Version == "old"
		})).Return(streamedResponse(http.StatusOK, []byte("print('hi')\n")), nil).Once()

		err := CatBundleFile(BundlePath{Bundle: "my-files", Path: "dags/a.py"}, "old", ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, mockStreamClient)
		s.NoError(err)
		s.Equal("print('hi')\n", out.String())
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("requires a file path", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		err := CatBundleFile(BundlePath{Bundle: "my-files"}, "", ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.EqualError(err, "my-files: is a directory")

		err = CatBundleFile(BundlePath{Bundle: "my-files", Path: "dags", TrailingSlash: true}, "", ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.EqualError(err, "my-files:dags/ is a directory")
	})

	s.Run("surfaces an API error", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := s.mockBundleLookup()
		mockStreamClient := s.streamClient()
		mockStreamClient.On("ReadBundleFile", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything).Return(streamedResponse(http.StatusNotFound, []byte(`{"message":"no file at dags/a.py"}`)), nil).Once()

		err := CatBundleFile(BundlePath{Bundle: "my-files", Path: "dags/a.py"}, "", ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, mockStreamClient)
		s.EqualError(err, "no file at dags/a.py")
	})
}

func (s *Suite) TestCopyBundleFilesToBundle() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("copies a local file", func() {
		out := &bytes.Buffer{}
		localFile := filepath.Join(s.T().TempDir(), "a.py")
		s.NoError(os.WriteFile(localFile, []byte("print('hi')\n"), 0o600))
		mockV1Alpha1Client := s.mockBundleLookup()
		mockV1Alpha1Client.On("WriteBundleFileWithBodyWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.WriteBundleFileParams) bool {
			return p.Path == "dags/a.py" && p.ExpectedVersion != nil && *p.ExpectedVersion == "old"
		}), bundleFileContentType, mock.MatchedBy(func(body io.Reader) bool {
			content, err := io.ReadAll(body)
			return err == nil && string(content) == "print('hi')\n"
		})).Return(&astrov1alpha1.WriteBundleFileResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      bundleVersionResponse(),
		}, nil).Once()

		err := CopyBundleFiles(localFile, "my-files:dags/a.py", &BundleCopyOptions{ExpectedVersion: "old"}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.NoError(err)
		s.Contains(out.String(), "Copied "+localFile+" to my-files:dags/a.py at version "+testBundleVersion)
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("copies from stdin", func() {
		out := &bytes.Buffer{}
		defer testUtil.MockUserInput(s.T(), "from stdin")()
		mockV1Alpha1Client := s.mockBundleLookup()
		mockV1Alpha1Client.On("WriteBundleFileWithBodyWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.WriteBundleFileParams) bool {
			return p.Path == "dags/a.py" && p.ExpectedVersion == nil
		}), bundleFileContentType, mock.MatchedBy(func(body io.Reader) bool {
			content, err := io.ReadAll(body)
			return err == nil && string(content) == "from stdin"
		})).Return(&astrov1alpha1.WriteBundleFileResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      bundleVersionResponse(),
		}, nil).Once()

		err := CopyBundleFiles("-", "my-files:dags/a.py", &BundleCopyOptions{}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.NoError(err)
		s.Contains(out.String(), "Copied stdin to my-files:dags/a.py")
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("copies a directory's contents with -r", func() {
		out := &bytes.Buffer{}
		fromDir := s.T().TempDir()
		s.NoError(os.MkdirAll(filepath.Join(fromDir, "finance"), 0o755))
		s.NoError(os.WriteFile(filepath.Join(fromDir, "a.py"), []byte("a"), 0o600))
		s.NoError(os.WriteFile(filepath.Join(fromDir, "finance", "b.py"), []byte("b"), 0o600))
		mockV1Alpha1Client := s.mockBundleLookup()
		mockStreamClient := s.streamClient()
		mockStreamClient.On("WriteBundleDirectoryWithBody", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.WriteBundleDirectoryParams) bool {
			return p.Path != nil && *p.Path == "dags" && p.IfExists != nil && *p.IfExists == astrov1alpha1.WriteBundleDirectoryParamsIfExistsREPLACE && p.ExpectedVersion != nil && *p.ExpectedVersion == "old"
		}), bundleFileContentType, mock.MatchedBy(func(body io.Reader) bool {
			files, err := bundleDirectoryFiles(body)
			return err == nil && len(files) == 2 && files["a.py"] == "a" && files["finance/b.py"] == "b"
		})).Return(directoryWritten(testBundleVersion), nil).Once()

		err := CopyBundleFiles(fromDir, "my-files:dags", &BundleCopyOptions{Recursive: true, IfExists: "replace", ExpectedVersion: "old"}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, mockStreamClient)
		s.NoError(err)
		s.Contains(out.String(), "Copied 2 files from "+fromDir+" to my-files:dags at version "+testBundleVersion)
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("refuses a directory without -r", func() {
		out := &bytes.Buffer{}
		fromDir := s.T().TempDir()
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		err := CopyBundleFiles(fromDir, "my-files:dags", &BundleCopyOptions{}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.ErrorContains(err, "is a directory: copy it with -r")
	})

	s.Run("refuses a directory with no files to copy, without making a request", func() {
		out := &bytes.Buffer{}
		fromDir := s.T().TempDir()
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		err := CopyBundleFiles(fromDir, "my-files:dags", &BundleCopyOptions{Recursive: true}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.EqualError(err, fromDir+" holds no files to copy")

		writeLocalFiles(s, fromDir, map[string]string{".env": "secret"})
		err = CopyBundleFiles(fromDir, "my-files:dags", &BundleCopyOptions{Recursive: true}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.EqualError(err, fromDir+" holds no files to copy once 1 excluded paths are skipped")
	})

	s.Run("copies a directory reached through a symbolic link", func() {
		out := &bytes.Buffer{}
		realDir := s.T().TempDir()
		writeLocalFiles(s, realDir, map[string]string{"a.py": "a"})
		link := filepath.Join(s.T().TempDir(), "link")
		s.NoError(os.Symlink(realDir, link))
		mockV1Alpha1Client := s.mockBundleLookup()
		mockStreamClient := s.streamClient()
		mockStreamClient.On("WriteBundleDirectoryWithBody", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything, bundleFileContentType, mock.MatchedBy(func(body io.Reader) bool {
			files, err := bundleDirectoryFiles(body)
			return err == nil && len(files) == 1 && files["a.py"] == "a"
		})).Return(directoryWritten(testBundleVersion), nil).Once()

		err := CopyBundleFiles(link, "my-files:dags", &BundleCopyOptions{Recursive: true}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, mockStreamClient)
		s.NoError(err)
		s.Contains(out.String(), "Copied 1 files from ")
	})

	s.Run("refuses a symlink in a directory", func() {
		out := &bytes.Buffer{}
		fromDir := s.T().TempDir()
		s.NoError(os.Symlink("/etc/hosts", filepath.Join(fromDir, "hosts")))
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		err := CopyBundleFiles(fromDir, "my-files:", &BundleCopyOptions{Recursive: true}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.ErrorContains(err, "is a symbolic link: skip it with --exclude")
	})

	s.Run("fails when the local path is missing", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		err := CopyBundleFiles(filepath.Join(s.T().TempDir(), "missing.py"), "my-files:dags/a.py", &BundleCopyOptions{}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.ErrorContains(err, "failed to read")
	})

	s.Run("copies a file into a bundle directory, keeping its name", func() {
		for _, dst := range []string{"my-files:dags/", "my-files:"} {
			out := &bytes.Buffer{}
			localFile := filepath.Join(s.T().TempDir(), "a.py")
			s.NoError(os.WriteFile(localFile, []byte("x"), 0o600))
			wantPath := "dags/a.py"
			if dst == "my-files:" {
				wantPath = "a.py"
			}
			mockV1Alpha1Client := s.mockBundleLookup()
			mockV1Alpha1Client.On("WriteBundleFileWithBodyWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.WriteBundleFileParams) bool {
				return p.Path == wantPath
			}), bundleFileContentType, mock.Anything).Return(&astrov1alpha1.WriteBundleFileResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusOK},
				JSON200:      bundleVersionResponse(),
			}, nil).Once()

			err := CopyBundleFiles(localFile, dst, &BundleCopyOptions{}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
			s.NoError(err, dst)
			s.Contains(out.String(), "to my-files:"+wantPath+" at version", dst)
			mockV1Alpha1Client.AssertExpectations(s.T())
		}
	})

	s.Run("refuses stdin into a bundle directory", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		err := CopyBundleFiles("-", "my-files:dags/", &BundleCopyOptions{}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.ErrorIs(err, errCopyStdinName)
	})

	s.Run("skips excluded files in a directory", func() {
		out := &bytes.Buffer{}
		fromDir := s.T().TempDir()
		for _, name := range []string{"a.py", ".env", "notes.log", "keep.log", "__pycache__/a.pyc", ".venv/pyvenv.cfg", "tools/pyvenv.cfg", "tools/bin/python"} {
			s.NoError(os.MkdirAll(filepath.Dir(filepath.Join(fromDir, name)), 0o755))
			s.NoError(os.WriteFile(filepath.Join(fromDir, name), []byte(name), 0o600))
		}
		s.NoError(os.WriteFile(filepath.Join(fromDir, ".gitignore"), []byte("*.log\n"), 0o600))
		mockV1Alpha1Client := s.mockBundleLookup()
		mockStreamClient := s.streamClient()
		mockStreamClient.On("WriteBundleDirectoryWithBody", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything, bundleFileContentType, mock.MatchedBy(func(body io.Reader) bool {
			files, err := bundleDirectoryFiles(body)
			_, hasA := files["a.py"]
			_, hasIgnore := files[".gitignore"]
			_, hasKeep := files["keep.log"]
			return err == nil && len(files) == 3 && hasA && hasIgnore && hasKeep
		})).Return(directoryWritten(testBundleVersion), nil).Once()

		opts := &BundleCopyOptions{Recursive: true, Filter: BundleFilterOptions{Includes: []string{"keep.log"}}}
		err := CopyBundleFiles(fromDir, "my-files:", opts, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, mockStreamClient)
		s.NoError(err)
		s.Contains(out.String(), "Skipped 5 paths")
		s.Contains(out.String(), "Copied 3 files")
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("keeps excluded files with --no-default-excludes and --no-gitignore", func() {
		out := &bytes.Buffer{}
		fromDir := s.T().TempDir()
		s.NoError(os.WriteFile(filepath.Join(fromDir, ".env"), []byte("x"), 0o600))
		s.NoError(os.WriteFile(filepath.Join(fromDir, "a.log"), []byte("x"), 0o600))
		s.NoError(os.WriteFile(filepath.Join(fromDir, ".gitignore"), []byte("*.log\n"), 0o600))
		mockV1Alpha1Client := s.mockBundleLookup()
		mockStreamClient := s.streamClient()
		mockStreamClient.On("WriteBundleDirectoryWithBody", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything, bundleFileContentType, mock.MatchedBy(func(body io.Reader) bool {
			files, err := bundleDirectoryFiles(body)
			return err == nil && len(files) == 3
		})).Return(directoryWritten(testBundleVersion), nil).Once()

		opts := &BundleCopyOptions{Recursive: true, Filter: BundleFilterOptions{NoDefaultExcludes: true, NoGitignore: true}}
		err := CopyBundleFiles(fromDir, "my-files:", opts, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, mockStreamClient)
		s.NoError(err)
		s.NotContains(out.String(), "Skipped")
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("honors the .gitignore of the enclosing repository", func() {
		out := &bytes.Buffer{}
		repo := s.T().TempDir()
		fromDir := filepath.Join(repo, "dags")
		s.NoError(os.MkdirAll(filepath.Join(repo, ".git"), 0o755))
		s.NoError(os.MkdirAll(fromDir, 0o755))
		s.NoError(os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("*.log\n/dags/local/\n"), 0o600))
		s.NoError(os.MkdirAll(filepath.Join(fromDir, "local"), 0o755))
		s.NoError(os.WriteFile(filepath.Join(fromDir, "local", "a.py"), []byte("x"), 0o600))
		s.NoError(os.WriteFile(filepath.Join(fromDir, "a.log"), []byte("x"), 0o600))
		s.NoError(os.WriteFile(filepath.Join(fromDir, "a.py"), []byte("x"), 0o600))
		mockV1Alpha1Client := s.mockBundleLookup()
		mockStreamClient := s.streamClient()
		mockStreamClient.On("WriteBundleDirectoryWithBody", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything, bundleFileContentType, mock.MatchedBy(func(body io.Reader) bool {
			files, err := bundleDirectoryFiles(body)
			_, hasA := files["a.py"]
			return err == nil && len(files) == 1 && hasA
		})).Return(directoryWritten(testBundleVersion), nil).Once()

		err := CopyBundleFiles(fromDir, "my-files:dags", &BundleCopyOptions{Recursive: true}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, mockStreamClient)
		s.NoError(err)
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("refuses flags that do not apply", func() {
		out := &bytes.Buffer{}
		localFile := filepath.Join(s.T().TempDir(), "a.py")
		s.NoError(os.WriteFile(localFile, []byte("x"), 0o600))
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		err := CopyBundleFiles(localFile, "my-files:a.py", &BundleCopyOptions{Version: "old"}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.ErrorIs(err, errCopyVersion)

		err = CopyBundleFiles(localFile, "my-files:a.py", &BundleCopyOptions{IfExists: "replace"}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.ErrorIs(err, errCopyIfExists)

		err = CopyBundleFiles(localFile, "my-files:a.py", &BundleCopyOptions{IfExists: "overwrite"}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.ErrorContains(err, `invalid --if-exists "overwrite"`)

		err = CopyBundleFiles("-", "my-files:a.py", &BundleCopyOptions{Recursive: true}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.ErrorIs(err, errCopyStdin)
	})

	s.Run("surfaces a version conflict", func() {
		out := &bytes.Buffer{}
		localFile := filepath.Join(s.T().TempDir(), "a.py")
		s.NoError(os.WriteFile(localFile, []byte("x"), 0o600))
		mockV1Alpha1Client := s.mockBundleLookup()
		mockV1Alpha1Client.On("WriteBundleFileWithBodyWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything, bundleFileContentType, mock.Anything).Return(&astrov1alpha1.WriteBundleFileResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusConflict},
			Body:         []byte(`{"message":"bundle has moved on; re-read and retry"}`),
		}, nil).Once()

		err := CopyBundleFiles(localFile, "my-files:a.py", &BundleCopyOptions{ExpectedVersion: "old"}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.EqualError(err, "bundle has moved on; re-read and retry")
	})
}

func (s *Suite) TestCopyBundleFilesFromBundle() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockReadFile := func() *astrov1alpha1_mocks.ClientInterface {
		mockStreamClient := s.streamClient()
		mockStreamClient.On("ReadBundleFile", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything).Return(streamedResponse(http.StatusOK, []byte("a")), nil).Once()
		return mockStreamClient
	}

	s.Run("copies a file to a local path", func() {
		out := &bytes.Buffer{}
		localFile := filepath.Join(s.T().TempDir(), "copy.py")
		mockV1Alpha1Client := s.mockBundleLookup()
		mockStreamClient := s.streamClient()
		mockStreamClient.On("ReadBundleFile", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.ReadBundleFileParams) bool {
			return p.Path == "dags/a.py" && p.Version != nil && *p.Version == "old"
		})).Return(streamedResponse(http.StatusOK, []byte("a")), nil).Once()

		err := CopyBundleFiles("my-files:dags/a.py", localFile, &BundleCopyOptions{Version: "old"}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, mockStreamClient)
		s.NoError(err)
		s.Contains(out.String(), "Copied my-files:dags/a.py at version "+testBundleVersion+" to "+localFile)
		content, err := os.ReadFile(localFile)
		s.NoError(err)
		s.Equal("a", string(content))
	})

	s.Run("leaves the local file as it was when the download fails", func() {
		out := &bytes.Buffer{}
		toDir := s.T().TempDir()
		localFile := filepath.Join(toDir, "a.py")
		s.NoError(os.WriteFile(localFile, []byte("before"), 0o600))
		mockV1Alpha1Client := s.mockBundleLookup()
		mockStreamClient := s.streamClient()
		resp := streamedResponse(http.StatusOK, nil)
		resp.Body = io.NopCloser(io.MultiReader(bytes.NewReader([]byte("part")), iotest.ErrReader(errors.New("connection reset"))))
		mockStreamClient.On("ReadBundleFile", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything).Return(resp, nil).Once()

		err := CopyBundleFiles("my-files:dags/a.py", localFile, &BundleCopyOptions{}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, mockStreamClient)
		s.EqualError(err, "connection reset")
		content, err := os.ReadFile(localFile)
		s.NoError(err)
		s.Equal("before", string(content))
		entries, err := os.ReadDir(toDir)
		s.NoError(err)
		s.Len(entries, 1)
	})

	s.Run("copies a file into an existing local directory", func() {
		out := &bytes.Buffer{}
		toDir := s.T().TempDir()
		mockV1Alpha1Client := s.mockBundleLookup()

		err := CopyBundleFiles("my-files:dags/a.py", toDir, &BundleCopyOptions{}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, mockReadFile())
		s.NoError(err)
		s.FileExists(filepath.Join(toDir, "a.py"))
	})

	s.Run("copies a file into a new local directory written with a trailing slash", func() {
		out := &bytes.Buffer{}
		toDir := filepath.Join(s.T().TempDir(), "out")
		mockV1Alpha1Client := s.mockBundleLookup()

		err := CopyBundleFiles("my-files:dags/a.py", toDir+"/", &BundleCopyOptions{}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, mockReadFile())
		s.NoError(err)
		s.FileExists(filepath.Join(toDir, "a.py"))
	})

	s.Run("refuses filter flags", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		opts := &BundleCopyOptions{Recursive: true, Filter: BundleFilterOptions{NoGitignore: true}}
		err := CopyBundleFiles("my-files:dags", s.T().TempDir(), opts, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.ErrorIs(err, errCopyFilterFlags)
	})

	s.Run("copies a file to stdout", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := s.mockBundleLookup()

		err := CopyBundleFiles("my-files:dags/a.py", "-", &BundleCopyOptions{}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, mockReadFile())
		s.NoError(err)
		s.Equal("a", out.String())
	})

	s.Run("copies a directory's contents with -r", func() {
		out := &bytes.Buffer{}
		toDir := filepath.Join(s.T().TempDir(), "out")
		mockV1Alpha1Client := s.mockBundleLookup()
		mockStreamClient := s.streamClient()
		mockStreamClient.On("ReadBundleDirectory", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.ReadBundleDirectoryParams) bool {
			return p.Path != nil && *p.Path == "dags" && p.Version != nil && *p.Version == "old"
		})).Return(streamedResponse(http.StatusOK, bundleDirectoryArchive(s, map[string]string{"a.py": "a", "finance/b.py": "b"})), nil).Once()

		err := CopyBundleFiles("my-files:dags", toDir, &BundleCopyOptions{Recursive: true, Version: "old"}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, mockStreamClient)
		s.NoError(err)
		s.Contains(out.String(), "Copied 2 files from my-files:dags at version "+testBundleVersion+" to "+toDir)
		content, err := os.ReadFile(filepath.Join(toDir, "finance", "b.py"))
		s.NoError(err)
		s.Equal("b", string(content))
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("refuses a tar path outside the local directory", func() {
		out := &bytes.Buffer{}
		parent := s.T().TempDir()
		mockV1Alpha1Client := s.mockBundleLookup()
		mockStreamClient := s.streamClient()
		mockStreamClient.On("ReadBundleDirectory", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything).Return(streamedResponse(http.StatusOK, bundleDirectoryArchive(s, map[string]string{"../escaped.py": "x"})), nil).Once()

		err := CopyBundleFiles("my-files:", filepath.Join(parent, "out"), &BundleCopyOptions{Recursive: true}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, mockStreamClient)
		s.Error(err)
		s.NoFileExists(filepath.Join(parent, "escaped.py"))
	})

	s.Run("refuses the bundle root without -r", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		err := CopyBundleFiles("my-files:", s.T().TempDir(), &BundleCopyOptions{}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.ErrorContains(err, "is a directory: copy it with -r")
	})

	s.Run("refuses flags that do not apply", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		err := CopyBundleFiles("my-files:a.py", "./a.py", &BundleCopyOptions{ExpectedVersion: "old"}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.ErrorIs(err, errCopyWriteFlags)

		err = CopyBundleFiles("my-files:dags", "-", &BundleCopyOptions{Recursive: true}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.ErrorIs(err, errCopyStdout)
	})
}

func (s *Suite) TestCopyBundleFilesWithinBundle() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("copies a file", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := s.mockBundleLookup()
		mockV1Alpha1Client.On("DuplicateBundleFileWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(r astrov1alpha1.MoveBundlePathRequest) bool {
			return r.FromPath == "dags/a.py" && *r.ToPath == "dags/b.py" && *r.IfExists == astrov1alpha1.MoveBundlePathRequestIfExistsREPLACE && *r.ExpectedVersion == "old"
		})).Return(&astrov1alpha1.DuplicateBundleFileResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      bundleVersionResponse(),
		}, nil).Once()

		err := CopyBundleFiles("my-files:dags/a.py", "my-files:dags/b.py", &BundleCopyOptions{IfExists: "replace", ExpectedVersion: "old"}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.NoError(err)
		s.Contains(out.String(), "Copied my-files:dags/a.py to my-files:dags/b.py at version "+testBundleVersion)
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("copies a file into a DST ending in a slash, keeping its name", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := s.mockBundleLookup()
		mockV1Alpha1Client.On("DuplicateBundleFileWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(r astrov1alpha1.MoveBundlePathRequest) bool {
			return r.FromPath == "dags/a.py" && *r.ToPath == "backup/a.py"
		})).Return(&astrov1alpha1.DuplicateBundleFileResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      bundleVersionResponse(),
		}, nil).Once()

		err := CopyBundleFiles("my-files:dags/a.py", "my-files:backup/", &BundleCopyOptions{}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.NoError(err)
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("copies a directory with -r", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := s.mockBundleLookup()
		mockV1Alpha1Client.On("DuplicateBundleDirectoryWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(r astrov1alpha1.MoveBundlePathRequest) bool {
			return r.FromPath == "dags" && *r.ToPath == "backup" && r.IfExists == nil && r.ExpectedVersion == nil
		})).Return(&astrov1alpha1.DuplicateBundleDirectoryResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      bundleVersionResponse(),
		}, nil).Once()

		err := CopyBundleFiles("my-files:dags", "my-files:backup", &BundleCopyOptions{Recursive: true}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.NoError(err)
		s.Contains(out.String(), "Copied my-files:dags to my-files:backup at version "+testBundleVersion)
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("resolves one bundle named by name and by ID with one lookup", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := s.mockBundleLookup()
		mockV1Alpha1Client.On("DuplicateBundleFileWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything).Return(&astrov1alpha1.DuplicateBundleFileResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      bundleVersionResponse(),
		}, nil).Once()

		err := CopyBundleFiles("my-files:a.py", "bundle-1:b.py", &BundleCopyOptions{}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.NoError(err)
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("streams a copy from an earlier version", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := s.mockBundleLookup()
		mockStreamClient := s.streamClient()
		mockStreamClient.On("ReadBundleFile", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.ReadBundleFileParams) bool {
			return p.Path == "a.py" && *p.Version == "old"
		})).Return(streamedResponse(http.StatusOK, []byte("a")), nil).Once()
		mockV1Alpha1Client.On("WriteBundleFileWithBodyWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.WriteBundleFileParams) bool {
			return p.Path == "b.py"
		}), bundleFileContentType, mock.Anything).Return(&astrov1alpha1.WriteBundleFileResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.BundleVersion{Version: "new"},
		}, nil).Once()

		err := CopyBundleFiles("my-files:a.py", "my-files:b.py", &BundleCopyOptions{Version: "old"}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, mockStreamClient)
		s.NoError(err)
		s.Contains(out.String(), "Copied my-files:a.py at version "+testBundleVersion+" to my-files:b.py at version new")
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("refuses flags that do not apply", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		err := CopyBundleFiles("my-files:a.py", "my-files:b.py", &BundleCopyOptions{IfExists: "merge"}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.ErrorIs(err, errMergeFile)

		err = CopyBundleFiles("my-files:", "my-files:b", &BundleCopyOptions{}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.ErrorContains(err, "is a directory: copy it with -r")
	})
}

func (s *Suite) TestCopyBundleFilesBetweenBundles() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("streams a file from one bundle into another", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := s.mockTwoBundleLookup()
		mockStreamClient := s.streamClient()
		mockStreamClient.On("ReadBundleFile", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.ReadBundleFileParams) bool {
			return p.Path == "a.py" && p.Version == nil
		})).Return(streamedResponse(http.StatusOK, []byte("a")), nil).Once()
		mockV1Alpha1Client.On("WriteBundleFileWithBodyWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-2", mock.MatchedBy(func(p *astrov1alpha1.WriteBundleFileParams) bool {
			return p.Path == "dags/a.py" && *p.ExpectedVersion == "old"
		}), bundleFileContentType, mock.MatchedBy(func(body io.Reader) bool {
			content, err := io.ReadAll(body)
			return err == nil && string(content) == "a"
		})).Return(&astrov1alpha1.WriteBundleFileResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.BundleVersion{Version: "new"},
		}, nil).Once()

		err := CopyBundleFiles("my-files:a.py", "other-files:dags/", &BundleCopyOptions{ExpectedVersion: "old"}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, mockStreamClient)
		s.NoError(err)
		s.Contains(out.String(), "Copied my-files:a.py at version "+testBundleVersion+" to other-files:dags/a.py at version new")
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("streams a directory into a bundle on another Deployment", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := s.mockBundleLookup()
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, "dep-2").Return(&astrov1.GetDeploymentResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1.Deployment{Id: "dep-2", OrganizationId: mockOrgID},
		}, nil).Once()
		name := "my-files"
		mockV1Alpha1Client.On("ListBundlesWithResponse", mock.Anything, mock.Anything, "dep-2", mock.Anything).Return(&astrov1alpha1.ListBundlesResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.BundlesPaginated{TotalCount: 1, Bundles: []astrov1alpha1.DeploymentBundle{{Id: "bundle-9", Name: &name}}},
		}, nil).Once()
		mockStreamClient := s.streamClient()
		mockStreamClient.On("ReadBundleDirectory", mock.Anything, mock.Anything, testBundleDeploymentID, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.ReadBundleDirectoryParams) bool {
			return *p.Path == "dags"
		})).Return(streamedResponse(http.StatusOK, bundleDirectoryArchive(s, map[string]string{"a.py": "a", "finance/b.py": "b"})), nil).Once()
		mockStreamClient.On("WriteBundleDirectoryWithBody", mock.Anything, mock.Anything, "dep-2", "bundle-9", mock.MatchedBy(func(p *astrov1alpha1.WriteBundleDirectoryParams) bool {
			return *p.Path == "dags" && *p.IfExists == astrov1alpha1.WriteBundleDirectoryParamsIfExistsMERGE
		}), bundleFileContentType, mock.MatchedBy(func(body io.Reader) bool {
			files, err := bundleDirectoryFiles(body)
			return err == nil && len(files) == 2 && files["finance/b.py"] == "b"
		})).Return(directoryWritten("new"), nil).Once()

		err := CopyBundleFiles("my-files:dags", "dep-2:my-files:dags", &BundleCopyOptions{Recursive: true, IfExists: "merge"}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, mockStreamClient)
		s.NoError(err)
		s.Contains(out.String(), "Copied my-files:dags at version "+testBundleVersion+" to dep-2:my-files:dags at version new")
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("reads SRC again, at the version read first, for a write the rate limits refuse", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := s.mockTwoBundleLookup()
		mockStreamClient := s.streamClient()
		archive := bundleDirectoryArchive(s, map[string]string{"a.py": "a"})
		mockStreamClient.On("ReadBundleDirectory", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.ReadBundleDirectoryParams) bool {
			return p.Version == nil
		})).Return(streamedResponse(http.StatusOK, archive), nil).Once()
		mockStreamClient.On("ReadBundleDirectory", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.ReadBundleDirectoryParams) bool {
			return p.Version != nil && *p.Version == testBundleVersion
		})).Return(streamedResponse(http.StatusOK, archive), nil).Once()
		var sent []map[string]string
		readBody := func(args mock.Arguments) {
			files, err := bundleDirectoryFiles(args.Get(6).(io.Reader))
			s.NoError(err)
			sent = append(sent, files)
		}
		rateLimited := &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{"Retry-After": {"0"}}, Body: io.NopCloser(strings.NewReader(""))}
		mockStreamClient.On("WriteBundleDirectoryWithBody", mock.Anything, mock.Anything, mock.Anything, "bundle-2", mock.Anything, bundleFileContentType, mock.Anything).Run(readBody).Return(rateLimited, nil).Once()
		mockStreamClient.On("WriteBundleDirectoryWithBody", mock.Anything, mock.Anything, mock.Anything, "bundle-2", mock.Anything, bundleFileContentType, mock.Anything).Run(readBody).Return(directoryWritten("new"), nil).Once()

		err := CopyBundleFiles("my-files:dags", "other-files:dags", &BundleCopyOptions{Recursive: true}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, mockStreamClient)
		s.NoError(err)
		s.Equal([]map[string]string{{"a.py": "a"}, {"a.py": "a"}}, sent)
		s.Contains(out.String(), "Copied my-files:dags at version "+testBundleVersion+" to other-files:dags at version new")
	})

	s.Run("surfaces an error reading SRC without writing", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := s.mockTwoBundleLookup()
		mockStreamClient := s.streamClient()
		mockStreamClient.On("ReadBundleDirectory", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything).Return(streamedResponse(http.StatusNotFound, []byte(`{"message":"no directory at dags"}`)), nil).Once()

		err := CopyBundleFiles("my-files:dags", "other-files:dags", &BundleCopyOptions{Recursive: true}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, mockStreamClient)
		s.EqualError(err, "no directory at dags")
	})

	s.Run("refuses --if-exists for a file", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := s.mockTwoBundleLookup()

		err := CopyBundleFiles("my-files:a.py", "other-files:a.py", &BundleCopyOptions{IfExists: "replace"}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
		s.ErrorIs(err, errCopyIfExists)
	})
}

func (s *Suite) TestCopyBundleFilesNeedsABundlePath() {
	out := &bytes.Buffer{}
	mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

	err := CopyBundleFiles("./a.py", "./b.py", &BundleCopyOptions{}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client, s.streamClient())
	s.ErrorIs(err, errCopyNoBundle)
}

func (s *Suite) TestMoveBundlePath() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	listing := func(mockV1Alpha1Client *astrov1alpha1_mocks.ClientWithResponsesInterface) {
		mockV1Alpha1Client.On("ListBundleFilesWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.ListBundleFilesParams) bool {
			return p.Path != nil && *p.Path == "dags" && p.Recursive == nil
		})).Return(&astrov1alpha1.ListBundleFilesResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &astrov1alpha1.BundleFileList{
				Path:        "dags",
				Directories: []string{"finance"},
				Files:       []astrov1alpha1.BundleFile{{Path: "dags/a.py", Digest: "sha256:aaa", Size: 1}},
			},
		}, nil).Once()
	}

	s.Run("moves a file", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := s.mockBundleLookup()
		listing(mockV1Alpha1Client)
		mockV1Alpha1Client.On("MoveBundleFileWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(r astrov1alpha1.MoveBundlePathRequest) bool {
			return r.FromPath == "dags/a.py" && *r.ToPath == "dags/b.py" && r.IfExists == nil
		})).Return(&astrov1alpha1.MoveBundleFileResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      bundleVersionResponse(),
		}, nil).Once()

		err := MoveBundlePath("my-files:dags/a.py", "my-files:dags/b.py", "", "", ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Contains(out.String(), "Moved my-files:dags/a.py to my-files:dags/b.py at version "+testBundleVersion)
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("moves a directory into a DST ending in a slash, checking it at the expected version", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := s.mockBundleLookup()
		mockV1Alpha1Client.On("ListBundleFilesWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.ListBundleFilesParams) bool {
			return p.Path == nil && *p.Version == "old"
		})).Return(&astrov1alpha1.ListBundleFilesResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.BundleFileList{Directories: []string{"staging"}, Files: []astrov1alpha1.BundleFile{}},
		}, nil).Once()
		mockV1Alpha1Client.On("MoveBundleDirectoryWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(r astrov1alpha1.MoveBundlePathRequest) bool {
			return r.FromPath == "staging" && *r.ToPath == "archive/staging" && *r.IfExists == astrov1alpha1.MoveBundlePathRequestIfExistsMERGE && *r.ExpectedVersion == "old"
		})).Return(&astrov1alpha1.MoveBundleDirectoryResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      bundleVersionResponse(),
		}, nil).Once()

		err := MoveBundlePath("my-files:staging", "my-files:archive/", "merge", "old", ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Contains(out.String(), "Moved my-files:staging to my-files:archive/staging at version "+testBundleVersion)
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("fails when nothing is at SRC", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := s.mockBundleLookup()
		listing(mockV1Alpha1Client)

		err := MoveBundlePath("my-files:dags/missing.py", "my-files:dags/b.py", "", "", ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client)
		s.EqualError(err, "no file or directory at my-files:dags/missing.py")
	})

	s.Run("refuses a move between bundles", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := s.mockTwoBundleLookup()

		err := MoveBundlePath("my-files:a.py", "other-files:a.py", "", "", ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client)
		s.ErrorIs(err, errMoveBetweenBundles)
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("refuses merge for a file", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := s.mockBundleLookup()
		listing(mockV1Alpha1Client)

		err := MoveBundlePath("my-files:dags/a.py", "my-files:dags/b.py", "merge", "", ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client)
		s.ErrorIs(err, errMergeFile)
	})

	s.Run("refuses arguments that do not apply", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		err := MoveBundlePath("./a.py", "my-files:a.py", "", "", ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client)
		s.ErrorIs(err, errMoveNoBundle)

		err = MoveBundlePath("my-files:", "my-files:b", "", "", ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client)
		s.ErrorIs(err, errMoveRoot)
	})
}

func (s *Suite) TestRemoveBundlePath() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("removes a file without asking", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := s.mockBundleLookup()
		mockV1Alpha1Client.On("DeleteBundleFileWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.DeleteBundleFileParams) bool {
			return p.Path == "dags/a.py" && p.ExpectedVersion != nil && *p.ExpectedVersion == "old"
		})).Return(&astrov1alpha1.DeleteBundleFileResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      bundleVersionResponse(),
		}, nil).Once()

		err := RemoveBundlePath(BundlePath{Bundle: "my-files", Path: "dags/a.py"}, false, "old", ws, testBundleDeploymentID, false, out, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Contains(out.String(), "Removed my-files:dags/a.py at version "+testBundleVersion)
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("removes a directory with -r, without asking with force", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := s.mockBundleLookup()
		mockV1Alpha1Client.On("DeleteBundleDirectoryWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.DeleteBundleDirectoryParams) bool {
			return p.Path != nil && *p.Path == "dags" && p.ExpectedVersion == nil
		})).Return(&astrov1alpha1.DeleteBundleDirectoryResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      bundleVersionResponse(),
		}, nil).Once()

		err := RemoveBundlePath(BundlePath{Bundle: "my-files", Path: "dags"}, true, "", ws, testBundleDeploymentID, true, out, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Contains(out.String(), "Removed my-files:dags at version "+testBundleVersion)
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("empties the bundle with -r on the root", func() {
		defer testUtil.MockUserInput(s.T(), "y")()
		out := &bytes.Buffer{}
		mockV1Alpha1Client := s.mockBundleLookup()
		mockV1Alpha1Client.On("DeleteBundleDirectoryWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.DeleteBundleDirectoryParams) bool {
			return p.Path == nil
		})).Return(&astrov1alpha1.DeleteBundleDirectoryResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      bundleVersionResponse(),
		}, nil).Once()

		err := RemoveBundlePath(BundlePath{Bundle: "my-files"}, true, "", ws, testBundleDeploymentID, false, out, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Contains(out.String(), "Removed my-files: at version "+testBundleVersion)
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("does not remove when the confirmation is declined", func() {
		defer testUtil.MockUserInput(s.T(), "n")()
		out := &bytes.Buffer{}
		mockV1Alpha1Client := s.mockBundleLookup()

		err := RemoveBundlePath(BundlePath{Bundle: "my-files", Path: "dags"}, true, "", ws, testBundleDeploymentID, false, out, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Contains(out.String(), "Canceling removal")
		mockV1Alpha1Client.AssertNotCalled(s.T(), "DeleteBundleDirectoryWithResponse")
	})

	s.Run("refuses the bundle root without -r", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		err := RemoveBundlePath(BundlePath{Bundle: "my-files"}, false, "", ws, testBundleDeploymentID, true, out, mockV1Client, mockV1Alpha1Client)
		s.ErrorContains(err, "is a directory: remove it with -r")
	})
}

func (s *Suite) TestUploadBundleDirectory() {
	s.Run("returns the error of packing a file, rather than the request's", func() {
		mockStreamClient := s.streamClient()
		mockStreamClient.On("WriteBundleDirectoryWithBody", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything, bundleFileContentType, mock.Anything).Run(func(args mock.Arguments) {
			_, _ = io.ReadAll(args.Get(6).(io.Reader))
		}).Return(nil, io.ErrUnexpectedEOF).Once()

		_, err := uploadBundleDirectory(s.T().TempDir(), &bundleSyncPlan{added: []string{"missing.py"}}, &astrov1alpha1.WriteBundleDirectoryParams{}, bundleRef{bundleID: "bundle-1"}, mockStreamClient)
		s.ErrorContains(err, "failed to read missing.py")
	})

	s.Run("packs the archive again for each write the rate limits refuse", func() {
		fromDir := s.T().TempDir()
		writeLocalFiles(s, fromDir, map[string]string{"a.py": "a"})
		mockStreamClient := s.streamClient()
		var sent []map[string]string
		readBody := func(args mock.Arguments) {
			files, err := bundleDirectoryFiles(args.Get(6).(io.Reader))
			s.NoError(err)
			sent = append(sent, files)
		}
		rateLimited := &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{"Retry-After": {"0"}}, Body: io.NopCloser(strings.NewReader(""))}
		mockStreamClient.On("WriteBundleDirectoryWithBody", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything, bundleFileContentType, mock.Anything).Run(readBody).Return(rateLimited, nil).Once()
		mockStreamClient.On("WriteBundleDirectoryWithBody", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything, bundleFileContentType, mock.Anything).Run(readBody).Return(directoryWritten("v2"), nil).Once()

		version, err := uploadBundleDirectory(fromDir, &bundleSyncPlan{added: []string{"a.py"}}, &astrov1alpha1.WriteBundleDirectoryParams{}, bundleRef{bundleID: "bundle-1"}, mockStreamClient)
		s.NoError(err)
		s.Equal("v2", version)
		s.Equal([]map[string]string{{"a.py": "a"}, {"a.py": "a"}}, sent)
	})

	s.Run("packs deletes, then files", func() {
		fromDir := s.T().TempDir()
		writeLocalFiles(s, fromDir, map[string]string{"foo": "now a file"})
		plan := &bundleSyncPlan{
			added:   []string{"foo"},
			deleted: []string{"foo/bar.py"},
		}

		var archive bytes.Buffer
		s.Require().NoError(packBundleDirectory(&archive, fromDir, plan))
		gzipReader, err := gzip.NewReader(&archive)
		s.Require().NoError(err)
		tarReader := tar.NewReader(gzipReader)
		var entries []string
		for {
			header, err := tarReader.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			s.Require().NoError(err)
			content, err := io.ReadAll(tarReader)
			s.Require().NoError(err)
			entries = append(entries, header.Name+"="+string(content))
		}
		s.Equal([]string{"__DEL__foo/bar.py=", "foo=now a file"}, entries)
	})
}

func (s *Suite) TestUnpackBundleDirectory() {
	s.Run("refuses two paths that differ only in case where the file system ignores case", func() {
		toDir := s.T().TempDir()
		s.NoError(os.WriteFile(filepath.Join(toDir, "probe"), nil, 0o600))
		_, err := os.Stat(filepath.Join(toDir, "PROBE"))
		ignoresCase := err == nil
		archive := bundleDirectoryArchive(s, map[string]string{"README.md": "upper", "readme.md": "lower"})

		files, err := unpackBundleDirectory(bytes.NewReader(archive), toDir, nil)
		if ignoresCase {
			s.ErrorContains(err, "in the bundle are one file in "+toDir+", whose file system ignores case")
			s.Equal(1, files)
		} else {
			s.NoError(err)
			s.Equal(2, files)
		}
	})

	s.Run("refuses an archive whose gzip checksum is wrong", func() {
		archive := bundleDirectoryArchive(s, map[string]string{"a.py": "a"})
		// The gzip trailer is the checksum, then the length, each 4 bytes.
		archive[len(archive)-8] ^= 0xff

		_, err := unpackBundleDirectory(bytes.NewReader(archive), s.T().TempDir(), nil)
		s.ErrorIs(err, gzip.ErrChecksum)
	})
}
