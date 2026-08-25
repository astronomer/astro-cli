package local

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/internal/localenv"
)

// envProject writes a project with the given [tool.astro.env] body and points
// ASTRO_HOME at a scratch dir, so project and global files are both isolated.
func envProject(t *testing.T, envBody string) (dir string) {
	t.Helper()
	dir = t.TempDir()
	manifest := "[project]\nname = 'demo'\nrequires-python = '>=3.10'\n\n[tool.astro]\nairflow = '3.1'\n" + envBody
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(".env\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ASTRO_HOME", t.TempDir())
	return dir
}

func envDeps(t *testing.T, dir, stdin string) (d Deps, stdout, stderr *bytes.Buffer) {
	t.Helper()
	stdout = &bytes.Buffer{}
	stderr = &bytes.Buffer{}
	d, _ = testDeps(t)
	d.Stdin = strings.NewReader(stdin)
	d.Stdout = stdout
	d.Stderr = stderr
	d.WorkingDir = func() (string, error) { return dir, nil }
	return d, stdout, stderr
}

func TestEnvSetGetRoundTrip(t *testing.T) {
	dir := envProject(t, "")

	// Set an env var from stdin (a non-TTY stdin reads the value).
	d, _, _ := envDeps(t, dir, "s3cr3t\n")
	if err := execute(t, d, "local", "env", "set", "API_TOKEN"); err != nil {
		t.Fatal(err)
	}
	// The value landed in the project .env at 0600.
	content, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "API_TOKEN=s3cr3t") {
		t.Fatalf(".env missing the value:\n%s", content)
	}

	// Get it back through the chain.
	d, out, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "get", "API_TOKEN"); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "s3cr3t" {
		t.Fatalf("get printed %q, want s3cr3t", out.String())
	}
}

func TestEnvGetJSONShape(t *testing.T) {
	dir := envProject(t, "")
	d, _, _ := envDeps(t, dir, "http://api\n")
	if err := execute(t, d, "local", "env", "set", "API_URL"); err != nil {
		t.Fatal(err)
	}
	d, out, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "get", "API_URL", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Kind, Name, Source, Value string
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if got.Kind != "env" || got.Name != "API_URL" || got.Source != "project" || got.Value != "http://api" {
		t.Fatalf("get json = %+v", got)
	}
}

func TestEnvSetConnFromURI(t *testing.T) {
	dir := envProject(t, "")
	d, _, _ := envDeps(t, dir, "postgres://u:p@host:5432/db\n")
	if err := execute(t, d, "local", "env", "set", "conn", "warehouse"); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "AIRFLOW_CONN_WAREHOUSE=") || !strings.Contains(string(content), `"conn_type":"postgres"`) {
		t.Fatalf("conn not stored as AIRFLOW_CONN JSON:\n%s", content)
	}
}

func TestEnvSetConnAndVarWithValueFlag(t *testing.T) {
	dir := envProject(t, "")
	// The conn/var subcommands inherit --value (persistent on `set`); no stdin.
	d, _, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "set", "conn", "http_api", "--value", `{"conn_type":"http","host":"api"}`); err != nil {
		t.Fatal(err)
	}
	d, _, _ = envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "set", "var", "region", "--value", "us-east-1"); err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(filepath.Join(dir, ".env"))
	for _, want := range []string{`AIRFLOW_CONN_HTTP_API=`, `"conn_type":"http"`, "AIRFLOW_VAR_REGION=us-east-1"} {
		if !strings.Contains(string(content), want) {
			t.Fatalf(".env missing %q:\n%s", want, content)
		}
	}
}

func TestEnvListJSONShapeAndOrphan(t *testing.T) {
	envBody := "\n[tool.astro.env]\nAPI_URL = {}\n"
	dir := envProject(t, envBody)
	// A declared value plus an orphan hand-added to the file.
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("API_URL=http://x\nLEFTOVER=old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	d, out, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "list", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	// NDJSON: one object per line.
	var declared, orphan bool
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var it struct {
			Kind, Name, Source string
			Orphan             bool
		}
		if err := json.Unmarshal([]byte(line), &it); err != nil {
			t.Fatalf("line %q: %v", line, err)
		}
		if it.Name == "API_URL" && it.Source == "project" && !it.Orphan {
			declared = true
		}
		if it.Name == "LEFTOVER" && it.Orphan {
			orphan = true
		}
		// No line ever carries a value key.
		if strings.Contains(line, "\"value\"") {
			t.Errorf("list output leaked a value: %s", line)
		}
	}
	if !declared || !orphan {
		t.Fatalf("declared=%v orphan=%v\n%s", declared, orphan, out.String())
	}
}

func TestEnvDeleteRemovesValue(t *testing.T) {
	dir := envProject(t, "")
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("A=1\nB=2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	d, _, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "delete", "A"); err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(filepath.Join(dir, ".env"))
	if strings.Contains(string(content), "A=1") || !strings.Contains(string(content), "B=2") {
		t.Fatalf("delete wrong: %s", content)
	}
	// Deleting an absent value is an error (non-zero exit).
	d, _, _ = envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "delete", "NOPE"); err == nil {
		t.Fatal("expected an error deleting an absent value")
	}
}

func TestEnvGlobalScope(t *testing.T) {
	dir := envProject(t, "")
	d, _, _ := envDeps(t, dir, "globalval\n")
	if err := execute(t, d, "local", "env", "set", "SHARED", "--global"); err != nil {
		t.Fatal(err)
	}
	// It went to the global file, not the project .env. Through GlobalEnvPath
	// rather than by joining a path here: the layout is that function's to own,
	// and restating it is how this test agreed with the bug where ASTRO_HOME was
	// read as .astro itself rather than as its parent.
	gp, err := localenv.GlobalEnvPath()
	if err != nil {
		t.Fatal(err)
	}
	gc, err := os.ReadFile(gp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gc), "SHARED=globalval") {
		t.Fatalf("global file missing value:\n%s", gc)
	}
	if _, err := os.Stat(filepath.Join(dir, ".env")); !os.IsNotExist(err) {
		t.Fatalf("project .env should not exist for a --global set")
	}
}

func TestEnvSetWarnsWhenNotGitignored(t *testing.T) {
	dir := envProject(t, "")
	// Remove the .gitignore so .env is not covered.
	if err := os.Remove(filepath.Join(dir, ".gitignore")); err != nil {
		t.Fatal(err)
	}
	d, _, stderr := envDeps(t, dir, "x\n")
	if err := execute(t, d, "local", "env", "set", "TOKEN"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "gitignore") {
		t.Fatalf("expected a gitignore warning on stderr, got: %q", stderr.String())
	}
}

func TestStartMissingEnvHint(t *testing.T) {
	envBody := "\n[tool.astro.env]\nAPI_TOKEN = {}\n"
	dir := envProject(t, envBody)
	d, _, _ := envDeps(t, dir, "")
	err := execute(t, d, "local", "start")
	if err == nil {
		t.Fatal("expected start to fail on a missing required value")
	}
	if !strings.Contains(err.Error(), "astro local env set API_TOKEN --project") {
		t.Fatalf("missing hint not in error:\n%s", err.Error())
	}
}
