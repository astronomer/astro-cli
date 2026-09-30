package local

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"

	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// tierClient is a workspace holding one undeclared object of each kind, whose
// values carry SECRET so a test can tell if one reaches output. It fails every
// read that asks for secret values, so a listing that asked would fail.
func tierClient(t *testing.T) *astrov1_mocks.ClientWithResponsesInterface {
	t.Helper()
	pw, host := "pw-SECRET", "db.cloud"
	rows := map[astrov1.ListEnvironmentObjectsParamsObjectType][]astrov1.EnvironmentObject{
		astrov1.ENVIRONMENTVARIABLE: {{
			ObjectKey: "TEAM_TOKEN", ObjectType: astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE,
			EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: "tok-SECRET", IsSecret: true},
		}},
		astrov1.AIRFLOWVARIABLE: {{
			ObjectKey: "threshold", ObjectType: astrov1.EnvironmentObjectObjectTypeAIRFLOWVARIABLE,
			AirflowVariable: &astrov1.EnvironmentObjectAirflowVariable{Value: "7-SECRET"},
		}},
		astrov1.CONNECTION: {{
			ObjectKey: "cloud_db", ObjectType: astrov1.EnvironmentObjectObjectTypeCONNECTION,
			Connection: &astrov1.EnvironmentObjectConnection{Type: "postgres", Host: &host, Password: &pw},
		}},
	}
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	for typ, objs := range rows {
		mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything,
			mock.MatchedBy(func(p *astrov1.ListEnvironmentObjectsParams) bool {
				return *p.ObjectType == typ && !*p.ShowSecrets
			})).
			Return(&astrov1.ListEnvironmentObjectsResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusOK},
				JSON200:      &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: objs, TotalCount: len(objs)},
			}, nil)
	}
	return mc
}

type envTierRow struct {
	Kind, Name, Source string
	Orphan             bool
	Applied            *bool
	DeclareHint        string `json:"declare_hint"`
}

func listJSON(t *testing.T, out string) map[string]envTierRow {
	t.Helper()
	rows := map[string]envTierRow{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var r envTierRow
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("json: %v\n%s", err, out)
		}
		rows[r.Kind+":"+r.Name] = r
	}
	return rows
}

// `env list` shows what the linked workspace holds undeclared, sourced to the
// workspace, applied unless a local source holds the name, with a declare
// hint. It reads presence only, and prints no value.
func TestEnvListShowsUndeclaredWorkspaceObjects(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	dir := workspaceEnvProject(t)
	isolateEnvSources(t, "TEAM_TOKEN", "AIRFLOW_VAR_THRESHOLD", "AIRFLOW_CONN_CLOUD_DB")
	// A local copy of the variable shadows the workspace's.
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("AIRFLOW_VAR_THRESHOLD=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, all := range []bool{false, true} {
		d, out, stderr := envDeps(t, dir, "")
		d.WorkspaceClients = workspaceClients(tierClient(t))
		args := []string{"local", "env", "list", "--output", "json"}
		if all {
			args = append(args, "--all")
		}
		if err := execute(t, d, args...); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out.String()+stderr.String(), "SECRET") {
			t.Fatalf("list printed a workspace value:\n%s", out.String())
		}
		rows := listJSON(t, out.String())
		for _, key := range []string{"env:TEAM_TOKEN", "conn:cloud_db"} {
			r, ok := rows[key]
			if !ok {
				t.Fatalf("all=%v: no row for %s in %v", all, key, rows)
			}
			if r.Source != "workspace (cmws)" || !r.Orphan || r.Applied == nil || !*r.Applied || r.DeclareHint == "" {
				t.Errorf("all=%v: %s = %+v, want an applied undeclared workspace row with a declare hint", all, key, r)
			}
		}
		var shadowed *envTierRow
		for _, r := range rows {
			if r.Kind == "var" && r.Source == "workspace (cmws)" {
				shadowed = &r
			}
		}
		if shadowed == nil || shadowed.Applied != nil {
			t.Errorf("all=%v: the workspace variable the .env shadows = %+v, want listed and not applied", all, shadowed)
		}
	}
}

// `env list` names, on stderr and without the value, a workspace key that
// cannot be an env-var name.
func TestEnvListNamesSkippedWorkspaceKeys(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	dir := workspaceEnvProject(t)
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).
		Return(&astrov1.ListEnvironmentObjectsResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{{
				ObjectKey: "bad-key", ObjectType: astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE,
				EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: "hidden-SECRET"},
			}}, TotalCount: 1},
		}, nil)
	d, out, stderr := envDeps(t, dir, "")
	d.WorkspaceClients = workspaceClients(mc)
	if err := execute(t, d, "local", "env", "list"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "workspace cmws holds bad-key,") {
		t.Errorf("stderr = %q, want the skipped key named", stderr.String())
	}
	if strings.Contains(out.String()+stderr.String(), "SECRET") {
		t.Error("list printed a workspace value")
	}
}

// Offline, `env list` prints one line saying the workspace was not read, and
// lists no workspace rows.
func TestEnvListOfflineWorkspaceNote(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	dir := workspaceEnvProject(t)
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("dial tcp: no route to host"))
	d, out, stderr := envDeps(t, dir, "")
	d.WorkspaceClients = workspaceClients(mc)
	if err := execute(t, d, "local", "env", "list"); err != nil {
		t.Fatal(err)
	}
	if got, want := stderr.String(), "workspace cmws not read (offline): its values are not listed\n"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
	if strings.Contains(out.String(), "workspace (cmws)") {
		t.Errorf("an offline listing sourced a row to the workspace:\n%s", out.String())
	}
}
