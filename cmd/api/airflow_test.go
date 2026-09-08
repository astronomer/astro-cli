package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/pkg/airflowapi"
	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/awsauth"
	"github.com/astronomer/astro-cli/pkg/instances"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/openapi"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
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
	assert.NotNil(t, cmd.PersistentFlags().Lookup("deployment"))
	assert.Equal(t, "d", cmd.PersistentFlags().ShorthandLookup("d").Name[:1])
	assert.NotNil(t, cmd.PersistentFlags().Lookup("url"))
	// The local-auth defaults, which reach a live token mint on the --url path
	// (see airflowtarget's NewTokenMinter) and are printed in --help. They come
	// from the runtime that provisions the account, so this asserts the wiring
	// rather than the value: nothing else in this package covered them.
	for name, want := range map[string]string{
		"username": airflowrt.Airflow2AdminUser,
		"password": airflowrt.Airflow2AdminPassword,
	} {
		flag := cmd.PersistentFlags().Lookup(name)
		require.NotNil(t, flag, name)
		assert.Equal(t, want, flag.DefValue, "--%s default must come from pkg/airflowrt", name)
	}

	// The two spellings this command shipped with still parse, marked deprecated.
	for _, name := range []string{"api-url", "deployment-id"} {
		flag := cmd.PersistentFlags().Lookup(name)
		require.NotNil(t, flag, name)
		assert.NotEmpty(t, flag.Deprecated, name)
	}
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

	// Check other flags exist
	assert.NotNil(t, cmd.Flags().Lookup("generate"))
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
	assert.Equal(t, "http://localhost:8080", airflowLocalhost)
	assert.Equal(t, "3.0.3", defaultAirflowVersion)
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

// --- initAirflowSpecCache ----------------------------------------------------

// targetFor resolves the target the way the command does, so a test exercises
// the real flag folding rather than a hand-built struct.
func targetFor(t *testing.T, opts *AirflowOptions) *airflowTarget {
	t.Helper()
	if opts.HTTPClient == nil {
		opts.HTTPClient = http.DefaultClient
	}
	if opts.ErrOut == nil {
		opts.ErrOut = new(bytes.Buffer)
	}
	if opts.Username == "" {
		// Nothing to mint with, so supply the credential instead — a test that
		// is not about the mint should not make a mint call.
		opts.RequestHeaders = append(opts.RequestHeaders, "Authorization: Bearer supplied")
	}
	target, err := resolveAirflowTarget(context.Background(), opts)
	require.NoError(t, err)
	return target
}

func TestInitAirflowSpecCache_AlreadyInitialized(t *testing.T) {
	cache, _ := openapi.NewAirflowCacheForVersion("3.0.3")
	opts := &AirflowOptions{
		URL:            "http://localhost:8080",
		RequestOptions: RequestOptions{specCache: cache},
	}
	opts.detectedVersion = "3.0.3"

	result, err := initAirflowSpecCache(context.Background(), opts, targetFor(t, opts))
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:8080/api/v2", result)
}

func TestInitAirflowSpecCache_ManualVersion(t *testing.T) {
	opts := &AirflowOptions{
		URL:            "http://localhost:8080/api/v2",
		AirflowVersion: "2.10.0",
	}

	result, err := initAirflowSpecCache(context.Background(), opts, targetFor(t, opts))
	require.NoError(t, err)
	// The /api/v2 the deprecated spelling carried is dropped, and the version
	// asked for decides the prefix.
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

	opts := &AirflowOptions{URL: ts.URL}

	result, err := initAirflowSpecCache(context.Background(), opts, targetFor(t, opts))
	require.NoError(t, err)
	assert.Equal(t, ts.URL+"/api/v2", result)
	assert.Equal(t, "3.0.3", opts.detectedVersion)
}

// An Astronomer-patched Airflow 2 answers an /api/v2 probe, so the generation
// has to be read off the version it reports rather than off the probe that
// answered. This is pkg/airflowapi's rule, and it now governs this command too.
func TestInitAirflowSpecCache_PatchedAirflow2GetsV1(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "version") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"version":"2.10.5+astro.4"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	opts := &AirflowOptions{URL: ts.URL}

	result, err := initAirflowSpecCache(context.Background(), opts, targetFor(t, opts))
	require.NoError(t, err)
	assert.Equal(t, ts.URL+"/api/v1", result)
}

// --- targeting ---------------------------------------------------------------

func TestResolveAirflowTarget_DefaultsToLocalhost(t *testing.T) {
	// A supplied Authorization header means nothing is minted, which is what
	// keeps this test off the network — whatever is listening on this machine's
	// port 8080 is nobody's business here.
	opts := &AirflowOptions{RequestOptions: RequestOptions{
		ErrOut:         new(bytes.Buffer),
		RequestHeaders: []string{"Authorization: Bearer supplied"},
	}}
	target, err := resolveAirflowTarget(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, airflowLocalhost, target.hostRoot)
	assert.True(t, target.isHTTP())
	assert.False(t, target.isNamedDeployment())
}

// The credential for a bare URL is minted at the instance's own /auth/token,
// through pkg/airflowapi's TokenMinter — the mint this command used to hand-roll.
func TestResolveAirflowTarget_MintsAToken(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/auth/token", r.URL.Path)
		_, _ = w.Write([]byte(`{"access_token":"minted"}`))
	}))
	defer ts.Close()

	opts := &AirflowOptions{URL: ts.URL, Username: "admin", Password: "admin"}
	target := targetFor(t, opts)
	assert.Equal(t, "Bearer minted", target.authorization)
}

// An Airflow 2 serves no /auth/token, so the username and password go on the
// request itself.
func TestResolveAirflowTarget_FallsBackToBasicAuth(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	opts := &AirflowOptions{URL: ts.URL, Username: "admin", Password: "admin"}
	target := targetFor(t, opts)
	assert.Equal(t, "Basic YWRtaW46YWRtaW4=", target.authorization)
}

// Every path into hostRoot strips the API prefix, because the generation is
// detected and put back on top. A link whose url carries one — and a control
// plane whose WebServerAirflowApiUrl already ends in /api/v2, which is what
// this repo's own fixture for it looks like — would otherwise be addressed at
// /api/v1/api/v2/dags.
func TestResolveAirflowTarget_LinkURLDropsTheAPIPrefix(t *testing.T) {
	writeAPIProject(t, `[project]
name = "demo"

[tool.astro]
airflow = "3.1"

[tool.astro.deployments.staging]
url = "https://airflow.staging.corp.dev/api/v1"
auth = { method = 'token', token-env = 'STAGING_AIRFLOW_TOKEN' }
`)
	t.Setenv("STAGING_AIRFLOW_TOKEN", "tok")

	opts := &AirflowOptions{Deployment: "staging", RequestOptions: RequestOptions{ErrOut: new(bytes.Buffer)}}
	target, err := resolveAirflowTarget(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, "https://airflow.staging.corp.dev", target.hostRoot)
	assert.Equal(t, "https://airflow.staging.corp.dev/api/v2", target.apiBase("3.0.3"))
}

// An Astro Deployment's WebServerAirflowApiUrl comes back with no scheme, and
// a manifest cannot hold one — it requires a full address — so this is the one
// place a schemeless URL enters. The transport adds the scheme, which is why
// the query commands always worked; this command builds its own requests, so
// the target has to carry the URL the transport settled on rather than the raw
// string it was handed. Without that, every -d against a deployment failed with
// "unsupported protocol scheme".
func TestHTTPTargetCarriesTheTransportsURL(t *testing.T) {
	opts := &AirflowOptions{RequestOptions: RequestOptions{ErrOut: new(bytes.Buffer)}}
	target, err := opts.httpTarget("prod", "airflow.corp.dev/api/v2", "")
	require.NoError(t, err)
	assert.Equal(t, "https://airflow.corp.dev", target.hostRoot)
	assert.Equal(t, "https://airflow.corp.dev/api/v2", target.apiBase("3.0.3"))
}

// The same, end to end: the request lands on the API rather than under a
// doubled prefix.
func TestRunAirflow_LinkWithAPrefixedURLLandsOnTheAPI(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v2/version", "/api/v1/version":
			_, _ = w.Write([]byte(`{"version":"3.0.3"}`))
		default:
			_, _ = w.Write([]byte(`{"dags":[]}`))
		}
	}))
	defer ts.Close()

	writeAPIProject(t, `[project]
name = "demo"

[tool.astro]
airflow = "3.1"

[tool.astro.deployments.staging]
url = "`+ts.URL+`/api/v1"
auth = { method = 'token', token-env = 'STAGING_AIRFLOW_TOKEN' }
`)
	t.Setenv("STAGING_AIRFLOW_TOKEN", "tok")

	opts := &AirflowOptions{
		Deployment:     "staging",
		RequestOptions: RequestOptions{Out: new(bytes.Buffer), ErrOut: new(bytes.Buffer), RequestPath: "/dags", RequestMethod: "GET"},
	}
	require.NoError(t, runAirflow(opts))

	mu.Lock()
	defer mu.Unlock()
	assert.NotContains(t, paths, "/api/v1/api/v2/dags")
	assert.Contains(t, paths, "/api/v2/dags")
}

// A named deployment whose version cannot be read fails, whatever was asked
// for. The generation decides the path, so carrying on would send a raw path
// somewhere this run could not work out — and report success doing it.
func TestRunAirflow_ProbeFailureOnANamedDeploymentFails(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/version") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"dags":[]}`))
	}))
	defer ts.Close()

	writeAPIProject(t, `[project]
name = "demo"

[tool.astro]
airflow = "3.1"

[tool.astro.deployments.staging]
url = "`+ts.URL+`"
auth = { method = 'token', token-env = 'STAGING_AIRFLOW_TOKEN' }
`)
	t.Setenv("STAGING_AIRFLOW_TOKEN", "tok")

	opts := &AirflowOptions{
		Deployment:     "staging",
		RequestOptions: RequestOptions{Out: new(bytes.Buffer), ErrOut: new(bytes.Buffer), RequestPath: "/dags", RequestMethod: "GET"},
	}
	err := runAirflow(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--airflow-version")

	mu.Lock()
	defer mu.Unlock()
	assert.NotContains(t, paths, "/dags", "a request went out under no API prefix at all")
}

// The localhost default is allowed to fall back — it may simply not be running
// — but the fallback still carries a generation prefix.
func TestInitAirflowSpecCache_FallbackKeepsThePrefix(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	errOut := new(bytes.Buffer)
	opts := &AirflowOptions{RequestOptions: RequestOptions{ErrOut: errOut, HTTPClient: http.DefaultClient}}
	// A target nobody named, pointed at a server that refuses the probe.
	target, err := opts.httpTarget("the Airflow on localhost", ts.URL, "")
	require.NoError(t, err)

	base, err := initAirflowSpecCache(context.Background(), opts, target)
	require.NoError(t, err)
	assert.Equal(t, ts.URL+"/api/v2", base)
	assert.Contains(t, errOut.String(), "Could not detect Airflow version")
}

// The generated curl command carries a placeholder, never the credential this
// run resolved. --verbose has always masked the same header.
func TestRunAirflow_GenerateWithholdsTheCredential(t *testing.T) {
	writeAPIProject(t, `[project]
name = "demo"

[tool.astro]
airflow = "3.1"

[tool.astro.deployments.staging]
url = "https://airflow.staging.corp.dev"
auth = { method = 'token', token-env = 'STAGING_AIRFLOW_TOKEN' }
`)
	t.Setenv("STAGING_AIRFLOW_TOKEN", "s3cr3t-not-for-your-scrollback")

	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	opts := &AirflowOptions{
		Deployment:     "staging",
		AirflowVersion: "3.0.3",
		RequestOptions: RequestOptions{
			Out: out, ErrOut: errOut,
			RequestPath: "/dags", RequestMethod: "GET",
			GenerateCurl: true,
		},
	}
	require.NoError(t, runAirflow(opts))
	assert.NotContains(t, out.String(), "s3cr3t-not-for-your-scrollback")
	assert.Contains(t, out.String(), "Authorization: Bearer $AIRFLOW_TOKEN")
	assert.Contains(t, errOut.String(), "The credential was withheld")
}

// The conflict message names the flags the reader typed, not the ones they did
// not: half of these four spellings are deprecated aliases.
func TestResolveAirflowTarget_ConflictNamesTheFlagsTyped(t *testing.T) {
	opts := &AirflowOptions{APIURL: "https://a.dev", DeploymentID: "clx"}
	_, err := resolveAirflowTarget(context.Background(), opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--api-url")
	assert.Contains(t, err.Error(), "--deployment-id")
	assert.NotContains(t, err.Error(), "--url and")
}

// --deployment falls through to an Astro Deployment id for a name no link
// declares, which is what --deployment-id always meant.
func TestResolveAirflowTarget_DeploymentID(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, err := config.GetCurrentContext()
	require.NoError(t, err)
	ctx.Token, ctx.Organization = "test-token", "test-org"
	require.NoError(t, ctx.SetContext())

	orig := deployment.GetDeploymentByID
	t.Cleanup(func() { deployment.GetDeploymentByID = orig })
	deployment.GetDeploymentByID = func(orgID, deploymentID string, _ astrov1.APIClient) (astrov1.Deployment, error) {
		assert.Equal(t, "test-org", orgID)
		assert.Equal(t, "clxyz123", deploymentID)
		return astrov1.Deployment{Id: deploymentID, WebServerAirflowApiUrl: "deployment.airflow.astronomer.io/api/v2"}, nil
	}

	opts := &AirflowOptions{Deployment: "clxyz123", RequestOptions: RequestOptions{ErrOut: new(bytes.Buffer)}}
	target, err := resolveAirflowTarget(context.Background(), opts)
	require.NoError(t, err)
	// A URL with no scheme gets https, and the /api/v2 the control plane
	// appends is dropped — the generation is detected, not taken on trust.
	assert.Equal(t, "https://deployment.airflow.astronomer.io", target.hostRoot)
	// The session token is stored without its scheme on some machines; the
	// header carries one either way.
	assert.Equal(t, "Bearer test-token", target.authorization)
}

// The deprecated flag still reaches the same place.
func TestResolveAirflowTarget_DeploymentIDAliasStillWorks(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	opts := &AirflowOptions{DeploymentID: "clxyz123", RequestOptions: RequestOptions{ErrOut: new(bytes.Buffer)}}
	_, err := resolveAirflowTarget(context.Background(), opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires cloud context")
}

// An explicit --username or --password is a claim about how to log in, so
// failing to log in is this command's failure rather than a shrug.
func TestResolveAirflowTarget_ExplicitCredentialsMustWork(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	opts := &AirflowOptions{
		URL:                 ts.URL,
		Username:            "admin",
		Password:            "wrong",
		CredentialsExplicit: true,
		RequestOptions:      RequestOptions{ErrOut: new(bytes.Buffer), HTTPClient: http.DefaultClient},
	}
	_, err := resolveAirflowTarget(context.Background(), opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}

func TestResolveAirflowTarget_URLDropsTheAPIPrefix(t *testing.T) {
	opts := &AirflowOptions{
		URL:            "https://airflow.corp.dev/api/v2",
		RequestOptions: RequestOptions{RequestHeaders: []string{"Authorization: Bearer supplied"}},
	}
	target, err := resolveAirflowTarget(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, "https://airflow.corp.dev", target.hostRoot)
	// The caller said how to prove themselves, so nothing is minted over it.
	assert.Empty(t, target.authorization)
}

func TestResolveAirflowTarget_APIURLIsAnAliasOfURL(t *testing.T) {
	opts := &AirflowOptions{
		APIURL:         "https://airflow.corp.dev/api/v1",
		RequestOptions: RequestOptions{RequestHeaders: []string{"Authorization: Bearer supplied"}},
	}
	target, err := resolveAirflowTarget(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, "https://airflow.corp.dev", target.hostRoot)
	// A URL is a guess someone typed, so a version probe that fails against it
	// warns and falls back rather than failing the command.
	assert.False(t, target.isNamedDeployment())
}

func TestResolveAirflowTarget_RefusesTwoTargets(t *testing.T) {
	cases := map[string]*AirflowOptions{
		"--url with --deployment":    {URL: "https://a.dev", Deployment: "prod"},
		"--url with --api-url":       {URL: "https://a.dev", APIURL: "https://b.dev"},
		"--url with --deployment-id": {URL: "https://a.dev", DeploymentID: "clx"},
	}
	for name, opts := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := resolveAirflowTarget(context.Background(), opts)
			require.Error(t, err)
		})
	}
}

// A deployment link the manifest declares is reached through the same
// resolution the query commands use.
func TestResolveAirflowTarget_ManifestLink(t *testing.T) {
	writeAPIProject(t, `[project]
name = "demo"

[tool.astro]
airflow = "3.1"

[tool.astro.deployments.staging]
url = "https://airflow.staging.corp.dev"
auth = { method = 'token', token-env = 'STAGING_AIRFLOW_TOKEN' }
`)
	t.Setenv("STAGING_AIRFLOW_TOKEN", "from-the-environment")

	opts := &AirflowOptions{Deployment: "staging", RequestOptions: RequestOptions{ErrOut: new(bytes.Buffer)}}
	target, err := resolveAirflowTarget(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, "staging", target.name)
	assert.Equal(t, "https://airflow.staging.corp.dev", target.hostRoot)
	assert.Equal(t, "Bearer from-the-environment", target.authorization)
	assert.True(t, target.isNamedDeployment())
}

// An MWAA link has no Airflow URL at all: its requests travel inside a signed
// AWS API call, so resolution hands back the door itself rather than a URL. The
// AWS half needs credentials no test machine should need, so this holds the
// seam that decides it: HTTPDoorFor refuses, and the target then carries the
// transport (TestRunAirflowThroughTransport_UnwrapsTheAnswer covers what
// happens after).
func TestMWAALinkHasNoHTTPDoor(t *testing.T) {
	m, err := manifest.Parse([]byte(`[project]
name = "demo"

[tool.astro]
airflow = "3.1"

[tool.astro.deployments.prod]
target = "mwaa"
environment = "orders-prod"

[tool.astro.targets.mwaa]
region = "us-east-1"
`))
	require.NoError(t, err)

	instance, ok := instances.Build(m).Lookup("prod")
	require.True(t, ok)
	// With the door wired, because that is what this asserts: an mwaa link has
	// no HTTP address even in a build that can reach it. A build carrying no
	// aws provider is refused earlier and for a different reason.
	d := instances.Deps{Providers: instances.Providers{manifest.AuthAWS: awsauth.Provider(awsauth.Options{})}}
	_, err = instance.HTTPDoorFor(context.Background(), d)
	require.ErrorIs(t, err, instances.ErrNotHTTP)
}

// stubTransport stands in for the MWAA door: it answers the generation probe
// like an Airflow 3, and everything else with the canned response.
type stubTransport struct{ resp airflowapi.Response }

func (s stubTransport) Do(_ context.Context, req airflowapi.Request) (airflowapi.Response, error) {
	if req.Path == "/version" {
		return airflowapi.Response{StatusCode: http.StatusOK, Body: []byte(`{"version":"3.0.3"}`)}, nil
	}
	return s.resp, nil
}

// The doc's open question 1, settled: InvokeRestApi wraps the answer in
// RestApiStatusCode and RestApiResponse, the transport maps both onto an
// ordinary airflowapi.Response, and this command prints exactly that — the same
// status and the same body it would print for any HTTP Airflow. The AWS
// envelope is machinery, not the answer.
func TestRunAirflowThroughTransport_UnwrapsTheAnswer(t *testing.T) {
	out := new(bytes.Buffer)
	target := &airflowTarget{
		name: "prod",
		transport: stubTransport{resp: airflowapi.Response{
			StatusCode: http.StatusOK,
			Body:       []byte(`{"dags":[{"dag_id":"orders_etl"}],"total_entries":1}`),
		}},
	}
	opts := &AirflowOptions{RequestOptions: RequestOptions{Out: out, FilterOutput: ".dags[].dag_id"}}

	require.NoError(t, runAirflowThroughTransport(context.Background(), opts, target, http.MethodGet, "/dags", nil))
	assert.Contains(t, out.String(), "orders_etl")
}

func TestRunAirflowThroughTransport_ReportsTheStatusItWasGiven(t *testing.T) {
	out := new(bytes.Buffer)
	target := &airflowTarget{
		name: "prod",
		transport: stubTransport{resp: airflowapi.Response{
			StatusCode: http.StatusNotFound,
			Body:       []byte(`{"detail":"DAG not found"}`),
		}},
	}
	opts := &AirflowOptions{RequestOptions: RequestOptions{Out: out}}

	err := runAirflowThroughTransport(context.Background(), opts, target, http.MethodGet, "/dags/nope", nil)
	var silent *SilentError
	require.ErrorAs(t, err, &silent)
	assert.Equal(t, http.StatusNotFound, silent.StatusCode)
	assert.Contains(t, out.String(), "DAG not found")
}

// writeAPIProject points config.WorkingPath at a fresh project carrying this
// manifest, so -d resolves against it.
func writeAPIProject(t *testing.T, toml string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(toml), 0o600))
	orig := config.WorkingPath
	config.WorkingPath = dir
	t.Cleanup(func() { config.WorkingPath = orig })
}

// The flags that only make sense against a URL are refused rather than quietly
// ignored on a door that has none.
func TestRunAirflowThroughTransport_RefusesURLOnlyFlags(t *testing.T) {
	opts := &AirflowOptions{RequestOptions: RequestOptions{Out: new(bytes.Buffer), GenerateCurl: true}}
	target := &airflowTarget{name: "prod"}
	err := runAirflowThroughTransport(context.Background(), opts, target, http.MethodGet, "/dags", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--generate")
	assert.Contains(t, err.Error(), "prod")
}

// --- airflowConnectionError --------------------------------------------------

func TestAirflowConnectionError_Localhost(t *testing.T) {
	err := airflowConnectionError("http://localhost:8080/api/v2/dags")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not connect to Airflow")
	assert.Contains(t, err.Error(), "localhost:8080")
	assert.Contains(t, err.Error(), "astro local start")
	assert.Contains(t, err.Error(), "--url")
	assert.Contains(t, err.Error(), "-d <name or id>")
}

func TestAirflowConnectionError_Loopback(t *testing.T) {
	err := airflowConnectionError("http://127.0.0.1:8080/api/v2/dags")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not connect to Airflow")
	assert.Contains(t, err.Error(), "astro local start")
}

func TestAirflowConnectionError_RemoteHost(t *testing.T) {
	err := airflowConnectionError("https://airflow.example.com/api/v2/dags")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not connect to Airflow")
	assert.Contains(t, err.Error(), "airflow.example.com")
	assert.NotContains(t, err.Error(), "astro local start")
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
	assert.Contains(t, err.Error(), "astro local start")

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
	assert.Contains(t, err.Error(), "astro local start")
}

// --- warning suppression on connection errors --------------------------------

func TestInitAirflowSpecCache_ConnectionError_SuppressesWarning(t *testing.T) {
	// Start and immediately close a server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	closedURL := ts.URL
	ts.Close()

	errOut := new(bytes.Buffer)
	opts := &AirflowOptions{
		URL:            closedURL,
		RequestOptions: RequestOptions{ErrOut: errOut, HTTPClient: http.DefaultClient},
	}
	target, err := resolveAirflowTarget(context.Background(), opts)
	require.NoError(t, err)

	_, err = initAirflowSpecCache(context.Background(), opts, target)
	require.NoError(t, err)
	assert.NotNil(t, opts.specCache)
	// The noisy warning should be suppressed for connection errors
	assert.Empty(t, errOut.String())
}
