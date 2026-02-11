package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers -----------------------------------------------------------------

func newTestServer(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(handler)
}

func newTestOpts(out *bytes.Buffer) *RequestOptions {
	return &RequestOptions{
		Out:    out,
		ErrOut: out,
	}
}

// --- executeRequest / executeSingleRequest -----------------------------------

func TestExecuteSingleRequest_GET(t *testing.T) {
	ts := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	defer ts.Close()

	var buf bytes.Buffer
	opts := newTestOpts(&buf)
	err := executeRequest(opts, "GET", ts.URL+"/test", "", nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "status")
	assert.Contains(t, buf.String(), "ok")
}

func TestExecuteSingleRequest_POST_WithParams(t *testing.T) {
	ts := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var body map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&body)
		require.NoError(t, err)
		assert.Equal(t, "test-value", body["key"])

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"created":true}`))
	})
	defer ts.Close()

	var buf bytes.Buffer
	opts := newTestOpts(&buf)
	params := map[string]interface{}{"key": "test-value"}
	err := executeRequest(opts, "POST", ts.URL+"/test", "", params)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "created")
}

func TestExecuteSingleRequest_WithAuth(t *testing.T) {
	ts := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	defer ts.Close()

	var buf bytes.Buffer
	opts := newTestOpts(&buf)
	err := executeRequest(opts, "GET", ts.URL, "Bearer test-token", nil)
	require.NoError(t, err)
}

func TestExecuteSingleRequest_CustomHeaders(t *testing.T) {
	ts := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "custom-value", r.Header.Get("X-Custom"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	defer ts.Close()

	var buf bytes.Buffer
	opts := newTestOpts(&buf)
	opts.RequestHeaders = []string{"X-Custom: custom-value"}
	err := executeRequest(opts, "GET", ts.URL, "", nil)
	require.NoError(t, err)
}

func TestExecuteSingleRequest_InvalidHeader(t *testing.T) {
	var buf bytes.Buffer
	opts := newTestOpts(&buf)
	opts.RequestHeaders = []string{"invalid-no-colon"}
	err := executeRequest(opts, "GET", "http://localhost:1", "", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid header format")
}

func TestExecuteSingleRequest_ErrorResponse_PrintsBody(t *testing.T) {
	ts := newTestServer(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad input","detail":"missing field"}`))
	})
	defer ts.Close()

	var buf bytes.Buffer
	opts := newTestOpts(&buf)
	err := executeRequest(opts, "GET", ts.URL, "", nil)
	// Should return a SilentError so the CLI exits non-zero
	require.Error(t, err)
	var silentErr *SilentError
	require.ErrorAs(t, err, &silentErr)
	assert.Equal(t, http.StatusBadRequest, silentErr.StatusCode)
	// The error body should still be printed to output
	assert.Contains(t, buf.String(), "bad input")
}

func TestExecuteSingleRequest_Silent(t *testing.T) {
	ts := newTestServer(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"should-not-appear"}`))
	})
	defer ts.Close()

	var buf bytes.Buffer
	opts := newTestOpts(&buf)
	opts.Silent = true
	err := executeRequest(opts, "GET", ts.URL, "", nil)
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

func TestExecuteSingleRequest_JQFilter(t *testing.T) {
	ts := newTestServer(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"items":[{"name":"a"},{"name":"b"}]}`))
	})
	defer ts.Close()

	var buf bytes.Buffer
	opts := newTestOpts(&buf)
	opts.FilterOutput = ".items[].name"
	err := executeRequest(opts, "GET", ts.URL, "", nil)
	require.NoError(t, err)
	assert.Equal(t, "a\nb\n", buf.String())
}

func TestExecuteSingleRequest_Verbose(t *testing.T) {
	ts := newTestServer(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	defer ts.Close()

	var buf bytes.Buffer
	opts := newTestOpts(&buf)
	opts.Verbose = true
	err := executeRequest(opts, "GET", ts.URL, "", nil)
	require.NoError(t, err)
	// Should contain request and response markers
	assert.Contains(t, buf.String(), ">")
	assert.Contains(t, buf.String(), "<")
}

func TestExecuteSingleRequest_ShowHeaders_SkippedWhenVerbose(t *testing.T) {
	ts := newTestServer(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Test", "test-value")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	defer ts.Close()

	// With only ShowResponseHeaders
	var buf1 bytes.Buffer
	opts1 := newTestOpts(&buf1)
	opts1.ShowResponseHeaders = true
	err := executeRequest(opts1, "GET", ts.URL, "", nil)
	require.NoError(t, err)
	output1 := buf1.String()

	// With both Verbose and ShowResponseHeaders (H3 fix: should not double-print)
	var buf2 bytes.Buffer
	opts2 := newTestOpts(&buf2)
	opts2.Verbose = true
	opts2.ShowResponseHeaders = true
	err = executeRequest(opts2, "GET", ts.URL, "", nil)
	require.NoError(t, err)
	output2 := buf2.String()

	// Count occurrences of "X-Test" - should appear exactly once in both
	count1 := strings.Count(output1, "X-Test")
	count2 := strings.Count(output2, "X-Test")
	assert.Equal(t, 1, count1, "ShowResponseHeaders only: X-Test should appear once")
	assert.Equal(t, 1, count2, "Verbose+ShowResponseHeaders: X-Test should appear once (not double-printed)")
}

// --- setCustomHeaders --------------------------------------------------------

func TestSetCustomHeaders(t *testing.T) {
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", http.NoBody)

	err := setCustomHeaders(req, []string{"X-Foo: bar", "X-Baz: qux"})
	require.NoError(t, err)
	assert.Equal(t, "bar", req.Header.Get("X-Foo"))
	assert.Equal(t, "qux", req.Header.Get("X-Baz"))
}

func TestSetCustomHeaders_Invalid(t *testing.T) {
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", http.NoBody)

	err := setCustomHeaders(req, []string{"no-colon-here"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid header format")
}

// --- Pagination --------------------------------------------------------------

func TestExecutePaginatedRequest(t *testing.T) {
	page := 0
	ts := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		page++
		offset := r.URL.Query().Get("offset")
		w.WriteHeader(http.StatusOK)
		switch {
		case offset == "" || offset == "0":
			_, _ = w.Write([]byte(`{"items":[{"id":1},{"id":2}],"totalCount":4,"limit":2,"offset":0}`))
		default:
			_, _ = w.Write([]byte(`{"items":[{"id":3},{"id":4}],"totalCount":4,"limit":2,"offset":2}`))
		}
	})
	defer ts.Close()

	var buf bytes.Buffer
	opts := newTestOpts(&buf)
	opts.Paginate = true
	err := executeRequest(opts, "GET", ts.URL, "", nil)
	require.NoError(t, err)
	assert.Equal(t, 2, page)
}

func TestExecutePaginatedRequest_Slurp(t *testing.T) {
	ts := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		offset := r.URL.Query().Get("offset")
		w.WriteHeader(http.StatusOK)
		switch {
		case offset == "" || offset == "0":
			_, _ = w.Write([]byte(`{"items":[{"id":1}],"totalCount":2,"limit":1,"offset":0}`))
		default:
			_, _ = w.Write([]byte(`{"items":[{"id":2}],"totalCount":2,"limit":1,"offset":1}`))
		}
	})
	defer ts.Close()

	var buf bytes.Buffer
	opts := newTestOpts(&buf)
	opts.Paginate = true
	opts.Slurp = true
	opts.FilterOutput = ".items | length"
	err := executeRequest(opts, "GET", ts.URL, "", nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "2")
}

// --- combinePages ------------------------------------------------------------

func TestCombinePages_Deterministic(t *testing.T) {
	// H4 fix: array field selection should be deterministic (alphabetically first)
	page := json.RawMessage(`{"alpha":[1,2],"beta":[3,4],"totalCount":4,"limit":2}`)
	result, err := combinePages([]json.RawMessage{page})
	require.NoError(t, err)

	var parsed map[string]interface{}
	err = json.Unmarshal(result, &parsed)
	require.NoError(t, err)
	// "alpha" should be selected because it comes first alphabetically
	assert.Contains(t, parsed, "alpha")
	assert.NotContains(t, parsed, "beta")
}

func TestCombinePages_ErrorOnBadPage(t *testing.T) {
	// M7 fix: should error on unmarshalable page, not silently skip
	pages := []json.RawMessage{
		json.RawMessage(`{"items":[1],"totalCount":2,"limit":1}`),
		json.RawMessage(`{invalid json`),
	}
	_, err := combinePages(pages)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshaling page 2")
}

func TestCombinePages_Empty(t *testing.T) {
	result, err := combinePages(nil)
	require.NoError(t, err)
	assert.Equal(t, "[]", string(result))
}

// --- generateCurl ------------------------------------------------------------

func TestGenerateCurl_GET(t *testing.T) {
	var buf bytes.Buffer
	err := generateCurl(&buf, "GET", "https://api.example.com/test", "Bearer tok", nil, nil, "")
	require.NoError(t, err)
	output := buf.String()

	assert.Contains(t, output, "curl")
	assert.Contains(t, output, "https://api.example.com/test")
	assert.Contains(t, output, "Authorization: Bearer tok")
	assert.NotContains(t, output, "-X") // GET is default
}

func TestGenerateCurl_POST_WithBody(t *testing.T) {
	var buf bytes.Buffer
	params := map[string]interface{}{"key": "value"}
	err := generateCurl(&buf, "POST", "https://api.example.com/test", "", nil, params, "")
	require.NoError(t, err)
	output := buf.String()

	assert.Contains(t, output, "-X")
	assert.Contains(t, output, "POST")
	assert.Contains(t, output, "Content-Type: application/json")
	assert.Contains(t, output, `"key":"value"`)
}

func TestGenerateCurl_ShellQuoting(t *testing.T) {
	// H1 fix: values with single quotes should be properly escaped
	var buf bytes.Buffer
	err := generateCurl(&buf, "GET", "https://api.example.com/test?q=it's", "Bearer it's-a-token", nil, nil, "")
	require.NoError(t, err)
	output := buf.String()

	// Should NOT contain bare single quotes that break quoting
	assert.NotContains(t, output, "it's-a-token'")
	assert.Contains(t, output, `'\''`) // The shell escape idiom
}

// --- shellQuote --------------------------------------------------------------

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "'simple'"},
		{"with spaces", "'with spaces'"},
		{"it's", `'it'\''s'`},
		{"", "''"},
		{"a'b'c", `'a'\''b'\''c'`},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, shellQuote(tt.input))
		})
	}
}

// --- addQueryParams / addQueryParam ------------------------------------------

func TestAddQueryParams_NestedMap(t *testing.T) {
	// M9 fix: nested maps should preserve parent key
	params := map[string]interface{}{
		"filter": map[string]interface{}{
			"status": "active",
		},
	}
	result := addQueryParams("https://api.example.com/test", params)
	assert.Contains(t, result, "filter%5Bstatus%5D=active") // filter[status]=active URL-encoded
}

func TestAddQueryParams_Array(t *testing.T) {
	params := map[string]interface{}{
		"tags": []interface{}{"a", "b"},
	}
	result := addQueryParams("https://api.example.com/test", params)
	assert.Contains(t, result, "tags%5B%5D=a") // tags[]=a
	assert.Contains(t, result, "tags%5B%5D=b") // tags[]=b
}

func TestAddQueryParams_Empty(t *testing.T) {
	result := addQueryParams("https://api.example.com/test", nil)
	assert.Equal(t, "https://api.example.com/test", result)
}

func TestAddQueryParam_TypeBranches(t *testing.T) {
	tests := []struct {
		name     string
		params   map[string]interface{}
		contains string
	}{
		{"string", map[string]interface{}{"key": "val"}, "key=val"},
		{"int", map[string]interface{}{"count": 42}, "count=42"},
		{"bool", map[string]interface{}{"enabled": true}, "enabled=true"},
		{"nil", map[string]interface{}{"empty": nil}, "empty="},
		{"default (float)", map[string]interface{}{"score": 3.14}, "score=3.14"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := addQueryParams("https://example.com/test", tt.params)
			assert.Contains(t, result, tt.contains)
		})
	}
}

// --- Response caching --------------------------------------------------------

func TestResponseCache_RoundTrip(t *testing.T) {
	ts := newTestServer(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"cached":"response"}`))
	})
	defer ts.Close()

	// Use a temp dir for cache
	tmpDir := t.TempDir()
	origHome := os.Getenv("ASTRO_HOME_DIR")
	t.Setenv("ASTRO_HOME_DIR", tmpDir)
	defer func() {
		if origHome != "" {
			t.Setenv("ASTRO_HOME_DIR", origHome)
		}
	}()

	// Save a response
	saveCachedResponse("GET", ts.URL+"/test", []byte(`{"cached":"response"}`), 200)

	// Load it back
	body, ok := loadCachedResponse("GET", ts.URL+"/test", 1*time.Hour)
	assert.True(t, ok)
	assert.Equal(t, `{"cached":"response"}`, string(body))
}

func TestResponseCache_Expired(t *testing.T) {
	// Manually write an expired cache entry
	method := "GET"
	url := "https://example.com/expired"
	path := responseCachePath(method, url)

	dir := filepath.Dir(path)
	_ = os.MkdirAll(dir, 0o755)

	cached := cachedResponse{
		Body:       []byte(`{"old":"data"}`),
		CachedAt:   time.Now().Add(-2 * time.Hour),
		StatusCode: 200,
	}
	data, _ := json.Marshal(cached)
	_ = os.WriteFile(path, data, 0o600)

	// Should not load (TTL = 1 hour, entry is 2 hours old)
	_, ok := loadCachedResponse(method, url, 1*time.Hour)
	assert.False(t, ok)
}

func TestResponseCache_Miss(t *testing.T) {
	_, ok := loadCachedResponse("GET", "https://example.com/nonexistent", 1*time.Hour)
	assert.False(t, ok)
}

// --- maskToken ---------------------------------------------------------------

func TestMaskToken(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Bearer abcdefghij", "abcd...ghij"},
		{"short", "****"},
		{"Bearer 12345678", "****"},
		{"Bearer 123456789", "1234...6789"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, maskToken(tt.input))
		})
	}
}

// --- addOffsetToURL ----------------------------------------------------------

func TestAddOffsetToURL(t *testing.T) {
	assert.Equal(t, "https://api.example.com/test", addOffsetToURL("https://api.example.com/test", 0))

	result := addOffsetToURL("https://api.example.com/test", 20)
	assert.Contains(t, result, "offset=20")

	// Preserves existing query params
	result = addOffsetToURL("https://api.example.com/test?limit=10", 20)
	assert.Contains(t, result, "offset=20")
	assert.Contains(t, result, "limit=10")
}

// --- GET params as query string ----------------------------------------------

func TestExecuteSingleRequest_GET_WithParams(t *testing.T) {
	ts := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "bar", r.URL.Query().Get("foo"))
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{}`)
	})
	defer ts.Close()

	var buf bytes.Buffer
	opts := newTestOpts(&buf)
	params := map[string]interface{}{"foo": "bar"}
	err := executeRequest(opts, "GET", ts.URL, "", params)
	require.NoError(t, err)
}

// --- readInputFile -----------------------------------------------------------

func TestReadInputFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "body.json")
	require.NoError(t, os.WriteFile(tmpFile, []byte(`{"from":"file"}`), 0o600))

	data, err := readInputFile(tmpFile)
	require.NoError(t, err)
	assert.Equal(t, `{"from":"file"}`, string(data))
}

func TestReadInputFile_NotFound(t *testing.T) {
	_, err := readInputFile("/nonexistent/file.json")
	require.Error(t, err)
}

// --- executeSingleRequest with RequestInputFile ------------------------------

func TestExecuteSingleRequest_InputFile(t *testing.T) {
	ts := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		body, _ := io.ReadAll(r.Body)
		assert.JSONEq(t, `{"from":"file"}`, string(body))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	defer ts.Close()

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "body.json")
	require.NoError(t, os.WriteFile(tmpFile, []byte(`{"from":"file"}`), 0o600))

	var buf bytes.Buffer
	opts := newTestOpts(&buf)
	opts.RequestInputFile = tmpFile
	err := executeRequest(opts, "POST", ts.URL, "", nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "ok")
}

func TestExecuteSingleRequest_InputFileWithParams(t *testing.T) {
	ts := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		// Params should become query string when input file is used
		assert.Equal(t, "bar", r.URL.Query().Get("foo"))
		body, _ := io.ReadAll(r.Body)
		assert.JSONEq(t, `{"from":"file"}`, string(body))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	defer ts.Close()

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "body.json")
	require.NoError(t, os.WriteFile(tmpFile, []byte(`{"from":"file"}`), 0o600))

	var buf bytes.Buffer
	opts := newTestOpts(&buf)
	opts.RequestInputFile = tmpFile
	params := map[string]interface{}{"foo": "bar"}
	err := executeRequest(opts, "POST", ts.URL, "", params)
	require.NoError(t, err)
}

// --- executeSingleRequest with empty response body ---------------------------

func TestExecuteSingleRequest_EmptyResponse(t *testing.T) {
	ts := newTestServer(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	defer ts.Close()

	var buf bytes.Buffer
	opts := newTestOpts(&buf)
	err := executeRequest(opts, "DELETE", ts.URL, "", nil)
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

// --- executeSingleRequest with CacheTTL (cache hit) --------------------------

func TestExecuteSingleRequest_CacheHit(t *testing.T) {
	callCount := 0
	ts := newTestServer(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"live":true}`))
	})
	defer ts.Close()

	var buf bytes.Buffer
	opts := newTestOpts(&buf)
	opts.CacheTTL = 1 * time.Hour

	// First call hits the server
	err := executeRequest(opts, "GET", ts.URL+"/cached", "", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)

	// Second call should use cache
	buf.Reset()
	err = executeRequest(opts, "GET", ts.URL+"/cached", "", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount) // No additional server call
	assert.Contains(t, buf.String(), "live")
}

// --- executeSingleRequest with Template output -------------------------------

func TestExecuteSingleRequest_Template(t *testing.T) {
	ts := newTestServer(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"name":"hello"}`))
	})
	defer ts.Close()

	var buf bytes.Buffer
	opts := newTestOpts(&buf)
	opts.Template = "Name is {{.name}}"
	err := executeRequest(opts, "GET", ts.URL, "", nil)
	require.NoError(t, err)
	assert.Equal(t, "Name is hello", buf.String())
}

// --- generateCurl additional scenarios ---------------------------------------

func TestGenerateCurl_CustomHeaders(t *testing.T) {
	var buf bytes.Buffer
	err := generateCurl(&buf, "GET", "https://example.com", "", []string{"X-Custom: val"}, nil, "")
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "X-Custom: val")
}

func TestGenerateCurl_GETWithParams(t *testing.T) {
	var buf bytes.Buffer
	params := map[string]interface{}{"q": "test"}
	err := generateCurl(&buf, "GET", "https://example.com/search", "", nil, params, "")
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "q=test")
	assert.NotContains(t, buf.String(), "-X") // GET omits -X
}

func TestGenerateCurl_POSTWithInputFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "body.json")
	require.NoError(t, os.WriteFile(tmpFile, []byte(`{"key":"val"}`), 0o600))

	var buf bytes.Buffer
	err := generateCurl(&buf, "POST", "https://example.com/test", "", nil, nil, tmpFile)
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "-X")
	assert.Contains(t, output, "POST")
	assert.Contains(t, output, `"key":"val"`)
	assert.Contains(t, output, "Content-Type: application/json")
}

func TestGenerateCurl_InputFileWithParams(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "body.json")
	require.NoError(t, os.WriteFile(tmpFile, []byte(`{}`), 0o600))

	var buf bytes.Buffer
	params := map[string]interface{}{"foo": "bar"}
	err := generateCurl(&buf, "POST", "https://example.com/test", "", nil, params, tmpFile)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "foo=bar") // params as query string
}

func TestGenerateCurl_InputFileNotFound(t *testing.T) {
	var buf bytes.Buffer
	err := generateCurl(&buf, "POST", "https://example.com", "", nil, nil, "/nonexistent/file")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading input file")
}

func TestGenerateCurl_NoToken(t *testing.T) {
	var buf bytes.Buffer
	err := generateCurl(&buf, "GET", "https://example.com", "", nil, nil, "")
	require.NoError(t, err)
	assert.NotContains(t, buf.String(), "Authorization")
}

// --- cleanupResponseCache ----------------------------------------------------

func TestCleanupResponseCache(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, responseCacheDir)
	require.NoError(t, os.MkdirAll(cacheDir, 0o755))

	// Create a "stale" file with old mod time
	staleFile := filepath.Join(cacheDir, "stale.json")
	require.NoError(t, os.WriteFile(staleFile, []byte(`{}`), 0o600))
	oldTime := time.Now().Add(-8 * 24 * time.Hour) // 8 days ago
	require.NoError(t, os.Chtimes(staleFile, oldTime, oldTime))

	// Create a "fresh" file
	freshFile := filepath.Join(cacheDir, "fresh.json")
	require.NoError(t, os.WriteFile(freshFile, []byte(`{}`), 0o600))

	// Point config to our temp dir
	origHome := config.HomeConfigPath
	config.HomeConfigPath = tmpDir
	defer func() { config.HomeConfigPath = origHome }()

	cleanupResponseCache()

	// Stale file should be removed
	_, err := os.Stat(staleFile)
	assert.True(t, os.IsNotExist(err), "stale file should be removed")

	// Fresh file should remain
	_, err = os.Stat(freshFile)
	assert.NoError(t, err, "fresh file should remain")
}

// --- loadCachedResponse with corrupt data ------------------------------------

func TestLoadCachedResponse_CorruptData(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := config.HomeConfigPath
	config.HomeConfigPath = tmpDir
	defer func() { config.HomeConfigPath = origHome }()

	// Write corrupt data to the cache path
	path := responseCachePath("GET", "https://example.com/corrupt")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(`not json`), 0o600))

	_, ok := loadCachedResponse("GET", "https://example.com/corrupt", 1*time.Hour)
	assert.False(t, ok)
}

// --- combinePages edge cases -------------------------------------------------

func TestCombinePages_NonObjectFirstPage(t *testing.T) {
	// First page is a JSON array, not an object
	pages := []json.RawMessage{
		json.RawMessage(`[1,2,3]`),
	}
	result, err := combinePages(pages)
	require.NoError(t, err)
	// Should wrap the raw messages in an array
	assert.Contains(t, string(result), "[")
}

func TestCombinePages_NoArrayField(t *testing.T) {
	// Object with no array fields (only scalars and pagination fields)
	pages := []json.RawMessage{
		json.RawMessage(`{"name":"test","totalCount":1,"limit":1}`),
	}
	result, err := combinePages(pages)
	require.NoError(t, err)
	// Should fall back to returning array of pages
	assert.Contains(t, string(result), "name")
}

// --- printHeaders ------------------------------------------------------------

func TestPrintHeaders(t *testing.T) {
	headers := http.Header{
		"Content-Type": {"application/json"},
		"X-Custom":     {"value"},
		"Status":       {"should-be-skipped"},
	}

	var buf bytes.Buffer
	printHeaders(&buf, headers, false)
	output := buf.String()

	assert.Contains(t, output, "Content-Type: application/json")
	assert.Contains(t, output, "X-Custom: value")
	assert.NotContains(t, output, "should-be-skipped", "Status header should be skipped")
}

func TestPrintHeaders_Colorized(t *testing.T) {
	headers := http.Header{
		"X-Test": {"val"},
	}

	var buf bytes.Buffer
	printHeaders(&buf, headers, true)
	assert.Contains(t, buf.String(), "\x1b[") // ANSI escape
	assert.Contains(t, buf.String(), "X-Test")
}

// --- printResponseHeaders status colors --------------------------------------

func TestPrintResponseHeaders_StatusColors(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{"success (green)", 200},
		{"redirect (yellow)", 301},
		{"client error (red)", 404},
		{"server error (red)", 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{
				StatusCode: tt.status,
				Status:     fmt.Sprintf("%d Test", tt.status),
				Proto:      "HTTP/1.1",
				Header:     http.Header{"X-Test": {"val"}},
			}
			var buf bytes.Buffer
			printResponseHeaders(&buf, resp)
			output := buf.String()
			assert.Contains(t, output, "<")
			assert.Contains(t, output, fmt.Sprintf("%d Test", tt.status))
		})
	}
}

// --- GetErrOut / GetHTTPClient defaults --------------------------------------

func TestGetErrOut_Default(t *testing.T) {
	opts := &RequestOptions{}
	assert.Equal(t, os.Stderr, opts.GetErrOut())
}

func TestGetErrOut_Custom(t *testing.T) {
	var buf bytes.Buffer
	opts := &RequestOptions{ErrOut: &buf}
	assert.Equal(t, &buf, opts.GetErrOut())
}

func TestGetHTTPClient_Default(t *testing.T) {
	opts := &RequestOptions{}
	assert.Equal(t, http.DefaultClient, opts.GetHTTPClient())
}

func TestGetHTTPClient_Custom(t *testing.T) {
	custom := &http.Client{Timeout: 5 * time.Second}
	opts := &RequestOptions{HTTPClient: custom}
	assert.Equal(t, custom, opts.GetHTTPClient())
}

// --- responseCacheKey --------------------------------------------------------

func TestResponseCacheKey_Deterministic(t *testing.T) {
	key1 := responseCacheKey("GET", "https://example.com/test")
	key2 := responseCacheKey("GET", "https://example.com/test")
	assert.Equal(t, key1, key2, "same inputs should produce same key")

	key3 := responseCacheKey("POST", "https://example.com/test")
	assert.NotEqual(t, key1, key3, "different methods should produce different keys")

	key4 := responseCacheKey("GET", "https://example.com/other")
	assert.NotEqual(t, key1, key4, "different URLs should produce different keys")
}

func TestResponseCacheKey_CaseInsensitiveMethod(t *testing.T) {
	key1 := responseCacheKey("get", "https://example.com/test")
	key2 := responseCacheKey("GET", "https://example.com/test")
	assert.Equal(t, key1, key2, "method should be case-insensitive")
}

// --- isConnectionError -------------------------------------------------------

func TestIsConnectionError(t *testing.T) {
	// nil error
	assert.False(t, isConnectionError(nil))

	// Regular error
	assert.False(t, isConnectionError(fmt.Errorf("some error")))

	// net.OpError (connection refused)
	opErr := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: fmt.Errorf("connection refused"),
	}
	assert.True(t, isConnectionError(opErr))

	// Wrapped net.OpError (as produced by fetchAirflowToken → client.Do)
	wrappedOpErr := fmt.Errorf("fetching auth token: %w", opErr)
	assert.True(t, isConnectionError(wrappedOpErr))

	// DNS error
	dnsErr := &net.DNSError{
		Err:  "no such host",
		Name: "nonexistent.example.com",
	}
	assert.True(t, isConnectionError(dnsErr))

	// Wrapped DNS error
	wrappedDNSErr := fmt.Errorf("executing request: %w", dnsErr)
	assert.True(t, isConnectionError(wrappedDNSErr))

	// SilentError (HTTP-level, not a connection error)
	silentErr := &SilentError{StatusCode: 404}
	assert.False(t, isConnectionError(silentErr))
}

// --- isLocalhostURL ----------------------------------------------------------

func TestIsLocalhostURL(t *testing.T) {
	tests := []struct {
		url      string
		expected bool
	}{
		{"http://localhost:8080/api/v2", true},
		{"http://127.0.0.1:8080/api/v2", true},
		{"http://[::1]:8080/api/v2", true},
		{"https://localhost:443/api/v2", true},
		{"http://example.com:8080/api/v2", false},
		{"https://deployment.airflow.astronomer.io/api/v2", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			assert.Equal(t, tt.expected, isLocalhostURL(tt.url))
		})
	}
}
