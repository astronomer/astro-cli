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

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/instances/instancestest"
)

// link builds a one-link set and returns that link's instance, so an auth test
// reads as the manifest a user would write.
func link(t *testing.T, body string) Instance {
	t.Helper()
	set := Build(instancestest.Manifest(t, body))
	return instancestest.OneLink(t, set.All(), set.Names())
}

func TestTokenMethodReadsItsEnvVar(t *testing.T) {
	i := link(t, "\n[tool.astro.deployments.staging]\nurl = 'https://airflow.staging.corp.dev'\nauth = { method = 'token', token-env = 'STAGING_AIRFLOW_TOKEN' }\n")
	src, _, err := credentials(context.Background(), i, i.URL, Deps{LookupEnv: instancestest.Env(map[string]string{"STAGING_AIRFLOW_TOKEN": "s3cr3t"})})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	if got := instancestest.Header(t, src); got != "Bearer s3cr3t" {
		t.Fatalf("header = %q", got)
	}
}

func TestMissingCredentialNamesTheVariableAndTheFix(t *testing.T) {
	i := link(t, "\n[tool.astro.deployments.staging]\nurl = 'https://airflow.staging.corp.dev'\nauth = { method = 'token', token-env = 'STAGING_AIRFLOW_TOKEN' }\n")
	_, _, err := credentials(context.Background(), i, i.URL, Deps{LookupEnv: instancestest.Env(nil)})
	if err == nil {
		t.Fatal("a missing credential resolved")
	}
	for _, want := range []string{`deployment "staging"`, "STAGING_AIRFLOW_TOKEN", "astro local env variable set STAGING_AIRFLOW_TOKEN --project"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message does not name %s: %s", want, err)
		}
	}

	// Exported but empty is the same miss: sending an empty credential would
	// only turn a half-finished setup into an unexplained 401.
	if _, _, err := credentials(context.Background(), i, i.URL, Deps{LookupEnv: instancestest.Env(map[string]string{"STAGING_AIRFLOW_TOKEN": ""})}); err == nil {
		t.Fatal("an empty credential resolved")
	}
}

func TestBasicMethodReadsBothVariables(t *testing.T) {
	i := link(t, "\n[tool.astro.deployments.legacy]\nurl = 'https://airflow.corp.dev'\nauth = { method = 'basic', username-env = 'AF_USER', password-env = 'AF_PASS' }\n")
	src, _, err := credentials(context.Background(), i, i.URL, Deps{LookupEnv: instancestest.Env(map[string]string{"AF_USER": "ada", "AF_PASS": "hunter2"})})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("ada:hunter2"))
	if got := instancestest.Header(t, src); got != want {
		t.Fatalf("header = %q, want %q", got, want)
	}

	// Half the pair set is the same missing-value report, naming the half that
	// is absent.
	_, _, err = credentials(context.Background(), i, i.URL, Deps{LookupEnv: instancestest.Env(map[string]string{"AF_USER": "ada"})})
	if err == nil || !strings.Contains(err.Error(), "AF_PASS") {
		t.Fatalf("err = %v, want one naming AF_PASS", err)
	}
}

func TestNoneMethodSendsNothing(t *testing.T) {
	i := link(t, "\n[tool.astro.deployments.open]\nurl = 'http://airflow.dev.corp'\nauth = { method = 'none' }\n")
	src, _, err := credentials(context.Background(), i, i.URL, Deps{LookupEnv: instancestest.Env(nil)})
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
	src, _, err := credentials(context.Background(), i, "", Deps{LookupEnv: instancestest.Env(map[string]string{EnvAPIToken: "ci-token"})})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	if got := instancestest.Header(t, src); got != "Bearer ci-token" {
		t.Fatalf("header = %q", got)
	}

	// A machine with a login: the session's bearer.
	src, _, err = credentials(context.Background(), i, "", Deps{
		LookupEnv: instancestest.Env(nil),
		Session:   func(context.Context) (string, error) { return "session-token", nil },
	})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	if got := instancestest.Header(t, src); got != "Bearer session-token" {
		t.Fatalf("header = %q", got)
	}
}

func TestAstroMethodReportsTheOutageItWasGiven(t *testing.T) {
	i := link(t, "\n[tool.astro.deployments.prod]\ndeployment = 'clm2xk9dq000108l7a2b3c4d5'\n")

	// No session wired at all reads as logged out, and says how to fix it.
	src, _, err := credentials(context.Background(), i, "", Deps{LookupEnv: instancestest.Env(nil)})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	_, _, err = src(context.Background())
	if err == nil || !strings.Contains(err.Error(), "astro login") {
		t.Fatalf("err = %v, want the logged-out message", err)
	}

	// A session that fails travels as its own named cause, not a stack trace.
	src, _, err = credentials(context.Background(), i, "", Deps{
		LookupEnv: instancestest.Env(nil),
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

func TestURLTargetTakesItsCredentialFromTheEnvironment(t *testing.T) {
	i := URLInstance("https://airflow.corp.dev")

	// Nothing set: nothing sent, so an open dev server just works.
	src, _, err := credentials(context.Background(), i, i.URL, Deps{LookupEnv: instancestest.Env(nil)})
	if err != nil || src != nil {
		t.Fatalf("credentials = %v, %v; want no credential and no error", src, err)
	}

	src, _, err = credentials(context.Background(), i, i.URL, Deps{LookupEnv: instancestest.Env(map[string]string{EnvToken: "t0k"})})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	if got := instancestest.Header(t, src); got != "Bearer t0k" {
		t.Fatalf("header = %q", got)
	}

	// Half a username/password pair is always a mistake.
	_, _, err = credentials(context.Background(), i, i.URL, Deps{LookupEnv: instancestest.Env(map[string]string{EnvUsername: "ada"})})
	if err == nil || !strings.Contains(err.Error(), EnvPassword) {
		t.Fatalf("err = %v, want one naming %s", err, EnvPassword)
	}
}

// urlPair is the environment a --url target with a username and password sees.
var urlPair = instancestest.Env(map[string]string{EnvUsername: "ada", EnvPassword: "hunter2"})

// An Airflow 3 takes no username and password on an API call, so a --url
// target exchanges them at the instance's own /auth/token, the way `astro api
// airflow --url` does, and sends the JWT that comes back.
func TestURLTargetMintsWithAUsernameAndPasswordOnAirflow3(t *testing.T) {
	var got struct {
		method             string
		username, password string
	}
	server := mintServer(t, &got)
	defer server.Close()

	i := URLInstance(server.URL)
	src, refresh, err := credentials(context.Background(), i, i.URL, Deps{LookupEnv: urlPair, HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	if got := instancestest.Header(t, src); got != "Bearer minted" {
		t.Fatalf("header = %q, want the minted token", got)
	}
	if got.method != http.MethodPost || got.username != "ada" || got.password != "hunter2" {
		t.Fatalf("minted with %s %s/%s, want a POST as ada", got.method, got.username, got.password)
	}
	if refresh == nil {
		t.Fatal("no refresh hook: an Airflow 3 token is short-lived and has to be re-mintable")
	}
}

// An Airflow 2 serves no /auth/token, and there the same pair goes on the
// request as basic auth, as it always did.
func TestURLTargetSendsBasicAuthOnAirflow2(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	i := URLInstance(server.URL)
	src, _, err := credentials(context.Background(), i, i.URL, Deps{LookupEnv: urlPair, HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("ada:hunter2"))
	if got := instancestest.Header(t, src); got != want {
		t.Fatalf("header = %q, want %q", got, want)
	}
}

// A mint the instance refuses fails the request and says what was being
// exchanged, rather than quietly sending the pair a JWT-only Airflow rejects.
func TestURLTargetMintRefusalNamesTheExchange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"detail":"Invalid credentials"}`))
	}))
	defer server.Close()

	i := URLInstance(server.URL)
	src, _, err := credentials(context.Background(), i, i.URL, Deps{LookupEnv: urlPair, HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	_, _, err = src(context.Background())
	if err == nil {
		t.Fatal("a refused mint produced a credential")
	}
	for _, want := range []string{EnvUsername, EnvPassword, "/auth/token", server.URL} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message does not name %s: %s", want, err)
		}
	}
}

// localProject writes a project pinning the given Airflow, which is what tells
// the resolver which credentials its local engine provisioned.
func localProject(t *testing.T, airflowVersion string) string {
	t.Helper()
	dir := t.TempDir()
	body := instancestest.PreambleFor(airflowVersion)
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
	src, refresh, err := credentials(context.Background(), i, server.URL, Deps{HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	if got := instancestest.Header(t, src); got != "Bearer minted" {
		t.Fatalf("header = %q, want the minted token", got)
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
	src, _, err := credentials(context.Background(), i, server.URL, Deps{HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	if got := instancestest.Header(t, src); got != "Bearer minted" {
		t.Fatalf("header = %q", got)
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
	username, password, _ := localAccount(airflow2, "standalone", project)
	if username != localUsername || password != "generated" {
		t.Fatalf("account = %s/%s, want %s with the generated password", username, password, localUsername)
	}

	// No file: the password the macOS launch shim seeds.
	username, password, _ = localAccount(airflow2, "standalone", localProject(t, "2.10.5"))
	if username != localUsername || password != localPassword {
		t.Fatalf("account = %s/%s, want the shim's %s/%s", username, password, localUsername, localPassword)
	}

	// Docker mode creates the account itself, so the file a previous
	// standalone run left behind names the wrong password and is ignored.
	username, password, _ = localAccount(airflow2, modeDocker, project)
	if username != localUsername || password != localPassword {
		t.Fatalf("account = %s/%s, want docker mode's %s/%s", username, password, localUsername, localPassword)
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
	username, password, _ := localAccount(airflow2, "standalone", project)
	if username != localUsername || password != "generated" {
		t.Fatalf("account = %s/%s, want the Airflow 2 account the record calls for", username, password)
	}

	// The reverse: a manifest edited down to 2 while an Airflow 3 runs.
	if username, password, _ = localAccount("3", "standalone", localProject(t, "2.10.5")); username != "" || password != "" {
		t.Fatalf("account = %s/%s, want no credentials for a running Airflow 3", username, password)
	}

	// A project deleted while its Airflow keeps running still resolves, because
	// nothing about the credential needed the directory.
	gone := localProject(t, "2.10.5")
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	if username, password, _ = localAccount(airflow2, "standalone", gone); username != localUsername || password != localPassword {
		t.Fatalf("account = %s/%s for a deleted project, want the shim's default", username, password)
	}
}

// TestLocalAccountFallsBackToTheManifest covers a record written before the
// generation was stored: the manifest is the only thing left to ask.
func TestLocalAccountFallsBackToTheManifest(t *testing.T) {
	if username, _, _ := localAccount("", "standalone", localProject(t, "2.10.5")); username != localUsername {
		t.Errorf("an old record for an Airflow 2 project got %q", username)
	}
	if username, _, _ := localAccount("", "standalone", localProject(t, "3.1")); username != "" {
		t.Errorf("an old record for an Airflow 3 project got %q", username)
	}
	// No manifest to fall back to reads as Airflow 3: what the v2 scaffold
	// writes.
	if username, password, err := localAccount("", "standalone", t.TempDir()); err != nil || username != "" || password != "" {
		t.Fatalf("account = %s/%s, %v; want no credentials", username, password, err)
	}
}

// An old record for a project that has not been migrated off [tool.astro]
// airflow still finds its generation: from the requirement beside the line, or
// from the line when it is all the manifest has. Before, the failed load read
// as Airflow 3, and an Airflow 2 got no credentials and answered 401.
func TestLocalAccountReadsAnUnmigratedManifest(t *testing.T) {
	for name, body := range map[string]string{
		// The record predates the requirement being the version, so the key
		// is what started the process, and wins where the two disagree.
		"leftover key that disagrees with the requirement": "[project]\nname = 'demo'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\nairflow = '2.10'\n",
		"leftover key and no requirement":                  "[project]\nname = 'demo'\n\n[tool.astro]\nairflow = '2.10'\n",
		"leftover key and a range":                         "[project]\nname = 'demo'\ndependencies = ['apache-airflow>=2.9']\n\n[tool.astro]\nairflow = '2.10'\n",
		// No key: the requirement.
		"no key": "[project]\nname = 'demo'\ndependencies = ['apache-airflow==2.10.*']\n\n[tool.astro]\n",
		// A problem that has nothing to do with the Airflow version does not
		// stop the question being answered.
		"an unrelated problem": "[project]\nname = 'demo'\ndependencies = ['apache-airflow==2.10.*']\n\n[tool.astro]\nairflw = '2'\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			username, _, err := localAccount("", "standalone", dir)
			if err != nil || username != localUsername {
				t.Errorf("account = %q, %v; want the Airflow 2 account", username, err)
			}
		})
	}
}

// A pyproject.toml that is not an Astro project is the same as none: the
// default, Airflow 3, as before the requirement became the version.
func TestLocalAccountReadsNoAstroSectionAsTheDefault(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = 'plain'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if username, password, err := localAccount("", "standalone", dir); err != nil || username != "" || password != "" {
		t.Errorf("account = %q/%q, %v; want no credentials and no error", username, password, err)
	}
}

// A manifest that is there and says no generation is an error, not Airflow 3.
func TestLocalAccountRefusesToGuessTheGeneration(t *testing.T) {
	for name, body := range map[string]string{
		"not TOML":                    "[project\n",
		"a range and no leftover key": "[project]\nname = 'demo'\ndependencies = ['apache-airflow>=2.9']\n\n[tool.astro]\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := localAccount("", "standalone", dir); err == nil {
				t.Error("want an error naming the manifest, got a guess")
			}
		})
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
	src, _, err := credentials(context.Background(), i, server.URL, Deps{HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte(localUsername+":"+localPassword))
	if got := instancestest.Header(t, src); got != want {
		t.Fatalf("header = %q, want %q", got, want)
	}
}
