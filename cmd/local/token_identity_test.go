package local

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/astronomer/astro-cli/internal/astrosession"
	"github.com/astronomer/astro-cli/internal/emenv"
	"github.com/astronomer/astro-cli/internal/instancelocate"
	"github.com/astronomer/astro-cli/pkg/instances"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// A core-tree run (astro af, astro local) has no root pre-run to apply
// ASTRO_API_TOKEN, so what it sends and which organization and Workspace it
// asks about is decided by internal/astrosession alone. These tests drive the
// production wiring (astrosession.BearerFor, instancelocate.New, emenv.Clients)
// against one fake API that stands in for both the control plane and the
// Airflow, with a saved login on the machine that has its own token and
// organization, and assert that no request under a token carries either.

const (
	// The saved login testUtil.InitTestConfig(LocalPlatform) writes.
	savedToken = "token"
	savedOrg   = "test-org-id"

	tokenOrg  = "org-from-token"
	tokenWS   = "ws-from-token"
	tokenDep  = "clm2xk9dq000108l7a2b3c4d9"
	etlDepID  = "clm2xk9dq000108l7a2b3c4e1"
	linkedWS  = "ws_abc123" // instancestest.Preamble's [tool.astro] workspace
	etlDepURL = "/api/v2"
)

// tokenKind is one kind of ASTRO_API_TOKEN, as its claims make it.
type tokenKind struct {
	name        string
	permissions []string
}

var (
	orgToken = tokenKind{"organization", []string{"organizationId:" + tokenOrg}}
	wsToken  = tokenKind{"workspace", []string{"organizationId:" + tokenOrg, "workspaceId:" + tokenWS}}
	depToken = tokenKind{"deployment", []string{"organizationId:" + tokenOrg, "workspaceId:" + tokenWS, "deploymentId:" + etlDepID}}
)

// fakeAstro is the control plane and an Airflow in one server.
type fakeAstro struct {
	*airflowStub
}

// newFakeAstro serves, under the token's organization only (so a request made
// under the saved organization is a 404 that shows in the record): the
// Deployment "etl" in linkedWS and in tokenWS, listed by workspace and read by
// id, the Airflow it points at, and one Environment Manager variable.
func newFakeAstro(t *testing.T) fakeAstro {
	t.Helper()
	stub := newAirflowStub(t)
	deployment := func(ws string) map[string]any {
		return map[string]any{"id": etlDepID, "name": "etl", "workspaceId": ws, "webServerAirflowApiUrl": stub.URL}
	}
	stub.routeGetFunc("/v1/organizations/"+tokenOrg+"/deployments", func(q url.Values) string {
		var out []map[string]any
		for _, ws := range q["workspaceIds"] {
			out = append(out, deployment(ws))
		}
		body, err := json.Marshal(map[string]any{"deployments": out, "totalCount": len(out), "limit": 1000, "offset": 0})
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	})
	body, err := json.Marshal(deployment(tokenWS))
	if err != nil {
		t.Fatal(err)
	}
	stub.route(http.MethodGet, "/v1/organizations/"+tokenOrg+"/deployments/"+etlDepID, string(body))
	stub.routeGetFunc("/v1/organizations/"+tokenOrg+"/environment-objects", func(q url.Values) string {
		var objects []map[string]any
		if q.Get("objectType") == "ENVIRONMENT_VARIABLE" {
			objects = append(objects, map[string]any{
				"objectKey": "DATA_WAREHOUSE_URI", "objectType": "ENVIRONMENT_VARIABLE",
				"environmentVariable": map[string]any{"value": "postgres://cloud", "isSecret": false},
			})
		}
		out, err := json.Marshal(map[string]any{"environmentObjects": objects, "totalCount": len(objects), "limit": 1000, "offset": 0})
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	})
	stub.route(http.MethodGet, "/api/v2/pools", `{"pools":[],"total_entries":0}`)
	return fakeAstro{stub}
}

// useFakeAstro points the CLI at f for host localhost, with the saved login a
// developer machine has.
func useFakeAstro(t *testing.T, f fakeAstro) {
	t.Helper()
	// Nothing here may reach the machine's keychain through the vault.
	keyring.MockInit()
	t.Cleanup(keyring.MockInit)
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	testUtil.UseLocalCoreURL(t, f.URL)
	t.Setenv("ASTRO_DOMAIN", "localhost")
}

func useToken(t *testing.T, kind tokenKind) string {
	t.Helper()
	token := testUtil.MintAPIToken(t, time.Now().Add(time.Hour), kind.permissions...)
	t.Setenv(astrosession.EnvAPIToken, token)
	return token
}

// productionDeps are the seams the binary wires: the real session, the real
// locator and the real Environment Manager client.
func productionDeps(t *testing.T, dir string) Deps {
	t.Helper()
	d, _, _ := queryDeps(t)
	d.WorkingDir = func() (string, error) { return dir, nil }
	d.Session = astrosession.BearerFor
	d.Locator = instancelocate.New
	d.WorkspaceClients = emenv.Clients
	return d
}

// noSavedLoginSent fails the test when any request carries the saved login's
// token, or names its organization.
func noSavedLoginSent(t *testing.T, f fakeAstro) {
	t.Helper()
	for _, req := range f.requests() {
		if auth := req.Header.Get("Authorization"); strings.Contains(auth, savedToken) && !strings.Contains(auth, ".") {
			t.Errorf("%s %s carried the saved login's token: %q", req.Method, req.Path, auth)
		}
		if strings.Contains(req.Path, savedOrg) || strings.Contains(req.Query, savedOrg) {
			t.Errorf("%s %s asked about the saved login's organization", req.Method, req.Path)
		}
	}
}

// everyRequestUnder asserts every request went out under token, and that the
// ones to the control plane named org.
func everyRequestUnder(t *testing.T, f fakeAstro, token string) {
	t.Helper()
	reqs := f.requests()
	if len(reqs) == 0 {
		t.Fatal("no request was made")
	}
	for _, req := range reqs {
		if got := req.Header.Get("Authorization"); got != "Bearer "+token {
			t.Errorf("%s %s sent %q, want the token from ASTRO_API_TOKEN", req.Method, req.Path, got)
		}
		if strings.HasPrefix(req.Path, "/v1/organizations/") && !strings.HasPrefix(req.Path, "/v1/organizations/"+tokenOrg+"/") {
			t.Errorf("%s %s is not in the token's organization", req.Method, req.Path)
		}
	}
	noSavedLoginSent(t, f)
}

func linkedProject(t *testing.T) string {
	t.Helper()
	return instanceProject(t, "")
}

// unlinkedProject links no Workspace, so a Deployment name has only the run's
// own selection to be looked up in.
func unlinkedProject(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "orders")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "[project]\nname = 'demo'\nrequires-python = '>=3.10'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\n"
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("ASTRO_HOME", t.TempDir())
	t.Setenv(instances.EnvVar, "")
	return dir
}

// routeGetFunc answers GET path with a body built from the request's query.
func (s *airflowStub) routeGetFunc(path string, respond func(url.Values) string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routes[http.MethodGet+" "+path] = stubRoute{status: http.StatusOK, respond: respond}
}

// requestsToPath is every GET of path, in order.
func (s *airflowStub) requestsToPath(path string) []stubRequest {
	var out []stubRequest
	for _, req := range s.requests() {
		if req.Method == http.MethodGet && req.Path == path {
			out = append(out, req)
		}
	}
	return out
}

func listedWorkspaces(f fakeAstro) []string {
	var out []string
	for _, req := range f.requestsToPath("/v1/organizations/" + tokenOrg + "/deployments") {
		q, _ := url.ParseQuery(req.Query)
		out = append(out, q["workspaceIds"]...)
	}
	return out
}

// af -d <name> looks the name up in the token's organization, and in the
// project's workspace when it links one, whatever the saved login holds.
func TestAFDeploymentNameUnderATokenUsesOnlyTheToken(t *testing.T) {
	for _, kind := range []tokenKind{orgToken, wsToken, depToken} {
		t.Run(kind.name, func(t *testing.T) {
			f := newFakeAstro(t)
			useFakeAstro(t, f)
			token := useToken(t, kind)
			d := productionDeps(t, linkedProject(t))
			if err := execute(t, d, "af", "pools", "list", "-d", "etl"); err != nil {
				t.Fatal(err)
			}
			everyRequestUnder(t, f, token)
			if got := listedWorkspaces(f); len(got) != 1 || got[0] != linkedWS {
				t.Errorf("listed workspaces %v, want the project's %s", got, linkedWS)
			}
			if !f.sawRequest(http.MethodGet, "/api/v2/pools") {
				t.Error("the Deployment's Airflow was never reached")
			}
		})
	}
}

// With no project link, a Workspace token resolves its own Workspace for the
// name, and so does a Deployment token, which carries one.
func TestAFDeploymentNameUnderAWorkspaceTokenResolvesItsOwnWorkspace(t *testing.T) {
	for _, kind := range []tokenKind{wsToken, depToken} {
		t.Run(kind.name, func(t *testing.T) {
			f := newFakeAstro(t)
			useFakeAstro(t, f)
			token := useToken(t, kind)
			d := productionDeps(t, unlinkedProject(t))
			if err := execute(t, d, "af", "pools", "list", "-d", "etl"); err != nil {
				t.Fatal(err)
			}
			everyRequestUnder(t, f, token)
			if got := listedWorkspaces(f); len(got) != 1 || got[0] != tokenWS {
				t.Errorf("listed workspaces %v, want the token's %s", got, tokenWS)
			}
		})
	}
}

// An Organization token has no Workspace, and the saved login's is not its
// own: the name cannot be looked up, and the advice is the resolver's for a
// Deployment name, not the saved login's "astro workspace switch".
func TestAFDeploymentNameUnderAnOrganizationTokenWithNoWorkspaceSaysSo(t *testing.T) {
	f := newFakeAstro(t)
	useFakeAstro(t, f)
	useToken(t, orgToken)
	d := productionDeps(t, unlinkedProject(t))
	err := execute(t, d, "af", "pools", "list", "-d", "etl")
	if err == nil {
		t.Fatal("a Deployment name with no Workspace to look in succeeded")
	}
	msg := err.Error()
	for _, want := range []string{`"etl" is not a Deployment id`, "the API token in ASTRO_API_TOKEN selects none", "so pass -d with the Deployment id, or run in a project that links its workspace"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q lacks %q", msg, want)
		}
	}
	if strings.Contains(msg, "workspace switch") {
		t.Errorf("error %q advises a switch no token run can make", msg)
	}
	if len(listedWorkspaces(f)) != 0 {
		t.Errorf("a listing was made with no Workspace: %v", listedWorkspaces(f))
	}
	noSavedLoginSent(t, f)
}

// af -d <id> reads the Deployment in the token's organization, not the saved
// login's.
func TestAFDeploymentIDUnderATokenUsesOnlyTheToken(t *testing.T) {
	for _, kind := range []tokenKind{orgToken, wsToken, depToken} {
		t.Run(kind.name, func(t *testing.T) {
			f := newFakeAstro(t)
			useFakeAstro(t, f)
			token := useToken(t, kind)
			d := productionDeps(t, linkedProject(t))
			if err := execute(t, d, "af", "pools", "list", "-d", etlDepID); err != nil {
				t.Fatal(err)
			}
			everyRequestUnder(t, f, token)
			if !f.sawRequest(http.MethodGet, "/v1/organizations/"+tokenOrg+"/deployments/"+etlDepID) {
				t.Errorf("the Deployment was not read by id in the token's organization: %v", f.requests())
			}
		})
	}
}

// local env list reads the project's workspace from Environment Manager with
// the token, in the token's organization.
func TestLocalEnvListWorkspaceSourceUnderATokenUsesOnlyTheToken(t *testing.T) {
	for _, kind := range []tokenKind{orgToken, wsToken, depToken} {
		t.Run(kind.name, func(t *testing.T) {
			f := newFakeAstro(t)
			useFakeAstro(t, f)
			token := useToken(t, kind)
			d := productionDeps(t, workspaceEnvProject(t))
			stdout := d.Stdout.(interface{ String() string })
			if err := execute(t, d, "local", "env", "list", "--all", "--output", "json"); err != nil {
				t.Fatal(err)
			}
			everyRequestUnder(t, f, token)
			if !strings.Contains(stdout.String(), "workspace (cmws)") {
				t.Errorf("the name did not resolve from the workspace:\n%s", stdout.String())
			}
			for _, req := range f.requests() {
				if !strings.Contains(req.Query, "workspaceId=cmws") {
					t.Errorf("%s %s?%s did not ask for the project's workspace", req.Method, req.Path, req.Query)
				}
			}
		})
	}
}

// noWorkspaceEnvProject is workspaceEnvProject's manifest with no `workspace`
// key, so the only Workspace a read could use is the token's own.
func noWorkspaceEnvProject(t *testing.T) string {
	t.Helper()
	dir := workspaceEnvProject(t)
	body := strings.Replace(workspaceEnvManifest, "workspace = 'cmws'\ndomain = 'localhost'\n", "", 1)
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A manifest that links no Workspace has the token's own under a Workspace or
// Deployment token, and the token's alone: never the saved login's.
func TestLocalEnvListTakesTheTokensWorkspaceWhenTheManifestNamesNone(t *testing.T) {
	for _, kind := range []tokenKind{wsToken, depToken} {
		t.Run(kind.name, func(t *testing.T) {
			f := newFakeAstro(t)
			useFakeAstro(t, f)
			token := useToken(t, kind)
			d := productionDeps(t, noWorkspaceEnvProject(t))
			stdout := d.Stdout.(interface{ String() string })
			if err := execute(t, d, "local", "env", "list", "--all", "--output", "json"); err != nil {
				t.Fatal(err)
			}
			everyRequestUnder(t, f, token)
			if !strings.Contains(stdout.String(), "workspace ("+tokenWS+")") {
				t.Errorf("the name did not resolve from the token's workspace:\n%s", stdout.String())
			}
			for _, req := range f.requests() {
				if !strings.Contains(req.Query, "workspaceId="+tokenWS) {
					t.Errorf("%s %s?%s did not ask for the token's workspace", req.Method, req.Path, req.Query)
				}
			}
		})
	}
}

// An Organization token names no Workspace, so with none in the manifest the
// read is refused naming the manifest key, and nothing is sent, under the
// token or the saved login.
func TestLocalEnvListUnderAnOrganizationTokenWithNoManifestWorkspaceReadsNothing(t *testing.T) {
	f := newFakeAstro(t)
	useFakeAstro(t, f)
	useToken(t, orgToken)
	d := productionDeps(t, noWorkspaceEnvProject(t))
	stdout := d.Stdout.(interface{ String() string })
	if err := execute(t, d, "local", "env", "list", "--all", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	if reqs := f.requests(); len(reqs) != 0 {
		t.Errorf("%d requests were made, want none: %+v", len(reqs), reqs)
	}
	if !strings.Contains(stdout.String(), "no workspace key") {
		t.Errorf("the listing does not name the missing manifest key:\n%s", stdout.String())
	}
}

// local env get resolves the same way: the token's Workspace when the
// manifest names none.
func TestLocalEnvGetTakesTheTokensWorkspaceWhenTheManifestNamesNone(t *testing.T) {
	f := newFakeAstro(t)
	useFakeAstro(t, f)
	token := useToken(t, wsToken)
	d := productionDeps(t, noWorkspaceEnvProject(t))
	stdout := d.Stdout.(interface{ String() string })
	if err := execute(t, d, "local", "env", "variable", "get", "DATA_WAREHOUSE_URI"); err != nil {
		t.Fatal(err)
	}
	everyRequestUnder(t, f, token)
	if !strings.Contains(stdout.String(), "postgres://cloud") {
		t.Errorf("get did not resolve from the token's workspace:\n%s", stdout.String())
	}
}

// Without a token nothing changes: the saved login's token and organization
// are what go out.
func TestWithoutATokenTheSavedLoginIsStillUsed(t *testing.T) {
	f := newFakeAstro(t)
	useFakeAstro(t, f)
	// The fake serves the token's organization only; serve the saved one too.
	f.routeGetFunc("/v1/organizations/"+savedOrg+"/environment-objects", func(q url.Values) string {
		return `{"environmentObjects":[],"totalCount":0,"limit":1000,"offset":0}`
	})
	f.route(http.MethodGet, "/v1/organizations/"+savedOrg+"/deployments/"+etlDepID, fmt.Sprintf(`{"id":%q,"name":"etl","workspaceId":%q,"webServerAirflowApiUrl":%q}`, etlDepID, linkedWS, f.URL))

	d := productionDeps(t, linkedProject(t))
	if err := execute(t, d, "af", "pools", "list", "-d", etlDepID); err != nil {
		t.Fatal(err)
	}
	var sawOrg bool
	for _, req := range f.requests() {
		if got := req.Header.Get("Authorization"); got != "Bearer "+savedToken {
			t.Errorf("%s %s sent %q, want the saved login's token", req.Method, req.Path, got)
		}
		sawOrg = sawOrg || strings.HasPrefix(req.Path, "/v1/organizations/"+savedOrg+"/")
	}
	if !sawOrg {
		t.Errorf("the saved login's organization was never asked: %v", f.requests())
	}

	d = productionDeps(t, workspaceEnvProject(t))
	if err := execute(t, d, "local", "env", "list", "--all", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	last := f.requests()[len(f.requests())-1]
	if last.Path != "/v1/organizations/"+savedOrg+"/environment-objects" || !strings.Contains(last.Header.Get("Authorization"), savedToken) {
		t.Errorf("env list went out as %s with %q, want the saved login's", last.Path, last.Header.Get("Authorization"))
	}
}

// A token that is expired, malformed or has no permissions is the run's error,
// worded as the root's setup words it, and the saved login is not used instead.
func TestAnUnusableTokenFailsTheCoreTreeAndFallsBackToNothing(t *testing.T) {
	cases := []struct {
		name  string
		token func(*testing.T) string
		want  error
		text  string
	}{
		{"expired", func(t *testing.T) string {
			return testUtil.MintAPIToken(t, time.Now().Add(-time.Hour), "organizationId:"+tokenOrg)
		}, astrosession.ErrAPITokenExpired, "API Token is expired"},
		{"malformed", func(*testing.T) string { return "not-a-token" }, astrosession.ErrAPITokenMalformed, "API Token is invalid or malformed"},
		{"no permissions", func(t *testing.T) string { return testUtil.MintAPIToken(t, time.Now().Add(time.Hour)) }, astrosession.ErrNotAPIToken, "ASTRO_API_TOKEN is not an Astro API token (it has no permissions): set a valid token, or unset it to use your login"},
	}
	for _, tc := range cases {
		for _, arg := range []string{"etl", etlDepID} {
			t.Run(tc.name+"/"+arg, func(t *testing.T) {
				f := newFakeAstro(t)
				useFakeAstro(t, f)
				t.Setenv(astrosession.EnvAPIToken, tc.token(t))
				d := productionDeps(t, linkedProject(t))
				err := execute(t, d, "af", "pools", "list", "-d", arg)
				if !errors.Is(err, tc.want) || !errors.Is(err, astrosession.ErrInvalidToken) {
					t.Fatalf("err = %v, want %v", err, tc.want)
				}
				if err.Error() != tc.text {
					t.Errorf("message = %q, want %q", err.Error(), tc.text)
				}
				if reqs := f.requests(); len(reqs) != 0 {
					t.Errorf("a request was made with an unusable token: %v", reqs)
				}
			})
		}
		t.Run(tc.name+"/local env list", func(t *testing.T) {
			f := newFakeAstro(t)
			useFakeAstro(t, f)
			t.Setenv(astrosession.EnvAPIToken, tc.token(t))
			keyring.MockInit()
			d := productionDeps(t, workspaceEnvProject(t))
			if err := execute(t, d, "local", "env", "list", "--all", "--output", "json"); err != nil {
				t.Fatal(err)
			}
			if out := d.Stdout.(interface{ String() string }).String(); !strings.Contains(out, "workspace (unavailable: unusable ASTRO_API_TOKEN)") {
				t.Errorf("the listing does not say the token is unusable:\n%s", out)
			}
			if reqs := f.requests(); len(reqs) != 0 {
				t.Errorf("a request was made with an unusable token: %v", reqs)
			}
		})
	}
}
