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
	"github.com/stretchr/testify/assert"
)

func TestFetchTemplateList_Success(t *testing.T) {
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

	assert.NoError(t, err)
	assert.Contains(t, templates, "etl")
	assert.Contains(t, templates, "dbt-on-astro")
}

func TestFetchTemplateList_BadRequest(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer mockServer.Close()
	RuntimeTemplateURL = mockServer.URL

	templates, err := FetchTemplateList()

	assert.Error(t, err)
	assert.Nil(t, templates)
	assert.Contains(t, err.Error(), "failed to get response")
	assert.Contains(t, err.Error(), "400")
}

func TestFetchTemplateList_Non200Response(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTemporaryRedirect)
		w.Write([]byte("test response"))
	}))
	defer mockServer.Close()

	RuntimeTemplateURL = mockServer.URL

	templates, err := FetchTemplateList()

	assert.Error(t, err)
	assert.Nil(t, templates)
	assert.Contains(t, err.Error(), "received non-200 status code")
	assert.Contains(t, err.Error(), "test response")
}

func TestFetchTemplateList_InvalidJSON(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "test-invalid-json")
	}))
	defer mockServer.Close()

	RuntimeTemplateURL = mockServer.URL

	templates, err := FetchTemplateList()

	assert.Error(t, err)
	assert.Nil(t, templates)
	assert.Contains(t, err.Error(), "failed to parse JSON response")
}

func TestFetchTemplateList_EmptyList(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := TemplatesResponse{Templates: []Template{}}
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	RuntimeTemplateURL = mockServer.URL

	templates, err := FetchTemplateList()

	assert.NoError(t, err)
	assert.Nil(t, templates)
	assert.Equal(t, 0, len(templates))
}

func TestInitFromTemplate_Non200Response(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer mockServer.Close()

	AstroTemplateRepoURL = mockServer.URL

	err := InitFromTemplate("test-template", "destination")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to download tarball")
	assert.Contains(t, err.Error(), "404")
	assert.Contains(t, err.Error(), "not found")
}

func TestInitFromTemplate_Success(t *testing.T) {
	mockTarballBuf, err := createMockTarballInMemory()
	if err != nil {
		t.Fatalf("failed to create mock tarball: %v", err)
	}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		_, err := w.Write(mockTarballBuf.Bytes())
		if err != nil {
			t.Fatalf("failed to serve mock tarball: %v", err)
		}
	}))
	defer mockServer.Close()

	AstroTemplateRepoURL = mockServer.URL

	tmpDir, err := os.MkdirTemp("", "temp")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	if err := os.MkdirAll(tmpDir, os.ModePerm); err != nil {
		t.Fatalf("failed to create destination directory: %v", err)
	}

	err = InitFromTemplate("A", tmpDir)
	assert.NoError(t, err)

	expectedFiles := []string{
		"file1.txt",
		"dags/dag.py",
		"include",
	}
	for _, file := range expectedFiles {
		exist, err := fileutil.Exists(filepath.Join(tmpDir, file), nil)
		assert.NoError(t, err)
		assert.True(t, exist)
	}
}

func TestInitFromTemplate_InvalidTarball(t *testing.T) {
	corruptedTarball := bytes.NewBufferString("this is not a valid tarball")

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		_, err := w.Write(corruptedTarball.Bytes())
		if err != nil {
			t.Fatalf("failed to serve mock tarball: %v", err)
		}
	}))
	defer mockServer.Close()

	AstroTemplateRepoURL = mockServer.URL

	tmpDir, err := os.MkdirTemp("", "temp")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	err = InitFromTemplate("test", tmpDir)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to extract tarball")
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
