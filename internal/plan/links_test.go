package plan

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

// With no link index, which is every machine until something writes one, every
// global reaches every project: the start gate's message and payload and the
// injected values are pinned as literals rather than derived, so a link filter
// that changed anything on such a machine fails here.
func TestNoLinkIndexLeavesTheStartGateUnchanged(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	write := func(env string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, project.Marker), []byte(manifestTOML+"\n[tool.astro.env]\n"+env), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	seedVault(t, "", "GLOBAL_TOKEN")
	seedVault(t, "", "UNDECLARED_GLOBAL")
	seedVault(t, dir, "PROJECT_ONLY")

	// The gate: one declared name resolves from the global tier, one is absent.
	write("GLOBAL_TOKEN = {}\nABSENT = {}\n")
	_, err := Build(dir, Options{Mode: localrt.ModeDocker})
	var missing *MissingEnvError
	if !errors.As(err, &missing) {
		t.Fatalf("want a missing-value error, got %v", err)
	}
	const wantMsg = "this project needs 1 environment value(s) that are not set on this machine:\n" +
		"  - env var ABSENT\n" +
		"      provide it:  astro local env variable set ABSENT --project\n" +
		"provide them, then run `astro local start` again — or start without them: `astro local start --allow-missing`."
	if got := missing.Error(); got != wantMsg {
		t.Errorf("gate message changed:\n got: %q\nwant: %q", got, wantMsg)
	}
	payload, err := json.Marshal(missing.Payload())
	if err != nil {
		t.Fatal(err)
	}
	const wantPayload = `{"error":"required environment values are not set on this machine","project":"` + "%s" + `","missing":[{"section":"env_var","name":"ABSENT","env_key":"ABSENT","set_command":"astro local env variable set ABSENT --project"}]}`
	if got, want := string(payload), jsonf(wantPayload, dir); got != want {
		t.Errorf("gate payload changed:\n got: %s\nwant: %s", got, want)
	}

	// Resolution: everything that reaches the project injects, the undeclared
	// global included.
	write("GLOBAL_TOKEN = {}\n")
	built, err := Build(dir, Options{Mode: localrt.ModeDocker})
	if err != nil {
		t.Fatal(err)
	}
	wantSecret := map[string]string{"GLOBAL_TOKEN": "from-vault", "PROJECT_ONLY": "from-vault", "UNDECLARED_GLOBAL": "from-vault"}
	if !maps.Equal(built.Plan.SecretEnv, wantSecret) {
		t.Errorf("SecretEnv = %v, want %v", built.Plan.SecretEnv, wantSecret)
	}
	for _, k := range []string{"GLOBAL_TOKEN", "PROJECT_ONLY", "UNDECLARED_GLOBAL"} {
		if v, ok := built.Plan.Env[k]; ok {
			t.Errorf("Env[%s] = %q: a vault value must never ride the plain env", k, v)
		}
	}
	if len(built.Plan.PassthroughEnv) != 0 {
		t.Errorf("PassthroughEnv = %v, want none", built.Plan.PassthroughEnv)
	}

	// Reading is all a start does: no index appears.
	if _, err := os.Stat(filepath.Join(home, ".astro", "secrets", "links.idx")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a start wrote the link index (stat: %v)", err)
	}
}

// jsonf fills the one %s in a JSON template with s, JSON-escaped.
func jsonf(tmpl, s string) string {
	enc, _ := json.Marshal(s) // a string always marshals
	quoted := string(enc)
	for i := 0; i+1 < len(tmpl); i++ {
		if tmpl[i] == '%' && tmpl[i+1] == 's' {
			return tmpl[:i] + quoted[1:len(quoted)-1] + tmpl[i+2:]
		}
	}
	return tmpl
}
