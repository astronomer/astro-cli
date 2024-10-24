package airflow

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/stretchr/testify/suite"
)

type RuntimeTemplateSuite struct {
	suite.Suite
}

func TestRuntimeTemplate(t *testing.T) {
	suite.Run(t, new(RuntimeTemplateSuite))
}

func (s *RuntimeTemplateSuite) TestFetchTemplateList() {
	s.Run("fetch runtime templates list successful request", func() {
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := TemplatesResponse{
				Templates: []Template{
					{Name: "etl"},
					{Name: "dbt-on-astro"},
				},
			}
			json.NewEncoder(w).Encode(response)
		}))
		defer mockServer.Close()

		RuntimeTemplateURL = mockServer.URL

		templates, err := FetchTemplateList()

		s.NoError(err)
		s.Contains(templates, "etl")
		s.Contains(templates, "dbt-on-astro")
	})

	s.Run("fetch runtime templates list with bad request", func() {
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "bad request", http.StatusBadRequest)
		}))
		defer mockServer.Close()
		RuntimeTemplateURL = mockServer.URL

		templates, err := FetchTemplateList()

		s.Error(err)
		s.Nil(templates)
		s.Contains(err.Error(), "failed to get response")
		s.Contains(err.Error(), "400")
	})

	s.Run("fetch runtime templates list with Non-200 response request", func() {
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTemporaryRedirect)
			w.Write([]byte("test response"))
		}))
		defer mockServer.Close()

		RuntimeTemplateURL = mockServer.URL

		templates, err := FetchTemplateList()

		s.Error(err)
		s.Nil(templates)
		s.Contains(err.Error(), "received non-200 status code")
		s.Contains(err.Error(), "test response")
	})

	s.Run("fetch runtime templates list with invalid JSON", func() {
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, "test-invalid-json")
		}))
		defer mockServer.Close()

		RuntimeTemplateURL = mockServer.URL

		templates, err := FetchTemplateList()

		s.Error(err)
		s.Nil(templates)
		s.Contains(err.Error(), "failed to parse JSON response")
	})

	s.Run("fetch runtime templates list with empty list response", func() {
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := TemplatesResponse{Templates: []Template{}}
			json.NewEncoder(w).Encode(response)
		}))
		defer mockServer.Close()

		RuntimeTemplateURL = mockServer.URL

		templates, err := FetchTemplateList()

		s.NoError(err)
		s.Nil(templates)
		s.Equal(0, len(templates))
	})
}

func (s *RuntimeTemplateSuite) TestInitFromTemplate() {
	s.Run("test initilaization of template based project with Non200Response", func() {
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "not found", http.StatusNotFound)
		}))
		defer mockServer.Close()

		AstroTemplateRepoURL = mockServer.URL

		err := InitFromTemplate("test-template", "destination")

		s.Error(err)
		s.Contains(err.Error(), "failed to download tarball")
		s.Contains(err.Error(), "404")
		s.Contains(err.Error(), "not found")
	})

	s.Run("test successfully initilaization of template based project", func() {
		mockTarballBuf, err := createMockTarballInMemory()
		if err != nil {
			s.Errorf(err, "failed to create mock tarball: %w", err)
		}

		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/gzip")
			_, err := w.Write(mockTarballBuf.Bytes())
			if err != nil {
				s.Errorf(err, "failed to serve mock tarball: %w", err)
			}
		}))
		defer mockServer.Close()

		AstroTemplateRepoURL = mockServer.URL

		tmpDir, err := os.MkdirTemp("", "temp")
		s.NoError(err)
		defer os.RemoveAll(tmpDir)

		if err := os.MkdirAll(tmpDir, os.ModePerm); err != nil {
			s.Errorf(err, "failed to create destination directory: %w", err)
		}

		err = InitFromTemplate("A", tmpDir)
		s.NoError(err)

		expectedFiles := []string{
			"file1.txt",
			"dags/dag.py",
			"include",
		}
		for _, file := range expectedFiles {
			exist, err := fileutil.Exists(filepath.Join(tmpDir, file), nil)
			s.NoError(err)
			s.True(exist)
		}
	})

	s.Run("test initilaization of template based project with invalid tarball", func() {
		corruptedTarball := bytes.NewBufferString("this is not a valid tarball")

		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/gzip")
			_, err := w.Write(corruptedTarball.Bytes())
			if err != nil {
				s.Errorf(err, "failed to serve mock tarball: %w", err)
			}
		}))
		defer mockServer.Close()

		AstroTemplateRepoURL = mockServer.URL

		tmpDir, err := os.MkdirTemp("", "temp")
		s.NoError(err)
		defer os.RemoveAll(tmpDir)

		err = InitFromTemplate("test", tmpDir)

		s.Error(err)
		s.Contains(err.Error(), "failed to extract tarball")
	})
}

func createMockTarballInMemory() (*bytes.Buffer, error) {
	buffer := new(bytes.Buffer)
	gzipWriter := gzip.NewWriter(buffer)
	tw := tar.NewWriter(gzipWriter)
	defer func() {
		tw.Close()
		gzipWriter.Close()
	}()

	entries := []struct {
		Name  string
		Body  string
		IsDir bool
	}{
		{"test/A/dags", "", true},
		{"test/A/dags/dag.py", "Hello, World", false},
		{"test/A/file1.txt", "Hello, again", false},
		{"test/A/include", "", true},
	}

	for _, entry := range entries {
		if entry.IsDir {
			hdr := &tar.Header{
				Name:     entry.Name,
				Mode:     0o755,
				Typeflag: tar.TypeDir,
			}
			if err := tw.WriteHeader(hdr); err != nil {
				return nil, fmt.Errorf("failed to write tar directory header: %w", err)
			}
		} else {
			hdr := &tar.Header{
				Name: entry.Name,
				Mode: 0o600,
				Size: int64(len(entry.Body)),
			}
			if err := tw.WriteHeader(hdr); err != nil {
				return nil, fmt.Errorf("failed to write tar file header: %w", err)
			}

			if _, err := tw.Write([]byte(entry.Body)); err != nil {
				return nil, fmt.Errorf("failed to write file content to tar: %w", err)
			}
		}
	}

	return buffer, nil
}
