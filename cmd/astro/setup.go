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
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/astrosession"
	"github.com/astronomer/astro-cli/internal/platform/astro/auth"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/internal/platform/astro/organization"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/logger"
	"github.com/astronomer/astro-cli/pkg/util"
)

var (
	authLogin             = auth.Login
	fetchDomainAuthConfig = auth.FetchDomainAuthConfig
	defaultDomain         = "astronomer.io"
	client                = httputil.NewHTTPClient()
	isDeploymentFile      = false
	parseAPIToken         = util.ParseAPIToken
	errNotAPIToken        = errors.New("the API token given does not appear to be an Astro API Token")
	errExpiredAPIToken    = errors.New("the API token given has expired")
)

const (
	topLvlCmd     = "astro"
	deploymentCmd = "deployment"
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

	// If the user is using dev commands no need to go through auth setup,
	// unless the workspace or deployment ID flag is set.
	if cmd.CalledAs() == "dev" && cmd.Parent().Use == topLvlCmd && !workspaceOrDeploymentIDFlagSet(cmd) {
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

	// completion command does not need auth setup
	if cmd.Parent().Use == "completion" {
		return nil
	}

	// context command does not need auth setup
	if cmd.Parent().Use == "context" {
		return nil
	}

	// if deployment inspect, create, or update commands are used
	deploymentCmds := []string{"inspect", "create", "update"}
	if util.Contains(deploymentCmds, cmd.CalledAs()) && cmd.Parent().Use == deploymentCmd {
		isDeploymentFile = true
	}

	// `astro auth token` prints a token for scripts and for Otto, which runs it
	// with no terminal. It renews the login it prints itself (FreshLogin) and
	// must never start the login flow, which would wait for a browser nobody
	// is watching. Credentials in the environment still apply.
	if isAuthTokenCmd(cmd) {
		// Quietly: the command's output is the token alone.
		_, err := ensureEnvCredentials(true, astroV1Client)
		return err
	}

	return ensureLogin(isDeploymentFile, astroV1Client)
}

func isAuthTokenCmd(cmd *cobra.Command) bool {
	return cmd.CalledAs() == "token" && cmd.Parent() != nil && cmd.Parent().Name() == "auth"
}

// EnsureLogin runs the login check Setup gives every cloud command before it
// calls the API: an API token, then API keys, then the current login, whose
// access token is refreshed when it is about to expire (and, with no login at
// all, the login flow). A command that skips the root's pre-run, as the core tree
// does, calls this before its own API calls.
func EnsureLogin(astroV1Client astrov1.APIClient) error {
	return ensureLogin(false, astroV1Client)
}

// ensureLogin is Setup's auth half, shared with EnsureLogin.
func ensureLogin(deploymentFile bool, astroV1Client astrov1.APIClient) error {
	used, err := ensureEnvCredentials(deploymentFile, astroV1Client)
	if err != nil || used {
		return err
	}
	return checkToken(astroV1Client, os.Stdout)
}

// ensureEnvCredentials sets up an API token or API keys from the environment,
// reporting whether one was found. They come before any saved login.
func ensureEnvCredentials(deploymentFile bool, astroV1Client astrov1.APIClient) (bool, error) {
	apiToken, err := checkAPIToken(deploymentFile, astroV1Client)
	if err != nil || apiToken {
		return apiToken, err
	}
	return checkAPIKeys(astroV1Client, deploymentFile)
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
	if force && envCredentials() {
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
		return fresh, fmt.Errorf("could not renew your Astro login, run 'astro login' to log in again: %w", err)
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
	errForceWithEnvCredentials = errors.New("--force renews a saved login, and the token here comes from the environment (ASTRO_API_TOKEN or an API key), which cannot be renewed")
)

// envCredentials reports whether the environment supplies the credentials
// Setup uses before any saved login.
func envCredentials() bool {
	return astrosession.HasAPIToken() || (os.Getenv("ASTRONOMER_KEY_ID") != "" && os.Getenv("ASTRONOMER_KEY_SECRET") != "")
}

func renewLogin(c *config.Context) error {
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
	return saveRenewedLogin(c, &res)
}

func loginContext(domain string) (config.Context, error) {
	if domain != "" {
		return context.GetContext(domain)
	}
	return context.GetCurrentContext()
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
	authConfig, err := fetchDomainAuthConfig(c.Domain)
	if err != nil {
		return err
	}
	res, err := refresh(c.RefreshToken, authConfig)
	if err != nil {
		return err
	}
	return saveRenewedToken(&c, &res)
}

func checkToken(astroV1Client astrov1.APIClient, out io.Writer) error {
	c, err := context.GetCurrentContext() // get current context
	if err != nil {
		return err
	}
	expireTime, _ := c.GetExpiresIn() //nolint:errcheck // error deliberately ignored in this shell code
	// check if user is logged in
	if c.Token == "Bearer " || c.Token == "" || c.Domain == "" {
		// guide the user through the login process if not logged in
		err := authLogin(c.Domain, "", astroV1Client, out, false, false, false)
		if err != nil {
			return err
		}

		return nil
	} else if isExpired(expireTime, auth.AccessTokenRefreshMargin) {
		authConfig, err := fetchDomainAuthConfig(c.Domain)
		if err != nil {
			return err
		}
		err = c.SetAuthTenant(authConfig.DomainURL, authConfig.ClientID)
		if err != nil {
			return err
		}
		res, err := refresh(c.RefreshToken, authConfig)
		if err != nil {
			// guide the user through the login process if refresh doesn't work
			err := authLogin(c.Domain, "", astroV1Client, out, false, false, false)
			if err != nil {
				return err
			}
			// authLogin already persisted the new context, so don't fall through
			// and overwrite it with the failed refresh's zero-value token
			return nil
		}
		return saveRenewedToken(&c, &res)
	}
	return nil
}

// saveRenewedToken persists the context with the renewed access token.
func saveRenewedToken(c *config.Context, res *TokenResponse) error {
	if err := saveRenewedLogin(c, res); err != nil {
		return err
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
	if err := c.SetSharedContextKey("token", "Bearer "+res.AccessToken); err != nil {
		return err
	}
	if err := c.SetSharedContextKey("refreshtoken", refreshToken); err != nil {
		return err
	}
	if err := c.SetSharedExpiresIn(res.ExpiresIn); err != nil {
		return err
	}
	return c.SetSharedContextKey("user_email", c.UserEmail)
}

// isExpired is true if now() + a threshold is after the given date
func isExpired(t time.Time, threshold time.Duration) bool {
	return time.Now().Add(threshold).After(t)
}

// Refresh gets a new access token from the provided refresh token,
// The request is used the default client_id and endpoint for device authentication.
func refresh(refreshToken string, authConfig auth.Config) (TokenResponse, error) {
	addr := authConfig.DomainURL + "oauth/token"
	data := url.Values{
		"client_id":     {authConfig.ClientID},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}

	client := &http.Client{}

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

func checkAPIKeys(astroV1Client astrov1.APIClient, isDeploymentFile bool) (bool, error) {
	// check os variables
	astronomerKeyID := os.Getenv("ASTRONOMER_KEY_ID")
	astronomerKeySecret := os.Getenv("ASTRONOMER_KEY_SECRET")
	if astronomerKeyID == "" || astronomerKeySecret == "" {
		return false, nil
	}
	if !isDeploymentFile {
		fmt.Println("Using an Astro API key")
		fmt.Println("\nWarning: Starting June 1st, 2024, Deployment API Keys will stop working. To ensure uninterrupted access to our services, we strongly recommend transitioning to Deployment API tokens. See https://www.astronomer.io/docs/astro/deployment-api-tokens")
	}

	domain, current := environmentDomain()
	authConfig, err := fetchDomainAuthConfig(domain)
	if err != nil {
		return false, err
	}

	// setup request
	addr := authConfig.DomainURL + "oauth/token"
	data := url.Values{
		"client_id":     {astronomerKeyID},
		"client_secret": {astronomerKeySecret},
		"audience":      {"astronomer-ee"},
		"grant_type":    {"client_credentials"},
	}

	doOptions := &httputil.DoOptions{
		Data:    []byte(data.Encode()),
		Context: http_context.Background(),
		Headers: map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		Path:    addr,
		Method:  http.MethodPost,
	}

	// execute request
	res, err := client.Do(doOptions)
	if err != nil {
		logger.Fatal(err)
		return false, fmt.Errorf("cannot getaccess token with API keys: %w", err)
	}
	defer res.Body.Close()

	// decode response
	var tokenRes TokenResponse

	err = json.NewDecoder(res.Body).Decode(&tokenRes)
	if err != nil {
		return false, fmt.Errorf("cannot decode response: %w", err)
	}

	if tokenRes.Error != nil {
		return false, errors.New(tokenRes.ErrorDescription)
	}

	c, err := useEnvironmentLogin(domain, current, "Bearer "+tokenRes.AccessToken, time.Now().Add(time.Duration(tokenRes.ExpiresIn)*time.Second))
	if err != nil {
		return false, err
	}
	orgs, err := organization.ListOrganizations(astroV1Client)
	if err != nil {
		return false, err
	}

	org := orgs[0]
	orgID := org.Id
	orgProduct := fmt.Sprintf("%s", *org.Product) //nolint:staticcheck // renders the typed Product enum as a plain string

	// get workspace ID
	deployments, err := deployment.ListDeployments("", orgID, astroV1Client)
	if err != nil {
		return false, errors.Wrap(err, organization.AstronomerConnectionErrMsg)
	}
	workspaceID = deployments[0].WorkspaceId

	useEnvironmentSelection(&c, orgID, orgProduct, workspaceID)
	return true, nil
}

func checkAPIToken(isDeploymentFile bool, astroV1Client astrov1.APIClient) (bool, error) {
	// check os variables
	// Read as every reader of the variable reads it: one holding only the
	// scheme ("Bearer ") is no token, and one holding the scheme too is the
	// token after it, so the login below does not carry it twice.
	stored, ok := astrosession.APIToken()
	if !ok {
		return false, nil
	}
	astroAPIToken := astrosession.Credential(stored)
	if !isDeploymentFile {
		fmt.Println("Using an Astro API Token")
	}

	// Parse the token to peek at the custom claims
	claims, err := parseAPIToken(astroAPIToken)
	if err != nil {
		return false, err
	}
	if len(claims.Permissions) == 0 {
		return false, errNotAPIToken
	}
	// A token with no expiry of its own reads as live for a year.
	expiresAt := time.Now().AddDate(1, 0, 0)
	if claims.ExpiresAt != nil {
		if claims.ExpiresAt.Before(time.Now()) {
			fmt.Printf("The given API Token %s has expired \n", claims.APITokenID)
			return false, errExpiredAPIToken
		}
		expiresAt = claims.ExpiresAt.Time
	}

	domain, current := environmentDomain()
	c, err := useEnvironmentLogin(domain, current, "Bearer "+astroAPIToken, expiresAt)
	if err != nil {
		return false, err
	}

	var wsID, orgID string
	for _, permission := range claims.Permissions {
		splitPermission := strings.Split(permission, ":")
		permissionType := splitPermission[0]
		id := splitPermission[1]
		switch permissionType {
		case "workspaceId":
			wsID = id
		case "organizationId":
			orgID = id
		}
	}

	orgs, err := organization.ListOrganizations(astroV1Client)
	if err != nil {
		return false, err
	}

	org := orgs[0]
	orgProduct := fmt.Sprintf("%s", *org.Product) //nolint:staticcheck // renders the typed Product enum as a plain string

	if wsID == "" {
		wsID = c.Workspace
	}

	useEnvironmentSelection(&c, orgID, orgProduct, wsID)
	return true, nil
}

// environmentDomain returns the domain a credential from the environment is
// used on: the current context's, or when there is none, ASTRO_DOMAIN's or
// astronomer.io. current reports the first case. It reads no saved login.
func environmentDomain() (domain string, current bool) {
	if d, err := config.GetCurrentDomain(); err == nil && context.Exists(d) {
		return d, true
	}
	if d := os.Getenv("ASTRO_DOMAIN"); d != "" {
		return d, false
	}
	return defaultDomain, false
}

// useEnvironmentLogin makes token, from the environment, the login of
// domain's context for this process only, and returns the context as the
// process now reads it. The saved login stays as it is. A domain that is not
// current is made current, and its context created when there is none, so
// the command stays on the host the environment names.
func useEnvironmentLogin(domain string, current bool, token string, expiresAt time.Time) (config.Context, error) {
	c := config.Context{Domain: domain}
	if err := c.UseEnvironmentLogin(token, expiresAt); err != nil {
		return c, err
	}
	if !current {
		if err := context.Switch(domain); err != nil {
			return c, err
		}
	}
	return c.GetContext()
}

// useEnvironmentSelection records the organization and workspace the
// environment's credential is for, for this process only, as the credential
// itself is.
func useEnvironmentSelection(c *config.Context, orgID, orgProduct, wsID string) {
	if err := c.SetEnvironmentContextKey("workspace", wsID); err != nil {
		fmt.Println("no workspace set")
	}
	if err := c.SetEnvironmentContextKey("organization", orgID); err != nil {
		fmt.Println("no organization context set")
		return
	}
	if err := c.SetEnvironmentContextKey("organization_product", orgProduct); err != nil {
		fmt.Println("no organization context set")
	}
}

func workspaceOrDeploymentIDFlagSet(cmd *cobra.Command) bool {
	wsID, _ := cmd.Flags().GetString("workspace-id")   //nolint:errcheck // error deliberately ignored in this shell code
	depID, _ := cmd.Flags().GetString("deployment-id") //nolint:errcheck // error deliberately ignored in this shell code
	return wsID != "" || depID != ""
}
