package plan

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/astronomer/astro-cli/internal/envresolve"
	"github.com/astronomer/astro-cli/internal/localenv"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/internal/userstate"
	"github.com/astronomer/astro-cli/internal/vaultenv"
	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/localrt"
	pkgmanifest "github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/secrets"
	"github.com/astronomer/astro-cli/pkg/util"
)

func init() {
	// Once per process. MockInit wipes the mock keyring, so calling it again
	// while encrypted values are already on disk is a genuinely orphaned vault —
	// pkg/secrets refuses that, correctly, and it looked like a test bug.
	keyring.MockInit()
}

const manifestTOML = `[project]
name = 'demo'
requires-python = '>=3.10'
dependencies = ['apache-airflow==3.1.*']

[tool.astro]
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
	flagSecrets := []string{"id=netrc,env=NETRC_CONTENT"}
	built, err := Build(dir, Options{Mode: localrt.ModeDocker, RequestedPort: 8080, StopWithSession: true, BuildSecretFlags: flagSecrets})
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
	if p.RequiresPython != ">=3.10" {
		t.Errorf("RequiresPython = %q, want >=3.10 (docker mode picks the image's Python from it)", p.RequiresPython)
	}
	if p.Mode != localrt.ModeDocker || !p.StopWithSession || p.RequestedPort != 8080 {
		t.Errorf("flags not carried: mode=%q stopWithSession=%v port=%d", p.Mode, p.StopWithSession, p.RequestedPort)
	}
	if !slices.Equal(p.BuildSecrets, flagSecrets) {
		t.Errorf("BuildSecrets = %q, want %q", p.BuildSecrets, flagSecrets)
	}
	if !strings.HasSuffix(p.Hostname, ".localhost") {
		t.Errorf("Hostname = %q, want a .localhost label", p.Hostname)
	}
	wantStateDir, _ := localrt.StateDir(dir)
	if p.StateDir != wantStateDir {
		t.Errorf("StateDir = %q, want %q", p.StateDir, wantStateDir)
	}
}

// A Docker-mode image installs from a generated requirements file, which
// cannot read [tool.uv.sources], so a git source reaches it as a direct
// reference.
func TestBuildWritesAGitSourceIntoTheDependencies(t *testing.T) {
	dir := t.TempDir()
	body := "[project]\nname = 'demo'\nrequires-python = '>=3.10'\ndependencies = ['apache-airflow==3.1.*', 'example-lib']\n\n[tool.astro]\n\n" +
		"[tool.uv.sources]\nexample-lib = { git = 'https://github.com/example-org/example-lib.git', rev = 'abc123' }\n"
	if err := os.WriteFile(filepath.Join(dir, project.Marker), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	built, err := Build(dir, Options{Mode: localrt.ModeDocker})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"apache-airflow==3.1.*", "example-lib @ git+https://github.com/example-org/example-lib.git@abc123"}
	if !slices.Equal(built.Plan.Dependencies, want) {
		t.Errorf("Dependencies = %q, want %q", built.Plan.Dependencies, want)
	}
}

// --build-secret replaces BUILD_SECRET_INPUT, which replaces the manifest's
// build-secrets: the first source that gives any is the whole list.
func TestBuildSecretPrecedence(t *testing.T) {
	declared := []string{"id=netrc,env=NETRC_CONTENT"}
	input := []string{"id=pip,env=PIP_CONF"}
	flag := []string{"id=ca,src=/etc/ca.pem"}
	for _, tc := range []struct {
		name  string
		flag  []string
		input string
		want  []string
	}{
		{name: "manifest", want: declared},
		{name: "BUILD_SECRET_INPUT over the manifest", input: input[0], want: input},
		{name: "flag over both", flag: flag, input: input[0], want: flag},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := newProject(t)
			pyproject := manifestTOML + "dockerfile = 'Dockerfile'\nbuild-secrets = ['" + declared[0] + "']\n"
			if err := os.WriteFile(filepath.Join(dir, project.Marker), []byte(pyproject), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM astrocrpublic.azurecr.io/runtime:3.1-2\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv(util.BuildSecretInputEnv, tc.input)

			built, err := Build(dir, Options{BuildSecretFlags: tc.flag})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(built.Plan.BuildSecrets, tc.want) {
				t.Errorf("BuildSecrets = %q, want %q", built.Plan.BuildSecrets, tc.want)
			}
		})
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

// A declared value satisfied only by the calling shell must reach docker mode
// by name: the plan carries it in PassthroughEnv, value-free, while
// file-held values travel in Env as before.
func TestBuildPassthroughEnv(t *testing.T) {
	dir := t.TempDir()
	manifest := manifestTOML + `
[tool.astro.env]
SHELL_ONLY = {}
FILE_HELD = {}
`
	if err := os.WriteFile(filepath.Join(dir, project.Marker), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("FILE_HELD=from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SHELL_ONLY", "from-shell")

	built, err := Build(dir, Options{Mode: localrt.ModeDocker})
	if err != nil {
		t.Fatal(err)
	}
	if got := built.Plan.PassthroughEnv; len(got) != 1 || got[0] != "SHELL_ONLY" {
		t.Errorf("PassthroughEnv = %v, want [SHELL_ONLY]", got)
	}
	if _, held := built.Plan.Env["SHELL_ONLY"]; held {
		t.Error("a shell-only value must not enter Plan.Env — it would be written to disk")
	}
	if built.Plan.Env["FILE_HELD"] != "from-file" {
		t.Errorf("Env[FILE_HELD] = %q, want from-file", built.Plan.Env["FILE_HELD"])
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
	for _, want := range []string{"API_URL", "astro local env variable set API_URL --project", "astro local env connection set warehouse --project", "astro local start"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
	if err.Payload() == nil {
		t.Error("Payload is nil")
	}
}

// seedVault writes one secret into the shared vault at the given scope. HOME is
// already a temp dir in these tests, which is what isolates pkg/secrets — it
// resolves the real home directory and deliberately ignores ASTRO_HOME.
func seedVault(t *testing.T, projectDir, name string) {
	t.Helper()
	w, err := vaultenv.NewWriter(projectDir)
	if err != nil {
		t.Fatalf("open vault writer: %v", err)
	}
	// A seeded global reaches every project, as one created before link
	// state existed does; the link tests write the rows they are about.
	w.NewAutoLink = true
	if _, err := w.Set(localenv.KindEnv, name, "from-vault"); err != nil {
		t.Fatalf("seed vault: %v", err)
	}
}

// A ~/.astro/env left by an older build is not a source: neither a declared
// name it holds nor an undeclared one reaches Airflow from it.
func TestALegacyGlobalEnvFileIsIgnored(t *testing.T) {
	dir := t.TempDir()
	manifest := manifestTOML + `
[tool.astro.env]
TOKEN = {}
`
	if err := os.WriteFile(filepath.Join(dir, project.Marker), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.MkdirAll(filepath.Join(home, ".astro"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".astro", "env"), []byte("TOKEN=from-global-file\nFILE_ONLY=from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	seedVault(t, dir, "TOKEN")

	built, err := Build(dir, Options{Mode: localrt.ModeDocker})
	if err != nil {
		t.Fatal(err)
	}
	if got := built.Plan.SecretEnv["TOKEN"]; got != "from-vault" {
		t.Errorf("SecretEnv[TOKEN] = %q, want the vault value", got)
	}
	for _, k := range []string{"TOKEN", "FILE_ONLY"} {
		if got, ok := built.Plan.Env[k]; ok {
			t.Errorf("Env[%s] = %q came from ~/.astro/env, which is no longer read", k, got)
		}
	}
	if got, ok := built.Plan.SecretEnv["FILE_ONLY"]; ok {
		t.Errorf("SecretEnv[FILE_ONLY] = %q came from ~/.astro/env, which is no longer read", got)
	}
}

// The other end of the same list: an exported shell variable beats the vault. A
// name the shell satisfied is not in Plan.Env at all, so a check keyed on Plan.Env
// left the secret in place and it overrode the override.
func TestShellBeatsTheVault(t *testing.T) {
	dir := t.TempDir()
	manifest := manifestTOML + `
[tool.astro.env]
TOKEN = {}
`
	if err := os.WriteFile(filepath.Join(dir, project.Marker), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	seedVault(t, dir, "TOKEN")
	t.Setenv("TOKEN", "from-shell")

	built, err := Build(dir, Options{Mode: localrt.ModeDocker})
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := built.Plan.SecretEnv["TOKEN"]; ok {
		t.Errorf("SecretEnv still carries TOKEN=%q, so the vault overrides an explicit shell value", got)
	}
	// It rides the passthrough list instead, by name, so docker can forward it
	// without the value touching disk.
	if got := built.Plan.PassthroughEnv; len(got) != 1 || got[0] != "TOKEN" {
		t.Errorf("PassthroughEnv = %v, want [TOKEN]", got)
	}
}

// An undeclared project secret still injects — the project tier is wholesale,
// like the project .env — but a plaintext .env entry of the same name still wins.
func TestProjectDotEnvBeatsAnUndeclaredVaultSecret(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, project.Marker), []byte(manifestTOML), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("SHADOWED=from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	seedVault(t, dir, "SHADOWED")
	seedVault(t, dir, "ONLY_VAULT")

	built, err := Build(dir, Options{Mode: localrt.ModeDocker})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := built.Plan.SecretEnv["SHADOWED"]; ok {
		t.Error("a plaintext .env entry must still win over a vaulted secret of the same name")
	}
	if got := built.Plan.SecretEnv["ONLY_VAULT"]; got != "from-vault" {
		t.Errorf("SecretEnv[ONLY_VAULT] = %q, want the undeclared project secret injected wholesale", got)
	}
}

// seedValue stores one value in the vault: global when projectDir is empty,
// reaching every project.
func seedValue(t *testing.T, projectDir string, kind localenv.Kind, name, value string) {
	t.Helper()
	w, err := vaultenv.NewWriter(projectDir)
	if err != nil {
		t.Fatalf("open vault writer: %v", err)
	}
	w.NewAutoLink = true
	if _, err := w.Set(kind, name, value); err != nil {
		t.Fatalf("seed vault: %v", err)
	}
}

// Nothing here is declared, and all of it reaches the project, so every name
// injects and the chain alone picks each winner: project .env > shell >
// project vault > global vault. A declaration is not what gets a value in.
func TestUndeclaredValuesInjectInChainOrder(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, project.Marker), []byte(manifestTOML), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("DOTENV_OVER_VAULT=from-dotenv\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	seedValue(t, "", localenv.KindConn, "shared_db", "postgres://global")
	seedValue(t, "", localenv.KindEnv, "PROJECT_OVER_GLOBAL", "from-global-vault")
	seedValue(t, dir, localenv.KindEnv, "PROJECT_OVER_GLOBAL", "from-project-vault")
	seedValue(t, "", localenv.KindEnv, "DOTENV_OVER_VAULT", "from-global-vault")
	seedValue(t, "", localenv.KindEnv, "SHELL_OVER_VAULT", "from-global-vault")
	t.Setenv("SHELL_OVER_VAULT", "from-shell")

	built, err := Build(dir, Options{Mode: localrt.ModeStandalone})
	if err != nil {
		t.Fatal(err)
	}
	env, secret := built.Plan.Env, built.Plan.SecretEnv
	checks := []struct {
		name      string
		m         map[string]string
		key, want string
	}{
		{"an undeclared linked global connection injects", secret, "AIRFLOW_CONN_SHARED_DB", `{"conn_type":"postgres","host":"global"}`},
		{"project scope beats global", secret, "PROJECT_OVER_GLOBAL", "from-project-vault"},
		{"the project .env beats the vault", env, "DOTENV_OVER_VAULT", "from-dotenv"},
	}
	for _, c := range checks {
		if got := c.m[c.key]; got != c.want {
			t.Errorf("%s: %s = %q, want %q", c.name, c.key, got, c.want)
		}
	}
	if v, ok := env["PROJECT_OVER_GLOBAL"]; ok {
		t.Errorf("Env[PROJECT_OVER_GLOBAL] = %q: the loser must not travel beside the winner", v)
	}
	if v, ok := secret["DOTENV_OVER_VAULT"]; ok {
		t.Errorf("SecretEnv[DOTENV_OVER_VAULT] = %q, want the .env value alone", v)
	}
	if v, ok := secret["SHELL_OVER_VAULT"]; ok {
		t.Errorf("SecretEnv[SHELL_OVER_VAULT] = %q would override the shell", v)
	}
	if v, ok := env["SHELL_OVER_VAULT"]; ok {
		t.Errorf("Env[SHELL_OVER_VAULT] = %q would override the shell", v)
	}
}

// An undeclared name the vault and the shell both hold resolves to
// the shell in both engines. Standalone inherits the shell, so nothing may
// carry the lower values; docker inherits nothing, so the name has to ride the
// passthrough list or the container gets no value at all.
func TestShellBeatsUndeclaredLowerSourcesInBothEngines(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, project.Marker), []byte(manifestTOML), 0o600); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	seedValue(t, "", localenv.KindEnv, "SHELL_WINS", "from-global-vault")
	seedValue(t, "", localenv.KindEnv, "VAULT_ONLY_SHELL", "from-global-vault")
	t.Setenv("SHELL_WINS", "from-shell")
	t.Setenv("VAULT_ONLY_SHELL", "from-shell")

	for _, mode := range []localrt.Mode{localrt.ModeStandalone, localrt.ModeDocker} {
		built, err := Build(dir, Options{Mode: mode})
		if err != nil {
			t.Fatal(err)
		}
		for _, k := range []string{"SHELL_WINS", "VAULT_ONLY_SHELL"} {
			if v, ok := built.Plan.Env[k]; ok {
				t.Errorf("%s: Env[%s] = %q would override the shell", mode, k, v)
			}
			if v, ok := built.Plan.SecretEnv[k]; ok {
				t.Errorf("%s: SecretEnv[%s] = %q would override the shell", mode, k, v)
			}
			if !slices.Contains(built.Plan.PassthroughEnv, k) {
				t.Errorf("%s: PassthroughEnv = %v, want %s so docker gets the shell value", mode, built.Plan.PassthroughEnv, k)
			}
		}
	}
}

// UndeclaredLocal names what a project gets from this machine undeclared, for
// check and package, and leaves the declared names out.
func TestUndeclaredLocal(t *testing.T) {
	dir := t.TempDir()
	manifest := manifestTOML + "\n[tool.astro.env]\nDECLARED = {}\n"
	if err := os.WriteFile(filepath.Join(dir, project.Marker), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("DECLARED=x\nFROM_DOTENV=y\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	seedValue(t, "", localenv.KindVar, "region", "us")
	// A stored key the Variable rule now rejects: listed as invalid, never
	// injected, so not something the project gets. Written to the store
	// directly, since the writer refuses the name.
	dirV, err := secrets.DefaultDir()
	if err != nil {
		t.Fatal(err)
	}
	store, err := secrets.NewKeyringStore(secrets.Config{Service: secrets.DefaultService, Dir: dirV})
	if err != nil {
		t.Fatal(err)
	}
	badKey, err := secrets.Key(secrets.KindVar, secrets.GlobalScope, "1bad")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set(badKey, "x"); err != nil {
		t.Fatal(err)
	}

	m, err := pkgmanifest.Load(filepath.Join(dir, project.Marker))
	if err != nil {
		t.Fatal(err)
	}
	got, err := UndeclaredLocal(dir, m)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"AIRFLOW_VAR_REGION", "FROM_DOTENV"}; !slices.Equal(got, want) {
		t.Errorf("UndeclaredLocal = %v, want %v", got, want)
	}
	note := UndeclaredNote(got, "")
	if !strings.HasSuffix(note, ").") {
		t.Errorf("note = %q, want it to end in a period", note)
	}
	for _, want := range []string{"AIRFLOW_VAR_REGION, FROM_DOTENV", "will not follow it to a Deployment", "astro local env <kind> declare NAME"} {
		if !strings.Contains(note, want) {
			t.Errorf("note = %q, want it to contain %q", note, want)
		}
	}
	if UndeclaredNote(nil, "") != "" {
		t.Error("no names must give no note")
	}
	if strings.Contains(note, "workspace") {
		t.Errorf("note = %q names a workspace the manifest does not link", note)
	}
	for _, names := range [][]string{nil, got} {
		ws := UndeclaredNote(names, "cmws")
		if !strings.HasSuffix(ws, ".") || strings.Contains(ws, "..") || strings.Contains(ws, ". .") {
			t.Errorf("note = %q, want sentences that each end in one period", ws)
		}
		for _, want := range []string{"workspace cmws holds, declared or not", "not checked here", "astro local env list"} {
			if !strings.Contains(ws, want) {
				t.Errorf("note = %q, want it to contain %q", ws, want)
			}
		}
	}
}

// A declared value that IS set, in the vault, behind a keyring that will not
// open. The run cannot start either way — but "not set on this machine" is the
// wrong thing to tell someone whose secret is sitting right there, and it is the
// only message they got: the resolver discarded a chain miss without asking any
// provider why.
func TestMissingValueExplainsAnUnreadableVault(t *testing.T) {
	dir := t.TempDir()
	manifest := manifestTOML + `
[tool.astro.env]
TOKEN = {}
`
	if err := os.WriteFile(filepath.Join(dir, project.Marker), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	seedVault(t, dir, "TOKEN")

	// Now the keyring stops answering, with the value already on disk.
	keyring.MockInitWithError(errors.New("no Secret Service available"))
	t.Cleanup(keyring.MockInit)

	_, err := Build(dir, Options{Mode: localrt.ModeDocker})
	var missing *MissingEnvError
	if !errors.As(err, &missing) {
		t.Fatalf("want a missing-value error, got %v", err)
	}
	if len(missing.Missing) != 1 {
		t.Fatalf("Missing = %+v, want one entry", missing.Missing)
	}
	note := missing.Missing[0].SourceNote
	if note == "" {
		t.Fatal("the missing value carries no cause, so the user is told it is simply not set")
	}
	if !strings.Contains(note, "keyring") {
		t.Errorf("SourceNote = %q, want it to name the keyring", note)
	}
}

// A declared Dockerfile reaches the plan, and an undeclared one leaves it empty.
//
// The empty case is half the point. Plan.Dockerfile switches docker mode from a
// generated image to running the project's file, so a value appearing when the
// manifest declared none would take every ordinary project down the wrong
// path — and the failure would be a docker build error naming nothing about the
// manifest.
//
// The populated case is the bug this fixes: Build never set the field, so a
// converted project that kept a load-bearing Dockerfile got a generated image
// from `astro local` while the desktop, inferring the tier from the file being
// present, built from the file. One project, two tools, two images.
func TestBuildCarriesTheDeclaredDockerfile(t *testing.T) {
	for _, tc := range []struct {
		name string
		decl string
		want string
	}{
		{"declared", "dockerfile = 'Dockerfile'\n", "Dockerfile"},
		{"declared in a subdirectory", "dockerfile = 'docker/Dockerfile'\n", "docker/Dockerfile"},
		{"not declared", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, project.Marker), []byte(manifestTOML+tc.decl), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("XDG_CACHE_HOME", t.TempDir())

			built, err := Build(dir, Options{Mode: localrt.ModeDocker})
			if err != nil {
				t.Fatal(err)
			}
			if built.Plan.Dockerfile != tc.want {
				t.Errorf("Plan.Dockerfile = %q, want %q", built.Plan.Dockerfile, tc.want)
			}
		})
	}
}

// A declared Dockerfile whose FROM names another Airflow than the requirement
// is refused before anything starts, in both modes: standalone installs the
// requirement, which is exactly the Airflow the image would not run.
func TestBuildRefusesADockerfileOfAnotherAirflow(t *testing.T) {
	for _, mode := range []localrt.Mode{"", localrt.ModeStandalone, localrt.ModeDocker} {
		for _, tc := range []struct {
			from    string
			refused bool
		}{
			{from: "astrocrpublic.azurecr.io/runtime:3.3-8", refused: true},
			{from: "quay.io/astronomer/astro-runtime:13.11.0", refused: true},
			{from: "astrocrpublic.azurecr.io/runtime:3.1-12", refused: false},
			{from: "astrocrpublic.azurecr.io/runtime@sha256:0123", refused: false},
		} {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, project.Marker), []byte(manifestTOML+"dockerfile = 'Dockerfile'\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM "+tc.from+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("XDG_CACHE_HOME", t.TempDir())

			_, err := Build(dir, Options{Mode: mode})
			var ve *pkgmanifest.ValidationError
			refused := errors.As(err, &ve) && len(ve.Problems) == 1 && ve.Problems[0].Code == pkgmanifest.CodeDockerfileAirflowMismatch
			if refused != tc.refused {
				t.Errorf("mode %q, FROM %s: err = %v, want refused %v", mode, tc.from, err, tc.refused)
			}
			if !tc.refused && err != nil {
				t.Errorf("mode %q, FROM %s: %v", mode, tc.from, err)
			}
		}
	}
}

// The runtime build reaches the plan, for Docker mode to build FROM.
func TestBuildCarriesTheRuntimeBuild(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, project.Marker), []byte(manifestTOML+"runtime = '3.1-12'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	built, err := Build(dir, Options{Mode: localrt.ModeDocker})
	if err != nil {
		t.Fatal(err)
	}
	if built.Plan.Runtime != "3.1-12" || built.Plan.AirflowVersion != "3.1" {
		t.Errorf("Plan.Runtime = %q, AirflowVersion = %q; want 3.1-12 and 3.1", built.Plan.Runtime, built.Plan.AirflowVersion)
	}
}

// The interpreter reaches the plan only when the manifest states no
// requires-python, and then as the version airflowrt.PythonFallback picks for
// the pin, the rule Astro Desktop applies too. Hand-written manifests, because
// a scaffolded one always states requires-python.
func TestBuildFallsBackToAPythonOnlyWhenRequiresPythonIsUnset(t *testing.T) {
	for _, tc := range []struct {
		name     string
		manifest string
		want     string
	}{
		{"stated", manifestTOML, ""},
		{"unset, airflow 3", "[project]\nname = 'demo'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\n", "3.12"},
		{"unset, airflow 2.7", "[project]\nname = 'demo'\ndependencies = ['apache-airflow==2.7.3']\n\n[tool.astro]\n", "3.11"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, project.Marker), []byte(tc.manifest), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("XDG_CACHE_HOME", t.TempDir())

			built, err := Build(dir, Options{})
			if err != nil {
				t.Fatal(err)
			}
			if built.Plan.PythonVersion != tc.want {
				t.Errorf("Plan.PythonVersion = %q, want %q", built.Plan.PythonVersion, tc.want)
			}
		})
	}
}
