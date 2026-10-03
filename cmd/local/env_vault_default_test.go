package local

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/internal/vaultenv"
	"github.com/astronomer/astro-cli/pkg/secrets"
)

// vaultDefaultCases are the three kinds, each with a value and the env key a
// plaintext copy would carry.
var vaultDefaultCases = []struct {
	noun, name, value, envKey string
}{
	{"variable", "API_TOKEN", "s3cr3t", "API_TOKEN"},
	{"connection", "db_main", "postgres://u:p@h:5432/db", "AIRFLOW_CONN_DB_MAIN"},
	{"airflow-variable", "region", "us-east-1", "AIRFLOW_VAR_REGION"},
}

// vaultMetas is what the shared vault lists, with each entry's plain marker.
func vaultMetas(t *testing.T) []secrets.Meta {
	t.Helper()
	dir, err := secrets.DefaultDir()
	if err != nil {
		t.Fatal(err)
	}
	store, err := secrets.NewKeyringStore(secrets.Config{Service: secrets.DefaultService, Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	metas, err := store.ListMeta()
	if err != nil {
		t.Fatal(err)
	}
	return metas
}

// legacyGlobalPath is ~/.astro/env under the test's isolated home: the file an
// older build wrote, which nothing reads or writes now.
func legacyGlobalPath(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(home, ".astro", "env")
}

// writeLegacyGlobal writes ~/.astro/env as an older build would have.
func writeLegacyGlobal(t *testing.T, body string) {
	t.Helper()
	p := legacyGlobalPath(t)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readLegacyGlobal(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(legacyGlobalPath(t))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Every kind, set with no store flag, goes to the vault encrypted, in both
// scopes, and nothing reaches a plaintext file.
func TestSetStoresEveryKindInTheVaultByDefault(t *testing.T) {
	for _, tc := range vaultDefaultCases {
		for _, scope := range []string{"--project", "--global"} {
			t.Run(tc.noun+" "+scope, func(t *testing.T) {
				dir := secretEnvProject(t, "")
				d, out, _ := envDeps(t, dir, "")
				if err := execute(t, d, "local", "env", tc.noun, "set", tc.name, "--value", tc.value, scope, "--output", "json"); err != nil {
					t.Fatal(err)
				}
				want := vaultenv.SourceProject
				if scope == "--global" {
					want = vaultenv.SourceGlobal
				}
				assertOneEncryptedSet(t, dir, out.Bytes(), tc.envKey, want)
			})
		}
	}
}

// assertOneEncryptedSet checks a default set: one encrypted vault entry, no
// plaintext copy anywhere, and the set's JSON naming the vault tier.
func assertOneEncryptedSet(t *testing.T, dir string, out []byte, envKey, wantScope string) {
	t.Helper()
	if keys := dotenvKeys(t, filepath.Join(dir, ".env")); hasKey(keys, envKey) {
		t.Errorf("a default set wrote the plaintext .env (keys: %v)", keys)
	}
	if _, err := os.Stat(legacyGlobalPath(t)); !os.IsNotExist(err) {
		t.Errorf("a default set wrote ~/.astro/env (stat: %v)", err)
	}
	if metas := vaultMetas(t); len(metas) != 1 || metas[0].Plain {
		t.Fatalf("vault = %+v, want one encrypted entry", metas)
	}
	var res envResult
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("decode the set result: %v", err)
	}
	if string(res.Scope) != wantScope {
		t.Errorf("set reported scope %q, want %q", res.Scope, wantScope)
	}
}

// --plain in a project goes to the project's .env, every kind.
func TestPlainProjectSetGoesToTheDotenv(t *testing.T) {
	for _, tc := range vaultDefaultCases {
		t.Run(tc.noun, func(t *testing.T) {
			dir := secretEnvProject(t, "")
			d, _, _ := envDeps(t, dir, "")
			if err := execute(t, d, "local", "env", tc.noun, "set", tc.name, "--value", tc.value, "--plain"); err != nil {
				t.Fatal(err)
			}
			if keys := dotenvKeys(t, filepath.Join(dir, ".env")); !hasKey(keys, tc.envKey) {
				t.Errorf("--plain should store it in .env (keys: %v)", keys)
			}
			if n := len(vaultFiles(t)); n != 0 {
				t.Errorf("--plain still reached the vault: %d entries", n)
			}
		})
	}
}

// --plain --global goes to the vault, marked plain and stored as written, and
// never to ~/.astro/env.
func TestPlainGlobalSetGoesToTheVaultMarkedPlain(t *testing.T) {
	dir := secretEnvProject(t, "")
	d, _, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "variable", "set", "LOG_LEVEL", "--value", "debug", "--plain", "--global"); err != nil {
		t.Fatal(err)
	}
	metas := vaultMetas(t)
	if len(metas) != 1 || !metas[0].Plain || metas[0].Key != "env:global:LOG_LEVEL" {
		t.Fatalf("vault = %+v, want one plain global", metas)
	}
	if _, err := os.Stat(legacyGlobalPath(t)); !os.IsNotExist(err) {
		t.Errorf("a plain global wrote ~/.astro/env (stat: %v)", err)
	}
	if got := getJSON(t, dir, "LOG_LEVEL", "--global"); got.Value != "debug" || got.Source != vaultenv.SourceGlobal {
		t.Errorf("get --global = %+v, want the plain global", got)
	}
	// A default set of the same name re-encrypts it in place: one entry.
	d, _, _ = envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "variable", "set", "LOG_LEVEL", "--value", "info", "--global"); err != nil {
		t.Fatal(err)
	}
	if metas := vaultMetas(t); len(metas) != 1 || metas[0].Plain {
		t.Fatalf("vault = %+v, want the one entry, now encrypted", metas)
	}
}

// --secret is a deprecated no-op: it stores in the vault, as no flag does.
// --secret=false still means plain, and --plain with --secret is refused.
func TestTheSecretFlagIsADeprecatedAlias(t *testing.T) {
	dir := secretEnvProject(t, "")
	d, _, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "variable", "set", "TOKEN", "--value", "x", "--secret"); err != nil {
		t.Fatal(err)
	}
	if n := len(vaultFiles(t)); n != 1 {
		t.Errorf("--secret should store in the vault: %d entries", n)
	}
	// Deprecated, so cobra prints the notice on use and help leaves it out.
	for _, verb := range []string{"set", "get", "delete"} {
		cmd, _, err := newRootCmd(d).Find([]string{"local", "env", "variable", verb})
		if err != nil {
			t.Fatal(err)
		}
		if f := cmd.Flags().Lookup("secret"); f == nil || f.Deprecated == "" || !strings.Contains(f.Deprecated, "--plain") {
			t.Errorf("%s --secret = %+v, want it deprecated in favor of --plain", verb, f)
		}
	}

	d, _, _ = envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "variable", "set", "LOG_LEVEL", "--value", "debug", "--secret=false"); err != nil {
		t.Fatal(err)
	}
	if keys := dotenvKeys(t, filepath.Join(dir, ".env")); !hasKey(keys, "LOG_LEVEL") {
		t.Errorf("--secret=false should keep its meaning, the plain .env (keys: %v)", keys)
	}

	for _, verb := range [][]string{{"set", "X", "--value", "1"}, {"get", "X"}, {"delete", "X"}} {
		d, _, _ = envDeps(t, dir, "")
		err := execute(t, d, append(append([]string{"local", "env", "variable"}, verb...), "--plain", "--secret")...)
		if err == nil || !strings.Contains(err.Error(), "--plain and --secret") {
			t.Errorf("%s --plain --secret = %v, want the conflict refused", verb[0], err)
		}
	}
	if keys := dotenvKeys(t, filepath.Join(dir, ".env")); hasKey(keys, "X") {
		t.Errorf("a refused set wrote .env (keys: %v)", keys)
	}
}

// delete in a project clears the name from both of its stores.
func TestDeleteClearsBothProjectStores(t *testing.T) {
	dir := secretEnvProject(t, "")

	d, _, _ := envDeps(t, dir, "v\n")
	if err := execute(t, d, "local", "env", "variable", "set", "TOKEN"); err != nil {
		t.Fatal(err)
	}
	writeEnvFile(t, dir, "TOKEN=hand\n")
	d, _, _ = envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "variable", "delete", "TOKEN"); err != nil {
		t.Fatalf("delete should find the vaulted copy: %v", err)
	}
	if n := len(vaultFiles(t)); n != 0 {
		t.Errorf("delete left %d vault entries", n)
	}
	if keys := dotenvKeys(t, filepath.Join(dir, ".env")); hasKey(keys, "TOKEN") {
		t.Errorf("delete left the plaintext copy (keys: %v)", keys)
	}

	d, _, _ = envDeps(t, dir, "")
	err := execute(t, d, "local", "env", "variable", "delete", "TOKEN")
	if err == nil || !strings.Contains(err.Error(), "or the vault") {
		t.Errorf("deleting what neither store holds should say both were checked: %v", err)
	}
}

// A ~/.astro/env left by an older build is ignored, silently: list shows
// none of it, delete --global finds nothing there, set --global of a name it
// holds is an ordinary new global, no command mentions the file, and the file
// is left byte for byte.
func TestTheLegacyGlobalFileIsIgnored(t *testing.T) {
	dir := secretEnvProject(t, "")
	const legacy = "OLD_ONLY=1\nMOVED=old\n"
	writeLegacyGlobal(t, legacy)
	noMention := func(what, s string) {
		t.Helper()
		if strings.Contains(s, ".astro/env") || strings.Contains(s, "OLD_ONLY") {
			t.Errorf("%s mentions the legacy file or its contents: %q", what, s)
		}
	}

	d, out, stderr := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "list"); err != nil {
		t.Fatal(err)
	}
	noMention("list", out.String()+stderr.String())

	d, _, stderr = envDeps(t, dir, "")
	err := execute(t, d, "local", "env", "variable", "delete", "OLD_ONLY", "--global")
	if err == nil || !strings.Contains(err.Error(), "not set") {
		t.Errorf("delete --global of a name only ~/.astro/env holds = %v, want not set", err)
	}
	if err != nil {
		noMention("delete", strings.ReplaceAll(err.Error(), `"OLD_ONLY"`, "")+stderr.String())
	}

	d, _, stderr = envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "variable", "set", "MOVED", "--value", "new", "--global"); err != nil {
		t.Fatal(err)
	}
	noMention("set", stderr.String())
	if r := reachOf(t, "env:global:MOVED"); r.Everywhere {
		t.Errorf("set --global of a name ~/.astro/env holds reaches %+v, want a new global's empty row", r)
	}

	if got := readLegacyGlobal(t); got != legacy {
		t.Errorf("~/.astro/env changed:\n%s\nwant\n%s", got, legacy)
	}
}

// assertReplacedPlain checks the vault after a --replace-secret set: a global
// is now one plain entry, and a project's encrypted copy is gone, replaced by
// its .env.
func assertReplacedPlain(t *testing.T, global bool) {
	t.Helper()
	metas := vaultMetas(t)
	if global && (len(metas) != 1 || !metas[0].Plain) {
		t.Errorf("global vault = %+v, want the entry now plain", metas)
	}
	if !global && len(metas) != 0 {
		t.Errorf("project vault = %+v, want the encrypted copy replaced by .env", metas)
	}
}

// --plain never silently turns an encrypted value into cleartext, in either
// scope: it is refused and the encrypted entry is left as it was, and
// --replace-secret is the explicit way through.
func TestPlainSetRefusesToReplaceAnEncryptedValue(t *testing.T) {
	for _, scope := range [][]string{{"--project"}, {"--global", "--auto-link"}} {
		t.Run(scope[0], func(t *testing.T) {
			dir := secretEnvProject(t, "")
			set := func(extra ...string) error {
				d, _, _ := envDeps(t, dir, "")
				args := append([]string{"local", "env", "variable", "set", "TOKEN", "--value", "v"}, scope...)
				return execute(t, d, append(args, extra...)...)
			}
			if err := set(); err != nil {
				t.Fatal(err)
			}
			err := set("--plain")
			if err == nil || !strings.Contains(err.Error(), "stored encrypted") || !strings.Contains(err.Error(), "--replace-secret") {
				t.Fatalf("--plain over an encrypted value = %v, want it refused naming --replace-secret", err)
			}
			if metas := vaultMetas(t); len(metas) != 1 || metas[0].Plain {
				t.Fatalf("after the refusal, vault = %+v, want the encrypted entry untouched", metas)
			}
			if keys := dotenvKeys(t, filepath.Join(dir, ".env")); hasKey(keys, "TOKEN") {
				t.Errorf("a refused set wrote .env (keys: %v)", keys)
			}

			if err := set("--plain", "--replace-secret"); err != nil {
				t.Fatalf("--replace-secret should allow it: %v", err)
			}
			assertReplacedPlain(t, scope[0] == "--global")
			// A plain value replaced by plain needs no flag.
			if err := set("--plain"); err != nil {
				t.Errorf("--plain over a plain value: %v", err)
			}
		})
	}
	// The override only means something with --plain.
	dir := secretEnvProject(t, "")
	d, _, _ := envDeps(t, dir, "")
	if err := execute(t, d, "local", "env", "variable", "set", "X", "--value", "v", "--replace-secret"); err == nil {
		t.Error("--replace-secret without --plain should be refused")
	}
}

// --auto-link is the desktop's term for a global with no link row, on set and
// link alike, and get labels that reach the same way. --everywhere, the name
// it had before it shipped, is gone.
func TestAutoLinkIsTheFlagAndTheLabel(t *testing.T) {
	dir := secretEnvProject(t, "")
	mustRun(t, dir, "variable", "set", "SHARED", "--value", "v", "--global", "--auto-link")
	if r := reachOf(t, "env:global:SHARED"); !r.Everywhere {
		t.Errorf("set --auto-link reach = %+v, want no row", r)
	}
	if _, stderr := mustRun(t, dir, "variable", "get", "SHARED"); !strings.Contains(stderr, "Reach: auto-linked (every project)") {
		t.Errorf("get stderr = %q, want the auto-linked label", stderr)
	}

	mustRun(t, dir, "variable", "link", "SHARED")
	if r := reachOf(t, "env:global:SHARED"); r.Everywhere {
		t.Fatalf("link reach = %+v, want a row", r)
	}
	if out, _ := mustRun(t, dir, "variable", "link", "SHARED", "--auto-link"); !strings.Contains(out, "is now auto-linked to every project") {
		t.Errorf("link --auto-link out = %q", out)
	}
	if r := reachOf(t, "env:global:SHARED"); !r.Everywhere {
		t.Errorf("link --auto-link reach = %+v, want no row", r)
	}

	for _, args := range [][]string{
		{"variable", "set", "OTHER", "--value", "v", "--global", "--everywhere"},
		{"variable", "link", "SHARED", "--everywhere"},
	} {
		if _, _, err := run(t, dir, args...); err == nil || !strings.Contains(err.Error(), "unknown flag: --everywhere") {
			t.Errorf("%v = %v, want --everywhere unknown", args, err)
		}
	}
}
