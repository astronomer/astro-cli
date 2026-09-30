package plan

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"

	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/pkg/localrt"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// linkedManifest links a workspace and declares one plain env var with a
// default, and nothing with a workspace source.
const linkedManifest = `[project]
name = 'demo'
requires-python = '>=3.10'
dependencies = ['apache-airflow==3.1.*']

[tool.astro]
workspace = 'cmws'
domain = 'localhost'

[tool.astro.env]
REGION = { default = 'us-east-1' }
`

// linkedRequiredManifest also declares a required value only the workspace
// holds.
const linkedRequiredManifest = linkedManifest + `DATA_WAREHOUSE_URI = { source = 'workspace' }
`

// typedWorkspace answers each object type with its own rows, as the API does:
// one call per type.
func typedWorkspace(rows map[astrov1.ListEnvironmentObjectsParamsObjectType][]astrov1.EnvironmentObject) *astrov1_mocks.ClientWithResponsesInterface {
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	for _, typ := range []astrov1.ListEnvironmentObjectsParamsObjectType{astrov1.ENVIRONMENTVARIABLE, astrov1.AIRFLOWVARIABLE, astrov1.CONNECTION} {
		mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything,
			mock.MatchedBy(func(p *astrov1.ListEnvironmentObjectsParams) bool { return *p.ObjectType == typ })).
			Return(okListResp(rows[typ]...), nil)
	}
	return mc
}

func strp(s string) *string { return &s }

// sampleWorkspace holds one of each kind, none of which linkedManifest
// declares, plus REGION, which it declares with a default.
func sampleWorkspace() *astrov1_mocks.ClientWithResponsesInterface {
	return typedWorkspace(map[astrov1.ListEnvironmentObjectsParamsObjectType][]astrov1.EnvironmentObject{
		astrov1.ENVIRONMENTVARIABLE: {
			{ObjectKey: "TEAM_TOKEN", ObjectType: astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE, EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: "cloud-token", IsSecret: true}},
			{ObjectKey: "REGION", ObjectType: astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE, EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: "eu-west-1"}},
			{ObjectKey: "EMPTY_ONE", ObjectType: astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE, EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: ""}},
		},
		astrov1.AIRFLOWVARIABLE: {
			{ObjectKey: "threshold", ObjectType: astrov1.EnvironmentObjectObjectTypeAIRFLOWVARIABLE, AirflowVariable: &astrov1.EnvironmentObjectAirflowVariable{Value: "7"}},
		},
		astrov1.CONNECTION: {
			{ObjectKey: "cloud_db", ObjectType: astrov1.EnvironmentObjectObjectTypeCONNECTION, Connection: &astrov1.EnvironmentObjectConnection{Type: "postgres", Host: strp("db.cloud"), Password: strp("pw")}},
		},
	})
}

// An undeclared workspace connection, Airflow variable and env var reach
// Airflow, off disk; an empty value is not injected as a blank.
func TestBuildInjectsUndeclaredWorkspaceObjects(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	dir := newWorkspaceProject(t, linkedManifest)

	built, err := Build(dir, Options{Mode: localrt.ModeDocker, WorkspaceProvider: emProvider(sampleWorkspace())})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	se := built.Plan.SecretEnv
	if !strings.Contains(se["AIRFLOW_CONN_CLOUD_DB"], "db.cloud") {
		t.Errorf("AIRFLOW_CONN_CLOUD_DB = %q, want the workspace connection", se["AIRFLOW_CONN_CLOUD_DB"])
	}
	if se["AIRFLOW_VAR_THRESHOLD"] != "7" {
		t.Errorf("AIRFLOW_VAR_THRESHOLD = %q, want 7", se["AIRFLOW_VAR_THRESHOLD"])
	}
	if se["TEAM_TOKEN"] != "cloud-token" {
		t.Errorf("TEAM_TOKEN = %q, want the workspace value", se["TEAM_TOKEN"])
	}
	if _, ok := se["EMPTY_ONE"]; ok {
		t.Error("an empty workspace value was injected as a blank")
	}
	for _, k := range []string{"AIRFLOW_CONN_CLOUD_DB", "AIRFLOW_VAR_THRESHOLD", "TEAM_TOKEN"} {
		if _, onDisk := built.Plan.Env[k]; onDisk {
			t.Errorf("%s traveled in Env, which docker writes to disk", k)
		}
	}
	if built.WorkspaceNote != "" {
		t.Errorf("WorkspaceNote = %q for a workspace that was read", built.WorkspaceNote)
	}
}

// The workspace beats a declaration's default, and every local tier beats the
// workspace, for declared and undeclared names alike. The shell beating an
// undeclared workspace name puts it on docker's passthrough list.
func TestBuildLocalSourcesBeatTheWorkspace(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	dir := newWorkspaceProject(t, linkedManifest)

	// Before the default is overridden: the workspace fills REGION.
	built, err := Build(dir, Options{WorkspaceProvider: emProvider(sampleWorkspace())})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := built.Plan.SecretEnv["REGION"]; got != "eu-west-1" {
		t.Fatalf("REGION = %q, want the workspace value over the declaration default", got)
	}
	if _, ok := built.Plan.Env["REGION"]; ok {
		t.Fatal("the losing default traveled beside the workspace value")
	}

	// Now local sources for each kind.
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("AIRFLOW_VAR_THRESHOLD=1\nREGION=local\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEAM_TOKEN", "from-shell")
	built, err = Build(dir, Options{Mode: localrt.ModeDocker, WorkspaceProvider: emProvider(sampleWorkspace())})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := built.Plan.Env["AIRFLOW_VAR_THRESHOLD"]; got != "1" {
		t.Errorf("AIRFLOW_VAR_THRESHOLD = %q, want the project .env value", got)
	}
	if got := built.Plan.Env["REGION"]; got != "local" {
		t.Errorf("REGION = %q, want the project .env value", got)
	}
	for _, k := range []string{"AIRFLOW_VAR_THRESHOLD", "REGION", "TEAM_TOKEN"} {
		if v, ok := built.Plan.SecretEnv[k]; ok {
			t.Errorf("%s: the losing workspace value %q traveled beside the local one", k, v)
		}
	}
	if !slices.Contains(built.Plan.PassthroughEnv, "TEAM_TOKEN") {
		t.Errorf("PassthroughEnv = %v, want TEAM_TOKEN so docker gets the shell's value", built.Plan.PassthroughEnv)
	}
	if !strings.Contains(built.Plan.SecretEnv["AIRFLOW_CONN_CLOUD_DB"], "db.cloud") {
		t.Error("the workspace connection nothing local holds was dropped")
	}
}

// ~/.astro/env, the lowest local tier, still beats the workspace.
func TestBuildGlobalFileBeatsTheWorkspace(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	dir := newWorkspaceProject(t, linkedManifest)
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".astro"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".astro", "env"), []byte("TEAM_TOKEN=global\nREGION=global\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	built, err := Build(dir, Options{WorkspaceProvider: emProvider(sampleWorkspace())})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, k := range []string{"TEAM_TOKEN", "REGION"} {
		if got := built.Plan.Env[k]; got != "global" {
			t.Errorf("%s = %q, want the ~/.astro/env value", k, got)
		}
		if _, ok := built.Plan.SecretEnv[k]; ok {
			t.Errorf("%s: the losing workspace value traveled beside the global file's", k)
		}
	}
}

// Offline, a start that needs nothing only the workspace holds proceeds, with
// a one-line note and the declaration default in place.
func TestBuildOfflineWorkspaceStartsWithANote(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	dir := newWorkspaceProject(t, linkedManifest)
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("dial tcp: no route to host"))

	built, err := Build(dir, Options{WorkspaceProvider: emProvider(mc)})
	if err != nil {
		t.Fatalf("Build: %v, want the start to proceed", err)
	}
	if !testUtil.StringContains([]string{"workspace cmws not read (offline)", "starting without its values"}, built.WorkspaceNote) {
		t.Errorf("WorkspaceNote = %q, want the offline note", built.WorkspaceNote)
	}
	if got := built.Plan.Env["REGION"]; got != "us-east-1" {
		t.Errorf("REGION = %q, want the declaration default", got)
	}
}

// Offline, a required value only the workspace could supply still refuses,
// with the offline cause.
func TestBuildOfflineRefusesAWorkspaceOnlyRequiredValue(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	dir := newWorkspaceProject(t, linkedRequiredManifest)
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("dial tcp: no route to host"))

	_, err := Build(dir, Options{WorkspaceProvider: emProvider(mc)})
	var missing *MissingEnvError
	if !errors.As(err, &missing) {
		t.Fatalf("err = %v, want MissingEnvError", err)
	}
	if !testUtil.StringContains([]string{"DATA_WAREHOUSE_URI", "could not reach"}, missing.Error()) {
		t.Fatalf("message should carry the offline cause:\n%s", missing.Error())
	}
}

// A project that links no workspace reads nothing and notes nothing.
func TestBuildUnlinkedProjectReadsNoWorkspace(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	dir := newWorkspaceProject(t, strings.Replace(linkedManifest, "workspace = 'cmws'\ndomain = 'localhost'\n", "", 1))
	mc := new(astrov1_mocks.ClientWithResponsesInterface) // never called
	built, err := Build(dir, Options{WorkspaceProvider: emProvider(mc)})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if built.WorkspaceNote != "" {
		t.Errorf("WorkspaceNote = %q for a project with no workspace", built.WorkspaceNote)
	}
	mc.AssertNotCalled(t, "ListEnvironmentObjectsWithResponse")
}

// A workspace key that cannot be an env-var name is not injected, and the
// start's note names it without its value.
func TestBuildNamesSkippedWorkspaceKeys(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	dir := newWorkspaceProject(t, linkedManifest)
	mc := typedWorkspace(map[astrov1.ListEnvironmentObjectsParamsObjectType][]astrov1.EnvironmentObject{
		astrov1.ENVIRONMENTVARIABLE: {{ObjectKey: "bad-key", ObjectType: astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE, EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: "hidden-value"}}},
	})
	built, err := Build(dir, Options{WorkspaceProvider: emProvider(mc)})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, ok := built.Plan.SecretEnv["bad-key"]; ok {
		t.Error("a key that is not an env-var name was injected")
	}
	if !strings.Contains(built.WorkspaceNote, "holds bad-key,") || strings.Contains(built.WorkspaceNote, "hidden-value") {
		t.Errorf("WorkspaceNote = %q, want the key named and no value", built.WorkspaceNote)
	}
}
