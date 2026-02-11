package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/openapi"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initTestConfig initializes the config with an in-memory filesystem for tests.
func initTestConfig(t *testing.T) {
	t.Helper()
	fs := afero.NewMemMapFs()
	configRaw := []byte(`context: astronomer_io
contexts:
  astronomer_io:
    domain: astronomer.io
    token: test-token
    organization: test-org
    workspace: test-ws
`)
	require.NoError(t, afero.WriteFile(fs, config.HomeConfigFile, configRaw, 0o777))
	config.InitConfig(fs)
	config.CFG.CloudAPIProtocol.SetHomeString("https")
}

// --- NewCloudCmd -------------------------------------------------------------

func TestNewCloudCmd(t *testing.T) {
	out := new(bytes.Buffer)
	cmd := NewCloudCmd(out)

	assert.Equal(t, "cloud <endpoint | operation-id>", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotEmpty(t, cmd.Example)

	// Check that subcommands are registered
	subcommands := cmd.Commands()
	names := make([]string, 0, len(subcommands))
	for _, sub := range subcommands {
		names = append(names, sub.Name())
	}
	assert.Contains(t, names, "ls")
	assert.Contains(t, names, "describe")
}

func TestCloudCmdFlags(t *testing.T) {
	out := new(bytes.Buffer)
	cmd := NewCloudCmd(out)

	// Request flags
	assert.NotNil(t, cmd.Flags().Lookup("method"))
	assert.NotNil(t, cmd.Flags().Lookup("field"))
	assert.NotNil(t, cmd.Flags().Lookup("raw-field"))
	assert.NotNil(t, cmd.Flags().Lookup("header"))
	assert.NotNil(t, cmd.Flags().Lookup("input"))
	assert.NotNil(t, cmd.Flags().Lookup("path-param"))

	// Output flags
	assert.NotNil(t, cmd.Flags().Lookup("include"))
	assert.NotNil(t, cmd.Flags().Lookup("paginate"))
	assert.NotNil(t, cmd.Flags().Lookup("slurp"))
	assert.NotNil(t, cmd.Flags().Lookup("silent"))
	assert.NotNil(t, cmd.Flags().Lookup("template"))
	assert.NotNil(t, cmd.Flags().Lookup("jq"))
	assert.NotNil(t, cmd.Flags().Lookup("verbose"))
	assert.NotNil(t, cmd.Flags().Lookup("cache"))

	// Other
	assert.NotNil(t, cmd.Flags().Lookup("generate"))
}

// --- isOperationID -----------------------------------------------------------

func TestIsOperationID(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"ListOrganizations", true},
		{"get_dag", true},
		{"version", true},
		{"/organizations", false},
		{"/dags/{dag_id}", false},
		{"/", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, isOperationID(tt.input))
		})
	}
}

// --- buildURL ----------------------------------------------------------------

func TestBuildURL(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		path     string
		expected string
	}{
		{"path with leading slash", "https://api.example.com/v1", "/organizations", "https://api.example.com/v1/organizations"},
		{"path without leading slash", "https://api.example.com/v1", "organizations", "https://api.example.com/v1/organizations"},
		{"base with trailing slash", "https://api.example.com/v1/", "/organizations", "https://api.example.com/v1/organizations"},
		{"both trailing and no leading", "https://api.example.com/v1/", "organizations", "https://api.example.com/v1/organizations"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, buildURL(tt.base, tt.path))
		})
	}
}

// --- fillPlaceholders --------------------------------------------------------

func TestFillPlaceholders(t *testing.T) {
	ctx := &config.Context{
		Organization: "org-123",
		Workspace:    "ws-456",
	}

	t.Run("fills organizationId", func(t *testing.T) {
		result, err := fillPlaceholders("/organizations/{organizationId}", ctx)
		require.NoError(t, err)
		assert.Equal(t, "/organizations/org-123", result)
	})

	t.Run("fills workspaceId", func(t *testing.T) {
		result, err := fillPlaceholders("/workspaces/{workspaceId}", ctx)
		require.NoError(t, err)
		assert.Equal(t, "/workspaces/ws-456", result)
	})

	t.Run("fills both placeholders", func(t *testing.T) {
		result, err := fillPlaceholders("/organizations/{organizationId}/workspaces/{workspaceId}", ctx)
		require.NoError(t, err)
		assert.Equal(t, "/organizations/org-123/workspaces/ws-456", result)
	})

	t.Run("leaves unknown placeholders", func(t *testing.T) {
		result, err := fillPlaceholders("/deployments/{deploymentId}", ctx)
		require.NoError(t, err)
		assert.Equal(t, "/deployments/{deploymentId}", result)
	})

	t.Run("case insensitive", func(t *testing.T) {
		result, err := fillPlaceholders("/organizations/{OrganizationId}", ctx)
		require.NoError(t, err)
		assert.Equal(t, "/organizations/org-123", result)
	})

	t.Run("error when organizationId missing", func(t *testing.T) {
		emptyCtx := &config.Context{}
		_, err := fillPlaceholders("/organizations/{organizationId}", emptyCtx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "organizationId not set")
	})

	t.Run("error when workspaceId missing", func(t *testing.T) {
		emptyCtx := &config.Context{}
		_, err := fillPlaceholders("/workspaces/{workspaceId}", emptyCtx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspaceId not set")
	})

	t.Run("no placeholders is a no-op", func(t *testing.T) {
		result, err := fillPlaceholders("/health", ctx)
		require.NoError(t, err)
		assert.Equal(t, "/health", result)
	})
}

// --- findMissingPathParams ---------------------------------------------------

func TestFindMissingPathParams(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected []string
	}{
		{"no params", "/health", nil},
		{"one missing", "/deployments/{deploymentId}", []string{"deploymentId"}},
		{"multiple missing", "/orgs/{orgId}/deps/{depId}", []string{"orgId", "depId"}},
		{"none missing (already replaced)", "/deployments/abc123", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findMissingPathParams(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// --- applyPathParams ---------------------------------------------------------

func TestApplyPathParams(t *testing.T) {
	t.Run("replaces single param", func(t *testing.T) {
		result, err := applyPathParams("/deployments/{deploymentId}", []string{"deploymentId=abc123"})
		require.NoError(t, err)
		assert.Equal(t, "/deployments/abc123", result)
	})

	t.Run("replaces multiple params", func(t *testing.T) {
		result, err := applyPathParams("/orgs/{orgId}/deps/{depId}", []string{"orgId=org1", "depId=dep2"})
		require.NoError(t, err)
		assert.Equal(t, "/orgs/org1/deps/dep2", result)
	})

	t.Run("leaves unmatched params", func(t *testing.T) {
		result, err := applyPathParams("/orgs/{orgId}/deps/{depId}", []string{"orgId=org1"})
		require.NoError(t, err)
		assert.Equal(t, "/orgs/org1/deps/{depId}", result)
	})

	t.Run("empty params is no-op", func(t *testing.T) {
		result, err := applyPathParams("/deployments/{deploymentId}", nil)
		require.NoError(t, err)
		assert.Equal(t, "/deployments/{deploymentId}", result)
	})

	t.Run("invalid format errors", func(t *testing.T) {
		_, err := applyPathParams("/test", []string{"no-equals"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid path param format")
	})
}

// --- resolveOperationID ------------------------------------------------------

func TestResolveOperationID(t *testing.T) {
	// Serve a minimal OpenAPI spec
	spec := openapi.OpenAPISpec{
		OpenAPI: "3.0.0",
		Info:    openapi.Info{Title: "Test", Version: "1.0"},
		Paths: map[string]openapi.PathItem{
			"/things": {
				Get: &openapi.Operation{OperationID: "listThings", Summary: "List things"},
			},
			"/version": {
				Get: &openapi.Operation{OperationID: "getVersion"},
			},
		},
	}
	body, _ := json.Marshal(spec)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer ts.Close()

	cache := openapi.NewCacheWithOptions(ts.URL, t.TempDir()+"/cache.json")

	t.Run("finds by operation ID", func(t *testing.T) {
		ep, err := resolveOperationID(cache, "listThings", "cloud")
		require.NoError(t, err)
		assert.Equal(t, "/things", ep.Path)
		assert.Equal(t, "GET", ep.Method)
	})

	t.Run("falls back to path", func(t *testing.T) {
		ep, err := resolveOperationID(cache, "version", "cloud")
		require.NoError(t, err)
		assert.Equal(t, "/version", ep.Path)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := resolveOperationID(cache, "nonExistent", "cloud")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
		assert.Contains(t, err.Error(), "astro api cloud ls")
	})
}

// --- initCloudSpecCache ------------------------------------------------------

func TestInitCloudSpecCache(t *testing.T) {
	initTestConfig(t)

	t.Run("initializes cache for default domain", func(t *testing.T) {
		opts := &CloudOptions{}
		ctx := &config.Context{Domain: "astronomer.io"}
		err := initCloudSpecCache(opts, ctx)
		require.NoError(t, err)
		assert.NotNil(t, opts.specCache)
	})

	t.Run("initializes cache for dev domain", func(t *testing.T) {
		opts := &CloudOptions{}
		ctx := &config.Context{Domain: "astronomer-dev.io"}
		err := initCloudSpecCache(opts, ctx)
		require.NoError(t, err)
		assert.NotNil(t, opts.specCache)
	})

	t.Run("no-op when cache already set", func(t *testing.T) {
		existingCache := openapi.NewCacheWithOptions("https://example.com/spec", "/tmp/test.json")
		opts := &CloudOptions{
			RequestOptions: RequestOptions{
				specCache: existingCache,
			},
		}
		ctx := &config.Context{Domain: "astronomer.io"}
		err := initCloudSpecCache(opts, ctx)
		require.NoError(t, err)
		// Should still be the same cache (not replaced)
		assert.Equal(t, existingCache, opts.specCache)
	})
}

// --- runCloudInteractive (requires cloud context, hard to unit test) ---------
// The runCloud and runCloudInteractive functions require full cloud context
// setup and are covered by integration tests. The utility functions they call
// (fillPlaceholders, applyPathParams, buildURL, etc.) are thoroughly tested
// above.

// --- placeholderRE -----------------------------------------------------------

func TestPlaceholderRE(t *testing.T) {
	tests := []struct {
		input   string
		matches []string
	}{
		{"/orgs/{orgId}", []string{"{orgId}"}},
		{"/orgs/{orgId}/deps/{depId}", []string{"{orgId}", "{depId}"}},
		{"/health", nil},
		{"/dags/{dag_id}", []string{"{dag_id}"}},
		{"/{a}/{b2c}", []string{"{a}", "{b2c}"}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			all := placeholderRE.FindAllString(tt.input, -1)
			if tt.matches == nil {
				assert.Empty(t, all)
			} else {
				assert.Equal(t, tt.matches, all)
			}
		})
	}
}

// --- NewCloudCmd RunE dispatch (no-args shows help) --------------------------

func TestCloudCmd_NoArgs_ShowsHelp(t *testing.T) {
	// When invoked without args, RunE calls runCloudInteractive which
	// requires a cloud context. Since we can't easily set that up, we
	// just verify the command accepts 0 or 1 args.
	out := new(bytes.Buffer)
	cmd := NewCloudCmd(out)
	// Verify Args validator
	err := cmd.Args(cmd, nil)
	assert.NoError(t, err) // MaximumNArgs(1) allows 0

	err = cmd.Args(cmd, []string{"one"})
	assert.NoError(t, err)

	err = cmd.Args(cmd, []string{"one", "two"})
	assert.Error(t, err) // Too many args
}

// --- generateCurl integration via NewCloudCmd --------------------------------
// Full runCloud tests require cloud context. The underlying executeRequest,
// generateCurl, fillPlaceholders, etc. are all tested via their own test files.

func TestCloudCmdLongDescription(t *testing.T) {
	// NewCloudCmd should produce a command whose Long description mentions Astro
	out := new(bytes.Buffer)
	cmd := NewCloudCmd(out)
	assert.Contains(t, cmd.Long, "Astro Cloud API")
	assert.Contains(t, cmd.Example, "astro api cloud")
}
