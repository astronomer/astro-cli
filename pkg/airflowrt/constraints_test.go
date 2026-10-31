package airflowrt

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePackageVersion(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "constraints.txt")
	content := "apache-airflow==2.10.0\napache-airflow-task-sdk==1.0.0\nother-package==3.2.1\n"
	require.NoError(t, os.WriteFile(f, []byte(content), 0o644))

	v, err := ParsePackageVersion(f, "apache-airflow")
	require.NoError(t, err)
	assert.Equal(t, "2.10.0", v)

	v, err = ParsePackageVersion(f, "apache-airflow-task-sdk")
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", v)

	_, err = ParsePackageVersion(f, "nonexistent")
	assert.Error(t, err)
}

func TestFetchConstraints_Cached(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, StandaloneDir)
	require.NoError(t, os.MkdirAll(cacheDir, 0o755))

	tag := "3.1-12"
	python := "3.12"

	constraints := "apache-airflow==2.10.0\napache-airflow-task-sdk==1.0.0\n"
	freeze := "apache-airflow==2.10.0\nsomething-else==1.2.3\n"

	require.NoError(t, os.WriteFile(
		filepath.Join(cacheDir, "constraints-3.1-12-python-3.12.txt"),
		[]byte(constraints), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(cacheDir, "freeze-3.1-12-python-3.12.txt"),
		[]byte(freeze), 0o644))

	result, err := FetchConstraints(dir, tag, python)
	require.NoError(t, err)
	assert.Equal(t, "2.10.0", result.AirflowVersion)
	assert.Equal(t, "1.0.0", result.TaskSDKVersion)
	assert.Contains(t, result.FreezePath, "freeze-3.1-12-python-3.12.txt")
}

func TestFetchConstraints_DefaultPython(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, StandaloneDir)
	require.NoError(t, os.MkdirAll(cacheDir, 0o755))

	constraints := "apache-airflow==2.10.0\n"
	freeze := "apache-airflow==2.10.0\n"

	// Write with default python version
	require.NoError(t, os.WriteFile(
		filepath.Join(cacheDir, "constraints-3.1-12-python-3.12.txt"),
		[]byte(constraints), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(cacheDir, "freeze-3.1-12-python-3.12.txt"),
		[]byte(freeze), 0o644))

	// Pass empty python version — should default to 3.12
	result, err := FetchConstraints(dir, "3.1-12", "")
	require.NoError(t, err)
	assert.Equal(t, "2.10.0", result.AirflowVersion)
}

// useTestServerClient points downloadClient at a client that trusts srv's
// certificate while keeping the production redirect policy.
func useTestServerClient(t *testing.T, srv *httptest.Server) {
	t.Helper()
	orig := downloadClient
	t.Cleanup(func() { downloadClient = orig })
	c := srv.Client()
	c.CheckRedirect = orig.CheckRedirect
	c.Timeout = orig.Timeout
	downloadClient = c
}

func TestDownloadFile_FollowsSameHostHTTPSRedirect(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/new", http.StatusFound)
	})
	mux.HandleFunc("/new", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("apache-airflow==3.1.0\n"))
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()
	useTestServerClient(t, srv)

	dest := filepath.Join(t.TempDir(), "constraints.txt")
	require.NoError(t, DownloadFile(srv.URL+"/old", dest))
	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, "apache-airflow==3.1.0\n", string(got))
}

func TestDownloadFile_RefusesRedirectToOtherHost(t *testing.T) {
	other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("evil==6.6.6\n"))
	}))
	defer other.Close()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/constraints.txt?X-Amz-Signature=secret", http.StatusFound)
	}))
	defer srv.Close()
	useTestServerClient(t, srv)

	dest := filepath.Join(t.TempDir(), "constraints.txt")
	err := DownloadFile(srv.URL+"/constraints.txt", dest)
	require.ErrorContains(t, err, "refusing redirect")
	assert.NotContains(t, err.Error(), "secret", "a presigned target's query must not be shown")
	assert.NoFileExists(t, dest)
}

func TestSameHostHTTPS(t *testing.T) {
	for to, ok := range map[string]bool{
		"https://cdn.astronomer.io/b":      true,
		"https://CDN.astronomer.io:443/b":  true,
		"https://cdn.astronomer.io:8443/b": false,
		"http://cdn.astronomer.io/b":       false,
		"https://evil.example.com/b":       false,
	} {
		from, err := http.NewRequest(http.MethodGet, "https://cdn.astronomer.io/a", http.NoBody)
		require.NoError(t, err)
		req, err := http.NewRequest(http.MethodGet, to, http.NoBody)
		require.NoError(t, err)
		assert.Equal(t, ok, sameHostHTTPS(req, []*http.Request{from}) == nil, to)
	}
}

func TestDownloadFile_RefusesRedirectToHTTP(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("evil==6.6.6\n"))
	}))
	defer plain.Close()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL+"/constraints.txt", http.StatusFound)
	}))
	defer srv.Close()
	useTestServerClient(t, srv)

	dest := filepath.Join(t.TempDir(), "constraints.txt")
	err := DownloadFile(srv.URL+"/constraints.txt", dest)
	require.ErrorContains(t, err, "refusing redirect")
	assert.NoFileExists(t, dest)
}
