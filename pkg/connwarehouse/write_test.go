package connwarehouse

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func pgWarehouse(name, pw string) Materialized {
	return Materialized{
		Name: name, ConnID: strings.TrimPrefix(name, namePrefix), ConnType: "postgres",
		Config: map[string]any{"type": "postgres", "host": "h", "port": 5432, "user": "u", "database": "db", "password": "${" + envVar(strings.TrimPrefix(name, namePrefix), "PASSWORD") + "}"},
		Env:    map[string]string{envVar(strings.TrimPrefix(name, namePrefix), "PASSWORD"): pw},
	}
}

// readFile reads a file under the test's temp dir. Taking path as a plain
// string param keeps gosec's G304 taint analysis quiet (an inline
// os.ReadFile(filepath.Join(...)) would trip it).
func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

func readMap(t *testing.T, path string) map[string]any {
	t.Helper()
	b := readFile(t, path)
	m := map[string]any{}
	if err := yaml.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return m
}

func TestWriteFreshConfig(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, []Materialized{pgWarehouse("airflow_pg", "s3cret")}); err != nil {
		t.Fatal(err)
	}

	wh := readMap(t, filepath.Join(dir, "warehouse.yml"))
	entry, ok := wh["airflow_pg"].(map[string]any)
	if !ok {
		t.Fatalf("airflow_pg entry missing: %+v", wh)
	}
	if entry["password"] != "${AIRFLOW_PG_PASSWORD}" {
		t.Errorf("password ref = %v", entry["password"])
	}

	env := readFile(t, filepath.Join(dir, ".env"))
	if !strings.Contains(string(env), `AIRFLOW_PG_PASSWORD="s3cret"`) {
		t.Errorf(".env missing managed secret:\n%s", env)
	}
	// Secret must not appear in the yaml.
	whBytes := readFile(t, filepath.Join(dir, "warehouse.yml"))
	if strings.Contains(string(whBytes), "s3cret") {
		t.Errorf("secret leaked into warehouse.yml:\n%s", whBytes)
	}
	// .env must be 0600 — but Windows doesn't honor Unix permission bits
	// (Chmod is a no-op there), so only assert this on Unix.
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(filepath.Join(dir, ".env"))
		if info.Mode().Perm() != 0o600 {
			t.Errorf(".env perm = %v, want 0600", info.Mode().Perm())
		}
	}
}

func TestWritePreservesUserEntriesAndReplacesManaged(t *testing.T) {
	dir := t.TempDir()
	// Pre-existing file: a user warehouse (with a comment) + a stale managed entry.
	seed := `# my hand-written warehouse
my_dwh:
  type: snowflake
  account: mine
  user: me
  password: ${MY_PW}
airflow_pg:
  type: postgres
  host: STALE
`
	if err := os.WriteFile(filepath.Join(dir, "warehouse.yml"), []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	// Pre-existing .env: a user secret + a stale managed one.
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("MY_PW=keepme\nAIRFLOW_PG_PASSWORD=stale\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Write(dir, []Materialized{pgWarehouse("airflow_pg", "fresh")}); err != nil {
		t.Fatal(err)
	}

	wh := readMap(t, filepath.Join(dir, "warehouse.yml"))
	if _, ok := wh["my_dwh"]; !ok {
		t.Error("user entry my_dwh was dropped")
	}
	entry := wh["airflow_pg"].(map[string]any)
	if entry["host"] == "STALE" {
		t.Error("stale managed entry not replaced")
	}

	env := readFile(t, filepath.Join(dir, ".env"))
	s := string(env)
	if !strings.Contains(s, "MY_PW=keepme") {
		t.Errorf("user .env line dropped:\n%s", s)
	}
	if strings.Contains(s, "AIRFLOW_PG_PASSWORD=stale") || strings.Count(s, "AIRFLOW_PG_PASSWORD") != 1 {
		t.Errorf("stale managed secret not cleanly replaced:\n%s", s)
	}
	if !strings.Contains(s, `AIRFLOW_PG_PASSWORD="fresh"`) {
		t.Errorf("new managed secret missing:\n%s", s)
	}
}

func TestWriteIdempotent(t *testing.T) {
	dir := t.TempDir()
	live := []Materialized{pgWarehouse("airflow_pg", "p")}
	if err := Write(dir, live); err != nil {
		t.Fatal(err)
	}
	wh1 := readFile(t, filepath.Join(dir, "warehouse.yml"))
	env1 := readFile(t, filepath.Join(dir, ".env"))
	if err := Write(dir, live); err != nil {
		t.Fatal(err)
	}
	wh2 := readFile(t, filepath.Join(dir, "warehouse.yml"))
	env2 := readFile(t, filepath.Join(dir, ".env"))
	if !bytes.Equal(wh1, wh2) {
		t.Errorf("warehouse.yml not stable across runs:\n%s\n---\n%s", wh1, wh2)
	}
	if !bytes.Equal(env1, env2) {
		t.Errorf(".env not stable across runs:\n%s\n---\n%s", env1, env2)
	}
}

func TestWriteEmptyRemovesManagedAndCleansUp(t *testing.T) {
	dir := t.TempDir()
	// User entry present alongside a managed one.
	seed := "my_dwh:\n  type: snowflake\n  account: a\n  user: u\n  password: ${X}\nairflow_pg:\n  type: postgres\n"
	_ = os.WriteFile(filepath.Join(dir, "warehouse.yml"), []byte(seed), 0o600)
	_ = os.WriteFile(filepath.Join(dir, ".env"), []byte("X=1\nAIRFLOW_PG_PASSWORD=old\n"), 0o600)

	// No live warehouses (e.g. all connections lost their credentials).
	if err := Write(dir, nil); err != nil {
		t.Fatal(err)
	}
	wh := readMap(t, filepath.Join(dir, "warehouse.yml"))
	if _, ok := wh["airflow_pg"]; ok {
		t.Error("managed entry should be gone")
	}
	if _, ok := wh["my_dwh"]; !ok {
		t.Error("user entry should survive")
	}
	env := readFile(t, filepath.Join(dir, ".env"))
	if strings.Contains(string(env), "AIRFLOW_PG_PASSWORD") {
		t.Errorf("managed secret should be gone:\n%s", env)
	}
	if !strings.Contains(string(env), "X=1") {
		t.Errorf("user secret should survive:\n%s", env)
	}
}

func TestWriteRemovesFilesWhenNothingRemains(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "warehouse.yml"), []byte("airflow_pg:\n  type: postgres\n"), 0o600)
	_ = os.WriteFile(filepath.Join(dir, ".env"), []byte("AIRFLOW_PG_PASSWORD=old\n"), 0o600)
	if err := Write(dir, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "warehouse.yml")); !os.IsNotExist(err) {
		t.Error("warehouse.yml should be removed when only managed entries existed")
	}
	if _, err := os.Stat(filepath.Join(dir, ".env")); !os.IsNotExist(err) {
		t.Error(".env should be removed when only managed secrets existed")
	}
}

func TestWriteMultilineSecretQuoted(t *testing.T) {
	dir := t.TempDir()
	pem := "-----BEGIN-----\nline2\nline3\n-----END-----"
	live := []Materialized{{
		Name: "airflow_kp", ConnID: "kp", ConnType: "snowflake",
		Config: map[string]any{"type": "snowflake", "private_key": "${AIRFLOW_KP_PRIVATE_KEY}"},
		Env:    map[string]string{"AIRFLOW_KP_PRIVATE_KEY": pem},
	}}
	if err := Write(dir, live); err != nil {
		t.Fatal(err)
	}
	env := readFile(t, filepath.Join(dir, ".env"))
	// Newlines escaped as \n inside double quotes → single dotenv line.
	if !strings.Contains(string(env), `AIRFLOW_KP_PRIVATE_KEY="-----BEGIN-----\nline2\nline3\n-----END-----"`) {
		t.Errorf("multiline secret not escaped as one quoted line:\n%s", env)
	}
}

// A .env the desktop wrote before the writer was shared carries its own banner.
// A rewrite replaces it with the current one instead of keeping both.
func TestWriteReplacesTheLegacyDesktopBanner(t *testing.T) {
	dir := t.TempDir()
	seed := "MY=1\n\n" + legacyEnvBanner + "\nAIRFLOW_PG_PASSWORD=\"old\"\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	live := []Materialized{{Name: "airflow_pg", Config: map[string]any{"type": "postgres"}, Env: map[string]string{"AIRFLOW_PG_PASSWORD": "new"}}}
	if err := Write(dir, live); err != nil {
		t.Fatal(err)
	}
	want := "MY=1\n\n" + envBanner + "\nAIRFLOW_PG_PASSWORD=\"new\"\n"
	if got := string(readFile(t, filepath.Join(dir, ".env"))); got != want {
		t.Errorf(".env =\n%s\nwant\n%s", got, want)
	}
}

// A .env write that fails strips the managed secrets already there, keeps the
// user's lines, and leaves warehouse.yml untouched: an old plaintext secret
// must not outlive a write that could not replace it.
func TestWriteFailureStripsStaleManagedSecrets(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	whPath := filepath.Join(dir, "warehouse.yml")
	if err := os.WriteFile(envPath, []byte("MINE=1\n"+envBanner+"\nAIRFLOW_OLD_PASSWORD=\"stale\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(whPath, []byte("airflow_old:\n  type: postgres\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	orig := writeFile
	t.Cleanup(func() { writeFile = orig })
	failed := false
	writeFile = func(path string, data []byte, perm os.FileMode) error {
		if strings.Contains(string(data), envBanner) {
			failed = true
			return errors.New("disk full")
		}
		return orig(path, data, perm)
	}
	live := []Materialized{{Name: "airflow_new", Config: map[string]any{"type": "postgres"}, Env: map[string]string{"AIRFLOW_NEW_PASSWORD": "new"}}}
	if err := Write(dir, live); err == nil {
		t.Fatal("Write reported success after its .env write failed")
	}
	if !failed {
		t.Fatal("the stub never failed a write")
	}
	if got := string(readFile(t, envPath)); got != "MINE=1\n" {
		t.Errorf(".env after a failed write = %q, want only the user's line", got)
	}
	if got := string(readFile(t, whPath)); got != "airflow_old:\n  type: postgres\n" {
		t.Errorf("warehouse.yml was rewritten after the .env write failed:\n%s", got)
	}
}
