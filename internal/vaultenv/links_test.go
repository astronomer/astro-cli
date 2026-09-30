package vaultenv

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/airflowenv"
	"github.com/astronomer/astro-cli/pkg/secrets"
	"github.com/astronomer/astro-cli/pkg/secrets/secretstest"
)

// isolatedHome points the shared vault at a temp home and returns the vault
// directory Load will read.
func isolatedHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir, err := secrets.DefaultDir()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// seedKeys writes each vault key into the shared vault with a value of its own
// name, so a resolved value says which entry it came from.
func seedKeys(t *testing.T, vaultDir string, keys []string) {
	t.Helper()
	store, err := secrets.NewKeyringStore(secrets.Config{Service: secrets.DefaultService, Dir: vaultDir})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range keys {
		if err := store.Set(k, "value of "+k); err != nil {
			t.Fatalf("seed %s: %v", k, err)
		}
	}
}

// envKeyOf is the Airflow env key a global vault key resolves under.
func envKeyOf(t *testing.T, vaultKey string) string {
	t.Helper()
	kind, _, name, err := secrets.ParseKey(vaultKey)
	if err != nil {
		t.Fatal(err)
	}
	envKey, ok := envKeyFor(kind, name)
	if !ok {
		t.Fatalf("%s has no env key", vaultKey)
	}
	return envKey
}

// The shared parity table through the production path: Load on the case's
// directory, then Tiers. Every key the case expects is listed as eligible and
// resolves through the providers; every other key is listed Unlinked and does
// not resolve. And the checkout Load built has to be the one the table names,
// which is what pins localrt.ProjectHome to the desktop's expectations.
func TestReachCasesThroughLoad(t *testing.T) {
	for _, c := range secretstest.ReachCases() {
		t.Run(c.Name, func(t *testing.T) {
			vaultDir := isolatedHome(t)
			l := secretstest.NewLayout(t)
			if why := c.Skip(l); why != "" {
				t.Skip(why)
			}
			seedKeys(t, vaultDir, secretstest.ReachVault)
			c.WriteIndex(t, vaultDir, l)

			s := Load(l.Dir(c.Place))
			if want := l.Checkout(c.Place); s.checkout != want {
				t.Errorf("checkout = %+v, want %+v", s.checkout, want)
			}

			eligible, unlinked := globalTierKeys(t, s)
			slices.Sort(eligible)
			if !slices.Equal(eligible, c.Want) {
				t.Errorf("eligible globals = %v, want %v", eligible, c.Want)
			}
			// Annotated, not dropped: the listing still shows all of them.
			if got := len(eligible) + len(unlinked); got != len(secretstest.ReachVault) {
				t.Errorf("global tier lists %d entries, want all %d the vault holds", got, len(secretstest.ReachVault))
			}

			assertLookups(t, s, c.Want)
			assertInjection(t, s, c.Want)
		})
	}
}

// globalTierKeys splits the global tier's listing into eligible and unlinked vault keys.
func globalTierKeys(t *testing.T, s *Source) (eligible, unlinked []string) {
	t.Helper()
	tiers := s.Tiers()
	global := tiers[len(tiers)-1]
	if global.Label != SourceGlobal {
		t.Fatalf("last tier is %q, want the global one", global.Label)
	}
	for _, e := range global.Entries {
		key, err := secrets.Key(vaultKind(e.Kind), secrets.GlobalScope, e.Name)
		if err != nil {
			t.Fatal(err)
		}
		if e.Unlinked {
			unlinked = append(unlinked, key)
		} else {
			eligible = append(eligible, key)
		}
	}
	return eligible, unlinked
}

// assertInjection checks a start injects exactly the globals that reach the
// checkout, with no schema declaring any of them: the link state is the one
// filter.
func assertInjection(t *testing.T, s *Source, want []string) {
	t.Helper()
	inj := s.SecretInjection()
	for _, key := range secretstest.ReachVault {
		v, ok := inj[envKeyOf(t, key)]
		reaches := slices.Contains(want, key)
		switch {
		case reaches && (!ok || v != "value of "+key):
			t.Errorf("%s reaches this checkout undeclared but was not injected (%q, %v)", key, v, ok)
		case !reaches && ok:
			t.Errorf("%s was injected into a checkout it does not reach", key)
		}
	}
}

// The warehouse feed and a start agree on what reaches a checkout: for every
// reach case, the connections ReachingConnections decodes are exactly the
// AIRFLOW_CONN_* names SecretInjection carries. Values are valid connection
// JSON here so both sides can decode every one.
func TestWarehouseFeedMatchesInjection(t *testing.T) {
	for _, c := range secretstest.ReachCases() {
		t.Run(c.Name, func(t *testing.T) {
			vaultDir := isolatedHome(t)
			l := secretstest.NewLayout(t)
			if why := c.Skip(l); why != "" {
				t.Skip(why)
			}
			store, err := secrets.NewKeyringStore(secrets.Config{Service: secrets.DefaultService, Dir: vaultDir})
			if err != nil {
				t.Fatal(err)
			}
			for _, k := range secretstest.ReachVault {
				if err := store.Set(k, `{"conn_type":"postgres","host":"h"}`); err != nil {
					t.Fatal(err)
				}
			}
			c.WriteIndex(t, vaultDir, l)

			s := Load(l.Dir(c.Place))
			var fed []string
			for _, conn := range s.ReachingConnections() {
				fed = append(fed, airflowenv.EnvKeyForConnID(conn.ConnID))
			}
			var injected []string
			for k := range s.SecretInjection() {
				if airflowenv.IsConnEnvKey(k) {
					injected = append(injected, k)
				}
			}
			slices.Sort(fed)
			slices.Sort(injected)
			if !slices.Equal(fed, injected) {
				t.Errorf("warehouse feed = %v, injected connections = %v; they must agree", fed, injected)
			}
		})
	}
}

// assertLookups checks every listed key resolves from the global tier exactly
// when it is in want.
func assertLookups(t *testing.T, s *Source, want []string) {
	t.Helper()
	for _, key := range secretstest.ReachVault {
		v, source, ok := lookup(s, envKeyOf(t, key))
		reaches := slices.Contains(want, key)
		switch {
		case reaches && (!ok || v != "value of "+key || source != SourceGlobal):
			t.Errorf("%s: lookup = %q from %q (%v), want its value from the global tier", key, v, source, ok)
		case !reaches && ok:
			t.Errorf("%s resolved (%q) for a checkout it does not reach", key, v)
		}
	}
}

// The start gate's note for a global linked elsewhere names where it is
// linked, marks the path that no longer exists, and names the index.
func TestDiagnoseNamesWhereAGlobalIsLinked(t *testing.T) {
	vaultDir := isolatedHome(t)
	l := secretstest.NewLayout(t)
	seedKeys(t, vaultDir, []string{"conn:global:warehouse", "env:global:NOWHERE"})
	other := l.Checkout(secretstest.PlaceOther).Path
	gone := filepath.Join(l.Root, "renamed-project")
	if err := secrets.UpdateLinks(vaultDir, func(m map[string]secrets.Reach) error {
		m["conn:global:warehouse"] = secrets.Reach{Projects: []string{other, gone}}
		m["env:global:NOWHERE"] = secrets.Reach{Projects: []string{}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	s := Load(l.Dir(secretstest.PlaceMain))
	d := globalDiagnoser(t, s)
	cause := d.Diagnose("AIRFLOW_CONN_WAREHOUSE")
	for _, want := range []string{`connection "warehouse"`, other, gone + " (missing)", "not to this project", secrets.LinksPath(vaultDir)} {
		if !strings.Contains(cause, want) {
			t.Errorf("cause = %q, want it to contain %q", cause, want)
		}
	}
	if strings.Contains(cause, other+" (missing)") {
		t.Errorf("cause = %q marks a path that exists as missing", cause)
	}
	if cause := d.Diagnose("NOWHERE"); !strings.Contains(cause, "linked to no project") {
		t.Errorf("cause = %q, want it to say the global is linked to no project", cause)
	}
	if cause := d.Diagnose("NEVER_SET"); cause != "" {
		t.Errorf("cause for a name the vault does not hold = %q, want none", cause)
	}
}

// An index this build cannot use fails the global tier closed: no global
// resolves or injects, the note names links.idx, and the project tier is
// untouched.
func TestUnusableIndexFailsTheGlobalTierClosed(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       error
	}{
		{"corrupt", `{"version":1,"links":`, secrets.ErrLinksUnreadable},
		{"newer", `{"version":2,"links":{}}`, secrets.ErrLinksTooNew},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vaultDir := isolatedHome(t)
			project := t.TempDir()
			seedProjectAndGlobal(t, vaultDir, project)
			if err := os.WriteFile(secrets.LinksPath(vaultDir), []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			assertGlobalTierClosed(t, Load(project))
			if _, err := secrets.OpenLinks(vaultDir); !errors.Is(err, tc.want) {
				t.Errorf("OpenLinks err = %v, want %v", err, tc.want)
			}
		})
	}
}

// seedProjectAndGlobal stores GLOBAL_TOKEN globally and PROJECT_TOKEN under project.
func seedProjectAndGlobal(t *testing.T, vaultDir, project string) {
	t.Helper()
	canonical, err := filepath.EvalSymlinks(project)
	if err != nil {
		t.Fatal(err)
	}
	store, err := secrets.NewKeyringStore(secrets.Config{Service: secrets.DefaultService, Dir: vaultDir})
	if err != nil {
		t.Fatal(err)
	}
	put(t, store, secrets.KindEnv, secrets.GlobalScope, "GLOBAL_TOKEN", "global")
	put(t, store, secrets.KindEnv, canonical, "PROJECT_TOKEN", "project")
}

// assertGlobalTierClosed checks nothing global resolves, injects or lists as
// eligible, the note names links.idx, and the project tier still works.
func assertGlobalTierClosed(t *testing.T, s *Source) {
	t.Helper()
	if _, _, ok := lookup(s, "GLOBAL_TOKEN"); ok {
		t.Error("a global resolved behind an index this build cannot use")
	}
	if v, source, ok := lookup(s, "PROJECT_TOKEN"); !ok || v != "project" || source != SourceProject {
		t.Errorf("project secret = %q from %q (%v); the project tier must keep working", v, source, ok)
	}
	if inj := s.SecretInjection(); inj["PROJECT_TOKEN"] != "project" || len(inj) != 1 {
		t.Errorf("injection = %v, want only the project secret", inj)
	}
	d := globalDiagnoser(t, s)
	if cause := d.Diagnose("GLOBAL_TOKEN"); !strings.Contains(cause, "links.idx") {
		t.Errorf("cause = %q, want it to name links.idx", cause)
	}
	if cause := d.Diagnose("NEVER_SET"); cause != "" {
		t.Errorf("cause for a name the vault does not hold = %q, want none", cause)
	}
	tiers := s.Tiers()
	for _, e := range tiers[len(tiers)-1].Entries {
		if !e.Unlinked {
			t.Errorf("global %s listed as eligible behind an unusable index", e.Name)
		}
	}
}

// Scoped entries never consult links: a row keyed like a project entry (which
// a writer refuses, but a peer could leave) changes nothing for it.
func TestProjectScopeIgnoresLinks(t *testing.T) {
	vaultDir := isolatedHome(t)
	project := t.TempDir()
	canonical, err := filepath.EvalSymlinks(project)
	if err != nil {
		t.Fatal(err)
	}
	key, err := secrets.Key(secrets.KindEnv, canonical, "TOKEN")
	if err != nil {
		t.Fatal(err)
	}
	seedKeys(t, vaultDir, []string{key})
	body := `{"version":1,"links":{` + jsonString(key) + `:{"projects":[]}}}`
	if err := os.WriteFile(secrets.LinksPath(vaultDir), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, source, ok := lookup(Load(project), "TOKEN"); !ok || source != SourceProject {
		t.Errorf("project secret did not resolve (%v, %q): links must not apply to it", ok, source)
	}
}

func jsonString(s string) string {
	return `"` + strings.ReplaceAll(s, `\`, `\\`) + `"`
}

// globalDiagnoser is the global tier's provider, as the resolver reaches it.
func globalDiagnoser(t *testing.T, s *Source) interface{ Diagnose(string) string } {
	t.Helper()
	ps := s.Providers()
	d, ok := ps[len(ps)-1].(interface{ Diagnose(string) string })
	if !ok || ps[len(ps)-1].Label() != SourceGlobal {
		t.Fatal("the global provider must diagnose a miss")
	}
	return d
}
