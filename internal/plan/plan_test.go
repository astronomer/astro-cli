package plan

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/internal/envresolve"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/internal/userstate"
	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

const manifestTOML = `[project]
name = 'demo'
requires-python = '>=3.10'

[tool.astro]
airflow = '3.1'
`

// newProject writes a minimal manifest into a fresh dir and points the cache
// at another, so Build resolves without touching real user state.
func newProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, project.Marker), []byte(manifestTOML), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	return dir
}

func TestChoosePortPrecedence(t *testing.T) {
	cases := []struct {
		name             string
		flag, user, want int
	}{
		{"flag wins over user state", 8080, 9090, 8080},
		{"user state when no flag", 0, 9090, 9090},
		{"no preference falls to the allocator", 0, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := choosePort(tc.flag, tc.user); got != tc.want {
				t.Errorf("choosePort(%d, %d) = %d, want %d", tc.flag, tc.user, got, tc.want)
			}
		})
	}
}

func TestBuildFillsPlanFromManifest(t *testing.T) {
	dir := newProject(t)
	built, err := Build(dir, Options{Mode: localrt.ModeDocker, RequestedPort: 8080, StopWithSession: true})
	if err != nil {
		t.Fatal(err)
	}
	p := built.Plan
	if p.ProjectPath != dir {
		t.Errorf("ProjectPath = %q, want %q", p.ProjectPath, dir)
	}
	if p.AirflowVersion != "3.1" {
		t.Errorf("AirflowVersion = %q, want 3.1", p.AirflowVersion)
	}
	if p.PythonVersion != "" {
		t.Errorf("PythonVersion = %q, want empty (uv reads requires-python)", p.PythonVersion)
	}
	if p.Mode != localrt.ModeDocker || !p.StopWithSession || p.RequestedPort != 8080 {
		t.Errorf("flags not carried: mode=%q stopWithSession=%v port=%d", p.Mode, p.StopWithSession, p.RequestedPort)
	}
	if !strings.HasSuffix(p.Hostname, ".localhost") {
		t.Errorf("Hostname = %q, want a .localhost label", p.Hostname)
	}
	wantStateDir, _ := localrt.StateDir(dir)
	if p.StateDir != wantStateDir {
		t.Errorf("StateDir = %q, want %q", p.StateDir, wantStateDir)
	}
}

func TestBuildPrefersUserStatePort(t *testing.T) {
	dir := newProject(t)
	if err := userstate.Save(dir, userstate.State{Port: 12345}); err != nil {
		t.Fatal(err)
	}
	built, err := Build(dir, Options{}) // no --port flag
	if err != nil {
		t.Fatal(err)
	}
	if built.Plan.RequestedPort != 12345 {
		t.Errorf("RequestedPort = %d, want 12345 from user state", built.Plan.RequestedPort)
	}
}

func TestPersistPort(t *testing.T) {
	dir := newProject(t)
	if err := PersistPort(dir, 10001); err != nil {
		t.Fatal(err)
	}
	us, err := userstate.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if us.Port != 10001 {
		t.Errorf("persisted port = %d, want 10001", us.Port)
	}
	// A zero port never overwrites a real preference.
	if err := PersistPort(dir, 0); err != nil {
		t.Fatal(err)
	}
	if us, _ := userstate.Load(dir); us.Port != 10001 {
		t.Errorf("PersistPort(0) clobbered the stored port: %d", us.Port)
	}
}

func TestBuildMissingProject(t *testing.T) {
	_, err := Build(t.TempDir(), Options{}) // no manifest anywhere above
	var notFound *project.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("want *project.NotFoundError, got %v", err)
	}
}

func TestMissingEnvError(t *testing.T) {
	err := &MissingEnvError{
		Project: "/p",
		Missing: []envresolve.Missing{
			{Section: envschema.SectionEnvVar, Name: "API_URL", EnvKey: "API_URL"},
			{Section: envschema.SectionConnection, Name: "warehouse", EnvKey: "AIRFLOW_CONN_WAREHOUSE"},
		},
	}
	msg := err.Error()
	// The one hint form is the exact set command per kind.
	for _, want := range []string{"API_URL", "astro local env set API_URL --project", "astro local env set conn warehouse --project", "astro local start"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
	if err.Payload() == nil {
		t.Error("Payload is nil")
	}
}
