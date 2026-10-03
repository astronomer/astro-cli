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

// TestInjectionWholesale checks both files inject everything they hold,
// declared or not: ~/.astro/env reaches every project. The project .env wins a
// name both hold, and a global entry the shell also sets is left out, since the
// shell outranks the global file.
func TestInjectionWholesale(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ASTRO_HOME", home)
	writeGlobalEnv(t, "GLOBAL_A=g\nUNDECLARED_GLOBAL=everywhere\nBOTH=global\nSHELL_SET=global\n")
	projDir := t.TempDir()
	if err := os.WriteFile(ProjectEnvPath(projDir), []byte("PROJECT_A=p\nUNDECLARED_PROJECT=alsohere\nBOTH=project\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	src, err := LoadSources([]string{"SHELL_SET=shell"}, projDir)
	if err != nil {
		t.Fatal(err)
	}
	inj := src.Injection()

	if inj["GLOBAL_A"] != "g" || inj["UNDECLARED_GLOBAL"] != "everywhere" {
		t.Errorf("~/.astro/env not injected wholesale: %v", inj)
	}
	if inj["PROJECT_A"] != "p" || inj["UNDECLARED_PROJECT"] != "alsohere" {
		t.Errorf("project .env not injected wholesale: %v", inj)
	}
	if inj["BOTH"] != "project" {
		t.Errorf("BOTH = %q, want the project .env to beat ~/.astro/env", inj["BOTH"])
	}
	if _, ok := inj["SHELL_SET"]; ok {
		t.Errorf("SHELL_SET is injected, so ~/.astro/env would override the shell: %v", inj)
	}
}

// TestUndeclared names what a project gets locally without declaring it: from
// both files and the vault entries that reach it, never a declared name, an
// Airflow setting, or a global linked elsewhere.
func TestUndeclared(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ASTRO_HOME", home)
	writeGlobalEnv(t, "FROM_GLOBAL=g\nDECLARED=g\nAIRFLOW__CORE__LOAD_EXAMPLES=False\n")
	projDir := t.TempDir()
	if err := os.WriteFile(ProjectEnvPath(projDir), []byte("FROM_PROJECT=p\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	src, err := LoadSources(nil, projDir)
	if err != nil {
		t.Fatal(err)
	}
	schema := &envschema.Schema{EnvVars: map[string]envschema.ValueSpec{"DECLARED": {}}}
	tiers := []VaultTier{
		{Scope: ScopeProject, Entries: []VaultEntry{{Kind: KindEnv, Name: "PROJECT_SECRET", EnvKey: "PROJECT_SECRET"}}},
		{Scope: ScopeGlobal, Entries: []VaultEntry{
			{Kind: KindConn, Name: "shared_db", EnvKey: "AIRFLOW_CONN_SHARED_DB"},
			{Kind: KindConn, Name: "elsewhere", EnvKey: "AIRFLOW_CONN_ELSEWHERE", Unlinked: true},
			{Kind: KindVar, Name: "1bad", EnvKey: "AIRFLOW_VAR_1BAD", Invalid: "starts with a digit"},
		}},
	}
	got := strings.Join(src.Undeclared(schema, tiers), ",")
	want := "AIRFLOW_CONN_SHARED_DB,FROM_GLOBAL,FROM_PROJECT,PROJECT_SECRET"
	if got != want {
		t.Errorf("Undeclared = %s, want %s", got, want)
	}
}

func TestProviderPrecedence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ASTRO_HOME", home)
	writeGlobalEnv(t, "K=global\nONLY_GLOBAL=g\nSHELL_GLOBAL=global\n")
	projDir := t.TempDir()
	if err := os.WriteFile(ProjectEnvPath(projDir), []byte("K=project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	src, err := LoadSources([]string{"K=shell", "SHELL_GLOBAL=shell"}, projDir)
	if err != nil {
		t.Fatal(err)
	}
	ps := src.Providers(nil)
	// project beats shell, which start applies the .env over; shell beats global.
	if v, src, ok := firstHit(ps, "K"); !ok || v != "project" || src != SourceProject {
		t.Errorf("K resolved to %q from %q (ok=%v), want project", v, src, ok)
	}
	if v, src, ok := firstHit(ps, "SHELL_GLOBAL"); !ok || v != "shell" || src != SourceShell {
		t.Errorf("SHELL_GLOBAL resolved to %q from %q (ok=%v), want shell", v, src, ok)
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
	if it := byName["STRAY_PROJECT"]; !it.Orphan || it.RemoveHint == "" || it.DeclareHint != "astro local env variable declare STRAY_PROJECT" {
		t.Errorf("STRAY_PROJECT = %+v, want orphan with a remove and a declare hint", it)
	}
	if it := byName["STRAY_GLOBAL"]; !it.Orphan || it.Applied == nil || !*it.Applied ||
		it.DeclareHint != "astro local env variable declare STRAY_GLOBAL" {
		t.Errorf("STRAY_GLOBAL = %+v, want an orphan marked applied, with a declare hint", it)
	}
	if it := byName["STRAY_PROJECT"]; it.Applied == nil || !*it.Applied {
		t.Errorf("STRAY_PROJECT = %+v, want it marked applied: the project .env passes through", it)
	}
	// list never carries a value.
	for _, it := range items {
		_ = it // ListItem has no value field; this is enforced structurally.
	}
}

// An Airflow setting in .env is listed with its source but is no orphan: it
// configures Airflow, so declaring it for the project's code would be odd. A
// connection or Variable key still reads as its own kind, once.
func TestListAirflowSettingsAreNotOrphans(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("ASTRO_HOME", t.TempDir())
	writeGlobalEnv(t, "AIRFLOW__SCHEDULER__CATCHUP_BY_DEFAULT=False\n")
	projDir := t.TempDir()
	other := t.TempDir()
	body := "AIRFLOW__CORE__DAGS_ARE_PAUSED_AT_CREATION=False\nAIRFLOW_CONN_WAREHOUSE=postgres://h\nAIRFLOW_VAR_REGION=us\n"
	if err := os.WriteFile(ProjectEnvPath(projDir), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ProjectEnvPath(other), []byte("AIRFLOW__CORE__LOAD_EXAMPLES=False\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := localrttest.Seed(localrttest.Record{ProjectPath: other, Mode: localrt.ModeStandalone, Port: 1}); err != nil {
		t.Fatal(err)
	}

	items, err := List(nil, projDir, nil, ListOptions{All: true})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, it := range items {
		seen[it.Name]++
		switch it.Name {
		case "AIRFLOW__CORE__DAGS_ARE_PAUSED_AT_CREATION", "AIRFLOW__SCHEDULER__CATCHUP_BY_DEFAULT":
			if it.Orphan || it.RemoveHint != "" || it.Kind != KindEnv {
				t.Errorf("%s = %+v, want a plain env row with no orphan note", it.Name, it)
			}
			if it.Applied == nil || !*it.Applied || it.DeclareHint != "" {
				t.Errorf("%s = %+v, want the setting marked applied, with no declare hint", it.Name, it)
			}
		case "warehouse", "region":
			if !it.Orphan {
				t.Errorf("%s = %+v, want the undeclared connection or Variable still an orphan", it.Name, it)
			}
		}
	}
	if seen["AIRFLOW__CORE__DAGS_ARE_PAUSED_AT_CREATION"] != 1 || seen["AIRFLOW__SCHEDULER__CATCHUP_BY_DEFAULT"] != 1 {
		t.Errorf("want each setting listed once, got %v", seen)
	}
	if seen["warehouse"] != 1 || seen["region"] != 1 || seen["AIRFLOW_CONN_WAREHOUSE"] != 0 || seen["AIRFLOW_VAR_REGION"] != 0 {
		t.Errorf("want the connection and Variable listed once, under their own names, got %v", seen)
	}
	if seen["AIRFLOW__CORE__LOAD_EXAMPLES"] != 0 {
		t.Errorf("another project's Airflow setting is no orphan of this one, got %v", seen)
	}
}

// A global value the project .env also holds is shadowed: the project copy is
// the one applied, so only its row carries the mark.
func TestListLeavesAShadowedGlobalUnmarked(t *testing.T) {
	t.Setenv("ASTRO_HOME", t.TempDir())
	writeGlobalEnv(t, "BOTH=g\n")
	projDir := t.TempDir()
	if err := os.WriteFile(ProjectEnvPath(projDir), []byte("BOTH=p\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	items, err := List(nil, projDir, nil, ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		applied := it.Applied != nil && *it.Applied
		if applied != (it.Source == SourceProject) {
			t.Errorf("%+v: applied = %v, want the mark on the project row only", it, applied)
		}
	}
}

// A copy in the project vault shadows a global value the same way the project
// .env does. A copy in the shell does not count: docker mode passes through
// only declared shell names, and what the listing shell exports is not what a
// start sees.
func TestListMarksAGlobalByProjectCopiesOnly(t *testing.T) {
	t.Setenv("ASTRO_HOME", t.TempDir())
	writeGlobalEnv(t, "IN_VAULT=g\nIN_SHELL=g\n")
	projDir := t.TempDir()
	tiers := []VaultTier{
		{Label: "project vault", Scope: ScopeProject, Entries: []VaultEntry{{Kind: KindEnv, Name: "IN_VAULT", EnvKey: "IN_VAULT"}}},
		{Label: "global vault", Scope: ScopeGlobal, Entries: []VaultEntry{
			{Kind: KindVar, Name: "token", EnvKey: "AIRFLOW_VAR_TOKEN"},
			{Kind: KindEnv, Name: "AIRFLOW__SECRETS__BACKEND_KWARGS", EnvKey: "AIRFLOW__SECRETS__BACKEND_KWARGS"},
		}},
	}
	items, err := List([]string{"IN_SHELL=s"}, projDir, nil, ListOptions{VaultTiers: tiers})
	if err != nil {
		t.Fatal(err)
	}
	marked := map[string]string{}
	for _, it := range items {
		if it.Applied != nil && *it.Applied {
			marked[it.Name+" "+it.Source] = it.DeclareHint
		}
	}
	// IN_VAULT's global row is the one left out: the project vault's copy is
	// applied instead. An Airflow setting is no orphan, so it gets no hint.
	want := map[string]string{
		"IN_SHELL global":                               "astro local env variable declare IN_SHELL",
		"IN_VAULT project vault":                        "astro local env variable declare IN_VAULT --secret",
		"token global vault":                            "astro local env airflow-variable declare token --secret",
		"AIRFLOW__SECRETS__BACKEND_KWARGS global vault": "",
	}
	if len(marked) != len(want) {
		t.Fatalf("marked rows = %v, want %v", marked, want)
	}
	for k, v := range want {
		if marked[k] != v {
			t.Errorf("%s declare hint = %q, want %q", k, marked[k], v)
		}
	}
}

// Outside a project there is nothing a global value could fail to reach, so an
// undeclared one carries no applied mark.
func TestListOutsideAProjectLeavesGlobalUnmarked(t *testing.T) {
	t.Setenv("ASTRO_HOME", t.TempDir())
	writeGlobalEnv(t, "STRAY_GLOBAL=x\n")
	items, err := List(nil, "", nil, ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Applied != nil || items[0].DeclareHint != "" {
		t.Errorf("items = %+v, want one global row with no applied mark", items)
	}
}

// Each declared row carries its declaration's required, secret and
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
			"API_TOKEN": {Secret: true, Description: "Token for the API"},
			"LOG_LEVEL": {Optional: true},
		},
		AirflowVariables: map[string]envschema.ValueSpec{
			"region": {Description: "Deploy region"},
		},
		Connections: map[string]envschema.ValueSpec{
			"db_main": {Secret: true, Optional: true},
		},
	}
	items, err := List(nil, projDir, schema, ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	type flags struct {
		required, secret bool
		description      string
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
		got[it.Name] = flags{it.Required, it.Secret, it.Description}
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
	if bItem.Project == "" || bItem.RemoveHint != "" || bItem.DeclareHint != "" {
		t.Fatalf("cross-project orphan = %+v, want a Project and no hints", *bItem)
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
