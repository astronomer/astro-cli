package secrets

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// Test credentials. Assertions compare them and report booleans and lengths,
// never the values.
var (
	testLogin  = Login{Token: "Bearer access-one", RefreshToken: "refresh-one"}
	testLogin2 = Login{Token: "Bearer access-two", RefreshToken: "refresh-two"}
)

func testLogins(t *testing.T, kr keyringAPI) (l *Logins, dir string) {
	t.Helper()
	dir = filepath.Join(t.TempDir(), "secrets")
	l, err := newLogins(dir, kr)
	if err != nil {
		t.Fatalf("newLogins: %v", err)
	}
	return l, dir
}

func sameLogin(t *testing.T, what string, got, want Login) {
	t.Helper()
	if got != want {
		t.Errorf("%s: token matches = %v (len %d), refresh token matches = %v (len %d)",
			what, got.Token == want.Token, len(got.Token), got.RefreshToken == want.RefreshToken, len(got.RefreshToken))
	}
}

func TestResolveMovesAPlaintextLoginIntoTheVault(t *testing.T) {
	kr := newFakeKeyring()
	l, _ := testLogins(t, kr)
	cfg := filepath.Join(t.TempDir(), "config.yaml")

	got, rewrite, err := l.Resolve(cfg, "astronomer_io", testLogin)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	sameLogin(t, "resolved login", got, testLogin)
	if rewrite == nil || *rewrite != configFields {
		t.Fatalf("rewrite = %v, want the in-vault fields", rewrite != nil)
	}
	again, rewrite, err := l.Resolve(cfg, "astronomer_io", *rewrite)
	if err != nil || rewrite != nil {
		t.Fatalf("Resolve of the rewritten fields: rewrite=%v err=%v", rewrite != nil, err)
	}
	sameLogin(t, "login read back from the vault", again, testLogin)
}

func TestResolveKeepsTheLoginInTheConfigWithoutAKeyring(t *testing.T) {
	kr := newFakeKeyring()
	kr.err = errors.New("no Secret Service available")
	l, dir := testLogins(t, kr)
	cfg := filepath.Join(t.TempDir(), "config.yaml")

	got, rewrite, err := l.Resolve(cfg, "astronomer_io", testLogin)
	if err != nil || rewrite != nil {
		t.Fatalf("Resolve: rewrite=%v err=%v, want the config left alone", rewrite != nil, err)
	}
	sameLogin(t, "resolved login", got, testLogin)

	fields := l.Save(cfg, "astronomer_io", testLogin2)
	sameLogin(t, "fields saved without a keyring", fields, testLogin2)

	if kr.gets != 1 {
		t.Errorf("keyring reads = %d, want 1: a failure is not retried in the same process", kr.gets)
	}
	if _, err := os.Stat(filepath.Join(dir, loginStamp)); err != nil {
		t.Errorf("no stamp after a failed attempt: %v", err)
	}
}

// A later process honors the stamp for loginRetryAfter, then tries again and
// moves the login once the keyring answers.
func TestALaterProcessRetriesAfterTheStampExpires(t *testing.T) {
	broken := newFakeKeyring()
	broken.err = errors.New("no Secret Service available")
	first, dir := testLogins(t, broken)
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	if _, rewrite, _ := first.Resolve(cfg, "astronomer_io", testLogin); rewrite != nil {
		t.Fatal("a broken keyring moved the login")
	}

	working := newFakeKeyring()
	second, err := newLogins(dir, working)
	if err != nil {
		t.Fatal(err)
	}
	if _, rewrite, _ := second.Resolve(cfg, "astronomer_io", testLogin); rewrite != nil {
		t.Fatal("the login moved while the stamp was fresh")
	}
	if working.gets != 0 {
		t.Errorf("keyring reads under a fresh stamp = %d, want 0", working.gets)
	}

	second.now = func() time.Time { return time.Now().Add(loginRetryAfter + time.Minute) }
	_, rewrite, err := second.Resolve(cfg, "astronomer_io", testLogin)
	if err != nil || rewrite == nil {
		t.Fatalf("after the stamp expired: rewrite=%v err=%v, want the login moved", rewrite != nil, err)
	}
	if _, err := os.Stat(filepath.Join(dir, loginStamp)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("stamp left after a success: %v", err)
	}
}

// A token refresh saves often; only the first save in a process may reach
// the keyring.
func TestRepeatedSavesReachTheKeyringOnce(t *testing.T) {
	kr := newFakeKeyring()
	l, _ := testLogins(t, kr)
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	for i := range 5 {
		fields := l.Save(cfg, "astronomer_io", Login{Token: "Bearer access-" + string(rune('a'+i)), RefreshToken: "refresh"})
		if fields != configFields {
			t.Fatalf("Save %d left the login in the config", i)
		}
	}
	if kr.gets != 1 || kr.sets != 1 {
		t.Errorf("keyring calls = %d reads, %d writes; want 1 and 1 (mint the key once, then cached)", kr.gets, kr.sets)
	}
	got, _, err := l.Resolve(cfg, "astronomer_io", configFields)
	if err != nil {
		t.Fatal(err)
	}
	if got.Token != "Bearer access-e" {
		t.Errorf("resolved token is not the last one saved (len %d)", len(got.Token))
	}
	if kr.gets != 1 {
		t.Errorf("keyring reads after a read = %d, want still 1", kr.gets)
	}
}

func TestSavingAnEmptyLoginSignsOut(t *testing.T) {
	kr := newFakeKeyring()
	l, _ := testLogins(t, kr)
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	l.Save(cfg, "astronomer_io", testLogin)
	fields := l.Save(cfg, "astronomer_io", Login{})
	if !fields.empty() {
		t.Errorf("signed-out fields are not empty")
	}
	if _, err := l.store.Get(loginKey(cfg, "astronomer_io")); !errors.Is(err, ErrNotFound) {
		t.Errorf("vault entry after sign-out: err = %v, want ErrNotFound", err)
	}
	// Signing out of a context that never had a vault entry is not an error.
	l.Save(cfg, "other_io", Login{})
}

// A tool without vault support signs out by emptying the config. That wins,
// and the vault entry goes once it is old enough not to be a login another
// process is still recording.
func TestEmptyConfigFieldsWinOverTheVault(t *testing.T) {
	kr := newFakeKeyring()
	l, _ := testLogins(t, kr)
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	l.Save(cfg, "astronomer_io", testLogin)
	key := loginKey(cfg, "astronomer_io")

	got, rewrite, err := l.Resolve(cfg, "astronomer_io", Login{})
	if err != nil || rewrite != nil || !got.empty() {
		t.Fatalf("Resolve of signed-out fields: empty=%v rewrite=%v err=%v", got.empty(), rewrite != nil, err)
	}
	if _, err := l.store.Get(key); err != nil {
		t.Errorf("a fresh entry was removed: %v", err)
	}

	l.now = func() time.Time { return time.Now().Add(staleLoginAge + time.Minute) }
	if _, _, err := l.Resolve(cfg, "astronomer_io", Login{}); err != nil {
		t.Fatal(err)
	}
	if _, err := l.store.Get(key); !errors.Is(err, ErrNotFound) {
		t.Errorf("stale entry after a signed-out read: err = %v, want ErrNotFound", err)
	}
}

// An older tool logs in over a login in the vault: its login is used, the
// vault's copy goes, and the context stays in the config from then on, through
// any number of reads and refreshes, in this process and others.
func TestAnOlderToolsLoginKeepsTheContextInTheConfig(t *testing.T) {
	kr := newFakeKeyring()
	l, dir := testLogins(t, kr)
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	if fields := l.Save(cfg, "astronomer_io", testLogin); fields != configFields {
		t.Fatal("the first login stayed in the config")
	}
	key := loginKey(cfg, "astronomer_io")

	got, rewrite, err := l.Resolve(cfg, "astronomer_io", testLogin2)
	if err != nil || rewrite != nil {
		t.Fatalf("Resolve of the older tool's login: rewrite=%v err=%v, want the config left alone", rewrite != nil, err)
	}
	sameLogin(t, "resolved login", got, testLogin2)
	if _, err := l.store.Get(key); !errors.Is(err, ErrNotFound) {
		t.Errorf("vault copy after the older tool's login: err = %v, want ErrNotFound", err)
	}
	if !l.keptInConfig(key) {
		t.Fatal("no marker for the context")
	}

	// What the desktop does all day: read, refresh, read.
	stored := testLogin2
	for i := range 50 {
		if _, rewrite, err := l.Resolve(cfg, "astronomer_io", stored); err != nil || rewrite != nil {
			t.Fatalf("read %d: rewrite=%v err=%v, want the config left alone", i, rewrite != nil, err)
		}
		refreshed := Login{Token: fmt.Sprintf("Bearer access-%d", i), RefreshToken: stored.RefreshToken}
		stored = l.Save(cfg, "astronomer_io", refreshed)
		sameLogin(t, fmt.Sprintf("fields of refresh %d", i), stored, refreshed)
	}
	if _, err := l.store.Get(key); !errors.Is(err, ErrNotFound) {
		t.Errorf("vault entry after the refreshes: err = %v, want ErrNotFound", err)
	}

	// Another process honors the marker without asking the keyring, so one
	// with no keyring at all keeps the login in the config too.
	noKeyring := newFakeKeyring()
	noKeyring.err = errors.New("no Secret Service available")
	other, err := newLogins(dir, noKeyring)
	if err != nil {
		t.Fatal(err)
	}
	if _, rewrite, _ := other.Resolve(cfg, "astronomer_io", stored); rewrite != nil {
		t.Error("another process moved a login kept in the config")
	}
	if fields := other.Save(cfg, "astronomer_io", testLogin); fields != testLogin {
		t.Error("another process saved a kept login somewhere other than the config")
	}
	if noKeyring.gets != 0 {
		t.Errorf("keyring reads for a kept context = %d, want 0", noKeyring.gets)
	}
}

// A vault-aware tool that could not reach the keyring removes the vault's copy
// before it writes a login to the config, so its login is moved once the vault
// is back, and the context is not kept in the config.
func TestAVaultAwareFallbackIsNotAnOlderTool(t *testing.T) {
	kr := newFakeKeyring()
	l, dir := testLogins(t, kr)
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	l.Save(cfg, "astronomer_io", testLogin)

	broken := newFakeKeyring()
	broken.err = errors.New("no Secret Service available")
	headless, err := newLogins(dir, broken)
	if err != nil {
		t.Fatal(err)
	}
	fields := headless.Save(cfg, "astronomer_io", testLogin2)
	sameLogin(t, "fields saved without a keyring", fields, testLogin2)

	// The failure stamped the vault; a later read, past the stamp, finds the
	// keyring answering again.
	l.now = func() time.Time { return time.Now().Add(loginRetryAfter + time.Minute) }
	_, rewrite, err := l.Resolve(cfg, "astronomer_io", fields)
	if err != nil || rewrite == nil {
		t.Fatalf("Resolve once the keyring is back: rewrite=%v err=%v, want the login moved", rewrite != nil, err)
	}
	if l.keptInConfig(loginKey(cfg, "astronomer_io")) {
		t.Error("a vault-aware tool's fallback kept the context in the config")
	}
	got, _, err := l.Resolve(cfg, "astronomer_io", configFields)
	if err != nil {
		t.Fatal(err)
	}
	sameLogin(t, "vault after the move", got, testLogin2)
}

// The same login in the config and the vault is a move whose config write
// failed. It is moved again, not taken for an older tool.
func TestTheSameLoginInBothPlacesIsMovedAgain(t *testing.T) {
	kr := newFakeKeyring()
	l, _ := testLogins(t, kr)
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	l.Save(cfg, "astronomer_io", testLogin)
	_, rewrite, err := l.Resolve(cfg, "astronomer_io", testLogin)
	if err != nil || rewrite == nil {
		t.Fatalf("Resolve: rewrite=%v err=%v, want the login moved", rewrite != nil, err)
	}
	if l.keptInConfig(loginKey(cfg, "astronomer_io")) {
		t.Error("the same login in both places kept the context in the config")
	}
}

// ResumeVault lets a kept context's login back into the vault. Should the
// older tool log in again, the context is kept in the config again.
func TestResumeVaultMovesTheLoginAgain(t *testing.T) {
	kr := newFakeKeyring()
	l, _ := testLogins(t, kr)
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	l.Save(cfg, "astronomer_io", testLogin)
	l.Resolve(cfg, "astronomer_io", testLogin2)
	key := loginKey(cfg, "astronomer_io")
	if !l.keptInConfig(key) {
		t.Fatal("setup: the context was not kept in the config")
	}

	if err := l.ResumeVault(cfg, "astronomer_io"); err != nil {
		t.Fatalf("ResumeVault: %v", err)
	}
	if _, rewrite, err := l.Resolve(cfg, "astronomer_io", testLogin2); err != nil || rewrite == nil {
		t.Fatalf("Resolve after ResumeVault: rewrite=%v err=%v, want the login moved", rewrite != nil, err)
	}
	if fields := l.Save(cfg, "astronomer_io", testLogin2); fields != configFields {
		t.Error("a save after ResumeVault stayed in the config")
	}

	third := Login{Token: "Bearer access-three", RefreshToken: "refresh-three"}
	if _, rewrite, _ := l.Resolve(cfg, "astronomer_io", third); rewrite != nil {
		t.Error("the older tool's next login was moved")
	}
	if !l.keptInConfig(key) {
		t.Error("the older tool's next login did not keep the context in the config")
	}
	if err := l.ResumeVault(cfg, "never_kept"); err != nil {
		t.Errorf("ResumeVault of a context that was never kept: %v", err)
	}
	var none *Logins
	if err := none.ResumeVault(cfg, "astronomer_io"); err != nil {
		t.Errorf("nil ResumeVault: %v", err)
	}
}

// Keeping one context in the config leaves every other one in the vault.
func TestKeepingAContextInTheConfigIsPerContext(t *testing.T) {
	kr := newFakeKeyring()
	l, _ := testLogins(t, kr)
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	otherCfg := filepath.Join(t.TempDir(), "config.yaml")
	l.Save(cfg, "astronomer_io", testLogin)
	l.Resolve(cfg, "astronomer_io", testLogin2)

	if fields := l.Save(cfg, "astronomer-dev_io", testLogin); fields != configFields {
		t.Error("another context's login stayed in the config")
	}
	if fields := l.Save(otherCfg, "astronomer_io", testLogin); fields != configFields {
		t.Error("the same context in another config file stayed in the config")
	}
	if _, rewrite, _ := l.Resolve(cfg, "astronomer-stage_io", testLogin2); rewrite == nil {
		t.Error("another context's plaintext login was not moved")
	}
}

func TestAMissingOrDamagedVaultLoginReadsAsSignedOut(t *testing.T) {
	kr := newFakeKeyring()
	l, _ := testLogins(t, kr)
	cfg := filepath.Join(t.TempDir(), "config.yaml")

	got, rewrite, err := l.Resolve(cfg, "astronomer_io", configFields)
	if err != nil || rewrite != nil || !got.empty() {
		t.Fatalf("missing entry: empty=%v rewrite=%v err=%v", got.empty(), rewrite != nil, err)
	}

	if err := l.store.Set(loginKey(cfg, "astronomer_io"), "not json"); err != nil {
		t.Fatal(err)
	}
	got, _, err = l.Resolve(cfg, "astronomer_io", configFields)
	if !errors.Is(err, ErrTampered) || !got.empty() {
		t.Errorf("damaged entry: empty=%v err=%v, want ErrTampered", got.empty(), err)
	}

	// A keyring that stops answering: signed out, with the reason, and not
	// asked again in this process.
	kr2 := newFakeKeyring()
	writer, dir := testLogins(t, kr2)
	writer.Save(cfg, "astronomer_io", testLogin)
	kr2.err = errors.New("keychain locked")
	reader, err := newLogins(dir, kr2)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		got, _, err = reader.Resolve(cfg, "astronomer_io", configFields)
		if !errors.Is(err, ErrKeyringUnavailable) || !got.empty() {
			t.Errorf("unreachable keyring: empty=%v err=%v, want ErrKeyringUnavailable", got.empty(), err)
		}
	}
	if kr2.gets != 2 { // the writer's mint, then the reader's one attempt
		t.Errorf("keyring reads = %d, want 2", kr2.gets)
	}
}

func TestLoginsAreKeyedByConfigFileAndContext(t *testing.T) {
	kr := newFakeKeyring()
	l, _ := testLogins(t, kr)
	cfgA := filepath.Join(t.TempDir(), "config.yaml")
	cfgB := filepath.Join(t.TempDir(), "config.yaml")
	l.Save(cfgA, "astronomer_io", testLogin)
	l.Save(cfgB, "astronomer_io", testLogin2)
	l.Save(cfgA, "astronomer-dev_io", testLogin2)
	for _, c := range []struct {
		cfg, ctx string
		want     Login
	}{{cfgA, "astronomer_io", testLogin}, {cfgB, "astronomer_io", testLogin2}, {cfgA, "astronomer-dev_io", testLogin2}} {
		got, _, err := l.Resolve(c.cfg, c.ctx, configFields)
		if err != nil {
			t.Fatal(err)
		}
		sameLogin(t, c.ctx, got, c.want)
	}
}

// The two tools resolve the config path their own way. A symlinked astro home,
// a file that does not exist yet, and a different spelling of one path must
// all name the same entry.
func TestLoginKeyIgnoresHowThePathIsSpelled(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(realDir, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	want := loginKey(filepath.Join(realDir, ".astro", "config.yaml"), "astronomer_io")
	for _, p := range []string{
		filepath.Join(link, ".astro", "config.yaml"),
		filepath.Join(realDir, ".astro", "..", ".astro", "config.yaml"),
	} {
		if got := loginKey(p, "ASTRONOMER_IO"); got != want {
			t.Errorf("key for %s differs", p)
		}
	}
	if err := os.MkdirAll(filepath.Join(realDir, ".astro"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got := loginKey(filepath.Join(link, ".astro", "config.yaml"), "astronomer_io"); got != want {
		t.Error("key changed once the directory existed")
	}
	if runtime.GOOS == "windows" {
		if got := loginKey(filepath.Join(realDir, ".ASTRO", "CONFIG.yaml"), "astronomer_io"); got != want {
			t.Error("key differs by case on Windows")
		}
	}
}

// Readers of environment values skip login entries: ParseKey rejects their
// kind as unknown, the error every reader in either tool already skips on.
func TestLoginEntriesAreInvisibleToEnvironmentReaders(t *testing.T) {
	kr := newFakeKeyring()
	l, _ := testLogins(t, kr)
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	l.Save(cfg, "astronomer_io", testLogin)
	metas, err := l.store.ListMeta()
	if err != nil || len(metas) != 1 {
		t.Fatalf("ListMeta: %d entries, err %v", len(metas), err)
	}
	if _, _, _, err := ParseKey(metas[0].Key); !errors.Is(err, ErrUnknownKind) {
		t.Errorf("ParseKey(login key) err = %v, want ErrUnknownKind", err)
	}
}

func TestANilLoginsKeepsLoginsInTheConfig(t *testing.T) {
	var l *Logins
	got, rewrite, err := l.Resolve("config.yaml", "astronomer_io", testLogin)
	if err != nil || rewrite != nil {
		t.Fatalf("Resolve: rewrite=%v err=%v", rewrite != nil, err)
	}
	sameLogin(t, "nil Resolve", got, testLogin)
	fields := l.Save("config.yaml", "astronomer_io", testLogin2)
	sameLogin(t, "nil Save", fields, testLogin2)
	if got, _, _ := l.Resolve("config.yaml", "astronomer_io", configFields); !got.empty() {
		t.Error("a nil Logins resolved an in-vault login to something")
	}
}

func TestOpenLoginsIsOffInTestBinaries(t *testing.T) {
	l, err := OpenLogins()
	if l != nil || err != nil {
		t.Fatalf("OpenLogins in a test = (%v, %v), want (nil, nil)", l != nil, err)
	}
}

type blockingKeyring struct{ release chan struct{} }

func (b blockingKeyring) Get(string, string) (string, error) { <-b.release; return "", nil }
func (b blockingKeyring) Set(string, string, string) error   { <-b.release; return nil }

func TestAKeyringThatDoesNotAnswerTimesOut(t *testing.T) {
	b := blockingKeyring{release: make(chan struct{})}
	t.Cleanup(func() { close(b.release) })
	l, _ := testLogins(t, timeoutKeyring{inner: b, timeout: 20 * time.Millisecond})
	cfg := filepath.Join(t.TempDir(), "config.yaml")

	start := time.Now()
	fields := l.Save(cfg, "astronomer_io", testLogin)
	sameLogin(t, "fields after a timeout", fields, testLogin)
	if waited := time.Since(start); waited > 5*time.Second {
		t.Errorf("Save waited %s on a keyring that never answers", waited)
	}
	if !errors.Is(l.unavailable, errKeyringTimeout) {
		t.Errorf("recorded failure = %v, want the timeout", l.unavailable)
	}
}

// A save that fails for one entry, not for the vault as a whole, keeps that
// login in the config and leaves every other login readable.
func TestOneFailedWriteDoesNotShutTheVault(t *testing.T) {
	kr := newFakeKeyring()
	l, _ := testLogins(t, kr)
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	l.Save(cfg, "astronomer_io", testLogin)
	// A directory where the other context's value file goes makes its write
	// fail.
	if err := os.Mkdir(l.store.path(loginKey(cfg, "other_io")), 0o700); err != nil {
		t.Fatal(err)
	}
	fields := l.Save(cfg, "other_io", testLogin2)
	sameLogin(t, "fields after a failed write", fields, testLogin2)
	if l.unavailable != nil {
		t.Errorf("one failed write shut the vault for the process: %v", l.unavailable)
	}
	got, _, err := l.Resolve(cfg, "astronomer_io", configFields)
	if err != nil {
		t.Fatalf("another login after a failed write: %v", err)
	}
	sameLogin(t, "another login after a failed write", got, testLogin)
}

// The stamp is machine-wide. A save in a session whose keyring works keeps
// its login in the vault, whatever another session recorded.
func TestASaveIgnoresTheStamp(t *testing.T) {
	kr := newFakeKeyring()
	l, dir := testLogins(t, kr)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, loginStamp), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	// Moving a login out of the config waits for the stamp.
	if _, rewrite, _ := l.Resolve(cfg, "other_io", testLogin2); rewrite != nil {
		t.Error("a fresh stamp did not hold off a move")
	}
	if fields := l.Save(cfg, "astronomer_io", testLogin); fields != configFields {
		t.Error("a fresh stamp kept a save out of a working vault")
	}
	// The save reached the keyring, so the stamp is gone.
	if _, err := os.Stat(filepath.Join(dir, loginStamp)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("stamp left after a successful save: %v", err)
	}
}

// Signing out never fails: the empty fields sign the context out even when
// the vault entry cannot be removed, and a later read removes it.
func TestSignOutSucceedsWhenTheEntryCannotBeRemoved(t *testing.T) {
	kr := newFakeKeyring()
	l, dir := testLogins(t, kr)
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	l.Save(cfg, "astronomer_io", testLogin)

	moved := dir + ".real"
	if err := os.Rename(dir, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(moved, dir); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if fields := l.Save(cfg, "astronomer_io", Login{}); !fields.empty() {
		t.Error("sign-out did not empty the fields")
	}
	if got, err := l.Read(cfg, "astronomer_io", Login{}); err != nil || !got.empty() {
		t.Errorf("signed-out fields read as a login: err=%v", err)
	}
}

func TestReadChangesNothing(t *testing.T) {
	kr := newFakeKeyring()
	l, dir := testLogins(t, kr)
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	got, err := l.Read(cfg, "astronomer_io", testLogin)
	if err != nil {
		t.Fatal(err)
	}
	sameLogin(t, "read of a plaintext login", got, testLogin)
	if !testLogin.InConfig() || configFields.InConfig() || (Login{}).InConfig() {
		t.Error("InConfig misclassifies the three states")
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Read created the vault: %v", err)
	}
	if kr.gets != 0 {
		t.Errorf("Read of a plaintext login reached the keyring %d times", kr.gets)
	}
}

// A long-lived process (Astro Desktop) whose keyring failed once does not stay
// shut out of the vault. Until it tries again, a login it saves stays in the
// config and other processes leave it there; once loginRetryAfter has passed,
// it reads the logins other processes keep in the vault.
func TestALongLivedProcessRetriesTheKeyring(t *testing.T) {
	kr := newFakeKeyring()
	kr.err = errors.New("the keychain prompt was not answered")
	desktop, dir := testLogins(t, kr)
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	if fields := desktop.Save(cfg, "astronomer_io", testLogin); fields != testLogin {
		t.Fatal("a failed keyring took the login out of the config")
	}
	kr.mu.Lock()
	kr.err = nil
	kr.mu.Unlock()

	// Later in the same run, the stamp has expired but this process saves
	// again without trying the keyring. Its login stays in the config, and
	// the stamp is renewed so another process does not move it.
	later := time.Now().Add(loginRetryAfter - time.Minute)
	desktop.now = func() time.Time { return later }
	stampPath := filepath.Join(dir, loginStamp)
	old := time.Now().Add(-loginRetryAfter - time.Minute)
	if err := os.Chtimes(stampPath, old, old); err != nil {
		t.Fatal(err)
	}
	if fields := desktop.Save(cfg, "astronomer_io", testLogin2); fields != testLogin2 {
		t.Fatal("a save within loginRetryAfter of the failure reached the vault")
	}
	cli, err := newLogins(dir, newFakeKeyring())
	if err != nil {
		t.Fatal(err)
	}
	if _, rewrite, _ := cli.Resolve(cfg, "astronomer_io", testLogin2); rewrite != nil {
		t.Error("another process moved a login the failed process saved, so that process would read it as signed out")
	}

	// Once loginRetryAfter has passed since its failure, the process tries
	// the keyring again and reads a login another process put in the vault.
	// The other process shares this one's keyring, so it mints the master
	// key this process then reads.
	cli, err = newLogins(dir, kr)
	if err != nil {
		t.Fatal(err)
	}
	if fields := cli.Save(cfg, "astronomer_io", testLogin); fields != configFields {
		t.Fatal("a working keyring did not take the login")
	}
	desktop.now = func() time.Time { return time.Now().Add(loginRetryAfter + time.Minute) }
	got, err := desktop.Read(cfg, "astronomer_io", configFields)
	if err != nil {
		t.Fatalf("the process did not try the keyring again: %v", err)
	}
	sameLogin(t, "login read after the retry", got, testLogin)
}

// A vault-aware save writes the vault, then the config. A read between the
// two sees a plaintext login in the config that differs from the vault's, but
// the vault entry is the newer of the two: it is no evidence of an older
// tool, and the newer login must survive. Once the config is written after
// the vault entry, the same difference is evidence again.
func TestASaveInFlightIsNotAnOlderTool(t *testing.T) {
	kr := newFakeKeyring()
	l, _ := testLogins(t, kr)
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(cfg, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(cfg, old, old); err != nil {
		t.Fatal(err)
	}
	// The config still holds testLogin when a save stores testLogin2.
	if fields := l.Save(cfg, "astronomer_io", testLogin2); fields != configFields {
		t.Fatal("the save did not reach the vault")
	}
	key := loginKey(cfg, "astronomer_io")
	if got, rewrite, err := l.Resolve(cfg, "astronomer_io", testLogin); err != nil || rewrite != nil {
		t.Fatalf("read mid-save: rewrite=%v err=%v, want the config left alone", rewrite != nil, err)
	} else {
		sameLogin(t, "login read mid-save", got, testLogin)
	}
	if l.keptInConfig(key) {
		t.Error("a save in flight kept the context in the config")
	}
	got, err := l.Read(cfg, "astronomer_io", configFields)
	if err != nil {
		t.Fatal(err)
	}
	sameLogin(t, "vault login after a read mid-save", got, testLogin2)

	// The config written after the vault entry: an older tool's login.
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(cfg, later, later); err != nil {
		t.Fatal(err)
	}
	if _, rewrite, _ := l.Resolve(cfg, "astronomer_io", testLogin); rewrite != nil || !l.keptInConfig(key) {
		t.Errorf("a login written after the vault's: rewrite=%v kept=%v, want kept in the config", rewrite != nil, l.keptInConfig(key))
	}
}

// A keyring that did not answer in time is not asked again by a save in
// another process within loginRetryAfter: a command that saves a token every
// time it runs would otherwise wait out the timeout on every run. A keyring
// that failed quickly is asked again (TestASaveIgnoresTheStamp).
func TestASaveDoesNotWaitAgainOnAKeyringThatTimedOut(t *testing.T) {
	slow := blockingKeyring{release: make(chan struct{})}
	t.Cleanup(func() { close(slow.release) })
	first, dir := testLogins(t, timeoutKeyring{inner: slow, timeout: 10 * time.Millisecond})
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	if fields := first.Save(cfg, "astronomer_io", testLogin); fields != testLogin {
		t.Fatal("a keyring that timed out took the login")
	}

	kr := newFakeKeyring()
	second, err := newLogins(dir, kr)
	if err != nil {
		t.Fatal(err)
	}
	if fields := second.Save(cfg, "astronomer_io", testLogin2); fields != testLogin2 {
		t.Error("a save within loginRetryAfter of a timeout did not keep the login in the config")
	}
	if kr.gets != 0 {
		t.Errorf("keyring reads after a recent timeout = %d, want 0", kr.gets)
	}

	second.now = func() time.Time { return time.Now().Add(loginRetryAfter + time.Minute) }
	if fields := second.Save(cfg, "astronomer_io", testLogin2); fields != configFields {
		t.Error("a save after loginRetryAfter did not try the keyring again")
	}
}

func TestMayMove(t *testing.T) {
	kr := newFakeKeyring()
	l, dir := testLogins(t, kr)
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	if (*Logins)(nil).MayMove(cfg, "astronomer_io") {
		t.Error("a nil Logins may move a login")
	}
	if !l.MayMove(cfg, "astronomer_io") {
		t.Error("a working vault may not move a login")
	}
	l.keepInConfig(loginKey(cfg, "astronomer_io"))
	if l.MayMove(cfg, "astronomer_io") {
		t.Error("a context kept in the config may move")
	}
	if !l.MayMove(cfg, "other_io") {
		t.Error("keeping one context in the config stopped another from moving")
	}
	if err := os.WriteFile(filepath.Join(dir, loginStamp), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if l.MayMove(cfg, "other_io") {
		t.Error("a login may move under a fresh stamp")
	}
	if kr.gets != 0 {
		t.Errorf("MayMove reached the keyring %d times", kr.gets)
	}
}
