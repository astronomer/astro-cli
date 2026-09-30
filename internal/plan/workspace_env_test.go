package plan

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/internal/emenv"
	"github.com/astronomer/astro-cli/internal/envresolve"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/localrt"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// workspaceManifest declares one required env var with a workspace source and a
// top-level workspace for it to resolve against.
const workspaceManifest = `[project]
name = 'demo'
requires-python = '>=3.10'
dependencies = ['apache-airflow==3.1.*']

[tool.astro]
workspace = 'cmws'
domain = 'localhost'

[tool.astro.env]
DATA_WAREHOUSE_URI = { source = 'workspace' }
`

func newWorkspaceProject(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, project.Marker), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	return dir
}

// emProvider wires a mock client into the constructor Options takes, so these
// tests still exercise the real emenv provider — its fetch, its scoping, and
// the messages asserted below — rather than a stub that would only prove plan
// calls something.
func emProvider(mc astrov1.APIClient) func(string, string, bool) envresolve.Provider {
	return func(workspace, domain string, reveal bool) envresolve.Provider {
		return emenv.NewProvider(workspace, domain, func(emenv.Login) astrov1.APIClient { return mc }, reveal)
	}
}

func okListResp(objs ...astrov1.EnvironmentObject) *astrov1.ListEnvironmentObjectsResponse {
	return &astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: objs, TotalCount: len(objs)},
	}
}

// A logged-in user's workspace value is fetched and layered into the Airflow
// environment the plan starts from.
func TestBuildInjectsWorkspaceValue(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	dir := newWorkspaceProject(t, workspaceManifest)

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).
		Return(okListResp(astrov1.EnvironmentObject{
			ObjectKey:           "DATA_WAREHOUSE_URI",
			ObjectType:          astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE,
			EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: "postgres://cloud"},
		}), nil)

	built, err := Build(dir, Options{WorkspaceProvider: emProvider(mc)})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := built.Plan.SecretEnv["DATA_WAREHOUSE_URI"]; got != "postgres://cloud" {
		t.Fatalf("injected secret env = %q, want the Environment Manager value", got)
	}
	if _, onDisk := built.Plan.Env["DATA_WAREHOUSE_URI"]; onDisk {
		t.Fatal("an Environment Manager value traveled in Env, which docker mode writes to disk")
	}
}

// Logged out, the workspace value cannot be fetched: start is gated with a
// message that names the cause and the fix.
func TestBuildLoggedOutGatesWithCause(t *testing.T) {
	testUtil.InitTestConfig(testUtil.Initial) // no cloud context
	dir := newWorkspaceProject(t, workspaceManifest)

	mc := new(astrov1_mocks.ClientWithResponsesInterface) // never called
	_, err := Build(dir, Options{WorkspaceProvider: emProvider(mc)})

	var missing *MissingEnvError
	if !errors.As(err, &missing) {
		t.Fatalf("err = %v, want MissingEnvError", err)
	}
	if !testUtil.StringContains([]string{"DATA_WAREHOUSE_URI", "not logged in to localhost", "Log in with `astro login localhost`", "astro local env variable set", "--allow-missing"}, missing.Error()) {
		t.Fatalf("message missing the cause, the fix, or the way past it:\n%s", missing.Error())
	}
	mc.AssertNotCalled(t, "ListEnvironmentObjectsWithResponse")
}

// --allow-missing starts past the same gap: the plan builds, the value is not
// invented, and the caller gets the missing name and its cause to warn with.
func TestBuildAllowMissingStartsWithoutTheValue(t *testing.T) {
	testUtil.InitTestConfig(testUtil.Initial) // no cloud context
	dir := newWorkspaceProject(t, workspaceManifest)

	mc := new(astrov1_mocks.ClientWithResponsesInterface) // never called
	built, err := Build(dir, Options{WorkspaceProvider: emProvider(mc), AllowMissing: true})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(built.StartedWithout) != 1 || built.StartedWithout[0].Name != "DATA_WAREHOUSE_URI" {
		t.Fatalf("StartedWithout = %+v, want DATA_WAREHOUSE_URI", built.StartedWithout)
	}
	if !strings.Contains(built.StartedWithout[0].SourceNote, "not logged in to localhost") {
		t.Errorf("cause = %q, want the not-logged-in cause", built.StartedWithout[0].SourceNote)
	}
	if _, ok := built.Plan.Env["DATA_WAREHOUSE_URI"]; ok {
		t.Error("a value was invented for a missing name")
	}
	if _, ok := built.Plan.SecretEnv["DATA_WAREHOUSE_URI"]; ok {
		t.Error("a value was invented for a missing name")
	}
}

// Without a client wired (the offline default), a workspace source resolves
// from nowhere and a required one gates as missing — never silently resolved.
func TestBuildNoClientGatesWorkspaceSource(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	dir := newWorkspaceProject(t, workspaceManifest)

	_, err := Build(dir, Options{})
	var missing *MissingEnvError
	if !errors.As(err, &missing) {
		t.Fatalf("err = %v, want MissingEnvError", err)
	}
}

// A workspace source with no top-level workspace in the manifest gates with a
// message naming the fix, and the API is never read.
func TestBuildNoWorkspaceInManifest(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	body := `[project]
name = 'demo'
requires-python = '>=3.10'
dependencies = ['apache-airflow==3.1.*']

[tool.astro]

[tool.astro.env]
DATA_WAREHOUSE_URI = { source = 'workspace' }
`
	dir := newWorkspaceProject(t, body)

	mc := new(astrov1_mocks.ClientWithResponsesInterface) // never called
	_, err := Build(dir, Options{WorkspaceProvider: emProvider(mc)})

	var missing *MissingEnvError
	if !errors.As(err, &missing) {
		t.Fatalf("err = %v, want MissingEnvError", err)
	}
	if !testUtil.StringContains([]string{"DATA_WAREHOUSE_URI", "sets no `workspace`"}, missing.Error()) {
		t.Fatalf("message should name the missing workspace:\n%s", missing.Error())
	}
	mc.AssertNotCalled(t, "ListEnvironmentObjectsWithResponse")
}

// Docker start writes Plan.Env into the compose file it leaves on disk, so an
// Environment Manager value travels as SecretEnv, which the compose process
// receives without the file holding it. Docker mode resolves the value exactly
// as standalone does rather than withholding it.
func TestBuildDockerCarriesWorkspaceValueOffDisk(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	dir := newWorkspaceProject(t, workspaceManifest)

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).
		Return(okListResp(astrov1.EnvironmentObject{
			ObjectKey:           "DATA_WAREHOUSE_URI",
			ObjectType:          astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE,
			EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: "postgres://cloud"},
		}), nil)

	built, err := Build(dir, Options{Mode: localrt.ModeDocker, WorkspaceProvider: emProvider(mc)})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := built.Plan.SecretEnv["DATA_WAREHOUSE_URI"]; got != "postgres://cloud" {
		t.Fatalf("secret env = %q, want the Environment Manager value", got)
	}
	if _, onDisk := built.Plan.Env["DATA_WAREHOUSE_URI"]; onDisk {
		t.Fatal("an Environment Manager value traveled in Env, which docker mode writes into the compose file")
	}
}

// A local file still wins over the workspace, and the file's value is the one
// that travels: the workspace value must not ride along in SecretEnv, where
// the engine could apply it over the file's.
func TestBuildLocalFileBeatsWorkspaceValue(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	dir := newWorkspaceProject(t, workspaceManifest)
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("DATA_WAREHOUSE_URI=postgres://local\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// The workspace holds the name too. It is read anyway, since the tier
	// supplies undeclared names, but the file still wins.
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).
		Return(okListResp(astrov1.EnvironmentObject{
			ObjectKey:           "DATA_WAREHOUSE_URI",
			ObjectType:          astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE,
			EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: "postgres://cloud"},
		}), nil)
	built, err := Build(dir, Options{Mode: localrt.ModeDocker, WorkspaceProvider: emProvider(mc)})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := built.Plan.Env["DATA_WAREHOUSE_URI"]; got != "postgres://local" {
		t.Fatalf("env = %q, want the project .env value", got)
	}
	if _, ok := built.Plan.SecretEnv["DATA_WAREHOUSE_URI"]; ok {
		t.Fatal("the losing workspace value traveled beside the file's")
	}
}
