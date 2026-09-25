package auth

import (
	"cmp"
	http_context "context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/browser"
	"github.com/pkg/errors"
	"golang.org/x/term"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/workspace"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/astroauth"
	"github.com/astronomer/astro-cli/pkg/domainutil"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/util"
)

const (
	cliChooseWorkspace     = "Please choose a workspace:"
	cliSetWorkspaceExample = "\nNo default workspace detected, you can list workspaces with \n\tastro workspace list\nand set your default workspace with \n\tastro workspace switch [WORKSPACEID]\n\n"

	configSetDefaultWorkspace = "\"%s\" Workspace found. This is your default Workspace.\n"

	registryAuthSuccessMsg = "Successfully authenticated to Astronomer"

	// AccessTokenRefreshMargin is how close to its expiry an access token is
	// refreshed rather than used.
	AccessTokenRefreshMargin = 5 * time.Minute
)

var (
	httpClient          = httputil.NewHTTPClient()
	openURL             = browser.OpenURL
	refreshAccessToken  = astroauth.RefreshToken
	stdinIsTerminal     = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }
	exitOnInterrupt     = os.Exit
	ErrorNoOrganization = errors.New("no organization found. Please contact your Astro Organization Owner to be invited to the organization")
	errEmailNotFound    = errors.New("cannot retrieve email")

	// ErrEmailVerificationPending reports a sign-up that worked but left the
	// email address unverified. The identity provider denies the authorization
	// with access_denied and puts the real message in error_description, so the
	// description is the only thing that tells this apart from a real refusal.
	ErrEmailVerificationPending = errors.New("your account is created but your email address is not verified yet")
)

// The length the generated organization name is held to: a shorter one gets a
// suffix, a longer one is cut.
const (
	minOrgNameLen = 3
	maxOrgNameLen = 50
)

// orgNameForbidden matches every run of characters an organization name cannot
// hold, so bootstrapOrganization can replace each run with a single dash.
var orgNameForbidden = regexp.MustCompile(`[^a-zA-Z0-9-]+`)

// signupVerificationPattern matches the denial the identity provider returns
// while an email address is unverified, for example "Thanks for signing up.
// Please check your inbox for a verification email to get started."
var signupVerificationPattern = regexp.MustCompile(`(?i)verif\w*\s+(your\s+)?email`)

var (
	callbackChannel = make(chan CallbackMessage, 1)
	callbackTimeout = time.Second * 300
	redirectURI     = "http://localhost:12345/callback"
	callbackServer  = "localhost:12345"
)

var authenticator = Authenticator{
	userInfoRequester: requestUserInfo,
	tokenRequester:    requestToken,
	callbackHandler:   authorizeCallbackHandler,
}

// Config is an alias for astroauth.AuthConfig.
type Config = astroauth.AuthConfig

func requestUserInfo(authConfig Config, accessToken string) (UserInfo, error) {
	addr := authConfig.DomainURL + "userinfo"
	ctx := http_context.Background()
	doOptions := &httputil.DoOptions{
		Context: ctx,
		Headers: map[string]string{"Content-Type": "application/json", "Authorization": fmt.Sprintf("Bearer %s", accessToken)},
		Path:    addr,
		Method:  http.MethodGet,
	}
	res, err := httpClient.Do(doOptions)
	if err != nil {
		return UserInfo{}, fmt.Errorf("cannot retrieve userinfo: %w", err)
	}
	defer res.Body.Close()

	var user UserInfo
	err = json.NewDecoder(res.Body).Decode(&user)
	if err != nil {
		return UserInfo{}, fmt.Errorf("cannot decode userinfo response: %w", err)
	}
	if user.Email == "" {
		return UserInfo{}, errEmailNotFound
	}
	return user, nil
}

// request a device code from auth0 for the user's cli
// Get user's token using PKCE flow
func requestToken(authConfig Config, verifier, code string) (Result, error) {
	addr := authConfig.DomainURL + "oauth/token"
	data := url.Values{
		"client_id":     {authConfig.ClientID},
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {redirectURI},
	}
	ctx := http_context.Background()
	doOptions := &httputil.DoOptions{
		Data:    []byte(data.Encode()),
		Context: ctx,
		Headers: map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		Path:    addr,
		Method:  http.MethodPost,
	}
	res, err := httpClient.Do(doOptions)
	if err != nil {
		return Result{}, fmt.Errorf("cannot retrieve token: %w", err)
	}
	defer res.Body.Close()

	var tokenRes postTokenResponse
	err = json.NewDecoder(res.Body).Decode(&tokenRes)
	if err != nil {
		return Result{}, fmt.Errorf("cannot decode response: %w", err)
	}

	if tokenRes.Error != nil {
		return Result{}, errors.New(tokenRes.ErrorDescription)
	}
	return Result{
		RefreshToken: tokenRes.RefreshToken,
		AccessToken:  tokenRes.AccessToken,
		ExpiresIn:    tokenRes.ExpiresIn,
	}, nil
}

// authorizeError turns the identity provider's error query parameters into an
// error. A denial that names an unverified email address is a sign-up that
// worked, so it gets its own error and the caller can say so.
func authorizeError(errorCode, errorDescription []string) error {
	if slices.Contains(errorCode, "access_denied") && signupVerificationPattern.MatchString(strings.Join(errorDescription, " ")) {
		return ErrEmailVerificationPending
	}
	return fmt.Errorf("Could not authorize your device. %s: %s", errorCode, errorDescription)
}

func authorizeCallbackHandler() (string, error) {
	m := http.NewServeMux()
	s := http.Server{Handler: m, ReadHeaderTimeout: 0}
	m.HandleFunc("/callback", func(w http.ResponseWriter, req *http.Request) {
		defer req.Body.Close()
		if errorCode, ok := req.URL.Query()["error"]; ok {
			callbackChannel <- CallbackMessage{err: authorizeError(errorCode, req.URL.Query()["error_description"])}
			resp := &http.Request{}
			http.Redirect(w, resp, "https://auth.astronomer.io/device/denied", http.StatusFound)
		} else {
			callbackChannel <- CallbackMessage{authorizationCode: req.URL.Query().Get("code")}
			resp := &http.Request{}
			http.Redirect(w, resp, "https://auth.astronomer.io/device/success", http.StatusFound)
		}
	})
	// Bind before serving, so an address already taken reaches the caller as an
	// error rather than ending the process from inside the goroutine below.
	listener, err := net.Listen("tcp", callbackServer)
	if err != nil {
		return "", fmt.Errorf("cannot open the login callback on %s: %w. Close any other astro login and try again", callbackServer, err)
	}
	go func() {
		if err := s.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			// The caller selects on this channel, so it hears a server that
			// stops early instead of waiting out the callback timeout.
			callbackChannel <- CallbackMessage{err: fmt.Errorf("the login callback server stopped: %w", err)}
		}
	}()

	// Wait for code on channel, or timeout
	authorizationCode := ""
	for authorizationCode == "" {
		select {
		case callbackMessage := <-callbackChannel:
			if callbackMessage.err != nil {
				return "", callbackMessage.err
			}
			authorizationCode = callbackMessage.authorizationCode
		case <-time.After(callbackTimeout):
			err := s.Shutdown(http_context.Background())
			if err != nil {
				fmt.Printf("error: %s", err)
			}
			return "", errors.New("the operation has timed out")
		}
	}
	err = s.Shutdown(http_context.Background())
	if err != nil {
		fmt.Printf("error: %s", err)
	}

	// return code
	return authorizationCode, nil
}

// quitOnInterrupt ends the process on ^C or SIGTERM while a login waits on the
// user. main turns the first signal into a context cancel, and neither the
// Enter prompt nor the callback server can see that context, so without this
// the first ^C is swallowed and the login waits out its five-minute timeout.
func quitOnInterrupt() (stop func()) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		select {
		case <-signals:
			fmt.Println()
			exitOnInterrupt(130) //nolint:mnd // the shell's exit status for a process ended by SIGINT
		case <-done:
		}
	}()
	return func() {
		signal.Stop(signals)
		close(done)
	}
}

func (a *Authenticator) authDeviceLogin(authConfig Config, shouldDisplayLoginLink, signup, force bool) (Result, error) {
	// Generate PKCE verifier and challenge
	token := make([]byte, 32)                            //nolint:mnd // the value is clear from context
	r := rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec // reviewed; not a new risk in this v1 code
	r.Read(token)
	verifier := util.Base64URLEncode(token)
	hash32 := sha256.Sum256([]byte(verifier)) // Sum256 returns a [32]byte
	hash := hash32[:]
	challenge := util.Base64URLEncode(hash)
	spinnerMessage := "Waiting for login to complete in browser"
	var res Result

	authorizeURL := fmt.Sprintf(
		"%sauthorize?audience=%s&client_id=%s&redirect_uri=%s&scope=openid profile email offline_access&response_type=code&response_mode=query&code_challenge=%s&code_challenge_method=S256",
		authConfig.DomainURL,
		authConfig.Audience,
		authConfig.ClientID,
		redirectURI,
		challenge,
	)

	authorizeURL = strings.Replace(authorizeURL, " ", "%20", -1)

	if force || signup {
		authorizeURL += "&prompt=login"
	}
	if signup {
		// screen_hint routes the universal login to its sign-up screen;
		// ext-signup-source tags the account the way the web flow's
		// ext-* params do, so the funnel can tell CLI signups apart.
		authorizeURL += "&screen_hint=signup&ext-signup-source=cli"
	}

	stopWatching := quitOnInterrupt()
	defer stopWatching()

	// A run without a terminal — a coding agent's shell, CI — never answers the
	// prompt below, so it takes the login-link path instead of blocking on stdin.
	if !shouldDisplayLoginLink && stdinIsTerminal() {
		action := "log in"
		if signup {
			action = "create your Astro account"
			fmt.Printf("Already have an Astro account? Log in from the same page, or run %s.\n", ansi.Cyan("astro login --signin"))
		}
		fmt.Printf("%s to open the browser to %s or %s to quit…", ansi.Green("Press Enter"), action, ansi.Red("^C"))
		_, err := fmt.Scanln()
		if err != nil {
			return Result{}, err
		}
		err = openURL(authorizeURL)
		if err != nil {
			fmt.Println("\nUnable to open the URL, please visit the following link: " + authorizeURL)
			fmt.Printf("\n")
		}
		err = ansi.Spinner(spinnerMessage, func() error {
			authorizationCode, err := a.callbackHandler()
			if err != nil {
				return err
			}
			res, err = a.tokenRequester(authConfig, verifier, authorizationCode)
			return err
		})
		if err != nil {
			return Result{}, err
		}
	} else {
		fmt.Println("Please visit the following link on a device with a browser: " + authorizeURL)
		authorizationCode, err := a.callbackHandler()
		if err != nil {
			return Result{}, err
		}
		res, err = a.tokenRequester(authConfig, verifier, authorizationCode)
		if err != nil {
			return Result{}, err
		}
	}

	return res, nil
}

// bootstrapOrganization creates a brand-new account's first organization and
// workspace, the same call the web onboarding makes. The org name comes from
// the email's local part; nothing else about the account exists yet to name
// it after.
func bootstrapOrganization() error {
	// The caller's context is stale: writeToContext persists through
	// SetContextKey, which writes the config file and never the struct. Read
	// the context back so the token is the one this login just minted.
	c, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	localPart, _, _ := strings.Cut(c.UserEmail, "@")
	orgName := strings.Trim(orgNameForbidden.ReplaceAllString(localPart, "-"), "-")
	if len(orgName) < minOrgNameLen {
		orgName += "-org"
	}
	if len(orgName) > maxOrgNameLen {
		orgName = orgName[:maxOrgNameLen]
	}

	body, err := json.Marshal(map[string]any{
		"organization": map[string]any{"name": orgName},
		"workspace":    map[string]any{"name": "Default"},
	})
	if err != nil {
		return err
	}
	addr := domainutil.GetURLToEndpoint("https", c.Domain, "private/v1alpha1/create-organization-and-workspace")
	doOptions := &httputil.DoOptions{
		Context: http_context.Background(),
		Headers: map[string]string{
			"Content-Type":              "application/json",
			"Authorization":             c.Token,
			"X-Astro-Client-Identifier": "cli",
		},
		Path:   addr,
		Method: http.MethodPost,
		Data:   body,
	}
	res, err := httpClient.Do(doOptions)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	// httputil.Do already turns 4xx and 5xx into an error. A create call may
	// answer 201, so accept every 2xx and reject the rest.
	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		out, _ := io.ReadAll(res.Body) //nolint:errcheck // the status line already names the failure; the body is best-effort detail
		return fmt.Errorf("creating your organization failed (HTTP %d): %s", res.StatusCode, string(out))
	}
	return nil
}

// resolveActiveOrg returns the org the current context points at. When a context
// org is set, we fetch it directly by ID — listing-and-searching with no limit
// silently fell back to the first page's [0] when the user belonged to more orgs
// than the API's default page size (#2083).
//
// If GetOrganization returns 403/404, the context org is no longer accessible
// (membership revoked, org deleted, or state left over from a prior account on
// the same domain), so we fall through to list-and-pick-first to keep the user
// usable. Properly scrubbing identity-scoped state on logout and hydrating it
// on login would let us drop that fallback — see #2097.
func resolveActiveOrg(c *config.Context, astroV1Client astrov1.APIClient) (*astrov1.Organization, error) {
	if c.Organization != "" {
		orgResp, err := astroV1Client.GetOrganizationWithResponse(http_context.Background(), c.Organization, &astrov1.GetOrganizationParams{})
		if err != nil {
			return nil, err
		}
		stale := orgResp.HTTPResponse != nil && (orgResp.HTTPResponse.StatusCode == http.StatusForbidden || orgResp.HTTPResponse.StatusCode == http.StatusNotFound)
		if !stale {
			if err := astrov1.NormalizeAPIError(orgResp.HTTPResponse, orgResp.Body); err != nil {
				return nil, err
			}
			if orgResp.JSON200 == nil {
				return nil, ErrorNoOrganization
			}
			return orgResp.JSON200, nil
		}
	}

	limit := 100
	orgsResp, err := astroV1Client.ListOrganizationsWithResponse(http_context.Background(), &astrov1.ListOrganizationsParams{Limit: &limit})
	if err != nil {
		return nil, err
	}
	if err := astrov1.NormalizeAPIError(orgsResp.HTTPResponse, orgsResp.Body); err != nil {
		return nil, err
	}
	orgs := orgsResp.JSON200.Organizations
	if len(orgs) == 0 {
		return nil, ErrorNoOrganization
	}
	return &orgs[0], nil
}

func switchToLastUsedWorkspace(c *config.Context, workspaces []astrov1.Workspace) (astrov1.Workspace, bool, error) {
	if c.LastUsedWorkspace != "" {
		for i := range workspaces {
			if c.LastUsedWorkspace == workspaces[i].Id {
				err := c.SetContextKey("workspace", workspaces[i].Id)
				if err != nil {
					return astrov1.Workspace{}, false, err
				}
				return workspaces[i], true, nil
			}
		}
	}
	return astrov1.Workspace{}, false, nil
}

// CheckUserSession checks the client status after a successful login. Callers
// outside a fresh login reuse it, and none of them sign up, so it answers the
// zero-organization question by asking the user.
func CheckUserSession(c *config.Context, astroV1Client astrov1.APIClient, out io.Writer) error {
	return checkUserSession(c, astroV1Client, out, false)
}

func checkUserSession(c *config.Context, astroV1Client astrov1.APIClient, out io.Writer, signup bool) error {
	// fetch self user based on token
	// we set CreateIfNotExist to true so we always create astro user when a successfully login
	createIfNotExist := true
	selfResp, err := astroV1Client.GetSelfUserWithResponse(http_context.Background(), &astrov1.GetSelfUserParams{
		CreateIfNotExist: &createIfNotExist,
	})
	if err != nil {
		return err
	}
	err = astrov1.NormalizeAPIError(selfResp.HTTPResponse, selfResp.Body)
	if err != nil {
		return err
	}
	activeOrg, err := resolveActiveOrg(c, astroV1Client)
	if errors.Is(err, ErrorNoOrganization) {
		// A zero-org login is either a brand-new account (create its first
		// org) or someone waiting on an invite (creating would burn their
		// one trial-org slot) — only they know which, so ask. --signup
		// preselects yes; it already declared the intent.
		create := signup
		if !create {
			create, _ = input.Confirm("No organization found. Create your own free organization now") //nolint:errcheck // a read error answers no, same as declining
		}
		if !create {
			return err
		}
		fmt.Fprintln(out, "Creating your organization…")
		if bootstrapErr := bootstrapOrganization(); bootstrapErr != nil {
			return bootstrapErr
		}
		activeOrg, err = resolveActiveOrg(c, astroV1Client)
	}
	if err != nil {
		return err
	}

	orgProduct := "HYBRID"
	if activeOrg.Product != nil {
		orgProduct = fmt.Sprintf("%s", *activeOrg.Product) //nolint:staticcheck // renders the typed Product enum as a plain string
	}
	err = c.SetOrganizationContext(activeOrg.Id, orgProduct)
	if err != nil {
		return err
	}
	workspaces, err := workspace.GetWorkspaces(astroV1Client)
	if err != nil {
		return err
	}
	if len(workspaces) == 1 {
		w := workspaces[0]
		err = c.SetContextKey("workspace", w.Id)
		if err != nil {
			return err
		}
		// update last used workspace ID
		err = c.SetContextKey("last_used_workspace", w.Id)
		if err != nil {
			return err
		}
		fmt.Printf(configSetDefaultWorkspace, w.Name)
	}
	if len(workspaces) > 1 {
		// try to switch to last used workspace in context
		w, isSwitched, err := switchToLastUsedWorkspace(c, workspaces)
		if err != nil {
			return err
		}
		if !isSwitched {
			// show switch menu with available workspace IDs
			fmt.Println("\n" + cliChooseWorkspace)
			err := workspace.Switch("", astroV1Client, out)
			if err != nil {
				fmt.Print(cliSetWorkspaceExample)
			}
		} else {
			fmt.Printf(configSetDefaultWorkspace, w.Name)
		}
	}
	return nil
}

// ShouldSignup reports whether a login to domain should open the sign-up screen
// rather than the sign-in screen. It says yes when the CLI holds nothing for
// that domain, which is the state a first-run user is in. Only production takes
// new accounts this way: the other environments are Astronomer's own, and
// whoever logs in to them already has an account.
func ShouldSignup(domain string) bool {
	domain = domainutil.FormatDomain(domain)
	return domain == domainutil.DefaultDomain && shouldSignup(context.GetContext, domain)
}

// getContext is a parameter, not a direct call, so a test does not need a real ~/.astro.
func shouldSignup(getContext func(domain string) (config.Context, error), domain string) bool {
	c, err := getContext(domain)
	if err != nil {
		return true
	}
	// Only a login to this domain writes these, so any one of them means the
	// account exists. An expired token counts as much as a fresh one: it is
	// stale, not proof that the user never signed up.
	return c.Token == "" && c.RefreshToken == "" && c.Organization == "" &&
		c.Workspace == "" && c.LastUsedWorkspace == "" && c.UserEmail == ""
}

// Login handles authentication to astronomer api and registry. Unless force,
// signup or a token asks for a new login, it first tries the login already
// saved for domain and opens the browser only when that login no longer works.
func Login(domain, token string, astroV1Client astrov1.APIClient, out io.Writer, shouldDisplayLoginLink, signup, force bool) error {
	domain = domainutil.FormatDomain(domain)
	authConfig, err := FetchDomainAuthConfig(domain)
	if err != nil {
		return err
	}

	if token == "" && !force && !signup {
		if res, ok := authenticator.savedLogin(domain, authConfig); ok {
			fmt.Printf("Using your saved login for %s\n", domain)
			return completeLogin(domain, res, astroV1Client, out, false)
		}
	}

	// Welcome User
	fmt.Print("Welcome to the Astro CLI 🚀\n")
	fmt.Print("To learn more about Astro, go to https://www.astronomer.io/docs\n")

	var res Result
	if token == "" {
		res, err = authenticator.authDeviceLogin(authConfig, shouldDisplayLoginLink, signup, force)
		if err != nil {
			return err
		}
	} else {
		fmt.Print("You are logging into Astro via an OAuth token\nThis token will expire in 1 hour and will not refresh\n")
		res = Result{
			AccessToken: token,
			ExpiresIn:   3600,
		}
	}

	// fetch user info base on access token
	userInfo, err := authenticator.userInfoRequester(authConfig, res.AccessToken)
	if err != nil {
		return err
	}
	// set email base on userinfo so it always match with access token
	res.UserEmail = userInfo.Email

	return completeLogin(domain, res, astroV1Client, out, signup)
}

// Switch points the CLI at domain using the login saved for it, and never opens
// a browser. With no working login it still switches, and says how to log in.
func Switch(domain string, astroV1Client astrov1.APIClient, out io.Writer) error {
	domain = domainutil.FormatDomain(domain)
	authConfig, fetchErr := FetchDomainAuthConfig(domain)
	if fetchErr == nil {
		if res, ok := authenticator.savedLogin(domain, authConfig); ok {
			return completeLogin(domain, res, astroV1Client, out, false)
		}
	}
	if err := context.Switch(domain); err != nil {
		return err
	}
	if fetchErr != nil {
		fmt.Fprintf(out, "Switched to %s, but could not check its login: %s\n", domain, fetchErr)
		return nil
	}
	fmt.Fprintf(out, "Switched to %s, but there is no working login for it. Run 'astro login %s' to log in.\n", domain, domain)
	return nil
}

// savedLogin tries the saved access token before the refresh token. An access
// token userinfo rejects, such as an API token saved from ASTRO_API_TOKEN,
// still leaves the refresh token to try.
func (a *Authenticator) savedLogin(domain string, authConfig Config) (Result, bool) {
	c, err := context.GetContext(domain)
	if err != nil {
		return Result{}, false
	}
	expiry, _ := c.GetExpiresIn() //nolint:errcheck // a missing expiry reads as zero, which is expired
	if accessToken := strings.TrimSpace(strings.TrimPrefix(c.Token, "Bearer ")); accessToken != "" && time.Now().Add(AccessTokenRefreshMargin).Before(expiry) {
		res := Result{AccessToken: accessToken, RefreshToken: c.RefreshToken, ExpiresIn: int64(time.Until(expiry).Seconds())}
		if a.withOwner(authConfig, &res) {
			return res, true
		}
	}
	if c.RefreshToken == "" {
		return Result{}, false
	}
	tok, err := refreshAccessToken(authConfig, c.RefreshToken)
	if err != nil {
		return Result{}, false
	}
	res := Result{AccessToken: tok.AccessToken, RefreshToken: cmp.Or(tok.RefreshToken, c.RefreshToken), ExpiresIn: tok.ExpiresIn}
	ok := a.withOwner(authConfig, &res)
	return res, ok
}

func (a *Authenticator) withOwner(authConfig Config, res *Result) bool {
	userInfo, err := a.userInfoRequester(authConfig, res.AccessToken)
	if err != nil {
		return false
	}
	res.UserEmail = userInfo.Email
	return true
}

func completeLogin(domain string, res Result, astroV1Client astrov1.APIClient, out io.Writer, signup bool) error {
	err := context.Switch(domain)
	if err != nil {
		return err
	}
	c, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	err = res.writeToContext(&c)
	if err != nil {
		return err
	}

	fmt.Printf("Logging in as %s\n", ansi.Green(res.UserEmail))

	err = checkUserSession(&c, astroV1Client, out, signup)
	if err != nil {
		return err
	}

	fmt.Println(registryAuthSuccessMsg)
	return nil
}

// Logout logs a user out of the docker registry. Will need to logout of Astro next.
func Logout(domain string, out io.Writer) {
	domain = domainutil.FormatDomain(domain)
	c, _ := context.GetContext(domain) //nolint:errcheck // falls back to the zero context in this v1 path

	err := c.SetContextKey("token", "")
	if err != nil {
		return
	}
	err = c.SetContextKey("refreshtoken", "")
	if err != nil {
		return
	}
	err = c.SetContextKey("user_email", "")
	if err != nil {
		return
	}

	if current, _ := config.GetCurrentDomain(); current == domain { //nolint:errcheck // no current context means there is none to reset
		err = config.ResetCurrentContext()
		if err != nil {
			fmt.Fprintln(out, "Failed to reset current context: ", err.Error())
			return
		}
	}

	fmt.Fprintln(out, "Successfully logged out of Astronomer")
}

func FetchDomainAuthConfig(domain string) (Config, error) {
	if !context.IsCloudDomain(domain) {
		return Config{}, errors.New("Error! Invalid domain. You are attempting to login into Astro. " +
			"Are you trying to authenticate to APC? If so, please change your current context with 'astro context switch'")
	}

	addr := domainutil.GetURLToEndpoint("https", domain, astroauth.AuthConfigEndpoint)

	ctx := http_context.Background()
	doOptions := &httputil.DoOptions{
		Context: ctx,
		Headers: map[string]string{
			"Content-Type":              "application/json",
			"X-Astro-Client-Identifier": "cli",
		},
		Path:   addr,
		Method: http.MethodGet,
	}
	res, err := httpClient.Do(doOptions)
	if err != nil {
		return Config{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return Config{}, errors.New("something went wrong! Try again or contact Astronomer Support")
	}

	var authConfig Config
	err = json.NewDecoder(res.Body).Decode(&authConfig)
	if err != nil {
		return Config{}, fmt.Errorf("cannot decode response: %w", err)
	}

	return authConfig, nil
}
