package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/openapi"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAirflowCmd(t *testing.T) {
	out := new(bytes.Buffer)
	cmd := NewAirflowCmd(out)

	assert.Equal(t, "airflow <endpoint | operation-id>", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotEmpty(t, cmd.Example)

	// Check that subcommands are registered
	subcommands := cmd.Commands()
	subcommandNames := make([]string, 0, len(subcommands))
	for _, sub := range subcommands {
		subcommandNames = append(subcommandNames, sub.Name())
	}
	assert.Contains(t, subcommandNames, "ls")
	assert.Contains(t, subcommandNames, "describe")
}

func TestAirflowCmdFlags(t *testing.T) {
	out := new(bytes.Buffer)
	cmd := NewAirflowCmd(out)

	// Check airflow-specific flags exist (these are persistent flags so they're inherited by subcommands)
	assert.NotNil(t, cmd.PersistentFlags().Lookup("api-url"))
	assert.NotNil(t, cmd.PersistentFlags().Lookup("deployment-id"))
	assert.NotNil(t, cmd.PersistentFlags().Lookup("organization-id"))
	assert.NotNil(t, cmd.PersistentFlags().Lookup("workspace-id"))
	assert.NotNil(t, cmd.PersistentFlags().Lookup("airflow-version"))

	// Check request flags exist
	assert.NotNil(t, cmd.Flags().Lookup("method"))
	assert.NotNil(t, cmd.Flags().Lookup("field"))
	assert.NotNil(t, cmd.Flags().Lookup("raw-field"))
	assert.NotNil(t, cmd.Flags().Lookup("header"))
	assert.NotNil(t, cmd.Flags().Lookup("input"))
	assert.NotNil(t, cmd.Flags().Lookup("path-param"))

	// Check output flags exist
	assert.NotNil(t, cmd.Flags().Lookup("include"))
	assert.NotNil(t, cmd.Flags().Lookup("paginate"))
	assert.NotNil(t, cmd.Flags().Lookup("slurp"))
	assert.NotNil(t, cmd.Flags().Lookup("silent"))
	assert.NotNil(t, cmd.Flags().Lookup("template"))
	assert.NotNil(t, cmd.Flags().Lookup("jq"))
	assert.NotNil(t, cmd.Flags().Lookup("verbose"))
	assert.NotNil(t, cmd.Flags().Lookup("cache"))

	// Check other flags exist
	assert.NotNil(t, cmd.Flags().Lookup("generate"))
}

func TestResolveAirflowAPIURL_Default(t *testing.T) {
	opts := &AirflowOptions{}

	baseURL, authToken, err := resolveAirflowAPIURL(opts)
	require.NoError(t, err)
	assert.Equal(t, airflowAPIDefaultURL, baseURL)
	// authToken may be empty (if Airflow is not running), "Bearer ..." (Airflow 3), or "Basic ..." (Airflow 2)
	if authToken != "" {
		hasValidPrefix := strings.HasPrefix(authToken, "Bearer ") || strings.HasPrefix(authToken, "Basic ")
		assert.True(t, hasValidPrefix, "authToken should be empty, start with 'Bearer ', or start with 'Basic '")
	}
}

func TestResolveAirflowAPIURL_Default_WithAuthHeader(t *testing.T) {
	// When user provides Authorization header, token fetch should be skipped
	opts := &AirflowOptions{
		RequestOptions: RequestOptions{
			RequestHeaders: []string{"Authorization: Bearer custom-token"},
		},
	}

	baseURL, authToken, err := resolveAirflowAPIURL(opts)
	require.NoError(t, err)
	assert.Equal(t, airflowAPIDefaultURL, baseURL)
	assert.Empty(t, authToken) // Token fetch skipped when user provides auth
}

func TestResolveAirflowAPIURL_CustomURL(t *testing.T) {
	customURL := "http://custom.airflow.example.com:8080/api/v2"
	opts := &AirflowOptions{
		APIURL: customURL,
	}

	baseURL, authToken, err := resolveAirflowAPIURL(opts)
	require.NoError(t, err)
	assert.Equal(t, customURL, baseURL)
	assert.Empty(t, authToken)
}

func TestResolveAirflowAPIURL_DeploymentID_NotCloudContext(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	opts := &AirflowOptions{
		DeploymentID: "test-deployment-id",
	}

	_, _, err := resolveAirflowAPIURL(opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires cloud context")
}

func TestResolveAirflowAPIURL_DeploymentID_NoToken(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	err := config.CFG.ProjectDeployment.SetProjectString("")
	require.NoError(t, err)

	// Set up context without token
	ctx, err := config.GetCurrentContext()
	require.NoError(t, err)
	ctx.Token = ""
	err = ctx.SetContext()
	require.NoError(t, err)

	opts := &AirflowOptions{
		DeploymentID: "test-deployment-id",
	}

	_, _, err = resolveAirflowAPIURL(opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not authenticated")
}

func TestResolveAirflowAPIURL_DeploymentID_Success(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	err := config.CFG.ProjectDeployment.SetProjectString("")
	require.NoError(t, err)

	// Set up context with token and organization
	ctx, err := config.GetCurrentContext()
	require.NoError(t, err)
	ctx.Token = "test-token"
	ctx.Organization = "test-org"
	err = ctx.SetContext()
	require.NoError(t, err)

	// Mock CoreGetDeployment
	expectedURL := "https://deployment.airflow.astronomer.io/api/v2"
	origCoreGetDeployment := deployment.CoreGetDeployment
	defer func() { deployment.CoreGetDeployment = origCoreGetDeployment }()

	deployment.CoreGetDeployment = func(orgID, deploymentID string, client astroplatformcore.CoreClient) (astroplatformcore.Deployment, error) {
		assert.Equal(t, "test-org", orgID)
		assert.Equal(t, "test-deployment-id", deploymentID)
		return astroplatformcore.Deployment{
			Id:                     deploymentID,
			WebServerAirflowApiUrl: expectedURL,
		}, nil
	}

	opts := &AirflowOptions{
		DeploymentID: "test-deployment-id",
	}

	baseURL, authToken, err := resolveAirflowAPIURL(opts)
	require.NoError(t, err)
	assert.Equal(t, expectedURL, baseURL)
	assert.Equal(t, "test-token", authToken)
}

func TestResolveAirflowAPIURL_DeploymentID_WithOrgOverride(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	err := config.CFG.ProjectDeployment.SetProjectString("")
	require.NoError(t, err)

	// Set up context with token but different organization
	ctx, err := config.GetCurrentContext()
	require.NoError(t, err)
	ctx.Token = "test-token"
	ctx.Organization = "context-org"
	err = ctx.SetContext()
	require.NoError(t, err)

	// Mock CoreGetDeployment
	expectedURL := "https://deployment.airflow.astronomer.io/api/v2"
	origCoreGetDeployment := deployment.CoreGetDeployment
	defer func() { deployment.CoreGetDeployment = origCoreGetDeployment }()

	deployment.CoreGetDeployment = func(orgID, deploymentID string, client astroplatformcore.CoreClient) (astroplatformcore.Deployment, error) {
		// Should use the override org, not context org
		assert.Equal(t, "override-org", orgID)
		assert.Equal(t, "test-deployment-id", deploymentID)
		return astroplatformcore.Deployment{
			Id:                     deploymentID,
			WebServerAirflowApiUrl: expectedURL,
		}, nil
	}

	opts := &AirflowOptions{
		DeploymentID:   "test-deployment-id",
		OrganizationID: "override-org",
	}

	baseURL, authToken, err := resolveAirflowAPIURL(opts)
	require.NoError(t, err)
	assert.Equal(t, expectedURL, baseURL)
	assert.Equal(t, "test-token", authToken)
}

func TestResolveAirflowAPIURL_DeploymentID_NoAirflowURL(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	err := config.CFG.ProjectDeployment.SetProjectString("")
	require.NoError(t, err)

	// Set up context with token and organization
	ctx, err := config.GetCurrentContext()
	require.NoError(t, err)
	ctx.Token = "test-token"
	ctx.Organization = "test-org"
	err = ctx.SetContext()
	require.NoError(t, err)

	// Mock CoreGetDeployment to return deployment without Airflow URL
	origCoreGetDeployment := deployment.CoreGetDeployment
	defer func() { deployment.CoreGetDeployment = origCoreGetDeployment }()

	deployment.CoreGetDeployment = func(orgID, deploymentID string, client astroplatformcore.CoreClient) (astroplatformcore.Deployment, error) {
		return astroplatformcore.Deployment{
			Id:                     deploymentID,
			WebServerAirflowApiUrl: "", // empty string means no URL configured
		}, nil
	}

	opts := &AirflowOptions{
		DeploymentID: "test-deployment-id",
	}

	_, _, err = resolveAirflowAPIURL(opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not have an Airflow API URL")
}

func TestNewAirflowListCmd(t *testing.T) {
	out := new(bytes.Buffer)
	parentOpts := &AirflowOptions{}
	cmd := NewAirflowListCmd(out, parentOpts)

	assert.Equal(t, "ls [filter]", cmd.Use)
	assert.Contains(t, cmd.Aliases, "list")
	assert.NotEmpty(t, cmd.Short)
	assert.Contains(t, cmd.Short, "Airflow")

	// Check flags exist
	assert.NotNil(t, cmd.Flags().Lookup("verbose"))
	assert.NotNil(t, cmd.Flags().Lookup("refresh"))

	// Check examples contain "airflow"
	assert.Contains(t, cmd.Example, "astro api airflow")
}

func TestNewAirflowDescribeCmd(t *testing.T) {
	out := new(bytes.Buffer)
	parentOpts := &AirflowOptions{}
	cmd := NewAirflowDescribeCmd(out, parentOpts)

	assert.Equal(t, "describe <endpoint>", cmd.Use)
	assert.NotEmpty(t, cmd.Short)

	// Check flags exist
	assert.NotNil(t, cmd.Flags().Lookup("method"))
	assert.NotNil(t, cmd.Flags().Lookup("refresh"))

	// Check examples contain "airflow" and airflow-specific paths
	assert.Contains(t, cmd.Example, "astro api airflow")
	assert.Contains(t, cmd.Example, "dag")
}

func TestAirflowConstants(t *testing.T) {
	// Verify the constants are set correctly
	assert.Equal(t, "http://localhost:8080/api/v2", airflowAPIDefaultURL)
	assert.Equal(t, "3.0.3", defaultAirflowVersion)
}

// --- basicAuth ---------------------------------------------------------------

func TestBasicAuth(t *testing.T) {
	result := basicAuth("admin", "admin")
	// "admin:admin" -> base64
	assert.Equal(t, "YWRtaW46YWRtaW4=", result)

	result = basicAuth("user", "pass")
	assert.Equal(t, "dXNlcjpwYXNz", result)
}

// --- airflowHostRoot ---------------------------------------------------------

func TestAirflowHostRoot(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"http://localhost:8080/api/v2", "http://localhost:8080"},
		{"http://localhost:8080/api/v1", "http://localhost:8080"},
		{"http://localhost:8080/api/v2/", "http://localhost:8080"},
		{"http://localhost:8080", "http://localhost:8080"},
		{"https://deployment.airflow.astronomer.io/api/v2", "https://deployment.airflow.astronomer.io"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, airflowHostRoot(tt.input))
		})
	}
}

// --- apiPrefixForVersion -----------------------------------------------------

func TestApiPrefixForVersion(t *testing.T) {
	tests := []struct {
		version  string
		expected string
	}{
		{"2.10.0", "/api/v1"},
		{"2.0.0", "/api/v1"},
		{"2.99.99", "/api/v1"},
		{"3.0.0", "/api/v2"},
		{"3.0.3", "/api/v2"},
		{"3.1.7+astro.1", "/api/v2"}, // build metadata stripped
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			assert.Equal(t, tt.expected, apiPrefixForVersion(tt.version))
		})
	}
}

// --- fetchAirflowToken -------------------------------------------------------

func TestFetchAirflowToken_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/auth/token", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token":"my-jwt-token"}`))
	}))
	defer ts.Close()

	token, err := fetchAirflowToken(http.DefaultClient, ts.URL+"/api/v2", "admin", "admin")
	require.NoError(t, err)
	assert.Equal(t, "Bearer my-jwt-token", token)
}

func TestFetchAirflowToken_FallbackToBasicAuth(t *testing.T) {
	// Airflow 2.x: /auth/token returns 404, so we fall back to basic auth
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	token, err := fetchAirflowToken(http.DefaultClient, ts.URL+"/api/v1", "admin", "admin")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(token, "Basic "))
}

func TestFetchAirflowToken_MethodNotAllowed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	defer ts.Close()

	token, err := fetchAirflowToken(http.DefaultClient, ts.URL, "admin", "admin")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(token, "Basic "))
}

func TestFetchAirflowToken_ErrorStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	_, err := fetchAirflowToken(http.DefaultClient, ts.URL, "admin", "wrong")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 401")
}

func TestFetchAirflowToken_EmptyToken(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token":""}`))
	}))
	defer ts.Close()

	_, err := fetchAirflowToken(http.DefaultClient, ts.URL, "admin", "admin")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no access_token")
}

// --- FetchAirflowVersion -----------------------------------------------------

func TestFetchAirflowVersion_V2First(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/version" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"version":"3.0.3"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	version, err := FetchAirflowVersion(http.DefaultClient, ts.URL+"/api/v2", "")
	require.NoError(t, err)
	assert.Equal(t, "3.0.3", version)
}

func TestFetchAirflowVersion_FallbackToV1(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/version" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"version":"2.10.0"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	version, err := FetchAirflowVersion(http.DefaultClient, ts.URL+"/api/v2", "")
	require.NoError(t, err)
	assert.Equal(t, "2.10.0", version)
}

func TestFetchAirflowVersion_WithAuthToken(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer my-token", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"version":"3.1.0"}`))
	}))
	defer ts.Close()

	version, err := FetchAirflowVersion(http.DefaultClient, ts.URL, "Bearer my-token")
	require.NoError(t, err)
	assert.Equal(t, "3.1.0", version)
}

func TestFetchAirflowVersion_AllFail(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	_, err := FetchAirflowVersion(http.DefaultClient, ts.URL, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not detect version")
}

func TestFetchAirflowVersion_EmptyVersion(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"version":""}`))
	}))
	defer ts.Close()

	_, err := FetchAirflowVersion(http.DefaultClient, ts.URL, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not detect version")
}

// --- resolveAirflowAPIURL with explicit credentials --------------------------

func TestResolveAirflowAPIURL_ExplicitCredentials_Failure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	opts := &AirflowOptions{
		APIURL:              ts.URL,
		Username:            "admin",
		Password:            "wrong",
		CredentialsExplicit: true,
		RequestOptions:      RequestOptions{ErrOut: new(bytes.Buffer)},
	}

	_, _, err := resolveAirflowAPIURL(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}

// --- initAirflowSpecCache ----------------------------------------------------

func TestInitAirflowSpecCache_AlreadyInitialized(t *testing.T) {
	cache, _ := openapi.NewAirflowCacheForVersion("3.0.3")
	opts := &AirflowOptions{
		RequestOptions: RequestOptions{specCache: cache},
	}

	result, err := initAirflowSpecCache(opts, "http://localhost:8080/api/v2", "")
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:8080/api/v2", result)
}

func TestInitAirflowSpecCache_ManualVersion(t *testing.T) {
	opts := &AirflowOptions{
		AirflowVersion: "2.10.0",
		RequestOptions: RequestOptions{ErrOut: new(bytes.Buffer)},
	}

	result, err := initAirflowSpecCache(opts, "http://localhost:8080/api/v2", "")
	require.NoError(t, err)
	// Should correct the URL to /api/v1 for Airflow 2.x
	assert.Equal(t, "http://localhost:8080/api/v1", result)
	assert.NotNil(t, opts.specCache)
	assert.Equal(t, "2.10.0", opts.detectedVersion)
}

func TestInitAirflowSpecCache_DetectsVersion(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "version") {
			w.WriteHeader(http.StatusOK)
			resp := map[string]string{"version": "3.0.3"}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	opts := &AirflowOptions{
		RequestOptions: RequestOptions{
			ErrOut:     new(bytes.Buffer),
			HTTPClient: http.DefaultClient,
		},
	}

	result, err := initAirflowSpecCache(opts, ts.URL+"/api/v2", "")
	require.NoError(t, err)
	assert.Equal(t, ts.URL+"/api/v2", result)
	assert.Equal(t, "3.0.3", opts.detectedVersion)
}

// --- airflowConnectionError --------------------------------------------------

func TestAirflowConnectionError_Localhost(t *testing.T) {
	err := airflowConnectionError("http://localhost:8080/api/v2/dags")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not connect to Airflow")
	assert.Contains(t, err.Error(), "localhost:8080")
	assert.Contains(t, err.Error(), "astro dev start")
	assert.Contains(t, err.Error(), "--api-url")
	assert.Contains(t, err.Error(), "--deployment-id")
}

func TestAirflowConnectionError_Loopback(t *testing.T) {
	err := airflowConnectionError("http://127.0.0.1:8080/api/v2/dags")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not connect to Airflow")
	assert.Contains(t, err.Error(), "astro dev start")
}

func TestAirflowConnectionError_RemoteHost(t *testing.T) {
	err := airflowConnectionError("https://airflow.example.com/api/v2/dags")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not connect to Airflow")
	assert.Contains(t, err.Error(), "airflow.example.com")
	assert.NotContains(t, err.Error(), "astro dev start")
	assert.Contains(t, err.Error(), "Check that the URL is correct")
}

// --- runAirflow connection error handling ------------------------------------

func TestRunAirflow_ConnectionRefused(t *testing.T) {
	// Start and immediately close a server to get a valid URL that refuses connections
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	closedURL := ts.URL
	ts.Close()

	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	opts := &AirflowOptions{
		APIURL:         closedURL,
		AirflowVersion: "3.0.3", // Skip version auto-detection
		RequestOptions: RequestOptions{
			Out:         out,
			ErrOut:      errOut,
			RequestPath: "/dags",
		},
	}

	err := runAirflow(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not connect to Airflow")
	// The closed httptest server is on 127.0.0.1, so we get the localhost suggestion
	assert.Contains(t, err.Error(), "astro dev start")

	// Should NOT print noisy warnings about auth or version detection
	assert.NotContains(t, errOut.String(), "could not fetch auth token")
	assert.NotContains(t, errOut.String(), "Could not detect Airflow version")
}

func TestRunAirflow_ConnectionRefused_OperationID(t *testing.T) {
	// Verify friendly error also works when using an operation ID
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	closedURL := ts.URL
	ts.Close()

	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	opts := &AirflowOptions{
		APIURL:         closedURL,
		AirflowVersion: "3.0.3",
		RequestOptions: RequestOptions{
			Out:         out,
			ErrOut:      errOut,
			RequestPath: "get_dags",
		},
	}

	err := runAirflow(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not connect to Airflow")
	assert.Contains(t, err.Error(), "astro dev start")
}

// --- warning suppression on connection errors --------------------------------

func TestResolveAirflowAPIURL_ConnectionError_SuppressesWarning(t *testing.T) {
	// Start and immediately close a server to get a refused connection
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	closedURL := ts.URL
	ts.Close()

	errOut := new(bytes.Buffer)
	opts := &AirflowOptions{
		APIURL: closedURL,
		RequestOptions: RequestOptions{
			ErrOut: errOut,
		},
	}

	baseURL, authToken, err := resolveAirflowAPIURL(opts)
	require.NoError(t, err)
	assert.Equal(t, closedURL, baseURL)
	assert.Empty(t, authToken)
	// The noisy warning should be suppressed for connection errors
	assert.Empty(t, errOut.String())
}

func TestInitAirflowSpecCache_ConnectionError_SuppressesWarning(t *testing.T) {
	// Start and immediately close a server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	closedURL := ts.URL
	ts.Close()

	errOut := new(bytes.Buffer)
	opts := &AirflowOptions{
		RequestOptions: RequestOptions{
			ErrOut: errOut,
		},
	}

	_, err := initAirflowSpecCache(opts, closedURL+"/api/v2", "")
	require.NoError(t, err)
	assert.NotNil(t, opts.specCache)
	// The noisy warning should be suppressed for connection errors
	assert.Empty(t, errOut.String())
}
