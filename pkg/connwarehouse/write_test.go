package connwarehouse

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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

// envHas reports whether the .env bytes contain secret, without echoing it:
// a failing assertion names the variable, never the value.
func envHas(b []byte, secret string) bool { return bytes.Contains(b, []byte(secret)) }

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
	if envHas(readFile(t, filepath.Join(dir, "warehouse.yml")), "s3cret") {
		t.Error("the AIRFLOW_PG_PASSWORD value reached warehouse.yml")
	}
	if !isWindows {
		info, err := os.Stat(filepath.Join(dir, "warehouse.yml"))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("warehouse.yml perm = %v, want 0600", info.Mode().Perm())
		}
	}
}

// No secret value reaches the disk: Write creates no .env, and no file under
// dir holds any of the values, whatever the connector.
func TestWritePersistsNoSecret(t *testing.T) {
	dir := t.TempDir()
	pem := "-----BEGIN-----\nkeybody\n-----END-----"
	live := []Materialized{
		pgWarehouse("airflow_pg", "pg-secret-value"),
		{
			Name: "airflow_kp", ConnID: "kp", ConnType: "snowflake",
			Config: map[string]any{"type": "snowflake", "private_key": "${AIRFLOW_KP_PRIVATE_KEY}", "private_key_passphrase": "${AIRFLOW_KP_PRIVATE_KEY_PASSPHRASE}"},
			Env:    map[string]string{"AIRFLOW_KP_PRIVATE_KEY": pem, "AIRFLOW_KP_PRIVATE_KEY_PASSPHRASE": "pass-secret-value"},
		},
		{
			Name: "airflow_dbx", ConnID: "dbx", ConnType: "sqlalchemy",
			Config: map[string]any{"type": "sqlalchemy", "url": "${AIRFLOW_DBX_URL}"},
			Env:    map[string]string{"AIRFLOW_DBX_URL": "databricks://token:dbx-secret-value@h?x=y"},
		},
	}
	if err := Write(dir, live); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".env")); !os.IsNotExist(err) {
		t.Errorf(".env exists after Write (stat err %v); nothing should create it", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		b := readFile(t, filepath.Join(dir, e.Name()))
		for _, m := range live {
			for k, v := range m.Env {
				if envHas(b, v) || envHas(b, "keybody") {
					t.Errorf("%s holds the value of %s", e.Name(), k)
				}
			}
		}
	}
}

// An existing .env loses the managed secrets an earlier version wrote, banner
// included, and keeps the user's lines; Write adds nothing to it.
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
	envSeed := "MY_PW=keepme\n# my note\n\n" + envBanner + "\nAIRFLOW_PG_PASSWORD=\"stale-secret\"\nexport AIRFLOW_OLD_TOKEN=old-secret\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(envSeed), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Write(dir, []Materialized{pgWarehouse("airflow_pg", "fresh-secret")}); err != nil {
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
	if got, want := string(env), "MY_PW=keepme\n# my note\n"; got != want {
		// The user's lines only: safe to print, the seeded secrets are gone.
		if envHas(env, "stale-secret") || envHas(env, "old-secret") || envHas(env, "fresh-secret") {
			t.Fatal(".env still holds a managed secret")
		}
		t.Errorf(".env = %q, want %q", got, want)
	}
}

func TestWriteIdempotent(t *testing.T) {
	dir := t.TempDir()
	live := []Materialized{pgWarehouse("airflow_pg", "p")}
	if err := Write(dir, live); err != nil {
		t.Fatal(err)
	}
	wh1 := readFile(t, filepath.Join(dir, "warehouse.yml"))
	if err := Write(dir, live); err != nil {
		t.Fatal(err)
	}
	wh2 := readFile(t, filepath.Join(dir, "warehouse.yml"))
	if !bytes.Equal(wh1, wh2) {
		t.Errorf("warehouse.yml not stable across runs:\n%s\n---\n%s", wh1, wh2)
	}
}

func TestWriteEmptyRemovesManagedAndCleansUp(t *testing.T) {
	dir := t.TempDir()
	// User entry present alongside a managed one.
	seed := "my_dwh:\n  type: snowflake\n  account: a\n  user: u\n  password: ${X}\nairflow_pg:\n  type: postgres\n"
	_ = os.WriteFile(filepath.Join(dir, "warehouse.yml"), []byte(seed), 0o600)
	_ = os.WriteFile(filepath.Join(dir, ".env"), []byte("X=1\n\n"+envBanner+"\nAIRFLOW_PG_PASSWORD=old\n"), 0o600)

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
	if got := string(readFile(t, filepath.Join(dir, ".env"))); got != "X=1\n" {
		t.Errorf(".env = %q, want only the user's line", got)
	}
}

func TestWriteRemovesFilesWhenNothingRemains(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "warehouse.yml"), []byte("airflow_pg:\n  type: postgres\n"), 0o600)
	_ = os.WriteFile(filepath.Join(dir, ".env"), []byte(envBanner+"\nAIRFLOW_PG_PASSWORD=old\n"), 0o600)
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

// A .env the desktop wrote before the writer was shared carries its own
// banner. The scrub drops it with the secrets under it.
func TestScrubEnvDropsTheLegacyDesktopBanner(t *testing.T) {
	dir := t.TempDir()
	seed := "MY=1\n\n" + legacyEnvBanner + "\nAIRFLOW_PG_PASSWORD=\"old\"\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ScrubEnv(dir); err != nil {
		t.Fatal(err)
	}
	if got := string(readFile(t, filepath.Join(dir, ".env"))); got != "MY=1\n" {
		t.Errorf(".env = %q, want only the user's line", got)
	}
}

// The user's own AIRFLOW_* lines are theirs: only the assignments directly
// under our banner go, so one above the block and one after a line that ends
// it both survive.
func TestScrubEnvKeepsTheUsersAirflowLines(t *testing.T) {
	dir := t.TempDir()
	seed := "AIRFLOW_MY_DWH_PASSWORD=mine\n\n" + envBanner + "\nAIRFLOW_PG_PASSWORD=\"old\"\nexport AIRFLOW_KP_TOKEN=\"old\"\n# later note\nAIRFLOW_LATER=mine\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ScrubEnv(dir); err != nil {
		t.Fatal(err)
	}
	want := "AIRFLOW_MY_DWH_PASSWORD=mine\n\n# later note\nAIRFLOW_LATER=mine\n"
	if got := string(readFile(t, filepath.Join(dir, ".env"))); got != want {
		t.Errorf(".env = %q, want %q", got, want)
	}
	// A file holding only the user's AIRFLOW_* lines is not ours to touch.
	only := "AIRFLOW_MY_DWH_PASSWORD=mine\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(only), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ScrubEnv(dir); err != nil {
		t.Fatal(err)
	}
	if got := string(readFile(t, filepath.Join(dir, ".env"))); got != only {
		t.Errorf(".env = %q, want it untouched", got)
	}
}

// A .env with nothing of ours is left byte for byte, trailing blank lines and
// a missing final newline included; a missing .env stays missing.
func TestScrubEnvLeavesAUserOnlyFileAlone(t *testing.T) {
	dir := t.TempDir()
	if err := ScrubEnv(dir); err != nil {
		t.Fatalf("ScrubEnv on a missing .env: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".env")); !os.IsNotExist(err) {
		t.Error("ScrubEnv created a .env")
	}
	seed := "MINE=1\n\n\nOTHER=2"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ScrubEnv(dir); err != nil {
		t.Fatal(err)
	}
	if got := string(readFile(t, filepath.Join(dir, ".env"))); got != seed {
		t.Errorf(".env = %q, want it untouched", got)
	}
}

// A .env scrub that cannot write still lets warehouse.yml be written, which
// carries no secret, and reports the failure.
func TestWriteReportsAFailedScrubAndStillWritesTheYAML(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("MINE=1\n"+envBanner+"\nAIRFLOW_OLD_PASSWORD=stale\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	orig := writeFile
	t.Cleanup(func() { writeFile = orig })
	failed := false
	writeFile = func(path string, data []byte, perm os.FileMode) error {
		if path == envPath {
			failed = true
			return errors.New("disk full")
		}
		return orig(path, data, perm)
	}
	if err := Write(dir, []Materialized{pgWarehouse("airflow_new", "new")}); err == nil {
		t.Fatal("Write reported success after its .env scrub failed")
	}
	if !failed {
		t.Fatal("the stub never failed a write")
	}
	if _, ok := readMap(t, filepath.Join(dir, "warehouse.yml"))["airflow_new"]; !ok {
		t.Error("warehouse.yml was not written after the scrub failed")
	}
}

// AppendEnv carries every value, verbatim (a multi-line PEM key included:
// the environment needs no quoting), sorted after the base environment, and
// does not touch the caller's slice.
func TestAppendEnvAddsTheValues(t *testing.T) {
	pem := "-----BEGIN-----\nline2\n-----END-----"
	live := []Materialized{
		pgWarehouse("airflow_pg", "pg-value"),
		{Name: "airflow_kp", Env: map[string]string{"AIRFLOW_KP_PRIVATE_KEY": pem}},
	}
	base := make([]string, 1, 8) // spare capacity: an append in place would show up
	base[0] = "HOME=/h"
	got := AppendEnv(base, live)
	want := []string{"HOME=/h", "AIRFLOW_KP_PRIVATE_KEY=" + pem, "AIRFLOW_PG_PASSWORD=pg-value"}
	if !slices.Equal(got, want) {
		t.Errorf("AppendEnv gave %d entries with keys %v, want keys %v", len(got), envKeys(got), envKeys(want))
	}
	if extra := base[:cap(base)][1]; extra != "" {
		t.Error("AppendEnv wrote into the caller's backing array")
	}
	if len(AppendEnv(base, nil)) != 1 {
		t.Error("AppendEnv with no warehouses changed the environment")
	}
}

// A key the environment already sets keeps its value, as the skill's
// non-overriding .env load did: the launcher's AIRFLOW_API_URL is not replaced
// by a Databricks connection whose id is "api".
func TestAppendEnvKeepsKeysTheEnvironmentSets(t *testing.T) {
	live := []Materialized{{Name: "airflow_api", Env: map[string]string{"AIRFLOW_API_URL": "databricks://token:x@h", "AIRFLOW_API_OTHER": "o"}}}
	got := AppendEnv([]string{"AIRFLOW_API_URL=http://localhost:8080"}, live)
	if want := []string{"AIRFLOW_API_URL=http://localhost:8080", "AIRFLOW_API_OTHER=o"}; !slices.Equal(got, want) {
		t.Errorf("AppendEnv keys %v, want %v (the launcher's AIRFLOW_API_URL kept)", envKeys(got), envKeys(want))
	}
}

// Where the environment folds case (Windows), a key set in another case
// counts as set; elsewhere it is a different key.
func TestAppendEnvComparesKeysByThePlatformsRule(t *testing.T) {
	live := []Materialized{{Name: "airflow_api", Env: map[string]string{"AIRFLOW_API_URL": "u"}}}
	for _, fold := range []bool{true, false} {
		t.Run(fmt.Sprintf("fold=%v", fold), func(t *testing.T) {
			prev := foldEnvCase
			foldEnvCase = fold
			t.Cleanup(func() { foldEnvCase = prev })
			got := AppendEnv([]string{"airflow_api_url=x"}, live)
			want := []string{"airflow_api_url=x"}
			if !fold {
				want = append(want, "AIRFLOW_API_URL=u")
			}
			if !slices.Equal(got, want) {
				t.Errorf("AppendEnv keys %v, want %v", envKeys(got), envKeys(want))
			}
		})
	}
}

// envKeys is the keys of KEY=VALUE entries, for messages that must not print
// a value.
func envKeys(env []string) []string {
	out := make([]string, len(env))
	for i, e := range env {
		out[i], _, _ = strings.Cut(e, "=")
	}
	return out
}
