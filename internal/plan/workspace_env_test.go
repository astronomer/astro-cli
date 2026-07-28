package plan

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/mock"

	astrov1 "github.com/astronomer/astro-cli/astro-client-v1"
	astrov1_mocks "github.com/astronomer/astro-cli/astro-client-v1/mocks"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/localrt"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// workspaceManifest declares one required env var with a workspace source and a
// top-level workspace for it to resolve against.
const workspaceManifest = `[project]
name = 'demo'
requires-python = '>=3.10'

[tool.astro]
airflow = '3.1'
workspace = 'cmws'

[tool.astro.env.vars.DATA_WAREHOUSE_URI]
required = true
source = 'workspace'
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

	built, err := Build(dir, Options{AstroV1Client: mc})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := built.Plan.Env["DATA_WAREHOUSE_URI"]; got != "postgres://cloud" {
		t.Fatalf("injected env = %q, want the Environment Manager value", got)
	}
}

// Logged out, the workspace value cannot be fetched: start is gated with a
// message that names the cause and the fix.
func TestBuildLoggedOutGatesWithCause(t *testing.T) {
	testUtil.InitTestConfig(testUtil.Initial) // no cloud context
	dir := newWorkspaceProject(t, workspaceManifest)

	mc := new(astrov1_mocks.ClientWithResponsesInterface) // never called
	_, err := Build(dir, Options{AstroV1Client: mc})

	var missing *MissingEnvError
	if !errors.As(err, &missing) {
		t.Fatalf("err = %v, want MissingEnvError", err)
	}
	if !testUtil.StringContains([]string{"DATA_WAREHOUSE_URI", "log in with 'astro login'", "astro local env set"}, missing.Error()) {
		t.Fatalf("message missing the cause or fix:\n%s", missing.Error())
	}
	mc.AssertNotCalled(t, "ListEnvironmentObjectsWithResponse")
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

[tool.astro]
airflow = '3.1'

[tool.astro.env.vars.DATA_WAREHOUSE_URI]
required = true
source = 'workspace'
`
	dir := newWorkspaceProject(t, body)

	mc := new(astrov1_mocks.ClientWithResponsesInterface) // never called
	_, err := Build(dir, Options{AstroV1Client: mc})

	var missing *MissingEnvError
	if !errors.As(err, &missing) {
		t.Fatalf("err = %v, want MissingEnvError", err)
	}
	if !testUtil.StringContains([]string{"DATA_WAREHOUSE_URI", "sets no `workspace`"}, missing.Error()) {
		t.Fatalf("message should name the missing workspace:\n%s", missing.Error())
	}
	mc.AssertNotCalled(t, "ListEnvironmentObjectsWithResponse")
}

// Docker start would write a resolved value into the on-disk compose file, so
// Environment Manager values are withheld in docker mode: a required workspace
// value gates the start with the standalone-or-set-locally message, and the
// API is never read.
func TestBuildDockerWithholdsWorkspaceValue(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	dir := newWorkspaceProject(t, workspaceManifest)

	mc := new(astrov1_mocks.ClientWithResponsesInterface) // never called
	_, err := Build(dir, Options{Mode: localrt.ModeDocker, AstroV1Client: mc})

	var missing *MissingEnvError
	if !errors.As(err, &missing) {
		t.Fatalf("err = %v, want MissingEnvError", err)
	}
	if !testUtil.StringContains([]string{"DATA_WAREHOUSE_URI", "standalone mode only", "--docker"}, missing.Error()) {
		t.Fatalf("message missing the docker-mode cause:\n%s", missing.Error())
	}
	mc.AssertNotCalled(t, "ListEnvironmentObjectsWithResponse")
}
