package astro

import (
	http_context "context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/astrosession"
	"github.com/astronomer/astro-cli/internal/platform/astro/auth"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/organization"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/logger"
)

var (
	authLogin             = auth.Login
	fetchDomainAuthConfig = auth.FetchDomainAuthConfig
	// ErrNotAPIToken, ErrAPITokenExpired and ErrAPITokenMalformed are the
	// ways ASTRO_API_TOKEN can be unusable. They are astrosession's, which
	// reads the token for both trees, so a token fails with the same sentence
	// in either. The run does not fall back to the saved login: the person
	// supplied a credential, and it is the one that failed.
	ErrNotAPIToken       = astrosession.ErrNotAPIToken
	ErrAPITokenExpired   = astrosession.ErrAPITokenExpired
	ErrAPITokenMalformed = astrosession.ErrAPITokenMalformed
	// ErrAPITokenOrganization is returned when the organization of an accepted
	// API token can't be looked up, which leaves the context
	// without one.
	ErrAPITokenOrganization = errors.New("cannot look up the Organization of the Astro credentials in the environment")
	// ErrLoginRenewal is a saved login whose access token has expired and
	// could not be renewed. FreshLogin and the login check both return it.
	ErrLoginRenewal = errors.New("could not renew your Astro login")
	// ErrConfigUnreadable is a home config file that exists and could not be
	// parsed, so no login can be read from it.
	ErrConfigUnreadable = errors.New("the Astro config file could not be read")
)

// renewalError is ErrLoginRenewal with its cause.
type renewalError struct{ cause error }

func (e *renewalError) Error() string {
	return fmt.Sprintf("%s, run 'astro login' to log in again: %v", ErrLoginRenewal, e.cause)
}
func (e *renewalError) Unwrap() []error { return []error{ErrLoginRenewal, e.cause} }

func renewalFailed(cause error) error { return &renewalError{cause: cause} }

const (
	topLvlCmd = "astro"
)

type TokenResponse struct {
	AccessToken      string  `json:"access_token"`
	RefreshToken     string  `json:"refresh_token"`
	IDToken          string  `json:"id_token"`
	TokenType        string  `json:"token_type"`
	ExpiresIn        int64   `json:"expires_in"`
	Scope            string  `json:"scope"`
	Error            *string `json:"error,omitempty"`
	ErrorDescription string  `json:"error_description,omitempty"`
}

type CustomClaims struct {
	OrgAuthServiceID      string   `json:"org_id"`
	Scope                 string   `json:"scope"`
	Permissions           []string `json:"permissions"`
	Version               string   `json:"version"`
	IsAstronomerGenerated bool     `json:"isAstronomerGenerated"`
	RsaKeyID              string   `json:"kid"`
	APITokenID            string   `json:"apiTokenId"`
	jwt.RegisteredClaims
}

func Setup(cmd *cobra.Command, astroV1Client astrov1.APIClient) error {
	// If the user is trying to login or logout no need to go through auth setup.
	if cmd.CalledAs() == "login" || cmd.CalledAs() == "logout" {
		return nil
	}

	// If the user is using flow commands no need to go through auth setup.
	if cmd.CalledAs() == "flow" && cmd.Parent().Use == topLvlCmd {
		return nil
	}

	// help command does not need auth setup
	if cmd.CalledAs() == "help" && cmd.Parent().Use == topLvlCmd {
		return nil
	}

	// version command does not need auth setup
	if cmd.CalledAs() == "version" && cmd.Parent().Use == topLvlCmd {
		return nil
	}

	// Completion never logs in, refreshes a token or calls the API: it runs on
	// every TAB, offline and logged out, and a prompt there has no terminal to
	// draw on. Its candidates are the command tree's own.
	if IsCompletion(cmd) {
		return nil
	}

	// Commands that work on this machine's own state (context, config,
	// telemetry) say so, on themselves or on their group.
	if needsNoLogin(cmd) {
		return nil
	}

	// `astro auth token` prints a token for scripts and for Otto, which runs it
	// with no terminal. It renews the login it prints itself (FreshLogin) and
	// must never start the login flow, which would wait for a browser nobody
	// is watching. Credentials in the environment still apply.
	if isAuthTokenCmd(cmd) {
		// Quietly: the command's output is the token alone.
		_, err := ensureEnvCredentials(astroV1Client)
		// An API token is saved before its organization is looked up, and
		// printing it needs no organization, so a lookup that failed for want
		// of the API (a rate limit, an outage) does not stop this command as it
		// stops the others. One the API refused did not fail for want of it:
		// the token no longer works, and printing it would hide that.
		if errors.Is(err, ErrAPITokenOrganization) && !httputil.HasStatus(err, http.StatusUnauthorized) && !httputil.HasStatus(err, http.StatusForbidden) {
			logger.Debug(err)
			return nil
		}
		return err
	}

	return ensureLogin(astroV1Client)
}

// IsCompletion reports whether cmd is part of shell completion: the hidden
// request commands the shell calls on TAB, and the completion script generators
// under "astro completion".
func IsCompletion(cmd *cobra.Command) bool {
	if cmd.Name() == cobra.ShellCompRequestCmd || cmd.Name() == cobra.ShellCompNoDescRequestCmd {
		return true
	}
	return cmd.Parent() != nil && cmd.Parent().Use == "completion"
}

// NoLoginAnnotation, set to "true" on a command or on a group, says that it
// and everything under it work on this machine's own state and need no login:
// Setup leaves them alone, so they never start the login flow.
const NoLoginAnnotation = "noLogin"

// needsNoLogin reports whether cmd, or a group it is in, carries
// NoLoginAnnotation.
func needsNoLogin(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Annotations[NoLoginAnnotation] == "true" {
			return true
		}
	}
	return false
}

func isAuthTokenCmd(cmd *cobra.Command) bool {
	return cmd.CalledAs() == "token" && cmd.Parent() != nil && cmd.Parent().Name() == "auth"
}

// EnsureLogin runs the login check Setup gives every cloud command before it
// calls the API: an API token, then the current login, whose
// access token is refreshed when it is about to expire (and, with no login at
// all, the login flow). A command that skips the root's pre-run, as the core tree
// does, calls this before its own API calls.
func EnsureLogin(astroV1Client astrov1.APIClient) error {
	return ensureLogin(astroV1Client)
}

// ensureLogin is Setup's auth half, shared with EnsureLogin.
func ensureLogin(astroV1Client astrov1.APIClient) error {
	used, err := ensureEnvCredentials(astroV1Client)
	if err != nil || used {
		return err
	}
	return checkToken(astroV1Client, os.Stderr)
}

// ensureEnvCredentials sets up an API token from the environment, reporting
// whether one was found. It comes before any saved login. With no token and
// both Deployment API key variables set, it fails here, before any request or
// prompt, as 1.x would have used the keys. A lone variable was ignored by 1.x
// and is here too, with a warning.
func ensureEnvCredentials(astroV1Client astrov1.APIClient) (bool, error) {
	apiToken, err := checkAPIToken(astroV1Client)
	if err != nil {
		return false, err
	}
	set := legacyKeysSet()
	if apiToken {
		// A token wins over the keys: a CI job that moves from keys to a
		// token may keep both set for a while.
		warnLegacyKeys(set)
		return true, nil
	}
	if len(set) == 2 {
		return false, ErrLegacyAPIKeys
	}
	warnLegacyKeys(set)
	return false, nil
}

// The Deployment API key variables, no longer used.
const (
	legacyKeyIDVar     = "ASTRONOMER_KEY_ID"
	legacyKeySecretVar = "ASTRONOMER_KEY_SECRET"
)

// ErrLegacyAPIKeys is the failure of a run that needs an Astro login when the
// environment holds both Deployment API key variables and no API token. The keys stopped
// authenticating on June 1st, 2024.
var ErrLegacyAPIKeys = fmt.Errorf("%s and %s (Deployment API keys) are no longer supported: set ASTRO_API_TOKEN to a Deployment API token instead", legacyKeyIDVar, legacyKeySecretVar)

// legacyKeysSet returns the Deployment API key variables that are set.
func legacyKeysSet() []string {
	var set []string
	for _, name := range []string{legacyKeyIDVar, legacyKeySecretVar} {
		if os.Getenv(name) != "" {
			set = append(set, name)
		}
	}
	return set
}

// warnLegacyKeys says once per run, on stderr, that the variables in set are
// not used.
func warnLegacyKeys(set []string) {
	if len(set) == 0 {
		return
	}
	verb, pronoun := "is", "it"
	if len(set) > 1 {
		verb, pronoun = "are", "them"
	}
	cliout.WarnOnce(os.Stderr, "legacy-api-keys", fmt.Sprintf("%s %s set but no longer used (Deployment API keys are not supported); unset %s", strings.Join(set, " and "), verb, pronoun))
}

// FreshLogin returns the context for domain, or the current context when
// domain is empty, renewing its access token first when it is about to
// expire, or with force whatever expiry the config records (for a token the
// platform refused although it looked current). The renewed login is saved
// for every context on its tenant, as checkToken saves one, but the context's
// other fields are left alone. It never starts the login flow: a login that
// cannot be renewed is an error, and a context that is not logged in comes back
// with an empty token.
//
// Renewals are serialized across processes. Several processes that find the
// same token stale, or were refused with it, at the same moment (Otto and its
// subagents each run this) renew it once: the others wait, re-read the login,
// and return the renewal (see renewedMeanwhile).
func FreshLogin(domain string, force bool) (config.Context, error) {
	if force && astrosession.HasAPIToken() {
		return config.Context{}, errForceWithEnvCredentials
	}
	c, err := loginContext(domain)
	if err != nil || c.Token == "" {
		return c, err
	}
	if c.RefreshToken == "" {
		if force {
			return c, errNoRefreshToken
		}
		return c, nil
	}
	if !force && !needsRenewal(&c) {
		return c, nil
	}

	unlock := config.LockLoginRenewal()
	defer unlock()
	config.ReloadHome()
	fresh, err := loginContext(domain)
	if err != nil || fresh.Token == "" {
		return fresh, err
	}
	if fresh.RefreshToken == "" {
		if force {
			return fresh, errNoRefreshToken
		}
		return fresh, nil
	}
	if renewedMeanwhile(c.Token, &fresh, force) {
		return fresh, nil
	}
	if err := renewLogin(&fresh); err != nil {
		return fresh, renewalFailed(err)
	}
	return loginContext(domain)
}

// renewedRecently is how new an access token must be for a forced renewal to
// take it as another process's renewal rather than the token that was refused.
const renewedRecently = time.Minute

func needsRenewal(c *config.Context) bool {
	expireTime, _ := c.GetExpiresIn() //nolint:errcheck // a missing expiry reads as expired, which renews
	return isExpired(expireTime, auth.AccessTokenRefreshMargin)
}

// renewedMeanwhile reports whether the login fresh, re-read under the renewal
// lock, needs no renewal after all. before is the token this process read
// before waiting for the lock.
//
//   - A token other than before: another process renewed it while this one
//     waited.
//   - Without force, a token no longer about to expire.
//   - With force, a token issued less than renewedRecently ago. The caller does
//     not say which token was refused, and a process started a moment after
//     another finished renewing reads the renewal as before. A token refused
//     within a minute of being issued is not one a second renewal would fix,
//     and the caller can ask again once the minute has passed. A token issued
//     more than that in this machine's future says its clock is wrong, not
//     that the token is new, and is renewed.
func renewedMeanwhile(before string, fresh *config.Context, force bool) bool {
	if fresh.Token != before {
		return true
	}
	if !force {
		return !needsRenewal(fresh)
	}
	issued, ok := tokenIssuedAt(fresh.Token)
	age := time.Since(issued)
	return ok && age < renewedRecently && age > -renewedRecently
}

// tokenIssuedAt reads the issued-at claim of an access token, without
// verifying it: it only decides whether to renew, never whether to trust.
func tokenIssuedAt(token string) (time.Time, bool) {
	var claims jwt.RegisteredClaims
	if _, _, err := jwt.NewParser().ParseUnverified(strings.TrimPrefix(token, "Bearer "), &claims); err != nil || claims.IssuedAt == nil {
		return time.Time{}, false
	}
	return claims.IssuedAt.Time, true
}

var (
	errNoRefreshToken          = errors.New("this login cannot be renewed because it has no refresh token, run 'astro login' to log in again")
	errForceWithEnvCredentials = errors.New("--force renews a saved login, and the token here comes from the environment (ASTRO_API_TOKEN), which cannot be renewed")
)

func renewLogin(c *config.Context) error {
	if renewalRetired(c.RefreshToken) {
		return errRenewalRetired
	}
	authConfig, err := fetchDomainAuthConfig(c.Domain)
	if err != nil {
		return err
	}
	if err := c.SetAuthTenant(authConfig.DomainURL, authConfig.ClientID); err != nil {
		return err
	}
	res, err := refresh(c.RefreshToken, authConfig)
	if err != nil {
		return err
	}
	// The login only: saveRenewedToken also rewrites the workspace from
	// LastUsedWorkspace, which would undo a workspace switch every time a
	// tool fetches a token in the background.
	if err := saveRenewedLogin(c, &res); err != nil {
		retireRenewal(c.RefreshToken)
		return err
	}
	return nil
}

func loginContext(domain string) (config.Context, error) {
	if domain != "" {
		return context.GetContext(domain)
	}
	c, err := context.GetCurrentContext()
	if err != nil {
		return c, currentContextError(err)
	}
	return c, nil
}

// RefreshLogin renews the current login's access token now, whatever expiry
// the config records, and saves it the way checkToken does. It is for a call
// the platform refused with 401 although the recorded expiry said the token
// was good. It never starts the login flow: an error means the session cannot
// be renewed and the user has to log in again.
func RefreshLogin() error {
	c, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if c.RefreshToken == "" {
		return errors.New("the current login has no refresh token")
	}
	return refreshLoginFrom(&c)
}

// refreshLoginFrom is RefreshLogin for the login c, which the caller read
// before the call that was refused with it.
func refreshLoginFrom(c *config.Context) error {
	if renewalRetired(c.RefreshToken) {
		return errRenewalRetired
	}
	// Serialized like every other renewal: a 401 reaches several processes at
	// once, and a refresh token the provider rotates fails all but the first.
	unlock := config.LockLoginRenewal()
	defer unlock()
	config.ReloadHome()
	fresh, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if fresh.Token != c.Token && fresh.Token != "" && fresh.Token != "Bearer " && !needsRenewal(&fresh) {
		return nil // another process renewed it while this one waited
	}
	if fresh.RefreshToken == "" {
		return errors.New("the current login has no refresh token")
	}
	if renewalRetired(fresh.RefreshToken) {
		return errRenewalRetired
	}
	authConfig, err := fetchDomainAuthConfig(fresh.Domain)
	if err != nil {
		return err
	}
	res, err := refresh(fresh.RefreshToken, authConfig)
	if err != nil {
		return err
	}
	if err := saveRenewedToken(&fresh, &res); err != nil {
		if errors.Is(err, errLoginNotSaved) {
			retireRenewal(fresh.RefreshToken)
		}
		return err
	}
	return nil
}

// checkToken makes sure the current context holds a login the command can
// use, and returns the reason when it cannot. Nothing here is swallowed except
// a renewal that fails while the access token it would replace is still good:
// that one is logged, and the command runs on the token it has.
func checkToken(astroV1Client astrov1.APIClient, out io.Writer) error {
	c, err := context.GetCurrentContext() // get current context
	if err != nil {
		return currentContextError(err)
	}
	expireTime, _ := c.GetExpiresIn() //nolint:errcheck // a missing expiry reads as expired, which renews
	// check if user is logged in
	if c.Token == "Bearer " || c.Token == "" || c.Domain == "" {
		// guide the user through the login process if not logged in
		return authLogin(c.Domain, "", astroV1Client, out, false, false, false)
	}
	if !isExpired(expireTime, auth.AccessTokenRefreshMargin) {
		return nil
	}
	renewed, err := renewCurrentLogin(&c)
	if err == nil {
		return nil
	}
	if !renewed.expired {
		// Inside the refresh margin but not past it the token still works, and a
		// provider that is down, or a config that will not save, is no reason to
		// fail a command that can run on it.
		if renewed.unsaved && claimUnsavedWarning() {
			// The provider answered and may have retired the old refresh token,
			// and the answer could not be kept: say so once, since a later run
			// that finds this token expired will ask for a login.
			cliout.Warn(out, fmt.Sprintf("your Astro login was renewed but could not be saved to %s (%v). This command continues on the current access token; run astro login if later commands ask you to log in again.", config.HomeConfigFile, err))
		} else {
			logger.Debugf("could not renew the Astro login, which has not expired yet; continuing on the current access token: %v", err)
		}
		return nil
	}
	if renewed.rejected {
		// The refresh token no longer works: a person can log in again, so
		// offer that. A run that may not ask reports the expiry instead.
		if loginErr := authLogin(c.Domain, "", astroV1Client, out, false, false, false); loginErr != nil {
			if errors.Is(loginErr, auth.ErrLoginNeeded) {
				return renewalFailed(err)
			}
			return loginErr
		}
		// authLogin already persisted the new context, so don't fall through
		// and overwrite it with the failed refresh's zero-value token
		return nil
	}
	return renewalFailed(err)
}

// renewal says how a renewal that failed failed.
type renewal struct {
	// expired: the login's access token is past its expiry, so the failure
	// leaves the command with nothing to run on.
	expired bool
	// rejected: the provider answered the refresh token and refused it. A
	// provider that could not be reached says nothing about the token.
	rejected bool
	// unsaved: the provider renewed the login and the config would not take it.
	unsaved bool
}

// softLockAttempts bounds how long a renewal that nothing depends on waits for
// another process's: a login whose access token still works runs on it.
const softLockAttempts = 1

var (
	retiredMu sync.Mutex
	// retiredTokens holds the refresh tokens this run spent at the provider
	// without being able to save the answer. A token that is no longer the
	// login's (another process or a new login replaced it) is not blocked.
	retiredTokens = map[string]bool{}
	// warnedUnsaved is whether the run has said so on stderr.
	warnedUnsaved bool
	// errRenewalRetired is a renewal this run already did at the provider and
	// could not save. The refresh token it spent may be retired, so the run
	// does not present it again.
	errRenewalRetired = errors.New("the login was renewed but could not be saved, so its refresh token is not used again in this run")
)

// retireRenewal records that a renewal that spent refreshToken reached the
// provider and was not saved.
func retireRenewal(refreshToken string) {
	retiredMu.Lock()
	defer retiredMu.Unlock()
	retiredTokens[refreshToken] = true
}

// claimUnsavedWarning reports whether the caller is the first in the run to
// warn that a renewal was not saved, and so should print it. Only the code that
// prints calls it: a renewal that retires a token silently must not use up the
// one warning.
func claimUnsavedWarning() bool {
	retiredMu.Lock()
	defer retiredMu.Unlock()
	first := !warnedUnsaved
	warnedUnsaved = true
	return first
}

// renewalRetired reports whether refreshToken is one the run spent and could
// not save the answer to.
func renewalRetired(refreshToken string) bool {
	retiredMu.Lock()
	defer retiredMu.Unlock()
	return retiredTokens[refreshToken]
}

// renewCurrentLogin renews the current context's access token from its refresh
// token and saves it, under the lock FreshLogin takes, so that processes which
// find the same token stale renew it once: the others wait, re-read the login,
// and take the renewal (a refresh token the provider rotates would fail the
// loser). c is updated to the login as the lock found it. A nil error means the
// login is renewed, by this process or another.
func renewCurrentLogin(c *config.Context) (renewal, error) {
	before, _ := c.GetExpiresIn() //nolint:errcheck // a missing expiry reads as expired, which renews
	var unlock func()
	if isExpired(before, 0) {
		unlock = config.LockLoginRenewal()
	} else {
		// The token still works: do not wait out another process's renewal.
		var contended bool
		unlock, contended = config.LockLoginRenewalFor(softLockAttempts)
		if contended {
			return renewal{}, errors.New("another process is renewing the login")
		}
	}
	defer unlock()
	config.ReloadHome()
	fresh, err := context.GetCurrentContext()
	if err != nil {
		// What the login looked like before the lock is all there is to judge
		// by: a token that was still good stays good.
		return renewal{expired: isExpired(before, 0)}, currentContextError(err)
	}
	expireTime, _ := fresh.GetExpiresIn() //nolint:errcheck // a missing expiry reads as expired, which renews
	state := renewal{expired: isExpired(expireTime, 0)}
	*c = fresh
	if renewalRetired(fresh.RefreshToken) && isExpired(expireTime, auth.AccessTokenRefreshMargin) {
		return state, errRenewalRetired
	}
	if fresh.Token != "" && fresh.Token != "Bearer " && !isExpired(expireTime, auth.AccessTokenRefreshMargin) {
		return state, nil // another process renewed it while this one waited
	}
	authConfig, err := fetchDomainAuthConfig(c.Domain)
	if err != nil {
		return state, err
	}
	if err := c.SetAuthTenant(authConfig.DomainURL, authConfig.ClientID); err != nil {
		return state, err
	}
	res, err := refresh(c.RefreshToken, authConfig)
	if err != nil {
		state.rejected = !httputil.IsTransportError(err)
		return state, err
	}
	if err := saveRenewedToken(c, &res); err != nil {
		if errors.Is(err, errLoginNotSaved) {
			retireRenewal(c.RefreshToken)
			state.unsaved = true
		}
		return state, err
	}
	return state, nil
}

// currentContextError says what is wrong with the current context in words that
// name the file when it is the file: a config that would not parse reads as no
// context at all, and "no context set" would send a person to log in when the
// file is what needs fixing.
func currentContextError(err error) error {
	if path, ok := config.UnreadableHomeConfig(); ok {
		return fmt.Errorf("%w: %s did not parse, so no login can be read from it: fix or remove it, then run astro login", ErrConfigUnreadable, path)
	}
	return err
}

// errLoginNotSaved marks a failure to save the renewed login itself, as
// opposed to the selections saved after it: only that failure leaves the
// refresh token it spent as the one on disk.
var errLoginNotSaved = errors.New("could not save the renewed login")

// saveRenewedToken persists the context with the renewed access token.
func saveRenewedToken(c *config.Context, res *TokenResponse) error {
	if err := saveRenewedLogin(c, res); err != nil {
		return fmt.Errorf("%w: %w", errLoginNotSaved, err)
	}
	if err := c.SetContextKey("workspace", c.Workspace); err != nil {
		return err
	}
	if err := c.SetContextKey("workspace", c.LastUsedWorkspace); err != nil {
		return err
	}
	if err := c.SetContextKey("organization", c.Organization); err != nil {
		return err
	}
	return c.SetContextKey("organization_product", c.OrganizationProduct)
}

// saveRenewedLogin persists the renewed login and nothing else.
func saveRenewedLogin(c *config.Context, res *TokenResponse) error {
	// A token renewed from the refresh token works on every host on the tenant,
	// so the whole login goes to them: its refresh token and email with it.
	// Sharing the access token alone would leave a host that held another
	// user's login calling as one user while it renews as the other.
	refreshToken := res.RefreshToken
	if refreshToken == "" {
		refreshToken = c.RefreshToken
	}
	return c.SetSharedLogin("Bearer "+res.AccessToken, refreshToken, res.ExpiresIn, c.UserEmail)
}

// isExpired is true if now() + a threshold is after the given date
func isExpired(t time.Time, threshold time.Duration) bool {
	return time.Now().Add(threshold).After(t)
}

// refreshTimeout is how long a token request may take.
const refreshTimeout = 20 * time.Second

// Refresh gets a new access token from the provided refresh token,
// The request is used the default client_id and endpoint for device authentication.
func refresh(refreshToken string, authConfig auth.Config) (TokenResponse, error) {
	addr := authConfig.DomainURL + "oauth/token"
	data := url.Values{
		"client_id":     {authConfig.ClientID},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}

	// A bound, since a renewal holds the login-renewal lock while it waits.
	// A token endpoint never redirects, and a followed 307 or 308 would send
	// the form, refresh token included, to the address it names.
	client := &http.Client{
		Timeout:       refreshTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	r, err := http.NewRequestWithContext(http_context.Background(), http.MethodPost, addr, strings.NewReader(data.Encode())) // URL-encoded payload
	if err != nil {
		return TokenResponse{}, fmt.Errorf("cannot get a new access token from the refresh token: %w", err)
	}
	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	res, err := client.Do(r)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("cannot get a new access token from the refresh token: %w", err)
	}
	defer res.Body.Close()

	// A server that is failing or limiting says nothing about the token (its
	// body is often a proxy's HTML): a transport-style failure, so no one is
	// offered a browser login for it.
	if res.StatusCode >= http.StatusInternalServerError || res.StatusCode == http.StatusTooManyRequests {
		return TokenResponse{}, fmt.Errorf("cannot get a new access token from the refresh token: %w", httputil.NewTransientStatusError(res.StatusCode))
	}

	if res.StatusCode >= http.StatusMultipleChoices && res.StatusCode < http.StatusBadRequest {
		return TokenResponse{}, fmt.Errorf("cannot get a new access token from the refresh token: the token endpoint answered with a redirect (%d), which is not followed", res.StatusCode)
	}

	var tokenRes TokenResponse

	err = json.NewDecoder(res.Body).Decode(&tokenRes)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("cannot decode response: %w", err)
	}

	if tokenRes.Error != nil {
		return TokenResponse{}, errors.New(tokenRes.ErrorDescription)
	}

	return tokenRes, nil
}

func checkAPIToken(astroV1Client astrov1.APIClient) (bool, error) {
	// The token is read as the core tree reads it, by one parser: a value
	// holding only the scheme ("Bearer ") is no token, one holding the scheme
	// too is the token after it, so the login below does not carry it twice,
	// and one that is malformed, expired or without permissions is the error.
	id, ok, err := astrosession.TokenIdentity()
	if !ok && err == nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	astroAPIToken := id.Credential
	logger.Debug("Using an Astro API Token")

	// A token with no expiry of its own reads as live for a year.
	expiresAt := id.ExpiresAt
	if expiresAt.IsZero() {
		expiresAt = time.Now().AddDate(1, 0, 0)
	}

	domain, err := environmentDomain()
	if err != nil {
		return false, err
	}
	if err := useEnvironmentLogin(domain, "Bearer "+astroAPIToken, expiresAt); err != nil {
		return false, err
	}

	wsID, orgID := id.Workspace, id.Organization

	// Record the claims' selection before the lookup, so a lookup that fails
	// softly (astro auth token) leaves the applied token with its own selection
	// and not the saved login's.
	if err := useEnvironmentSelection(&config.Context{Domain: domain}, orgID, "", wsID); err != nil {
		return false, err
	}
	_, orgProduct, err := environmentOrganization(astroV1Client)
	if err != nil {
		return false, err
	}

	// A token with no workspaceId claim, an Organization token, selects no
	// workspace, and the run has none unless a flag or the project's link names
	// one. The workspace the interactive login last used is a selection nobody
	// made in this run, so it is not inherited (the empty value below hides it,
	// for this process only).
	// The selection is recorded under the context the login was given to, by
	// its domain, not under whatever domain the config re-read says.
	// Again, now with the product the lookup found: the earlier call could not
	// know it.
	if err := useEnvironmentSelection(&config.Context{Domain: domain}, orgID, orgProduct, wsID); err != nil {
		return false, err
	}
	return true, nil
}

// useEnvironmentLogin makes token, from the environment, the login of
// domain's context for this process only, and that host the current one
// (config.GetCurrentDomain reads the host of an environment login first). The
// saved login and the saved current context are neither read nor changed, and
// no context is saved for the host: the environment login stands in for the
// context's entry in memory (Context.ContextExists), and no later save writes
// it. A failure is the run's: without the overlay the context would read as
// the saved login.
func useEnvironmentLogin(domain, token string, expiresAt time.Time) error {
	c := config.Context{Domain: domain}
	if err := c.UseEnvironmentLogin(token, expiresAt); err != nil {
		return fmt.Errorf("cannot apply ASTRO_API_TOKEN to this run: %w", err)
	}
	return nil
}

// environmentOrganization looks up the organization the credentials in the
// environment belong to, and its product. The product cannot be skipped: it is
// what tells a hosted organization from a hybrid one (organization.IsOrgHosted),
// and an environment login keeps it only for the process, so every run asks.
// Every failure is ErrAPITokenOrganization, which stops the command rather
// than letting it run without an organization.
func environmentOrganization(astroV1Client astrov1.APIClient) (orgID, orgProduct string, err error) {
	orgs, err := organization.ListOrganizations(astroV1Client)
	if err != nil {
		return "", "", fmt.Errorf("%w: %w", ErrAPITokenOrganization, err)
	}
	if len(orgs) == 0 {
		return "", "", fmt.Errorf("%w: the credentials can see no organization", ErrAPITokenOrganization)
	}
	if orgs[0].Product == nil {
		return "", "", fmt.Errorf("%w: organization %s names no product", ErrAPITokenOrganization, orgs[0].Id)
	}
	return orgs[0].Id, string(*orgs[0].Product), nil
}

// useEnvironmentSelection records the organization and workspace the
// environment's credential is for, for this process only, as the credential
// itself is. An empty wsID is recorded as empty, which hides the saved one.
// A failure is returned: a run that cannot record them would read the saved
// login's, which the token never chose.
func useEnvironmentSelection(c *config.Context, orgID, orgProduct, wsID string) error {
	for _, field := range []struct{ key, value string }{
		{"workspace", wsID},
		{"organization", orgID},
		{"organization_product", orgProduct},
	} {
		if err := c.SetEnvironmentContextKey(field.key, field.value); err != nil {
			return fmt.Errorf("cannot apply ASTRO_API_TOKEN's %s to this run: %w", field.key, err)
		}
	}
	return nil
}
