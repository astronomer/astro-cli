package config

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/flock"
	"github.com/spf13/viper"

	"github.com/astronomer/astro-cli/pkg/fsatomic"
	"github.com/astronomer/astro-cli/pkg/secrets"
)

// A context's access and refresh tokens live in the shared secrets vault, not
// in the home config: secrets.Logins decides where each login is kept and what
// the config's token fields say about it, for the CLI and Astro Desktop alike.
// This file is the CLI's side of it. Every read of a context resolves its
// login through the vault, and every write of a token field goes through it,
// so the rest of the CLI sees Context.Token and Context.RefreshToken exactly
// as before.

const (
	tokenField        = "token"
	refreshTokenField = "refreshtoken"
)

var (
	// openLogins opens the vault once per process. A test that wants one sets
	// loginsOverride instead; secrets.OpenLogins is off in test binaries.
	openLogins = sync.OnceValue(func() *secrets.Logins {
		l, err := secrets.OpenLogins()
		if err != nil {
			return nil
		}
		return l
	})
	loginsOverride *secrets.Logins
	// loginWarning makes the warning about an unreadable login print once.
	loginWarning sync.Once
	// signedOutChecked holds the contexts whose sign-out this process has
	// already carried into the vault, so that is done once per process.
	signedOutChecked sync.Map
)

// logins returns the vault logins are kept in. nil, with no usable vault
// directory, keeps them in the config.
func logins() *secrets.Logins {
	if loginsOverride != nil {
		return loginsOverride
	}
	return openLogins()
}

func isLoginField(field string) bool {
	return field == tokenField || field == refreshTokenField
}

// resolveLogin fills c's tokens from the vault, given the fields its config
// entry under cKey holds. A login the config holds moves into the vault, and
// the config records it, provided the file on disk still holds that login:
// this process read the config when it started, and moving a login another
// tool has replaced since would put the older one in the vault. Fields that
// say the context is signed out (an older tool's sign-out) remove the vault
// entry left behind, on the same condition and once per process.
func resolveLogin(cKey string, c *Context) {
	stored := secrets.Login{Token: c.Token, RefreshToken: c.RefreshToken}
	var (
		login secrets.Login
		moved *secrets.Login
		err   error
	)
	if logins() != nil && resolvesInVault(cKey, stored) {
		login, moved, err = logins().Resolve(HomeConfigFile, cKey, stored)
	} else {
		login, err = logins().Read(HomeConfigFile, cKey, stored)
	}
	if err != nil {
		loginWarning.Do(func() {
			fmt.Fprintf(os.Stderr, "Could not read your saved Astro login from the local secrets vault: %s. Log in again to continue.\n", err)
		})
	}
	c.Token, c.RefreshToken = login.Token, login.RefreshToken
	if moved == nil {
		return
	}
	putContextField(cKey, tokenField, moved.Token)
	putContextField(cKey, refreshTokenField, moved.RefreshToken)
	// A failed write leaves the login in both places, which the next read
	// resolves the same way.
	_ = saveConfig(viperHome, HomeConfigFile) //nolint:errcheck // see above
}

// resolvesInVault reports whether the fields stored for the context under
// cKey go through Resolve, which can change the vault: a login the config
// holds, or signed-out fields not yet checked in this process, either one
// only while the file on disk still holds them.
func resolvesInVault(cKey string, stored secrets.Login) bool {
	switch {
	case stored.InConfig():
		// Before reading the file again: for a context kept in the config,
		// or without a keyring, this is every read.
		if !logins().MayMove(HomeConfigFile, cKey) {
			return false
		}
	case stored == secrets.Login{}:
		if _, done := signedOutChecked.LoadOrStore(HomeConfigFile+"\x00"+cKey, true); done {
			return false
		}
	default:
		return false
	}
	onDisk, ok := diskLogin(cKey)
	return ok && onDisk == stored
}

// diskLogin reads the token fields of the context under cKey from the home
// config file as it is now, not as this process loaded it. ok is false when
// the file cannot be read or holds no such context.
func diskLogin(cKey string) (login secrets.Login, ok bool) {
	v, ok := readHomeFromDisk()
	if !ok {
		return secrets.Login{}, false
	}
	return contextLogin(v, cKey)
}

// readHomeFromDisk reads the home config file as it is now, into a viper of
// its own. ok is false when it cannot be read.
func readHomeFromDisk() (v *viper.Viper, ok bool) {
	v = viper.New()
	v.SetFs(configFs)
	v.SetConfigFile(HomeConfigFile)
	if err := readConfigFile(v, configFs); err != nil {
		return nil, false
	}
	return v, true
}

// contextLogin returns the token fields v holds for the context under cKey.
// ok is false when v holds no such context.
func contextLogin(v *viper.Viper, cKey string) (login secrets.Login, ok bool) {
	key := contextsKey + "." + cKey
	if !v.IsSet(key) {
		return secrets.Login{}, false
	}
	m := v.GetStringMap(key)
	return secrets.Login{Token: stringField(m, tokenField), RefreshToken: stringField(m, refreshTokenField)}, true
}

// Writing the home config writes everything this process holds in memory,
// which it read when it started. Another process may have saved a login
// since, and writing back the token fields this process loaded would undo it:
// a login another tool has moved into the vault would reappear in the config,
// looking newer than the vault's copy, and read as an older tool using the
// context. So a write takes the token fields of every context this process
// has not changed from the file as it is on disk (takeUnchangedLogins).
var (
	loginWritesMu sync.Mutex
	// loginWrites holds the keys of the contexts whose token fields this
	// process has set since it last wrote the home config or read it again.
	loginWrites = map[string]bool{}
)

// markLoginWrite records that this process set the token fields of the
// context under cKey, so the next write keeps them.
func markLoginWrite(cKey string) {
	loginWritesMu.Lock()
	defer loginWritesMu.Unlock()
	loginWrites[strings.ToLower(cKey)] = true
}

func wroteLogin(cKey string) bool {
	loginWritesMu.Lock()
	defer loginWritesMu.Unlock()
	return loginWrites[strings.ToLower(cKey)]
}

// forgetLoginWrites is called once the home config on disk holds what this
// process holds in memory: after it writes the file, or reads it again.
func forgetLoginWrites() {
	loginWritesMu.Lock()
	defer loginWritesMu.Unlock()
	clear(loginWrites)
}

// takeUnchangedLogins sets the token fields of every context in the home
// config that this process has not changed to what the file on disk holds
// now. The caller holds the config's lock. A context the file does not hold,
// or a file that cannot be read, is left as this process holds it.
func takeUnchangedLogins() {
	contexts := viperHome.GetStringMap(contextsKey)
	if len(contexts) == 0 {
		return
	}
	disk, ok := readHomeFromDisk()
	if !ok {
		return
	}
	for cKey := range contexts {
		if wroteLogin(cKey) {
			continue
		}
		onDisk, ok := contextLogin(disk, cKey)
		if !ok {
			continue
		}
		if held, _ := contextLogin(viperHome, cKey); held == onDisk {
			continue
		}
		storeContextField(cKey, tokenField, onDisk.Token)
		storeContextField(cKey, refreshTokenField, onDisk.RefreshToken)
	}
}

// putLoginField sets one token field of the context under cKey, in memory,
// storing the login in the vault and its config fields in the context.
//
// A login that cannot be read is not overwritten with an empty field. Callers
// write back fields they read, and a read that failed gave them empty ones;
// storing those would delete a login over a keyring that was briefly
// unavailable. Signing out, which does mean to empty both, is SignOut.
func putLoginField(cKey, field, value string) {
	if writesEnvironmentLogin(cKey, field, value) {
		return
	}
	ctxMap := viperHome.GetStringMap(contextsKey + "." + cKey)
	stored := secrets.Login{Token: stringField(ctxMap, tokenField), RefreshToken: stringField(ctxMap, refreshTokenField)}
	login, err := logins().Read(HomeConfigFile, cKey, stored)
	if err != nil && value == "" {
		return
	}
	if field == tokenField {
		login.Token = value
	} else {
		login.RefreshToken = value
	}
	fields := logins().Save(HomeConfigFile, cKey, login)
	putContextField(cKey, tokenField, fields.Token)
	putContextField(cKey, refreshTokenField, fields.RefreshToken)
}

// saveContextLogin stores login as the whole login of the context under cKey
// and returns the token fields its config entry should hold. A login equal to
// the one the context holds now is not a change: it is what a caller that read
// the context writes back with some other field, so it is not saved again,
// and the next write takes the context's token fields from disk like any
// other unchanged login (takeUnchangedLogins). Like putLoginField, it does not
// overwrite a login that cannot be read with empty fields: those are what
// reading it gave the caller.
func saveContextLogin(cKey string, login secrets.Login) secrets.Login {
	ctxMap := viperHome.GetStringMap(contextsKey + "." + cKey)
	stored := secrets.Login{Token: stringField(ctxMap, tokenField), RefreshToken: stringField(ctxMap, refreshTokenField)}
	held, err := logins().Read(HomeConfigFile, cKey, stored)
	login = withoutEnvironmentLogin(cKey, login, held)
	if (err == nil && held == login) || (err != nil && login == secrets.Login{}) {
		return stored
	}
	markLoginWrite(cKey)
	return logins().Save(HomeConfigFile, cKey, login)
}

// SignOut removes the login of this context and of every other context on its
// identity provider tenant: their vault entries go, and their token fields are
// left empty, which reads as signed out to this and every earlier build.
func (c *Context) SignOut() error {
	cKey, err := c.GetContextKey()
	if err != nil {
		return err
	}
	for _, key := range append([]string{cKey}, contextKeysSharingLogin(cKey)...) {
		logins().Save(HomeConfigFile, key, secrets.Login{})
		putContextField(key, tokenField, "")
		putContextField(key, refreshTokenField, "")
	}
	return saveConfig(viperHome, HomeConfigFile)
}

// ResumeLoginVault lets this context's login back into the vault after it was
// kept in the config for an older tool that uses the same context (see
// secrets.Logins). The next read or save moves it.
func (c *Context) ResumeLoginVault() error {
	cKey, err := c.GetContextKey()
	if err != nil {
		return err
	}
	return logins().ResumeVault(HomeConfigFile, cKey)
}

// LockLoginRenewal serializes renewing a login across astro processes, and
// returns the func that releases it. Callers re-read the login once they hold
// it (ReloadHome), so a process that waited sees a renewal the holder saved.
//
// A separate lock from the config's own: saving the renewed login takes that
// one, and a renewal holds this one across the network call. A holder can
// spend longer than one Lock wait on that call, so a timeout is retried a few
// times; past that, or when the lock cannot be taken at all, the renewal goes
// ahead unlocked, which costs at worst a second refresh, as before the lock.
func LockLoginRenewal() (unlock func()) {
	noop := func() {}
	if err := os.MkdirAll(HomeConfigPath, dirPerm); err != nil {
		return noop
	}
	for range renewalLockAttempts {
		unlock, err := fsatomic.Lock(HomeConfigFile + ".renew.lock")
		if err == nil {
			return unlock
		}
		if !errors.Is(err, fsatomic.ErrLockTimeout) {
			break
		}
	}
	return noop
}

// renewalLockAttempts times fsatomic.LockTimeout is how long a renewal waits
// for another process's.
const renewalLockAttempts = 6

// ReloadHome rereads the home config from disk, for a caller that must see
// what another process has saved since this one loaded it. It reads under the
// config's lock, as saveConfig writes, so a save another astro process has
// begun is finished before it reads; without the lock it reads anyway.
func ReloadHome() {
	lock := flock.New(HomeConfigFile + ".lock")
	ctx, cancel := context.WithTimeout(context.Background(), lockTimeout)
	defer cancel()
	if locked, err := lock.TryLockContext(ctx, lockRetryInterval); err == nil && locked {
		defer func() { _ = lock.Unlock() }() //nolint:errcheck // closing the read is all that is left to do
	}
	initHome(configFs)
}

func stringField(m map[string]interface{}, field string) string {
	s, _ := m[field].(string)
	return s
}

// Credentials from the environment (ASTRO_API_TOKEN, API keys) belong to the
// process that was given them. UseEnvironmentLogin hands one to the context a
// command runs on, and SetEnvironmentContextKey the organization and workspace
// it is for: every read of that context in this process returns them, with no
// refresh token, and nothing stores them. The saved login and the context's
// saved selection stay as they were, for the next command run without the
// variable, and a token that turns out to be bad is never kept.
type environmentLogin struct {
	cKey      string
	token     string
	expiresAt time.Time
	// fields holds the selection made for the credential, by config field.
	fields map[string]string
}

var (
	envLoginMu sync.Mutex
	// envLoginHeld is this process's environment login, if it has one.
	envLoginHeld *environmentLogin
)

// environmentFields are the fields SetEnvironmentContextKey sets.
var environmentFields = []string{"organization", "organization_product", "workspace"}

var errNoEnvironmentLogin = errors.New("this context has no login from the environment")

// UseEnvironmentLogin makes token, a credential from the environment, this
// context's login for the rest of the process, expiring at expiresAt. It is
// not saved, and replaces any environment login the process held before.
func (c *Context) UseEnvironmentLogin(token string, expiresAt time.Time) error {
	cKey, err := c.GetContextKey()
	if err != nil {
		return err
	}
	envLoginMu.Lock()
	defer envLoginMu.Unlock()
	envLoginHeld = &environmentLogin{cKey: cKey, token: token, expiresAt: expiresAt, fields: map[string]string{}}
	return nil
}

// SetEnvironmentContextKey is SetContextKey for the organization and workspace
// an environment credential is for: like the credential, the value holds for
// this process only. The context must have an environment login.
func (c *Context) SetEnvironmentContextKey(key, value string) error {
	cKey, err := c.GetContextKey()
	if err != nil {
		return err
	}
	key = strings.ToLower(key)
	if !slices.Contains(environmentFields, key) {
		return fmt.Errorf("%s is not set from the environment", key)
	}
	envLoginMu.Lock()
	defer envLoginMu.Unlock()
	if !holdsEnvironmentLogin(cKey) {
		return errNoEnvironmentLogin
	}
	envLoginHeld.fields[key] = value
	return nil
}

// holdsEnvironmentLogin reports whether the process's environment login is
// for the context under cKey. The caller holds envLoginMu.
func holdsEnvironmentLogin(cKey string) bool {
	return envLoginHeld != nil && strings.EqualFold(envLoginHeld.cKey, cKey)
}

// envLogin returns a copy of the environment login held for the context
// under cKey.
func envLogin(cKey string) (environmentLogin, bool) {
	envLoginMu.Lock()
	defer envLoginMu.Unlock()
	if !holdsEnvironmentLogin(cKey) {
		return environmentLogin{}, false
	}
	l := *envLoginHeld
	l.fields = maps.Clone(l.fields)
	return l, true
}

// isEnvironmentToken reports whether token is the one this process took from
// the environment.
func isEnvironmentToken(token string) bool {
	envLoginMu.Lock()
	defer envLoginMu.Unlock()
	return envLoginHeld != nil && envLoginHeld.token == token
}

func forgetEnvironmentLogin() {
	envLoginMu.Lock()
	defer envLoginMu.Unlock()
	envLoginHeld = nil
}

// writesEnvironmentLogin reports whether setting field of the context under
// cKey to value would store what the environment's login reads as: its token,
// under any context, or the empty refresh token it reads with. Callers write
// back fields they read (an organization switch rewrites the token,
// SetContext the whole context), and storing those would replace the saved
// login with the environment's, or sign it out. A login renewed from the saved
// refresh token is neither, and is stored.
func writesEnvironmentLogin(cKey, field, value string) bool {
	switch field {
	case tokenField:
		return isEnvironmentToken(value)
	case refreshTokenField:
		_, ok := envLogin(cKey)
		return ok && value == ""
	}
	return false
}

// withoutEnvironmentLogin is writesEnvironmentLogin for a whole login: each
// field that would store the environment's login keeps held, the login saved
// for the context.
func withoutEnvironmentLogin(cKey string, login, held secrets.Login) secrets.Login {
	if writesEnvironmentLogin(cKey, tokenField, login.Token) {
		login.Token = held.Token
	}
	if writesEnvironmentLogin(cKey, refreshTokenField, login.RefreshToken) {
		login.RefreshToken = held.RefreshToken
	}
	return login
}

// writesEnvironmentField reports whether setting field of the context under
// cKey to value writes back the selection made for the environment's login,
// which is not stored, as writesEnvironmentLogin does for its token. Any other
// value for that field is the command's own choice: it is stored, and from
// then on the process reads the stored value like every other.
func writesEnvironmentField(cKey, field string, value interface{}) bool {
	envLoginMu.Lock()
	defer envLoginMu.Unlock()
	if !holdsEnvironmentLogin(cKey) {
		return false
	}
	held, ok := envLoginHeld.fields[field]
	if !ok {
		return false
	}
	if s, isString := value.(string); isString && s == held {
		return true
	}
	delete(envLoginHeld.fields, field)
	return false
}

// applyEnvironmentLogin puts l in c in place of the saved login and selection.
func applyEnvironmentLogin(l *environmentLogin, c *Context) {
	c.Token, c.RefreshToken = l.token, ""
	for field, value := range l.fields {
		switch field {
		case "organization":
			c.Organization = value
		case "organization_product":
			c.OrganizationProduct = value
		case "workspace":
			c.Workspace = value
		}
	}
}
