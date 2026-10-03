package local

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/zalando/go-keyring"

	"github.com/astronomer/astro-cli/internal/emenv"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// workspaceClients answers every login with one fake client.
func workspaceClients(c astrov1.APIClient) emenv.ClientFactory {
	return func(emenv.Login) astrov1.APIClient { return c }
}

const workspaceEnvManifest = `[project]
name = 'demo'
requires-python = '>=3.10'
dependencies = ['apache-airflow==3.1.*']

[tool.astro]
workspace = 'cmws'
domain = 'localhost'

[tool.astro.env]
DATA_WAREHOUSE_URI = { source = 'workspace' }
`

func workspaceEnvProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(workspaceEnvManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ASTRO_HOME", t.TempDir())
	// A set also clears the vault's copy of the name; keep that out of the
	// developer's own vault. See envProject.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	keyring.MockInit()
	t.Cleanup(keyring.MockInit)
	return dir
}

func warehouseClient() *astrov1_mocks.ClientWithResponsesInterface {
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).
		Return(&astrov1.ListEnvironmentObjectsResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &astrov1.EnvironmentObjectsPaginated{
				EnvironmentObjects: []astrov1.EnvironmentObject{{
					ObjectKey:           "DATA_WAREHOUSE_URI",
					ObjectType:          astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE,
					EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: "postgres://cloud"},
				}},
				TotalCount: 1,
			},
		}, nil)
	return mc
}

// `env list` labels a workspace-source name with the workspace source,
// value-free.
func TestEnvListShowsWorkspaceSource(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	dir := workspaceEnvProject(t)
	d, out, _ := envDeps(t, dir, "")
	d.WorkspaceClients = workspaceClients(warehouseClient())

	if err := execute(t, d, "local", "env", "list", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	var item struct {
		Name, Source string
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &item); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if item.Name != "DATA_WAREHOUSE_URI" || item.Source != "workspace (cmws)" {
		t.Fatalf("list item = %+v, want workspace source", item)
	}
	if strings.Contains(out.String(), "postgres://cloud") {
		t.Fatal("list leaked the value")
	}
}

// `env get` reveals the workspace value through the chain.
func TestEnvGetFromWorkspace(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	dir := workspaceEnvProject(t)
	d, out, _ := envDeps(t, dir, "")
	d.WorkspaceClients = workspaceClients(warehouseClient())

	if err := execute(t, d, "local", "env", "variable", "get", "DATA_WAREHOUSE_URI", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Source, Value string
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if got.Source != "workspace (cmws)" || got.Value != "postgres://cloud" {
		t.Fatalf("get json = %+v, want workspace/postgres://cloud", got)
	}
}

// `env list` and `env get` read the workspace under the manifest's
// organization, not the login's.
func TestEnvListAndGetReadUnderTheManifestsOrganization(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	dir := workspaceEnvProject(t)
	body := strings.Replace(workspaceEnvManifest, "domain = 'localhost'\n", "domain = 'localhost'\norganization = 'clother'\n", 1)
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"local", "env", "list", "--output", "json"},
		{"local", "env", "variable", "get", "DATA_WAREHOUSE_URI", "--output", "json"},
	} {
		mc := warehouseClient()
		d, out, _ := envDeps(t, dir, "")
		d.WorkspaceClients = workspaceClients(mc)
		if err := execute(t, d, args...); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "workspace (cmws)") {
			t.Fatalf("%v did not resolve from the workspace:\n%s", args, out.String())
		}
		for _, call := range mc.Calls {
			if org := call.Arguments.String(1); org != "clother" {
				t.Fatalf("%v read under organization %q, want the manifest's clother", args, org)
			}
		}
		if len(mc.Calls) == 0 {
			t.Fatalf("%v never read the workspace", args)
		}
	}
}

// A local set overrides the workspace source; get then reports the project
// source.
func TestEnvLocalSetOverridesWorkspace(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	dir := workspaceEnvProject(t)

	d, _, _ := envDeps(t, dir, "postgres://local\n")
	d.WorkspaceClients = workspaceClients(warehouseClient())
	if err := execute(t, d, "local", "env", "variable", "set", "DATA_WAREHOUSE_URI"); err != nil {
		t.Fatal(err)
	}

	d, out, _ := envDeps(t, dir, "")
	d.WorkspaceClients = workspaceClients(warehouseClient())
	if err := execute(t, d, "local", "env", "variable", "get", "DATA_WAREHOUSE_URI", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Source, Value string
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if got.Source != "project" || got.Value != "postgres://local" {
		t.Fatalf("get json = %+v, want the local value to win", got)
	}
}

// Logged out, list shows the source as unavailable and never fails.
func TestEnvListLoggedOutUnavailable(t *testing.T) {
	testUtil.InitTestConfig(testUtil.Initial) // no cloud context
	dir := workspaceEnvProject(t)
	d, out, _ := envDeps(t, dir, "")
	d.WorkspaceClients = workspaceClients(warehouseClient()) // present but unused when logged out

	if err := execute(t, d, "local", "env", "list", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	var item struct {
		Name, Source string
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &item); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if !strings.HasPrefix(item.Source, "workspace (unavailable:") {
		t.Fatalf("source = %q, want an unavailable workspace label", item.Source)
	}
}
