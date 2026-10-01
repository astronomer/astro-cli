package plan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/astronomer/astro-cli/internal/localenv"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/internal/vaultenv"
	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

// A name the project .env and the shell both hold starts with the .env value:
// Plan.Env carries the whole file and both engines set it over the inherited
// environment. The resolver has to rank the file the same way, or list and
// check credit the shell with a value Airflow never sees. Below the file, the
// shell still beats the vault.
func TestProjectDotEnvBeatsTheShellAndTheShellBeatsTheVault(t *testing.T) {
	dir := t.TempDir()
	manifest := manifestTOML + `
[tool.astro.env]
BOTH = {}
SHELL_OVER_VAULT = {}
`
	if err := os.WriteFile(filepath.Join(dir, project.Marker), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("BOTH=from-file\nUNDECLARED_BOTH=from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	seedVault(t, dir, "SHELL_OVER_VAULT")
	t.Setenv("BOTH", "from-shell")
	t.Setenv("UNDECLARED_BOTH", "from-shell")
	t.Setenv("SHELL_OVER_VAULT", "from-shell")

	built, err := Build(dir, Options{Mode: localrt.ModeDocker})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"BOTH", "UNDECLARED_BOTH"} {
		if got := built.Plan.Env[k]; got != "from-file" {
			t.Errorf("Env[%s] = %q, want the .env value", k, got)
		}
	}
	if got := built.Plan.PassthroughEnv; len(got) != 1 || got[0] != "SHELL_OVER_VAULT" {
		t.Errorf("PassthroughEnv = %v, want [SHELL_OVER_VAULT]: the .env names travel in Env", got)
	}
	if got, ok := built.Plan.SecretEnv["SHELL_OVER_VAULT"]; ok {
		t.Errorf("SecretEnv carries SHELL_OVER_VAULT=%q, so the vault beat the shell", got)
	}

	schema := &envschema.Schema{EnvVars: map[string]envschema.ValueSpec{"BOTH": {}, "SHELL_OVER_VAULT": {}}}
	items, err := localenv.List(os.Environ(), dir, schema, localenv.ListOptions{VaultProviders: vaultenv.Load(dir).Providers()})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"BOTH": localenv.SourceProject, "SHELL_OVER_VAULT": localenv.SourceShell}
	for _, it := range items {
		w, ok := want[it.Name]
		if !ok {
			continue
		}
		if it.Source != w || !it.Resolved {
			t.Errorf("list row %s: source %q resolved=%v, want %q resolved", it.Name, it.Source, it.Resolved, w)
		}
		delete(want, it.Name)
	}
	if len(want) != 0 {
		t.Errorf("list is missing rows for %v", want)
	}
}
