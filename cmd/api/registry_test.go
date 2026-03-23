package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRegistryCmd_SubcommandsRegistered(t *testing.T) {
	cmd := NewRegistryCmd(&bytes.Buffer{})

	names := make([]string, 0, len(cmd.Commands()))
	for _, sub := range cmd.Commands() {
		names = append(names, sub.Name())
	}

	assert.Contains(t, names, "ls")
	assert.Contains(t, names, "describe")
}

func TestNewRegistryCmd_Flags(t *testing.T) {
	cmd := NewRegistryCmd(&bytes.Buffer{})

	// Persistent flags
	assert.NotNil(t, cmd.PersistentFlags().Lookup("registry-url"))

	// Request flags (same as airflow/cloud)
	assert.NotNil(t, cmd.Flags().Lookup("method"))
	assert.NotNil(t, cmd.Flags().Lookup("field"))
	assert.NotNil(t, cmd.Flags().Lookup("raw-field"))
	assert.NotNil(t, cmd.Flags().Lookup("header"))
	assert.NotNil(t, cmd.Flags().Lookup("input"))
	assert.NotNil(t, cmd.Flags().Lookup("path-param"))

	// Output flags
	assert.NotNil(t, cmd.Flags().Lookup("include"))
	assert.NotNil(t, cmd.Flags().Lookup("silent"))
	assert.NotNil(t, cmd.Flags().Lookup("template"))
	assert.NotNil(t, cmd.Flags().Lookup("jq"))
	assert.NotNil(t, cmd.Flags().Lookup("verbose"))
	assert.NotNil(t, cmd.Flags().Lookup("generate"))
}

func TestResolveRegistryURL(t *testing.T) {
	tests := []struct {
		name     string
		flagURL  string
		envURL   string
		expected string
	}{
		{
			name:     "default",
			expected: defaultRegistryURL,
		},
		{
			name:     "flag takes precedence",
			flagURL:  "https://custom.example.com/",
			envURL:   "https://env.example.com",
			expected: "https://custom.example.com",
		},
		{
			name:     "env used when no flag",
			envURL:   "https://env.example.com/",
			expected: "https://env.example.com",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envURL != "" {
				t.Setenv(registryURLEnv, tc.envURL)
			}
			got := resolveRegistryURL(tc.flagURL)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestRegistryBuildGenericURL(t *testing.T) {
	base := "https://airflow.apache.org/registry"

	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "with leading slash",
			path:     "/providers.json",
			expected: "https://airflow.apache.org/registry/api/providers.json",
		},
		{
			name:     "without leading slash",
			path:     "providers.json",
			expected: "https://airflow.apache.org/registry/api/providers.json",
		},
		{
			name:     "nested path",
			path:     "/providers/amazon/versions.json",
			expected: "https://airflow.apache.org/registry/api/providers/amazon/versions.json",
		},
		{
			name:     "modules.json global",
			path:     "/modules.json",
			expected: "https://airflow.apache.org/registry/api/modules.json",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := registryBuildGenericURL(base, tc.path)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestRegistryCmd_Execute_PathAccess(t *testing.T) {
	payload := `{"providers":[{"id":"amazon"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/providers.json", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	var out bytes.Buffer
	cmd := NewRegistryCmd(&out)
	cmd.SetArgs([]string{"/providers.json", "--registry-url", srv.URL})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, out.String(), "amazon")
}

func TestRegistryCmd_Execute_PathWithJQ(t *testing.T) {
	payload := `{"providers":[{"id":"amazon"},{"id":"google"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	var out bytes.Buffer
	cmd := NewRegistryCmd(&out)
	cmd.SetArgs([]string{"/providers.json", "--registry-url", srv.URL, "--jq", ".providers[0].id"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Equal(t, "amazon\n", out.String())
}

func TestRegistryCmd_Execute_GenerateCurl(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRegistryCmd(&out)
	cmd.SetArgs([]string{"/providers.json", "--registry-url", "https://example.com/registry", "--generate"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, out.String(), "curl")
	assert.Contains(t, out.String(), "https://example.com/registry/api/providers.json")
}

func TestRegistryCmd_Execute_IncludeHeaders(t *testing.T) {
	payload := `{"providers":[]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Test", "hello")
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	var out bytes.Buffer
	cmd := NewRegistryCmd(&out)
	cmd.SetArgs([]string{"/providers.json", "--registry-url", srv.URL, "-i"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, out.String(), "X-Test")
}

func TestRegistryCmd_Execute_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	var out bytes.Buffer
	cmd := NewRegistryCmd(&out)
	cmd.SetArgs([]string{"/nonexistent.json", "--registry-url", srv.URL})

	err := cmd.Execute()
	require.Error(t, err)

	var silentErr *SilentError
	require.ErrorAs(t, err, &silentErr)
	assert.Equal(t, http.StatusNotFound, silentErr.StatusCode)
}

func TestRegistryCmd_Execute_500WithBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer srv.Close()

	var out bytes.Buffer
	cmd := NewRegistryCmd(&out)
	cmd.SetArgs([]string{"/providers.json", "--registry-url", srv.URL})

	err := cmd.Execute()
	require.Error(t, err)

	var silentErr *SilentError
	require.ErrorAs(t, err, &silentErr)
	assert.Equal(t, http.StatusInternalServerError, silentErr.StatusCode)
	assert.Contains(t, out.String(), "internal server error")
}

func TestRegistryCmd_Execute_ConnectionError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closedURL := srv.URL
	srv.Close()

	var out bytes.Buffer
	cmd := NewRegistryCmd(&out)
	cmd.SetArgs([]string{"/providers.json", "--registry-url", closedURL})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not connect to registry")
}

func TestRegistryCmd_Execute_CustomHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "bar", r.Header.Get("X-Foo"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	var out bytes.Buffer
	cmd := NewRegistryCmd(&out)
	cmd.SetArgs([]string{"/providers.json", "--registry-url", srv.URL, "-H", "X-Foo:bar"})

	err := cmd.Execute()
	require.NoError(t, err)
}

func TestRegistryCmd_Execute_SilentMode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"providers":[]}`))
	}))
	defer srv.Close()

	var out bytes.Buffer
	cmd := NewRegistryCmd(&out)
	cmd.SetArgs([]string{"/providers.json", "--registry-url", srv.URL, "--silent"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Empty(t, out.String())
}

func TestRegistryCmd_Execute_OperationID(t *testing.T) {
	// Serve the OpenAPI spec for operation ID resolution
	specJSON := `{
		"openapi": "3.0.0",
		"info": {"title": "test", "version": "1.0"},
		"paths": {
			"/api/providers.json": {
				"get": {
					"operationId": "listProviders",
					"summary": "List all providers",
					"responses": {"200": {"description": "OK"}}
				}
			},
			"/api/providers/{providerId}/modules.json": {
				"get": {
					"operationId": "getProviderModulesLatest",
					"summary": "Get provider modules",
					"parameters": [
						{"name": "providerId", "in": "path", "required": true, "schema": {"type": "string"}}
					],
					"responses": {"200": {"description": "OK"}}
				}
			}
		}
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/openapi.json":
			_, _ = w.Write([]byte(specJSON))
		case "/api/providers.json":
			_, _ = w.Write([]byte(`{"providers":[{"id":"amazon"}]}`))
		case "/api/providers/amazon/modules.json":
			_, _ = w.Write([]byte(`{"modules":[{"name":"S3Hook"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	t.Run("operation ID without path params", func(t *testing.T) {
		var out bytes.Buffer
		cmd := NewRegistryCmd(&out)
		cmd.SetArgs([]string{"listProviders", "--registry-url", srv.URL})

		err := cmd.Execute()
		require.NoError(t, err)
		assert.Contains(t, out.String(), "amazon")
	})

	t.Run("operation ID with path params", func(t *testing.T) {
		var out bytes.Buffer
		cmd := NewRegistryCmd(&out)
		cmd.SetArgs([]string{"getProviderModulesLatest", "-p", "providerId=amazon", "--registry-url", srv.URL})

		err := cmd.Execute()
		require.NoError(t, err)
		assert.Contains(t, out.String(), "S3Hook")
	})

	t.Run("missing path param shows error", func(t *testing.T) {
		var out bytes.Buffer
		cmd := NewRegistryCmd(&out)
		cmd.SetArgs([]string{"getProviderModulesLatest", "--registry-url", srv.URL})

		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing path parameter")
		assert.Contains(t, err.Error(), "providerId")
	})

	t.Run("unknown operation ID shows error", func(t *testing.T) {
		var out bytes.Buffer
		cmd := NewRegistryCmd(&out)
		cmd.SetArgs([]string{"nonExistentOperation", "--registry-url", srv.URL})

		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestRegistryCmd_Execute_Template(t *testing.T) {
	payload := `{"providers":[{"id":"amazon"},{"id":"google"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	var out bytes.Buffer
	cmd := NewRegistryCmd(&out)
	cmd.SetArgs([]string{"/providers.json", "--registry-url", srv.URL, "--template", `{{range .providers}}{{.id}} {{end}}`})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, out.String(), "amazon")
	assert.Contains(t, out.String(), "google")
}

func TestRegistryCmd_Execute_NoArgs_ShowsHelp(t *testing.T) {
	// When run with no args and the spec isn't reachable, it should fall back gracefully.
	// We test that it attempts to show interactive guidance.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"openapi": "3.0.0",
			"info": {"title": "test", "version": "1.0"},
			"paths": {
				"/api/providers.json": {
					"get": {
						"operationId": "listProviders",
						"responses": {"200": {"description": "OK"}}
					}
				}
			}
		}`))
	}))
	defer srv.Close()

	var out bytes.Buffer
	cmd := NewRegistryCmd(&out)
	cmd.SetArgs([]string{"--registry-url", srv.URL})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, out.String(), "astro api registry ls")
}

func TestRegistryConnectionError(t *testing.T) {
	err := registryConnectionError("https://example.com/api/providers.json")
	assert.Contains(t, err.Error(), "could not connect to registry")
	assert.Contains(t, err.Error(), "--registry-url")
	assert.Contains(t, err.Error(), "ASTRO_REGISTRY_URL")
}

func TestRegistryCmd_Execute_NestedPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/providers/amazon/9.22.0/modules.json", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"modules":[]}`))
	}))
	defer srv.Close()

	var out bytes.Buffer
	cmd := NewRegistryCmd(&out)
	cmd.SetArgs([]string{"/providers/amazon/9.22.0/modules.json", "--registry-url", srv.URL})

	err := cmd.Execute()
	require.NoError(t, err)

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	assert.Contains(t, got, "modules")
}
