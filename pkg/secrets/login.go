package secrets

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/pkg/fsatomic"
)

// Astro logins in the shared vault.
//
// The CLI and Astro Desktop both keep a login per context in the shared config
// file (~/.astro/config.yaml): an access token and a refresh token beside the
// context's domain, email and expiry. Logins moves the two tokens into this
// vault and leaves everything else where it was. Both tools call it with the
// fields they read from the config and write back the fields it returns, so the
// rules below are applied identically by each.
//
// # What the config holds
//
// A context's token and refresh token fields are in one of three states:
//
//   - LoginInVault in the token field and an empty refresh token: the login is
//     in the vault. LoginInVault is the value builds without vault support
//     already read as signed out, so an older CLI or desktop asks the user to
//     log in rather than sending a credential that is not there.
//   - Both empty: signed out. A tool that does not know about the vault signs
//     out by emptying the fields, so this state wins over any vault entry.
//   - Anything else: the login is in the config as written. That is a login
//     an older tool saved, or one saved while the vault could not be reached.
//     It is the newest login there is, so Resolve uses it as it is and, unless
//     the context is kept in the config (below), moves it into the vault.
//
// A login is only ever taken out of the config after the vault has accepted
// it, so no failure in between loses it: the worst case is the same login in
// both places, and the next read moves it again.
//
// # When the vault cannot be reached
//
// The vault needs the OS keyring for its master key, and headless Linux, SSH
// sessions, containers and CI often have none. A login is never refused for
// that: it stays in the config exactly as before this existed. A failed attempt
// is remembered for loginRetryAfter, in the process that made it and in the
// others, so a machine without a keyring does not pay for a keyring attempt on
// every command; after that the next read tries again, and moves the login
// into the vault once a keyring answers. A long-lived process such as
// Astro Desktop therefore recovers from a keyring that failed once, rather
// than reading every login another tool moves into the vault as signed out
// until it restarts. A save ignores the stamp, since another session's
// failure says nothing about this one, unless the stamp records a keyring that
// did not answer in time: asking that one again would make every save wait out
// the timeout, and a command that saves a token each time it runs (with
// ASTRO_API_TOKEN set) would wait on every run. Reading the master key is
// bounded by loginKeyringTimeout, because an unattended keyring can wait for
// an unlock prompt nobody will answer.
//
// # Alongside an older tool
//
// A tool without vault support reads a moved login as signed out and asks for
// a login, which it writes to the config. Moving that login again would sign
// the older tool out again, and a user who keeps both would be asked to log in
// over and over. So a context the older tool has shown it uses is kept in the
// config from then on: its login is stored there as written by every
// vault-aware tool, exactly as before the vault existed, and both tools share
// it.
//
// The evidence is a login in the config that differs from the one the vault
// holds for the context. A vault-aware tool never leaves that behind: it
// writes a login to the config only when the vault refused it, and then it
// removes the vault's copy first (Save). Only a tool that does not know the
// vault writes over the placeholder while the vault still holds a login. The
// same login in both places is a move whose config write failed, not
// evidence, and moving it again is safe. Nor is a vault entry written after
// the config: that is a vault-aware save between its two writes (or one whose
// config write failed), and both are left alone.
//
// The decision is recorded per context in a marker file in the vault
// directory, named like the context's entry and holding nothing, so that every
// vault-aware tool honors it and checking it needs no keyring. The vault's
// copy of the login is removed then, leaving the config as the one source of
// truth.
//
// Nothing clears the marker by itself. An older tool may go unused for weeks
// and then log in again, and any automatic return to the vault would sign it
// out once more. The user asks for the vault back (ResumeVault), and should the
// older tool log in again after that, the context is kept in the config again.
//
// # A lost master key
//
// A vault whose master key is gone refuses to make a new one while it holds
// encrypted values (ErrVaultOrphaned). Logins do not count toward that,
// because logging in again recovers them: the new key is made, the logins
// left behind are removed, and each context reads as signed out, or uses a
// login its config holds, until the next login.
//
// Credentials supplied through the environment, such as ASTRO_API_TOKEN, are
// read by each tool before it looks at a saved login, and nothing here changes
// that.

// LoginInVault is the token field's value for a login held in the vault.
const LoginInVault = "Bearer "

// Login is the credential half of one context in the shared config: the two
// fields Logins moves. Both are kept exactly as the config spells them,
// including a "Bearer " scheme on the token.
type Login struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refreshtoken"`
}

func (l Login) empty() bool { return l.Token == "" && l.RefreshToken == "" }

func (l Login) inVault() bool { return l.Token == LoginInVault && l.RefreshToken == "" }

// configFields is what the config holds for a login kept in the vault.
var configFields = Login{Token: LoginInVault}

const (
	// loginKind prefixes the vault key of a login. It is not a Kind: ParseKey
	// rejects it as unknown, so every reader of environment values, in either
	// tool and in any version, skips these entries.
	loginKind = "login"
	// loginStamp records a failed keyring attempt, by its modification time.
	// Not ".json", so ListMeta and scanValues skip it.
	loginStamp = "login-keyring-unavailable"
	// loginInConfigPrefix starts the name of a context's marker file, which
	// keeps its login in the config. Not ".json", so ListMeta and scanValues
	// skip it.
	loginInConfigPrefix = "login-in-config-"
	// loginRetryAfter is how long a failed attempt keeps other processes from
	// trying the keyring again.
	loginRetryAfter = time.Hour
	// staleLoginAge protects a login another process has just put in the vault
	// and not yet recorded in the config: Resolve removes an entry the config
	// no longer points at only once it is older than this.
	staleLoginAge = 2 * time.Minute
)

// loginKeyringTimeout bounds one keyring call. Generous where the keyring
// prompts the user and the wait is a person deciding, short where an
// unattended session can block on an unlock prompt indefinitely.
var loginKeyringTimeout = func() time.Duration {
	switch runtime.GOOS {
	case "darwin", goosWindows:
		return promptingKeyringTimeout
	default:
		return unattendedKeyringTimeout
	}
}()

const (
	goosWindows              = "windows"
	promptingKeyringTimeout  = 2 * time.Minute
	unattendedKeyringTimeout = 15 * time.Second
)

// Logins keeps Astro logins in the vault. A nil *Logins is valid and keeps
// every login in the config, which is what a tool without a usable vault
// directory gets.
type Logins struct {
	store *keyringStore
	now   func() time.Time

	mu sync.Mutex
	// unavailable is the vault failure this process recorded at failedAt.
	// Until loginRetryAfter has passed, no call touches the keyring again.
	unavailable error
	failedAt    time.Time
}

// ErrLoginsUnavailable reports a vault this process has failed to reach
// recently, which it does not try again yet.
var ErrLoginsUnavailable = fmt.Errorf("%w: not retried yet", ErrKeyringUnavailable)

// OpenLogins opens the logins in the shared vault: DefaultService and
// DefaultDir, which is what makes a login saved by the CLI readable by
// Astro Desktop and the other way round.
//
// In a test binary it returns nil, so logins stay in the config: a test that
// saves a login through either tool must never reach the developer's real
// vault or keyring. A test that wants the vault opens one with NewLogins on a
// directory of its own.
func OpenLogins() (*Logins, error) {
	if testing.Testing() {
		return nil, nil
	}
	dir, err := DefaultDir()
	if err != nil {
		return nil, err
	}
	return NewLogins(dir)
}

// NewLogins opens logins in the vault at dir, under DefaultService.
func NewLogins(dir string) (*Logins, error) {
	return newLogins(dir, timeoutKeyring{inner: osKeyring{}, timeout: loginKeyringTimeout})
}

func newLogins(dir string, kr keyringAPI) (*Logins, error) {
	s, err := newKeyringStore(Config{Service: DefaultService, Dir: dir}, kr)
	if err != nil {
		return nil, err
	}
	return &Logins{store: s, now: time.Now}, nil
}

// Read returns the login a context's config fields stand for, changing
// nothing. configFile is the config file the fields were read from and
// contextKey the context's key in it; together they name the vault entry, so
// two config files (ASTRO_HOME) never share a login.
//
// A login the config points to in the vault that cannot be read comes back
// empty, as a signed-out context, with the reason; the caller can tell the
// user, and the next login replaces it.
func (l *Logins) Read(configFile, contextKey string, stored Login) (Login, error) {
	switch {
	case stored.inVault():
		if l == nil {
			return Login{}, nil
		}
		login, err := l.get(loginKey(configFile, contextKey))
		if errors.Is(err, ErrNotFound) {
			return Login{}, nil
		}
		if err != nil {
			return Login{}, err
		}
		return login, nil
	case stored.empty():
		return Login{}, nil
	}
	return stored, nil
}

// InConfig reports whether config fields hold a login as written, which
// Resolve moves into the vault.
func (l Login) InConfig() bool { return !l.inVault() && !l.empty() }

// Resolve is Read, and also moves a login the config holds into the vault
// when the vault can take it, returning the fields to write back; rewrite is
// nil when the config should not change. A caller that read stored some time
// ago should check the config still holds it first: moving a login another
// tool has since replaced would put the older one in the vault.
//
// Fields that say the context is signed out also remove the vault entry they
// leave behind, once it is old enough not to be a login another process is
// still recording.
func (l *Logins) Resolve(configFile, contextKey string, stored Login) (login Login, rewrite *Login, err error) {
	login, err = l.Read(configFile, contextKey, stored)
	if l == nil {
		return login, nil, err
	}
	switch {
	case stored.empty():
		l.removeStale(loginKey(configFile, contextKey))
	case stored.InConfig():
		rewrite = l.moveIn(configFile, loginKey(configFile, contextKey), stored)
	}
	return login, rewrite, err
}

// MayMove reports whether Resolve could move a login the config holds for
// this context into the vault now. False means it certainly would not: the
// context is kept in the config, or the vault failed recently. It touches
// neither the keyring nor the vault's entries, so a caller can ask it before
// any costlier check of its own.
func (l *Logins) MayMove(configFile, contextKey string) bool {
	if l == nil {
		return false
	}
	return !l.keptInConfig(loginKey(configFile, contextKey)) && l.reachable(anyStamp) == nil
}

// moveIn moves a login the config holds into the vault and returns the fields
// the config should hold instead, or nil to leave it as it is: the vault is
// out of reach, the context is kept in the config, or this login shows a tool
// without vault support uses the context, which keeps it there from now on.
func (l *Logins) moveIn(configFile, key string, stored Login) *Login {
	if l.keptInConfig(key) {
		return nil
	}
	if l.reachable(anyStamp) != nil {
		return nil
	}
	prev, err := l.get(key)
	switch {
	case err == nil && prev != stored && l.storedSince(key, configFile):
		// A save that has written the vault and not yet the config, or
		// failed to write the config: not evidence of an older tool. Leave
		// both as they are; the config is about to say where the login is.
		return nil
	case err == nil && prev != stored:
		l.keepInConfig(key)
		return nil
	case errors.Is(err, ErrKeyringUnavailable):
		return nil
	}
	// No entry, the same login, or an entry that cannot be read: the login
	// replaces it.
	if l.set(key, stored, anyStamp) != nil {
		return nil
	}
	moved := configFields
	return &moved
}

// storedSince reports whether the vault entry under key was written after the
// config file was last written. A tool without vault support writes the config
// after the vault entry it makes stale; a vault-aware save writes the vault
// first and the config next, so for a moment the vault is the newer of the
// two. Equal times count as the config being newer, which is what a coarse
// file clock gives a quick succession of writes.
func (l *Logins) storedSince(key, configFile string) bool {
	entry, err := os.Stat(l.store.path(key))
	if err != nil {
		return false
	}
	cfg, err := os.Stat(configFile)
	if err != nil {
		return false
	}
	return entry.ModTime().After(cfg.ModTime())
}

// keepInConfig keeps the context's login in the config from now on: it
// records the marker and then removes the vault's copy, which the config's
// login has replaced. Without the marker the copy stays, so the next read
// comes to the same decision.
func (l *Logins) keepInConfig(key string) {
	if _, err := prepareDir(l.store.dir, true); err != nil {
		return
	}
	if err := fsatomic.WriteFile(l.inConfigPath(key), nil, valuePerm); err != nil {
		return
	}
	_ = l.store.Delete(key) //nolint:errcheck // a copy left behind is removed by the next save
}

// keptInConfig reports whether the context's login is kept in the config. It
// reads only the marker, never the keyring.
func (l *Logins) keptInConfig(key string) bool {
	_, err := os.Stat(l.inConfigPath(key))
	return err == nil
}

func (l *Logins) inConfigPath(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(l.store.dir, loginInConfigPrefix+hex.EncodeToString(sum[:]))
}

// ResumeVault lets a context's login back into the vault after it was kept in
// the config for a tool without vault support: the next read or save moves
// it. That tool then reads the context as signed out, and should it log in
// again, the context goes back to the config.
func (l *Logins) ResumeVault(configFile, contextKey string) error {
	if l == nil {
		return nil
	}
	err := os.Remove(l.inConfigPath(loginKey(configFile, contextKey)))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("let the login back into the vault: %w", err)
	}
	return nil
}

// Save stores login for a context and returns the fields its config should
// hold. It never fails: when the vault cannot take the login, it comes back to
// be written to the config as it is.
//
// An empty login signs the context out. The fields come back empty, which
// reads as signed out whatever the vault holds, and the vault entry is
// removed; one that cannot be removed now goes on a later Resolve.
//
// A context kept in the config for a tool without vault support (see
// "Alongside an older tool") gets its login back to write as it is.
func (l *Logins) Save(configFile, contextKey string, login Login) Login {
	if l == nil {
		return login
	}
	key := loginKey(configFile, contextKey)
	if login.empty() {
		_ = l.store.Delete(key) //nolint:errcheck // the empty fields already sign the context out
		return Login{}
	}
	if l.keptInConfig(key) {
		_ = l.store.Delete(key) //nolint:errcheck // usually absent; the config's copy wins either way
		return login
	}
	// The stamp is machine-wide, and a keyring another session could not
	// reach is no reason to take this session's login out of the vault, so
	// only a stamp left by a keyring that did not answer in time holds a save
	// back: trying again would make every save wait out the timeout.
	if err := l.set(key, login, timeoutStamp); err != nil {
		// The config now holds the login, and an older copy left in the vault
		// would only be misleading. Removing it needs no keyring.
		_ = l.store.Delete(key) //nolint:errcheck // best effort; the config's copy wins either way
		if errors.Is(err, ErrLoginsUnavailable) {
			// Not tried, because this process failed recently. Renew the
			// stamp so that other processes do not move this login straight
			// back into a vault this process cannot read.
			l.stamp(l.timedOut())
		}
		return login
	}
	return configFields
}

func (l *Logins) get(key string) (Login, error) {
	if err := l.reachable(ignoreStamp); err != nil {
		return Login{}, err
	}
	raw, err := l.store.Get(key)
	if err != nil {
		if errors.Is(err, ErrKeyringUnavailable) {
			l.fail(err)
		}
		return Login{}, err
	}
	var login Login
	if err := json.Unmarshal([]byte(raw), &login); err != nil {
		return Login{}, fmt.Errorf("%w: the saved login is not readable", ErrTampered)
	}
	return login, nil
}

func (l *Logins) set(key string, login Login, rule stampRule) error {
	if err := l.reachable(rule); err != nil {
		return err
	}
	raw, err := json.Marshal(login)
	if err != nil {
		return err
	}
	if err := l.store.Set(key, string(raw)); err != nil {
		// Only a vault that cannot be opened stops later attempts: a write
		// that failed for one entry says nothing about the others.
		if errors.Is(err, ErrKeyringUnavailable) {
			l.fail(err)
		}
		return err
	}
	l.clearStamp()
	return nil
}

// stampRule says which stamps hold a keyring attempt back.
type stampRule int

const (
	// ignoreStamp: reading a login the config says is in the vault. The
	// stamp is machine-wide, and another session's failure is no reason to
	// report this one signed out.
	ignoreStamp stampRule = iota
	// anyStamp: moving a login out of the config, which can wait.
	anyStamp
	// timeoutStamp: saving a login, held back only by a keyring that did
	// not answer in time (see Save).
	timeoutStamp
)

// stampTimeout is the content of a stamp left by a keyring that did not
// answer in time.
const stampTimeout = "timeout"

// reachable reports a vault this process, or under rule a recent process, has
// failed to reach.
func (l *Logins) reachable(rule stampRule) error {
	l.mu.Lock()
	if l.unavailable != nil && l.now().Sub(l.failedAt) >= loginRetryAfter {
		l.unavailable = nil
	}
	err := l.unavailable
	l.mu.Unlock()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrLoginsUnavailable, err)
	}
	if rule == ignoreStamp {
		return nil
	}
	path := filepath.Join(l.store.dir, loginStamp)
	fi, err := os.Stat(path)
	if err != nil || l.now().Sub(fi.ModTime()) >= loginRetryAfter {
		return nil
	}
	if rule == timeoutStamp {
		raw, err := os.ReadFile(path)
		if err != nil || string(raw) != stampTimeout {
			return nil
		}
	}
	return fmt.Errorf("%w: it was unavailable %s ago", ErrKeyringUnavailable, l.now().Sub(fi.ModTime()).Round(time.Second))
}

// fail remembers a vault failure for loginRetryAfter: in this process, and in
// others by the stamp.
func (l *Logins) fail(err error) {
	l.mu.Lock()
	if l.unavailable == nil {
		l.unavailable, l.failedAt = err, l.now()
	}
	l.mu.Unlock()
	l.stamp(errors.Is(err, errKeyringTimeout))
}

// timedOut reports whether the failure this process recorded was a keyring
// that did not answer in time.
func (l *Logins) timedOut() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return errors.Is(l.unavailable, errKeyringTimeout)
}

// stamp records a failed keyring attempt for other processes, and whether the
// keyring did not answer in time.
func (l *Logins) stamp(timeout bool) {
	if _, perr := prepareDir(l.store.dir, true); perr != nil {
		return
	}
	var content []byte
	if timeout {
		content = []byte(stampTimeout)
	}
	stamp := filepath.Join(l.store.dir, loginStamp)
	_ = fsatomic.WriteFile(stamp, content, valuePerm) //nolint:errcheck // without the stamp the next process just tries again
}

func (l *Logins) clearStamp() {
	_ = os.Remove(filepath.Join(l.store.dir, loginStamp)) //nolint:errcheck // absent is the usual case
}

// removeStale removes the vault entry of a context the config says is signed
// out. A tool without vault support signs out by emptying the config, so this
// is where its sign-out reaches the vault. An entry younger than staleLoginAge
// is left: it can be a login another process has stored and is about to record
// in the config.
func (l *Logins) removeStale(key string) {
	if _, err := prepareDir(l.store.dir, false); err != nil {
		return
	}
	fi, err := os.Stat(l.store.path(key))
	if err != nil || l.now().Sub(fi.ModTime()) < staleLoginAge {
		return
	}
	_ = l.store.Delete(key) //nolint:errcheck // best effort; the config already reads as signed out
}

// isLoginKey reports whether a vault key names a login.
func isLoginKey(key string) bool { return strings.HasPrefix(key, loginKind+sep) }

// loginKey names a context's login in the vault: the config file, by a hash of
// its canonical path, and the context's key in it.
func loginKey(configFile, contextKey string) string {
	sum := sha256.Sum256([]byte(canonicalConfigPath(configFile)))
	return loginKind + sep + hex.EncodeToString(sum[:16]) + sep + strings.ToLower(contextKey)
}

// canonicalConfigPath spells a config file's path the same way whichever tool
// resolved it and whether or not it exists yet: absolute and clean, with
// symlinks resolved as far as the path exists, and case-folded on Windows,
// where two spellings that differ only in case are one file.
func canonicalConfigPath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = filepath.Clean(p)
	}
	dir, rest := abs, ""
	for {
		if resolved, err := filepath.EvalSymlinks(dir); err == nil {
			abs = filepath.Join(resolved, rest)
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		rest = filepath.Join(filepath.Base(dir), rest)
		dir = parent
	}
	if runtime.GOOS == goosWindows {
		abs = strings.ToLower(abs)
	}
	return abs
}

// errKeyringTimeout reports a keyring call that did not return in time.
var errKeyringTimeout = errors.New("the OS keyring did not respond")

// timeoutKeyring bounds reads of the master key. A read that times out keeps
// running in the background; its result is discarded.
type timeoutKeyring struct {
	inner   keyringAPI
	timeout time.Duration
}

type keyringResult struct {
	value string
	err   error
}

func (k timeoutKeyring) Get(service, account string) (string, error) {
	return k.call(func() (string, error) { return k.inner.Get(service, account) })
}

// Set is not bounded. It only runs to store a new master key, after a Get
// has answered, and abandoning it is unsafe: finishing after another process
// stored its own key, it would replace that key and orphan its values.
func (k timeoutKeyring) Set(service, account, value string) error {
	return k.inner.Set(service, account, value)
}

func (k timeoutKeyring) call(fn func() (string, error)) (string, error) {
	done := make(chan keyringResult, 1)
	go func() {
		v, err := fn()
		done <- keyringResult{v, err}
	}()
	timer := time.NewTimer(k.timeout)
	defer timer.Stop()
	select {
	case r := <-done:
		return r.value, r.err
	case <-timer.C:
		return "", fmt.Errorf("%w after %s", errKeyringTimeout, k.timeout)
	}
}
