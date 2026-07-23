package manifest

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func write(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pyproject.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const full = `
[project]
name = "my-pipelines"
requires-python = ">=3.11"
dependencies = ["pandas>=2.1", "apache-airflow-providers-snowflake"]

[tool.astro]
airflow = "3.1"
packages = ["libpq-dev", "build-essential"]

[tool.astro.target.astro]
image = { os = "ubi", python = "3.12" }
system-packages = ["libaio"]

[tool.astro.deployments.preview]
target = "astro"
workspace = "ws-abc"
deployment = "dep-preview"

[tool.astro.deployments.prod]
target = "astro"
workspace = "ws-abc"
deployment = "dep-xyz"

[tool.astro.env.connections.warehouse]
conn_type = "snowflake"
required = true
`

func TestLoadFull(t *testing.T) {
	m, err := Load(write(t, full))
	if err != nil {
		t.Fatal(err)
	}

	wantProject := Project{
		Name:           "my-pipelines",
		RequiresPython: ">=3.11",
		Dependencies:   []string{"pandas>=2.1", "apache-airflow-providers-snowflake"},
	}
	if !reflect.DeepEqual(m.Project, wantProject) {
		t.Errorf("Project = %#v, want %#v", m.Project, wantProject)
	}

	if m.Astro.AirflowVersion != "3.1" {
		t.Errorf("AirflowVersion = %q, want %q", m.Astro.AirflowVersion, "3.1")
	}

	wantPackages := []string{"libpq-dev", "build-essential"}
	if !reflect.DeepEqual(m.Astro.Packages, wantPackages) {
		t.Errorf("Packages = %#v, want %#v", m.Astro.Packages, wantPackages)
	}

	wantDeployments := map[string]Deployment{
		"preview": {Target: "astro", Workspace: "ws-abc", Deployment: "dep-preview"},
		"prod":    {Target: "astro", Workspace: "ws-abc", Deployment: "dep-xyz"},
	}
	if !reflect.DeepEqual(m.Astro.Deployments, wantDeployments) {
		t.Errorf("Deployments = %#v, want %#v", m.Astro.Deployments, wantDeployments)
	}

	wantTarget := map[string]any{
		"image":           map[string]any{"os": "ubi", "python": "3.12"},
		"system-packages": []any{"libaio"},
	}
	if !reflect.DeepEqual(m.Astro.Targets["astro"], wantTarget) {
		t.Errorf("Targets[astro] = %#v, want %#v", m.Astro.Targets["astro"], wantTarget)
	}

	wantEnv := map[string]any{
		"connections": map[string]any{
			"warehouse": map[string]any{"conn_type": "snowflake", "required": true},
		},
	}
	if !reflect.DeepEqual(m.Astro.Env, wantEnv) {
		t.Errorf("Env = %#v, want %#v", m.Astro.Env, wantEnv)
	}
}

func TestLoadMinimal(t *testing.T) {
	m, err := Load(write(t, "[project]\nname = \"etl\"\n\n[tool.astro]\nairflow = \"3\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Project.Name != "etl" || m.Astro.AirflowVersion != "3" {
		t.Errorf("got %#v", m)
	}
	if m.Astro.Deployments != nil || m.Astro.Env != nil || m.Astro.Targets != nil || m.Astro.Packages != nil {
		t.Errorf("absent sections should stay nil, got %#v", m.Astro)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "pyproject.toml"))
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("want fs.ErrNotExist, got %v", err)
	}
}

func TestLoadBadTOML(t *testing.T) {
	path := write(t, "[project\nname=")
	_, err := Load(path)
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("want *ParseError, got %T: %v", err, err)
	}
	if pe.Path != path {
		t.Errorf("ParseError.Path = %q, want %q", pe.Path, path)
	}
}

func TestLoadNoAstroSection(t *testing.T) {
	_, err := Load(write(t, "[project]\nname = \"plain-python\"\n"))
	if !errors.Is(err, ErrNoAstroSection) {
		t.Errorf("want ErrNoAstroSection, got %v", err)
	}
}

func TestValidation(t *testing.T) {
	cases := []struct {
		name     string
		content  string
		wantKeys []string
	}{
		{
			name:     "missing project name",
			content:  "[tool.astro]\nairflow = \"3.1\"\n",
			wantKeys: []string{"project.name"},
		},
		{
			name:     "bad project name",
			content:  "[project]\nname = \"-bad-\"\n\n[tool.astro]\nairflow = \"3.1\"\n",
			wantKeys: []string{"project.name"},
		},
		{
			name:     "missing airflow",
			content:  "[project]\nname = \"p\"\n\n[tool.astro]\ndeployments = {}\n",
			wantKeys: []string{"tool.astro.airflow"},
		},
		{
			name:     "bad airflow version",
			content:  "[project]\nname = \"p\"\n\n[tool.astro]\nairflow = \"three\"\n",
			wantKeys: []string{"tool.astro.airflow"},
		},
		{
			name:     "empty package entry",
			content:  "[project]\nname = \"p\"\n\n[tool.astro]\nairflow = \"3.1\"\npackages = [\"libpq-dev\", \"  \"]\n",
			wantKeys: []string{"tool.astro.packages[1]"},
		},
		{
			name: "incomplete deployment",
			content: `
[project]
name = "p"

[tool.astro]
airflow = "3.1"

[tool.astro.deployments.prod]
target = "astro"
`,
			wantKeys: []string{
				"tool.astro.deployments.prod.deployment",
				"tool.astro.deployments.prod.workspace",
			},
		},
		{
			name:    "several at once",
			content: "[tool.astro]\nairflow = \"v3\"\n\n[tool.astro.deployments.d]\nworkspace = \"w\"\ndeployment = \"x\"\n",
			wantKeys: []string{
				"project.name",
				"tool.astro.airflow",
				"tool.astro.deployments.d.target",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := write(t, tc.content)
			_, err := Load(path)
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("want *ValidationError, got %T: %v", err, err)
			}
			if ve.Path != path {
				t.Errorf("ValidationError.Path = %q, want %q", ve.Path, path)
			}
			var keys []string
			for _, p := range ve.Problems {
				keys = append(keys, p.Key)
			}
			if !reflect.DeepEqual(keys, tc.wantKeys) {
				t.Errorf("problem keys = %v, want %v", keys, tc.wantKeys)
			}
		})
	}
}

func TestParseWithoutPath(t *testing.T) {
	_, err := Parse([]byte("[tool.astro]\nairflow = \"3.1\"\n"))
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *ValidationError, got %T: %v", err, err)
	}
	if ve.Path != "" {
		t.Errorf("Path should be empty for Parse, got %q", ve.Path)
	}
}

func TestAirflowVersions(t *testing.T) {
	good := []string{"3", "3.1", "3.1.2", "2.10"}
	bad := []string{"three", "3.", ".1", "3.1.2.3", "v3", "3.x", ""}
	for _, v := range good {
		if !airflowVersionRe.MatchString(v) {
			t.Errorf("%q should be accepted", v)
		}
	}
	for _, v := range bad {
		if airflowVersionRe.MatchString(v) {
			t.Errorf("%q should be rejected", v)
		}
	}
}
