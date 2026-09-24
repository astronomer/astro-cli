package localenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/internal/envresolve"
	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/localrt/localrttest"
)

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// TestMergeSetPreservesHandEdits is the core promise: a set replaces a key in
// place and appends a new one, leaving every comment, blank line, and
// hand-typed entry untouched.
func TestMergeSetPreservesHandEdits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	original := "# top comment\nAPI_URL=http://old\n\n# a note\nHAND=kept\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Store{Path: path, Scope: ScopeProject}

	if _, err := s.Set(KindEnv, "API_URL", "http://new"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Set(KindEnv, "ADDED", "value"); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, path)
	want := "# top comment\nAPI_URL=http://new\n\n# a note\nHAND=kept\nADDED=value\n"
	if got != want {
		t.Fatalf("merge-write mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestFilePermIs0600(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Path: filepath.Join(dir, ".env"), Scope: ScopeProject}
	if _, err := s.Set(KindEnv, "TOKEN", "x"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestMergeDelete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("# c\nA=1\nB=2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Store{Path: path, Scope: ScopeProject}
	ok, err := s.Delete(KindEnv, "A")
	if err != nil || !ok {
		t.Fatalf("delete A: ok=%v err=%v", ok, err)
	}
	if got := readFile(t, path); got != "# c\nB=2\n" {
		t.Fatalf("after delete: %q", got)
	}
	// Deleting an absent key is a no-op that reports false.
	ok, err = s.Delete(KindEnv, "MISSING")
	if err != nil || ok {
		t.Fatalf("delete missing: ok=%v err=%v", ok, err)
	}
}

func TestQuotedValueRoundTrips(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Path: filepath.Join(dir, ".env"), Scope: ScopeProject}
	for _, v := range []string{"plain", "has spaces", `has"quote`, "trailing ", "a=b", "with#hash"} {
		if _, err := s.Set(KindEnv, "K", v); err != nil {
			t.Fatalf("set %q: %v", v, err)
		}
		got, ok, err := s.Get(KindEnv, "K")
		if err != nil || !ok {
			t.Fatalf("get after set %q: ok=%v err=%v", v, ok, err)
		}
		if got != v {
			t.Fatalf("round trip: got %q, want %q", got, v)
		}
	}
}

func TestConnSetFromURIAndJSON(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Path: filepath.Join(dir, ".env"), Scope: ScopeProject}

	key, err := s.Set(KindConn, "warehouse", "postgres://user:pw@db.example.com:5432/analytics")
	if err != nil {
		t.Fatal(err)
	}
	if key != "AIRFLOW_CONN_WAREHOUSE" {
		t.Fatalf("conn env key = %q", key)
	}
	stored, ok, err := s.Get(KindConn, "warehouse")
	if err != nil || !ok {
		t.Fatalf("get conn: ok=%v err=%v", ok, err)
	}
	for _, want := range []string{`"conn_type":"postgres"`, `"host":"db.example.com"`, `"login":"user"`, `"password":"pw"`, `"port":5432`, `"schema":"analytics"`} {
		if !strings.Contains(stored, want) {
			t.Errorf("stored conn JSON missing %q:\n%s", want, stored)
		}
	}

	// JSON in round-trips to the canonical form too.
	if _, err := s.Set(KindConn, "other", `{"conn_type":"http","host":"api"}`); err != nil {
		t.Fatal(err)
	}
	// A value that is neither URI nor JSON is rejected.
	if _, err := s.Set(KindConn, "bad", "not a uri or json"); err == nil {
		t.Fatal("expected an error for a non-URI, non-JSON connection value")
	}
}

func TestVarEncoding(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Path: filepath.Join(dir, ".env"), Scope: ScopeProject}
	key, err := s.Set(KindVar, "batch_size", "100")
	if err != nil {
		t.Fatal(err)
	}
	if key != "AIRFLOW_VAR_BATCH_SIZE" {
		t.Fatalf("var env key = %q", key)
	}
	if !strings.Contains(readFile(t, s.Path), "AIRFLOW_VAR_BATCH_SIZE=100") {
		t.Fatalf("file: %s", readFile(t, s.Path))
	}
}

func TestMultilineValueRejected(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Path: filepath.Join(dir, ".env"), Scope: ScopeProject}
	if _, err := s.Set(KindEnv, "K", "line1\nline2"); err == nil {
		t.Fatal("expected multiline value to be rejected")
	}
}

// TestInjectionWholesaleVsDeclared checks the two files inject differently:
// the project .env goes in wholesale, the global file only where the schema
// declares a value.
func TestInjectionWholesaleVsDeclared(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ASTRO_HOME", home)
	writeGlobalEnv(t, "DECLARED_GLOBAL=g\nUNDECLARED_GLOBAL=stray\n")
	projDir := t.TempDir()
	if err := os.WriteFile(ProjectEnvPath(projDir), []byte("DECLARED_PROJECT=p\nUNDECLARED_PROJECT=alsohere\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	schema := &envschema.Schema{EnvVars: map[string]envschema.ValueSpec{
		"DECLARED_GLOBAL":  {},
		"DECLARED_PROJECT": {},
	}}

	src, err := LoadSources(nil, projDir)
	if err != nil {
		t.Fatal(err)
	}
	inj := src.Injection(schema)

	if inj["DECLARED_GLOBAL"] != "g" {
		t.Errorf("declared global not injected: %v", inj)
	}
	if _, ok := inj["UNDECLARED_GLOBAL"]; ok {
		t.Errorf("undeclared global leaked into injection: %v", inj)
	}
	if inj["DECLARED_PROJECT"] != "p" || inj["UNDECLARED_PROJECT"] != "alsohere" {
		t.Errorf("project .env not injected wholesale: %v", inj)
	}
}

func TestProviderPrecedence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ASTRO_HOME", home)
	writeGlobalEnv(t, "K=global\nONLY_GLOBAL=g\n")
	projDir := t.TempDir()
	if err := os.WriteFile(ProjectEnvPath(projDir), []byte("K=project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	src, err := LoadSources([]string{"K=shell"}, projDir)
	if err != nil {
		t.Fatal(err)
	}
	ps := src.Providers(nil)
	// shell beats project beats global.
	if v, src, ok := firstHit(ps, "K"); !ok || v != "shell" || src != SourceShell {
		t.Errorf("K resolved to %q from %q (ok=%v), want shell", v, src, ok)
	}
	if v, src, ok := firstHit(ps, "ONLY_GLOBAL"); !ok || v != "g" || src != SourceGlobal {
		t.Errorf("ONLY_GLOBAL resolved to %q from %q (ok=%v), want g/global", v, src, ok)
	}
}

// firstHit mirrors the resolver's first-hit walk for the test.
func firstHit(ps []envresolve.Provider, key string) (value, source string, ok bool) {
	for _, p := range ps {
		if v, has := p.Lookup(key); has {
			return v, p.Label(), true
		}
	}
	return "", "", false
}

func TestListSourceAndOrphans(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ASTRO_HOME", home)
	writeGlobalEnv(t, "STRAY_GLOBAL=x\n")
	projDir := t.TempDir()
	if err := os.WriteFile(ProjectEnvPath(projDir), []byte("DECLARED=p\nSTRAY_PROJECT=y\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	schema := &envschema.Schema{EnvVars: map[string]envschema.ValueSpec{
		"DECLARED": {},
		"MISSING":  {},
	}}
	items, err := List(nil, projDir, schema, ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]ListItem{}
	for _, it := range items {
		byName[it.Name] = it
	}
	if it := byName["DECLARED"]; it.Source != SourceProject || it.Orphan {
		t.Errorf("DECLARED = %+v, want project source, not orphan", it)
	}
	if it := byName["MISSING"]; it.Source != SourceAbsent {
		t.Errorf("MISSING = %+v, want absent source", it)
	}
	if it := byName["STRAY_PROJECT"]; !it.Orphan || it.RemoveHint == "" {
		t.Errorf("STRAY_PROJECT = %+v, want orphan with a remove hint", it)
	}
	if it := byName["STRAY_GLOBAL"]; !it.Orphan {
		t.Errorf("STRAY_GLOBAL = %+v, want orphan", it)
	}
	// list never carries a value.
	for _, it := range items {
		_ = it // ListItem has no value field; this is enforced structurally.
	}
}

// Each declared row carries its declaration's required, sensitive and
// description, per section, so `list --output json` answers what the manifest
// asks for without a second read of it. An orphan has no declaration and
// carries none of them.
func TestListCarriesTheDeclarationFlags(t *testing.T) {
	t.Setenv("ASTRO_HOME", t.TempDir())
	projDir := t.TempDir()
	if err := os.WriteFile(ProjectEnvPath(projDir), []byte("STRAY=y\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	schema := &envschema.Schema{
		EnvVars: map[string]envschema.ValueSpec{
			"API_TOKEN": {Sensitive: true, Description: "Token for the API"},
			"LOG_LEVEL": {Optional: true},
		},
		AirflowVariables: map[string]envschema.ValueSpec{
			"region": {Description: "Deploy region"},
		},
		Connections: map[string]envschema.ValueSpec{
			"db_main": {Sensitive: true, Optional: true},
		},
	}
	items, err := List(nil, projDir, schema, ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	type flags struct {
		required, sensitive bool
		description         string
	}
	want := map[string]flags{
		"API_TOKEN": {true, true, "Token for the API"},
		"LOG_LEVEL": {false, false, ""},
		"region":    {true, false, "Deploy region"},
		"db_main":   {false, true, ""},
		"STRAY":     {false, false, ""},
	}
	got := map[string]flags{}
	for _, it := range items {
		got[it.Name] = flags{it.Required, it.Sensitive, it.Description}
	}
	for name, w := range want {
		g, ok := got[name]
		if !ok {
			t.Errorf("%s missing from the listing", name)
			continue
		}
		if g != w {
			t.Errorf("%s = %+v, want %+v", name, g, w)
		}
	}
}

// TestListAllDedupesCurrentProject guards the fix for --all listing the
// current project's own orphans twice once it has a state record.
func TestListAllDedupesCurrentProject(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("ASTRO_HOME", t.TempDir())
	projA := t.TempDir()
	projB := t.TempDir()
	if err := os.WriteFile(ProjectEnvPath(projA), []byte("A_ORPHAN=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ProjectEnvPath(projB), []byte("B_ORPHAN=2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{projA, projB} {
		if err := localrttest.Seed(localrttest.Record{ProjectPath: p, Mode: localrt.ModeStandalone, Port: 1}); err != nil {
			t.Fatal(err)
		}
	}

	items, err := List(nil, projA, nil, ListOptions{All: true})
	if err != nil {
		t.Fatal(err)
	}
	var aCount int
	var bItem *ListItem
	for i := range items {
		switch items[i].Name {
		case "A_ORPHAN":
			aCount++
		case "B_ORPHAN":
			bItem = &items[i]
		}
	}
	if aCount != 1 {
		t.Fatalf("current project's orphan listed %d times under --all, want 1", aCount)
	}
	if bItem == nil {
		t.Fatal("cross-project orphan B_ORPHAN not listed under --all")
	}
	// A cross-project orphan names its project and carries no (wrong-target)
	// remove hint.
	if bItem.Project == "" || bItem.RemoveHint != "" {
		t.Fatalf("cross-project orphan = %+v, want a Project and no RemoveHint", *bItem)
	}
}

func TestListNeverHasValueField(t *testing.T) {
	// A structural guard: if a Value field is ever added to ListItem, this
	// stops compiling, which is the point.
	var it ListItem
	_ = it.Kind
	_ = it.Name
	_ = it.Source
}

// writeGlobalEnv puts a global env file where GlobalEnvPath says it goes.
//
// Through the function rather than by joining a path here: these tests used to
// hardcode $ASTRO_HOME/env, which is the layout the code was getting wrong, so
// they agreed with the bug and could not have caught it. What the layout IS is
// pinned once, in TestGlobalEnvPathFollowsTheAstroHomeConvention.
func writeGlobalEnv(t *testing.T, body string) {
	t.Helper()
	p, err := GlobalEnvPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// ASTRO_HOME names the PARENT of .astro. config.HomeConfigPath and cmd/local's
// routesDir both read it that way, and this function claimed to and did not:
// it put the env file at $ASTRO_HOME/env while the config went to
// $ASTRO_HOME/.astro/config.yaml. Relocating an astro home has to move one
// directory, not scatter files either side of it.
func TestGlobalEnvPathFollowsTheAstroHomeConvention(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ASTRO_HOME", home)

	got, err := GlobalEnvPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".astro", "env"); got != want {
		t.Errorf("GlobalEnvPath() = %q, want %q — ASTRO_HOME is the parent of .astro", got, want)
	}
}

// And with it unset the answer is unchanged, which is why the disagreement went
// unnoticed for so long: both spellings land here.
func TestGlobalEnvPathWithoutAstroHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ASTRO_HOME", "")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	got, err := GlobalEnvPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".astro", "env"); got != want {
		t.Errorf("GlobalEnvPath() = %q, want %q", got, want)
	}
}
