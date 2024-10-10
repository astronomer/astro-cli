package runtimetemplateclient

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
)

var originalRepoURL string

type TemplateClientTestSuite struct {
	suite.Suite
	ts      *httptest.Server
	destDir string
}

func (s *TemplateClientTestSuite) SetupTest() {
	originalRepoURL = astroTemplateRepoURL

	s.ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/tarball/main") {
			s.T().Fatalf("Unexpected URL: %v", r.URL.Path)
		}
	}))

	var err error
	s.destDir, err = os.MkdirTemp("", "dest-")
	s.Require().NoError(err)
}

func (s *TemplateClientTestSuite) TearDownTest() {
	astroTemplateRepoURL = originalRepoURL

	s.ts.Close()
	os.RemoveAll(s.destDir)
}

func (s *TemplateClientTestSuite) TestDownloadAndExtractTemplate_HTTPError() {
	s.ts.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Not Found", http.StatusNotFound)
	})

	astroTemplateRepoURL = s.ts.URL

	client := NewruntimeTemplateClient()

	err := client.DownloadAndExtractTemplate("test", s.destDir)
	s.Error(err)
	s.Contains(err.Error(), "failed to download tarball: API error (404): Not Found")
}

func (s *TemplateClientTestSuite) TestDownloadAndExtractTemplate_Success() {
	client := NewruntimeTemplateClient()
	err := client.DownloadAndExtractTemplate("etl", s.destDir)
	s.NoError(err)

	expectedFiles := []string{
		"requirements.txt",
		"packages.txt",
	}

	for _, file := range expectedFiles {
		filePath := filepath.Join(s.destDir, file)
		_, err := os.Stat(filePath)
		s.NoError(err, "Expected file %s to be extracted, but it was not", file)
	}
}

func (s *TemplateClientTestSuite) TestDownloadAndExtractTemplate_Fail() {
	client := NewruntimeTemplateClient()
	err := client.DownloadAndExtractTemplate("test", s.destDir)
	s.Error(err)

	s.Contains(err.Error(), "template directory test not found")
}

func TestTemplateClientTestSuite(t *testing.T) {
	suite.Run(t, new(TemplateClientTestSuite))
}
