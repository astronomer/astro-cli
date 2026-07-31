package instances

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
	"github.com/astronomer/astro-cli/pkg/airflowrt"
)

// env builds a LookupEnv over a fixed map, so no test touches the process
// environment.
func env(pairs map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		v, ok := pairs[name]
		return v, ok
	}
}

// link builds a one-link set and returns that link's instance, so an auth test
// reads as the manifest a user would write.
func link(t *testing.T, body string) Instance {
	t.Helper()
	set := Build(Inputs{ProjectPath: filepath.Join(t.TempDir(), "orders"), Manifest: parseManifest(t, body)})
	all := set.All()
	if len(all) != 1 {
		t.Fatalf("expected one link, got %v", set.Names())
	}
	return all[0]
}

// header runs a credential source and returns the Authorization header it
// would produce, which is what actually matters about it.
func header(t *testing.T, src airflowapi.CredentialSource) string {
	t.Helper()
	if src == nil {
		return ""
	}
	scheme, value, err := src(context.Background())
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	if scheme == "" {
		return ""
	}
	return scheme + " " + value
}

func TestTokenMethodReadsItsEnvVar(t *testing.T) {
	i := link(t, "\n[tool.astro.deployments.staging]\nurl = 'https://airflow.staging.corp.dev'\nauth = { method = 'token', token-env = 'STAGING_AIRFLOW_TOKEN' }\n")
	src, _, err := credentials(i, i.URL, Deps{LookupEnv: env(map[string]string{"STAGING_AIRFLOW_TOKEN": "s3cr3t"})})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	if got := header(t, src); got != "Bearer s3cr3t" {
		t.Fatalf("header = %q", got)
	}
}

func TestMissingCredentialNamesTheVariableAndTheFix(t *testing.T) {
	i := link(t, "\n[tool.astro.deployments.staging]\nurl = 'https://airflow.staging.corp.dev'\nauth = { method = 'token', token-env = 'STAGING_AIRFLOW_TOKEN' }\n")
	_, _, err := credentials(i, i.URL, Deps{LookupEnv: env(nil)})
	if err == nil {
		t.Fatal("a missing credential resolved")
	}
	for _, want := range []string{`instance "staging"`, "STAGING_AIRFLOW_TOKEN", "astro local env set STAGING_AIRFLOW_TOKEN --project"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message does not name %s: %s", want, err)
		}
	}

	// Exported but empty is the same miss: sending an empty credential would
	// only turn a half-finished setup into an unexplained 401.
	if _, _, err := credentials(i, i.URL, Deps{LookupEnv: env(map[string]string{"STAGING_AIRFLOW_TOKEN": ""})}); err == nil {
		t.Fatal("an empty credential resolved")
	}
}

func TestBasicMethodReadsBothVariables(t *testing.T) {
	i := link(t, "\n[tool.astro.deployments.legacy]\nurl = 'https://airflow.corp.dev'\nauth = { method = 'basic', username-env = 'AF_USER', password-env = 'AF_PASS' }\n")
	src, _, err := credentials(i, i.URL, Deps{LookupEnv: env(map[string]string{"AF_USER": "ada", "AF_PASS": "hunter2"})})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("ada:hunter2"))
	if got := header(t, src); got != want {
		t.Fatalf("header = %q, want %q", got, want)
	}

	// Half the pair set is the same missing-value report, naming the half that
	// is absent.
	_, _, err = credentials(i, i.URL, Deps{LookupEnv: env(map[string]string{"AF_USER": "ada"})})
	if err == nil || !strings.Contains(err.Error(), "AF_PASS") {
		t.Fatalf("err = %v, want one naming AF_PASS", err)
	}
}

func TestNoneMethodSendsNothing(t *testing.T) {
	i := link(t, "\n[tool.astro.deployments.open]\nurl = 'http://airflow.dev.corp'\nauth = { method = 'none' }\n")
	src, _, err := credentials(i, i.URL, Deps{LookupEnv: env(nil)})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	if src != nil {
		t.Fatalf("the none method produced a credential source")
	}
}

func TestAstroMethodPrefersTheAPITokenThenTheSession(t *testing.T) {
	i := link(t, "\n[tool.astro.deployments.prod]\ndeployment = 'clm2xk9dq000108l7a2b3c4d5'\n")

	// CI: the API token stands in for a login on the machine.
	src, _, err := credentials(i, "", Deps{LookupEnv: env(map[string]string{EnvAPIToken: "ci-token"})})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	if got := header(t, src); got != "Bearer ci-token" {
		t.Fatalf("header = %q", got)
	}

	// A machine with a login: the session's bearer.
	src, _, err = credentials(i, "", Deps{
		LookupEnv: env(nil),
		Session:   func(context.Context) (string, error) { return "session-token", nil },
	})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	if got := header(t, src); got != "Bearer session-token" {
		t.Fatalf("header = %q", got)
	}
}

func TestAstroMethodReportsTheOutageItWasGiven(t *testing.T) {
	i := link(t, "\n[tool.astro.deployments.prod]\ndeployment = 'clm2xk9dq000108l7a2b3c4d5'\n")

	// No session wired at all reads as logged out, and says how to fix it.
	src, _, err := credentials(i, "", Deps{LookupEnv: env(nil)})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	_, _, err = src(context.Background())
	if err == nil || !strings.Contains(err.Error(), "astro login") {
		t.Fatalf("err = %v, want the logged-out message", err)
	}

	// A session that fails travels as its own named cause, not a stack trace.
	src, _, err = credentials(i, "", Deps{
		LookupEnv: env(nil),
		Session: func(context.Context) (string, error) {
			return "", errors.New("your session expired — log in again with `astro login`")
		},
	})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	if _, _, err = src(context.Background()); err == nil || !strings.Contains(err.Error(), "session expired") {
		t.Fatalf("err = %v, want the expiry cause", err)
	}
}

func TestGoogleMethodCarriesTheADCToken(t *testing.T) {
	i := link(t, "\n[tool.astro.deployments.legacy]\nurl = 'https://airflow.internal.corp'\nauth = { method = 'google' }\n")
	src, _, err := credentials(i, i.URL, Deps{
		LookupEnv:   env(nil),
		GoogleToken: func(context.Context) (string, error) { return "ya29.token", nil },
	})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	if got := header(t, src); got != "Bearer ya29.token" {
		t.Fatalf("header = %q", got)
	}
}

func TestGoogleMethodNamesTheMissingChain(t *testing.T) {
	i := link(t, "\n[tool.astro.deployments.prod]\ntarget = 'composer'\nenvironment = 'orders-prod'\n")
	src, _, err := credentials(i, "https://composer.example", Deps{
		LookupEnv:   env(nil),
		GoogleToken: func(context.Context) (string, error) { return "", ErrNoGoogleCredentials },
	})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	_, _, err = src(context.Background())
	if err == nil || !strings.Contains(err.Error(), "gcloud auth application-default login") {
		t.Fatalf("err = %v, want the ADC message", err)
	}
}

func TestURLTargetTakesItsCredentialFromTheEnvironment(t *testing.T) {
	i := URLInstance("https://airflow.corp.dev")

	// Nothing set: nothing sent, so an open dev server just works.
	src, _, err := credentials(i, i.URL, Deps{LookupEnv: env(nil)})
	if err != nil || src != nil {
		t.Fatalf("credentials = %v, %v; want no credential and no error", src, err)
	}

	src, _, err = credentials(i, i.URL, Deps{LookupEnv: env(map[string]string{EnvToken: "t0k"})})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	if got := header(t, src); got != "Bearer t0k" {
		t.Fatalf("header = %q", got)
	}

	// Half a username/password pair is always a mistake.
	_, _, err = credentials(i, i.URL, Deps{LookupEnv: env(map[string]string{EnvUsername: "ada"})})
	if err == nil || !strings.Contains(err.Error(), EnvPassword) {
		t.Fatalf("err = %v, want one naming %s", err, EnvPassword)
	}
}

// localProject writes a project pinning the given Airflow, which is what tells
// the resolver which credentials its local engine provisioned.
func localProject(t *testing.T, airflowVersion string) string {
	t.Helper()
	dir := t.TempDir()
	body := "[project]\nname = 'demo'\nrequires-python = '>=3.10'\n\n[tool.astro]\nairflow = '" + airflowVersion + "'\n"
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// mintServer stands in for a local Airflow's /auth/token, recording how it was
// asked and answering with a token.
func mintServer(t *testing.T, got *struct {
	method             string
	username, password string
},
) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/token" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		got.method = r.Method
		if r.Method == http.MethodPost {
			var body struct{ Username, Password string }
			if err := decodeJSON(r, &body); err != nil {
				t.Errorf("decode mint request: %v", err)
			}
			got.username, got.password = body.Username, body.Password
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"minted"}`))
	}))
}

// TestLocalMintsWithoutCredentialsOnAirflow3 covers the common local case: both
// engines run Airflow 3 under the simple auth manager with all-admins on, which
// mints for whoever asks, so there is nothing to send.
func TestLocalMintsWithoutCredentialsOnAirflow3(t *testing.T) {
	var got struct {
		method             string
		username, password string
	}
	server := mintServer(t, &got)
	defer server.Close()

	project := localProject(t, "3.1")
	i := Instance{Name: LocalName, Kind: KindLocal, Source: SourceRunning, URL: server.URL, Project: project, AirflowMajor: "3"}
	src, refresh, err := credentials(i, server.URL, Deps{HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	if header(t, src) != "Bearer minted" {
		t.Fatalf("header = %q, want the minted token", header(t, src))
	}
	if got.method != http.MethodGet || got.username != "" {
		t.Fatalf("minted with %s %s/%s, want a credential-less GET", got.method, got.username, got.password)
	}
	if refresh == nil {
		t.Fatal("no refresh hook: a short-lived local token has to be re-mintable")
	}
}

// TestLocalMintsWithTheAdminAccountOnAirflow2 covers the other engine path:
// Airflow 2 runs standalone with basic auth and a real admin account.
func TestLocalMintsWithTheAdminAccountOnAirflow2(t *testing.T) {
	var got struct {
		method             string
		username, password string
	}
	server := mintServer(t, &got)
	defer server.Close()

	project := localProject(t, "2.10.5")
	i := Instance{Name: LocalName, Kind: KindLocal, Source: SourceRunning, URL: server.URL, Project: project, AirflowMajor: airflow2}
	src, _, err := credentials(i, server.URL, Deps{HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	if header(t, src) != "Bearer minted" {
		t.Fatalf("header = %q", header(t, src))
	}
	if got.method != http.MethodPost || got.username != localUsername || got.password != localPassword {
		t.Fatalf("minted with %s %s/%s, want a POST as %s", got.method, got.username, got.password, localUsername)
	}
}

// TestLocalUsesTheGeneratedStandalonePassword covers the Airflow 2 standalone
// that generates a password into its AIRFLOW_HOME instead of taking admin.
func TestLocalUsesTheGeneratedStandalonePassword(t *testing.T) {
	project := localProject(t, "2.10.5")
	writePasswordFile(t, project, "generated\n")
	username, password := localAccount(airflow2, project)
	if username != localUsername || password != "generated" {
		t.Fatalf("account = %s/%s, want %s with the generated password", username, password, localUsername)
	}

	// No file: the password the macOS launch shim seeds.
	username, password = localAccount(airflow2, localProject(t, "2.10.5"))
	if username != localUsername || password != localPassword {
		t.Fatalf("account = %s/%s, want the shim's %s/%s", username, password, localUsername, localPassword)
	}
}

// TestLocalAccountFollowsTheRunningProcess: the generation comes from the
// runtime record, not the manifest. An edit to the pin, an unparseable file, or
// a deleted project cannot change what a process that is already running
// accepts.
func TestLocalAccountFollowsTheRunningProcess(t *testing.T) {
	project := localProject(t, "3.1") // the pin now says 3
	writePasswordFile(t, project, "generated\n")
	// The record says the process was started for Airflow 2, and it is the
	// process that has to be talked to.
	username, password := localAccount(airflow2, project)
	if username != localUsername || password != "generated" {
		t.Fatalf("account = %s/%s, want the Airflow 2 account the record calls for", username, password)
	}

	// The reverse: a manifest edited down to 2 while an Airflow 3 runs.
	if username, password = localAccount("3", localProject(t, "2.10.5")); username != "" || password != "" {
		t.Fatalf("account = %s/%s, want no credentials for a running Airflow 3", username, password)
	}

	// A project deleted while its Airflow keeps running still resolves, because
	// nothing about the credential needed the directory.
	gone := localProject(t, "2.10.5")
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	if username, password = localAccount(airflow2, gone); username != localUsername || password != localPassword {
		t.Fatalf("account = %s/%s for a deleted project, want the shim's default", username, password)
	}
}

// TestLocalAccountFallsBackToTheManifest covers a record written before the
// generation was stored: the manifest is the only thing left to ask.
func TestLocalAccountFallsBackToTheManifest(t *testing.T) {
	if username, _ := localAccount("", localProject(t, "2.10.5")); username != localUsername {
		t.Errorf("an old record for an Airflow 2 project got %q", username)
	}
	if username, _ := localAccount("", localProject(t, "3.1")); username != "" {
		t.Errorf("an old record for an Airflow 3 project got %q", username)
	}
	// No manifest to fall back to reads as Airflow 3: what the v2 scaffold
	// writes, and all docker mode runs.
	if username, password := localAccount("", t.TempDir()); username != "" || password != "" {
		t.Fatalf("account = %s/%s, want no credentials", username, password)
	}
}

// TestPasswordFileShapes: the file is written by `airflow standalone`, so what
// it holds is not this code's choice. Whitespace-only counts as absent.
func TestPasswordFileShapes(t *testing.T) {
	cases := []struct{ content, want string }{
		{"s3cret\n", "s3cret"},
		{"s3cret\r\n", "s3cret"},
		{"", localPassword},
		{"\n\n", localPassword},
	}
	for _, tc := range cases {
		project := localProject(t, "2.10.5")
		writePasswordFile(t, project, tc.content)
		if got := localPasswordFor(project); got != tc.want {
			t.Errorf("%q -> %q, want %q", tc.content, got, tc.want)
		}
	}
}

func writePasswordFile(t *testing.T, projectPath, content string) {
	t.Helper()
	home := filepath.Join(projectPath, airflowrt.StandaloneDir)
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, localPasswordFile), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestLocalFallsBackToBasicOnAirflow2 covers an Airflow 2 that serves no
// /auth/token: the minter turns the same credentials into basic auth.
func TestLocalFallsBackToBasicOnAirflow2(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	project := localProject(t, "2.10.5")
	i := Instance{Name: LocalName, Kind: KindLocal, Source: SourceRunning, URL: server.URL, Project: project, AirflowMajor: airflow2}
	src, _, err := credentials(i, server.URL, Deps{HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte(localUsername+":"+localPassword))
	if got := header(t, src); got != want {
		t.Fatalf("header = %q, want %q", got, want)
	}
}
