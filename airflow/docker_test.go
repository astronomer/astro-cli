package airflow

import (
	"io"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/airflow/mocks"
	airflowTypes "github.com/astronomer/astro-cli/airflow/types"
)

var errMockDocker = errors.New("mock docker compose error")

func (s *Suite) TestRepositoryName() {
	s.Equal(repositoryName("test-repo"), "test-repo/airflow")
}

func (s *Suite) TestImageName() {
	s.Equal(ImageName("test-repo", "0.15.0"), "test-repo/airflow:0.15.0")
}

func (s *Suite) TestSanitizeImageName() {
	// Already-valid names must pass through byte-for-byte. Changing them would
	// rename cached image tags and orphan users' local images.
	unchanged := []string{
		"simple",
		"project_name",
		"my-project",
		"my-project_abc123", // typical ProjectNameUnique output
		"a__b",              // docker allows a double-underscore separator
		"foo.bar",
		"tmp155bkx9_684ec5",
	}
	for _, in := range unchanged {
		s.Equal(in, sanitizeImageName(in), "valid name %q must not change", in)
	}

	// Malformed names must become valid docker image names. These are the
	// shapes that broke `astro dev` builds from random temp-dir names.
	fixes := map[string]string{
		"a-_b":                  "a-b",                  // the reported "-_" double separator
		"__x":                   "x",                    // leading separators trimmed
		"-lead":                 "lead",                 // leading separator trimmed
		"trail-":                "trail",                // trailing separator trimmed
		"!!!":                   "project",              // all punctuation -> non-empty fallback
		"tmp-155-bkx-9-_684ec5": "tmp-155-bkx-9-684ec5", // the exact failing shape
		"UPPER":                 "upper",                // docker image names are lowercase
	}
	for in, want := range fixes {
		got := sanitizeImageName(in)
		s.Equal(want, got, "input %q", in)
		s.True(validImageName.MatchString(got), "sanitized %q -> %q must be a valid image name", in, got)
	}

	// Whatever the input, the result is always a valid, non-empty image name.
	for _, in := range []string{"", "-", "___", ".-.", "a-_-_-b", "9", "-_-"} {
		got := sanitizeImageName(in)
		s.NotEmpty(got)
		s.True(validImageName.MatchString(got), "sanitized %q -> %q must be a valid image name", in, got)
	}
}

func (s *Suite) TestNewDAGChecker() {
	_, err := NewDAGChecker(s.T().TempDir(), "", "Dockerfile", "")
	s.NoError(err)
}

var errExecMock = errors.New("docker is not running")

func (s *Suite) TestDAGCheckerPytest() {
	checker := DAGChecker{}
	s.Run("success", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", mock.Anything, airflowTypes.ImageBuildConfig{Path: checker.airflowHome, NoCache: false}).Return(nil).Once()
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, []string{}, mock.Anything, airflowTypes.ImageBuildConfig{Path: checker.airflowHome, NoCache: false}).Return("0", nil).Once()

		checker.imageHandler = imageHandler

		resp, err := checker.Pytest("", "", "", "", nil)

		s.NoError(err)
		s.Equal("", resp)
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("success custom image", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("TagLocalImage", mock.Anything).Return(nil)
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, []string{}, mock.Anything, airflowTypes.ImageBuildConfig{Path: checker.airflowHome, NoCache: false}).Return("0", nil).Once()

		checker.imageHandler = imageHandler

		resp, err := checker.Pytest("", "custom-image-name", "", "", nil)

		s.NoError(err)
		s.Equal("", resp)
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("unexpected exit code", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", mock.Anything, airflowTypes.ImageBuildConfig{Path: checker.airflowHome, NoCache: false}).Return(nil).Once()
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, []string{}, mock.Anything, airflowTypes.ImageBuildConfig{Path: checker.airflowHome, NoCache: false}).Return("1", nil).Once()

		mockResponse := "1"
		checker.imageHandler = imageHandler

		resp, err := checker.Pytest("", "", "", "", nil)
		s.Contains(err.Error(), "something went wrong while Pytesting your Dags")
		s.Equal(mockResponse, resp)
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("internal error exit code 10 reported as failure", func() {
		// exit code 10 substring-contains "0"; the old check reported it as a pass
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", mock.Anything, airflowTypes.ImageBuildConfig{Path: checker.airflowHome, NoCache: false}).Return(nil).Once()
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, []string{}, mock.Anything, airflowTypes.ImageBuildConfig{Path: checker.airflowHome, NoCache: false}).Return("10", nil).Once()

		checker.imageHandler = imageHandler

		resp, err := checker.Pytest("", "", "", "", nil)
		s.Contains(err.Error(), "something went wrong while Pytesting your Dags")
		s.Equal("10", resp)
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("interrupt exit code 130 reported as failure", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", mock.Anything, airflowTypes.ImageBuildConfig{Path: checker.airflowHome, NoCache: false}).Return(nil).Once()
		imageHandler.On("Pytest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, []string{}, mock.Anything, airflowTypes.ImageBuildConfig{Path: checker.airflowHome, NoCache: false}).Return("130", nil).Once()

		checker.imageHandler = imageHandler

		resp, err := checker.Pytest("", "", "", "", nil)
		s.Contains(err.Error(), "something went wrong while Pytesting your Dags")
		s.Equal("130", resp)
		imageHandler.AssertExpectations(s.T())
	})

	s.Run("image build failure", func() {
		imageHandler := new(mocks.ImageHandler)
		imageHandler.On("Build", "", mock.Anything, airflowTypes.ImageBuildConfig{Path: checker.airflowHome, NoCache: false}).Return(errMockDocker).Once()

		checker.imageHandler = imageHandler

		_, err := checker.Pytest("", "", "", "", nil)
		s.ErrorIs(err, errMockDocker)
		imageHandler.AssertExpectations(s.T())
	})
}

// parseProject is a project directory holding the DAG integrity test that
// Parse runs, at the path a 1.x project keeps it.
func parseProject(s *Suite) string {
	dir := s.T().TempDir()
	s.Require().NoError(os.MkdirAll(filepath.Join(dir, ".astro"), 0o755))
	s.Require().NoError(os.WriteFile(filepath.Join(dir, DefaultTestPath), []byte("def test_dags(): pass\n"), 0o600))
	return dir
}

func (s *Suite) TestDAGCheckerParse() {
	for _, tc := range []struct {
		name     string
		exitCode string
		wantErr  string
	}{
		{"success", "0", ""},
		{"exit code 1", "1", "See above for errors detected in your Dags"},
		{"exit code 2", "2", "something went wrong while parsing your Dags"},
		// exit codes 10 and 130 (Ctrl-C) substring-contain "1"; the old check
		// reported them as a clean DAG error
		{"internal error exit code 10", "10", "something went wrong while parsing your Dags"},
		{"interrupt exit code 130", "130", "something went wrong while parsing your Dags"},
	} {
		s.Run(tc.name, func() {
			project := parseProject(s)
			imageHandler := new(mocks.ImageHandler)
			// The integrity test is run where it is, not looked for under tests/.
			imageHandler.On("Pytest", DefaultTestPath, project, mock.Anything, mock.Anything, []string{}, mock.Anything, airflowTypes.ImageBuildConfig{Path: project, NoCache: false}).Return(tc.exitCode, nil).Once()
			checker := DAGChecker{airflowHome: project, imageHandler: imageHandler}

			err := checker.Parse("", "test", nil)
			if tc.wantErr == "" {
				s.NoError(err)
			} else {
				s.ErrorContains(err, tc.wantErr)
			}
			imageHandler.AssertExpectations(s.T())
		})
	}

	s.Run("file does not exists", func() {
		checker := DAGChecker{airflowHome: s.T().TempDir(), imageHandler: new(mocks.ImageHandler)}

		r, w, _ := os.Pipe()
		os.Stdout = w

		err := checker.Parse("", "test", nil)
		s.NoError(err)

		w.Close()
		out, _ := io.ReadAll(r)

		s.Contains(string(out), "Skipping the DAG parse check")
		s.Contains(string(out), "add it back to the project")
		s.NotContains(string(out), "astro dev parse", "astro dev parse does not exist in v2")
		s.NotContains(string(out), "run `astro dev init`", "astro dev init does not exist in v2")
	})

	s.Run("invalid file name", func() {
		checker := DAGChecker{airflowHome: "\x0004"}

		err := checker.Parse("", "test", nil)
		s.ErrorContains(err, "invalid argument")
	})
}
