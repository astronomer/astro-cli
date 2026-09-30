package vaultenv

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/astronomer/astro-cli/pkg/secrets"
)

func init() {
	// The master key is keyed by the keyring service name, not by any path, so
	// isolating HOME is no defense: without this the first test to read a value
	// reaches the developer's real login keychain and can block on a prompt.
	keyring.MockInit()
}

// testVault is a real keyring-backed store over a temp directory — the
// encryption and the per-key files are the parts worth exercising, and MockInit
// keeps the master key in memory.
func testVault(t *testing.T) secrets.Store {
	t.Helper()
	store, err := secrets.NewKeyringStore(secrets.Config{
		Service: secrets.DefaultService,
		Dir:     filepath.Join(t.TempDir(), "secrets"),
	})
	if err != nil {
		t.Fatalf("open test vault: %v", err)
	}
	return store
}

// newSource is Load with the store and the already-canonical scope supplied,
// so a test can count what reaches the store or stand in a vault that fails.
// Load is the one production path.
func newSource(store secrets.Store, scope string) *Source {
	return &Source{store: store, scope: scope}
}

// put writes one entry under the shared grammar.
func put(t *testing.T, store secrets.Store, kind secrets.Kind, scope, name, value string) {
	t.Helper()
	key, err := secrets.Key(kind, scope, name)
	if err != nil {
		t.Fatalf("key(%s,%s,%s): %v", kind, scope, name, err)
	}
	if err := store.Set(key, value); err != nil {
		t.Fatalf("set %s: %v", key, err)
	}
}

// lookup walks a source's providers the way the resolver does.
func lookup(s *Source, envKey string) (value, source string, ok bool) {
	for _, p := range s.Providers() {
		if v, ok := p.Lookup(envKey); ok {
			return v, p.Label(), true
		}
	}
	return "", "", false
}

// The point of the whole package: a value the desktop wrote under this
// project's path resolves here, and one written under another project's does
// not leak into it.
func TestProjectSecretResolvesOnlyForItsOwnProject(t *testing.T) {
	store := testVault(t)
	mine, theirs := t.TempDir(), t.TempDir()
	put(t, store, secrets.KindEnv, mine, "TOKEN", "mine")
	put(t, store, secrets.KindEnv, theirs, "TOKEN", "theirs")

	got, source, ok := lookup(newSource(store, mine), "TOKEN")
	if !ok {
		t.Fatal("want the project's own secret to resolve")
	}
	if got != "mine" {
		t.Errorf("value = %q, want the value keyed to this project", got)
	}
	if source != SourceProject {
		t.Errorf("source = %q, want %q", source, SourceProject)
	}
}

// The index is built forwards for this reason. airflowenv upper-cases, so a
// Variable stored as "my_var" is asked about as AIRFLOW_VAR_MY_VAR — an
// implementation that derived the vault key from the env key would look for
// "MY_VAR" and miss it entirely.
func TestLowerCaseVariableNameStillResolves(t *testing.T) {
	store := testVault(t)
	put(t, store, secrets.KindVar, secrets.GlobalScope, "my_var", "found")

	got, _, ok := lookup(newSource(store, ""), "AIRFLOW_VAR_MY_VAR")
	if !ok {
		t.Fatal("a lower-case Variable name must still resolve under its upper-cased env key")
	}
	if got != "found" {
		t.Errorf("value = %q, want %q", got, "found")
	}
}

// Two names that encode to one env var is a real state the stores allow. Which
// one wins matters less than that it is always the same one, and the same one
// the desktop picks: sorted order, last name winning.
func TestNameCollisionResolvesDeterministically(t *testing.T) {
	store := testVault(t)
	put(t, store, secrets.KindVar, secrets.GlobalScope, "my_var", "lower")
	put(t, store, secrets.KindVar, secrets.GlobalScope, "MY_VAR", "upper")

	// "MY_VAR" sorts before "my_var" (upper-case first), so the lower-case key
	// is written last and wins.
	for range 5 {
		got, _, ok := lookup(newSource(store, ""), "AIRFLOW_VAR_MY_VAR")
		if !ok {
			t.Fatal("want a value")
		}
		if got != "lower" {
			t.Fatalf("value = %q, want the sorted-last name to win every time", got)
		}
	}
}

// The tiebreak is on the NAME, and this is the case that tells the two rules
// apart. A connection "x" and an env var literally named "AIRFLOW_CONN_X" both
// encode to AIRFLOW_CONN_X. By name, "AIRFLOW_CONN_X" < "x", so the connection is
// last and wins. By full vault key, "conn:global:x" < "env:global:AIRFLOW_CONN_X",
// so the env var would win instead — the kind token deciding rather than the
// name, which is neither what this package documents nor what the desktop does.
func TestCollisionAcrossKindsResolvesOnTheName(t *testing.T) {
	store := testVault(t)
	put(t, store, secrets.KindConn, secrets.GlobalScope, "x", `{"conn_type":"postgres"}`)
	put(t, store, secrets.KindEnv, secrets.GlobalScope, "AIRFLOW_CONN_X", "from-the-env-kind")

	got, _, ok := lookup(newSource(store, ""), "AIRFLOW_CONN_X")
	if !ok {
		t.Fatal("want a value")
	}
	if got != `{"conn_type":"postgres"}` {
		t.Errorf("value = %q, want the connection to win: the name decides, not the kind", got)
	}
}

// A name that cannot be an env var at all is skipped rather than indexed.
// secrets.Key only rejects a colon, so a peer tool can store env:<scope>:A=B —
// and standalone would then set an OS variable named "A=B" while docker declared
// it in the compose file and never resolved it.
func TestMalformedEnvNameIsSkipped(t *testing.T) {
	store := testVault(t)
	put(t, store, secrets.KindEnv, secrets.GlobalScope, "A=B", "nope")
	put(t, store, secrets.KindEnv, secrets.GlobalScope, "GOOD", "yes")

	s := newSource(store, "")
	if _, _, ok := lookup(s, "A=B"); ok {
		t.Error("a name that is not a legal env-var identifier must not resolve")
	}
	if v, _, ok := lookup(s, "GOOD"); !ok || v != "yes" {
		t.Errorf("the valid entry beside it must still resolve: %q %v", v, ok)
	}
	if inj := s.SecretInjection(); len(inj) != 1 || inj["GOOD"] != "yes" {
		t.Errorf("injection = %v, want only the valid entry", inj)
	}
}

// A project secret beats a global one of the same name, which is what putting
// the two tiers at different positions in the chain buys.
func TestProjectTierBeatsGlobalTier(t *testing.T) {
	store := testVault(t)
	dir := t.TempDir()
	put(t, store, secrets.KindEnv, secrets.GlobalScope, "TOKEN", "global")
	put(t, store, secrets.KindEnv, dir, "TOKEN", "project")

	got, source, _ := lookup(newSource(store, dir), "TOKEN")
	if got != "project" {
		t.Errorf("value = %q, want the project tier to win", got)
	}
	if source != SourceProject {
		t.Errorf("source = %q, want %q", source, SourceProject)
	}
}

// Outside a project there is no project tier at all, so a global value is what
// resolves and it says so.
func TestOutsideAProjectOnlyTheGlobalTierExists(t *testing.T) {
	store := testVault(t)
	put(t, store, secrets.KindEnv, secrets.GlobalScope, "TOKEN", "global")

	s := newSource(store, "")
	if n := len(s.Providers()); n != 1 {
		t.Errorf("provider count = %d, want only the global tier", n)
	}
	_, source, ok := lookup(s, "TOKEN")
	if !ok || source != SourceGlobal {
		t.Errorf("source = %q ok = %v, want %q", source, ok, SourceGlobal)
	}
}

// countingStore records what reaches the store.
type countingStore struct {
	secrets.Store
	gets int
}

func (c *countingStore) Get(key string) (string, error) {
	c.gets++
	return c.Store.Get(key)
}

// Building the index must not decrypt anything. A CLI invocation that resolves
// every name from a file would otherwise open the keychain for nothing — and on
// a machine whose keyring prompts, that is a dialog per command.
func TestIndexingReadsNoValues(t *testing.T) {
	inner := testVault(t)
	put(t, inner, secrets.KindEnv, secrets.GlobalScope, "TOKEN", "s3cret")
	put(t, inner, secrets.KindEnv, secrets.GlobalScope, "OTHER", "v")
	store := &countingStore{Store: inner}

	s := newSource(store, "")
	// A miss walks the whole index and must still read nothing.
	if _, _, ok := lookup(s, "NOT_THERE"); ok {
		t.Fatal("unexpected hit")
	}
	if store.gets != 0 {
		t.Errorf("%d value reads while resolving nothing; want 0", store.gets)
	}

	// A hit reads exactly the one entry it resolved.
	if _, _, ok := lookup(s, "TOKEN"); !ok {
		t.Fatal("want a hit")
	}
	if store.gets != 1 {
		t.Errorf("value reads = %d, want exactly the one entry that resolved", store.gets)
	}
}

// deadVault stands in for a machine with no reachable keyring: the listing
// works (it is a directory read) and every value read fails.
type deadVault struct{ secrets.Store }

func (deadVault) Get(string) (string, error) { return "", secrets.ErrKeyringUnavailable }

// A keyring that will not open must not look like an empty vault. The value
// misses, and the label and diagnosis say why — that difference is what turns
// "not set anywhere" into something the user can act on.
func TestUnreachableKeyringExplainsItself(t *testing.T) {
	inner := testVault(t)
	put(t, inner, secrets.KindEnv, secrets.GlobalScope, "TOKEN", "s3cret")

	s := newSource(deadVault{inner}, "")
	if _, _, ok := lookup(s, "TOKEN"); ok {
		t.Fatal("a value that cannot be decrypted must not resolve")
	}
	p := s.Providers()[0]
	// Label has no unavailable variant: these providers sit in the ordinary
	// chain, where one that holds no value never has Label called. The cause
	// travels through Diagnose, which the resolver does consult.
	if p.Label() != SourceGlobal {
		t.Errorf("label = %q, want the plain tier name", p.Label())
	}
	d, ok := p.(interface{ Diagnose(string) string })
	if !ok {
		t.Fatal("the provider must diagnose a miss")
	}
	if cause := d.Diagnose("TOKEN"); !strings.Contains(cause, "keyring") {
		t.Errorf("cause = %q, want it to name the keyring", cause)
	}
	// A name this tier simply does not hold is the ordinary case and must not
	// attach a note to every unset value in the project.
	if cause := d.Diagnose("NEVER_SET"); cause != "" {
		t.Errorf("cause for an unheld name = %q, want none", cause)
	}
}

// listFailsVault cannot even be listed.
type listFailsVault struct{ secrets.Store }

func (listFailsVault) ListMeta() ([]secrets.Meta, error) {
	return nil, errors.New("permission denied")
}

// A vault that cannot be listed is a whole-source outage, not a per-value one,
// and must leave the file sources alone rather than failing the command.
func TestUnlistableVaultIsASilentMiss(t *testing.T) {
	s := newSource(listFailsVault{testVault(t)}, "")
	if _, _, ok := lookup(s, "TOKEN"); ok {
		t.Fatal("want a miss")
	}
	if inj := s.SecretInjection(); len(inj) != 0 {
		t.Errorf("injection = %v, want empty", inj)
	}
}

// Everything that reaches a project injects, declared or not: the project's own
// secrets, and every global whose link state includes this checkout. There is
// no schema to consult; a declaration is a requirement, checked elsewhere.
func TestInjectionTakesEverythingThatReaches(t *testing.T) {
	store := testVault(t)
	dir := t.TempDir()
	put(t, store, secrets.KindEnv, dir, "PROJECT_ONLY", "p")
	put(t, store, secrets.KindEnv, secrets.GlobalScope, "UNDECLARED", "u")
	put(t, store, secrets.KindConn, secrets.GlobalScope, "shared_db", "postgres://h/d")

	inj := newSource(store, dir).SecretInjection()

	if inj["PROJECT_ONLY"] != "p" {
		t.Errorf("a project secret must inject without being declared: %v", inj)
	}
	if inj["UNDECLARED"] != "u" {
		t.Errorf("an undeclared global that reaches the project must inject: %v", inj)
	}
	if inj["AIRFLOW_CONN_SHARED_DB"] != "postgres://h/d" {
		t.Errorf("an undeclared global connection must inject as AIRFLOW_CONN_*: %v", inj)
	}
}

// And within injection the project tier still wins, since both tiers land in
// one environment.
func TestInjectionPrefersTheProjectTier(t *testing.T) {
	store := testVault(t)
	dir := t.TempDir()
	put(t, store, secrets.KindEnv, dir, "TOKEN", "project")
	put(t, store, secrets.KindEnv, secrets.GlobalScope, "TOKEN", "global")

	if got := newSource(store, dir).SecretInjection()["TOKEN"]; got != "project" {
		t.Errorf("TOKEN = %q, want the project tier to win", got)
	}
}

// Connections and Variables ride the same tiers as plain env vars, under the
// env-var names Airflow resolves natively.
func TestConnectionsAndVariablesEncodeToTheirAirflowNames(t *testing.T) {
	store := testVault(t)
	put(t, store, secrets.KindConn, secrets.GlobalScope, "my_db", "postgres://h/d")
	put(t, store, secrets.KindVar, secrets.GlobalScope, "batch_size", "50")

	s := newSource(store, "")
	if v, _, ok := lookup(s, "AIRFLOW_CONN_MY_DB"); !ok || v != "postgres://h/d" {
		t.Errorf("connection = %q ok = %v", v, ok)
	}
	if v, _, ok := lookup(s, "AIRFLOW_VAR_BATCH_SIZE"); !ok || v != "50" {
		t.Errorf("variable = %q ok = %v", v, ok)
	}
}

// Load is the production path — DefaultDir and the real store constructor — and
// must work on a machine whose vault directory does not exist yet.
func TestLoadOnAMachineWithNoVaultYet(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads this on Windows

	s := Load("")
	if _, _, ok := lookup(s, "TOKEN"); ok {
		t.Fatal("an empty vault must resolve nothing")
	}
	if p := s.Providers()[0]; strings.Contains(p.Label(), "unavailable") {
		t.Errorf("label = %q, want a plain label: no vault yet is not an outage", p.Label())
	}
}
