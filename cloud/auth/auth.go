package auth

import (
	http_context "context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/lucsky/cuid"
	"github.com/pkg/browser"
	"github.com/pkg/errors"

	astro "github.com/astronomer/astro-cli/astro-client"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/cloud/workspace"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/domainutil"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/util"
)

const (
	AuthFlowCurrent       = "ORG_FIRST"
	AuthFlowIdentityFirst = "IDENTITY_FIRST"

	authConfigEndpoint = "auth-config"
	orgLookupEndpoint  = "organization-lookup"

	cliChooseWorkspace     = "Please choose a workspace:"
	cliSetWorkspaceExample = "\nNo default workspace detected, you can list workspaces with \n\tastro workspace list\nand set your default workspace with \n\tastro workspace switch [WORKSPACEID]\n\n"

	configSetDefaultWorkspace = "\n\"%s\" Workspace found. This is your default Workspace.\n"

	registryAuthSuccessMsg = "\nSuccessfully authenticated to Astronomer"
)

var (
	httpClient          = httputil.NewHTTPClient()
	openURL             = browser.OpenURL
	ErrorNoOrganization = errors.New("no Organization found. Please contact your Astro Organization Owner to be invited to the organization")
)

var (
	err             error
	callbackChannel = make(chan string, 1)
	callbackTimeout = time.Second * 300
	redirectURI     = "http://localhost:12345/callback"
	userEmail       = ""
)

var authenticator = Authenticator{
	orgChecker:      orgLookup,
	tokenRequester:  requestToken,
	callbackHandler: authorizeCallbackHandler,
}

func orgLookup(domain string) (string, error) {
	addr := domainutil.GetURLToEndpoint("https", domain, orgLookupEndpoint)

	ctx := http_context.Background()
	reqData, err := json.Marshal(orgLookupRequest{Email: userEmail})
	if err != nil {
		return "", err
	}
	doOptions := &httputil.DoOptions{
		Data:    reqData,
		Context: ctx,
		Headers: map[string]string{"x-request-id": "cli-auth-" + cuid.New(), "Content-Type": "application/json; charset=utf-8"},
		Path:    addr,
		Method:  http.MethodPost,
	}
	res, err := httpClient.Do(doOptions)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	orgs := orgLookupResults{}
	err = json.NewDecoder(res.Body).Decode(&orgs)
	if err != nil {
		return "", fmt.Errorf("cannot decode response: %w", err)
	}
	if len(orgs.OrganizationIds) == 0 {
		return "", errors.New("")
	}
	return orgs.OrganizationIds[0], nil
}

// request a device code from auth0 for the user's cli
// Get user's token using PKCE flow
func requestToken(authConfig astro.AuthConfig, verifier, code string) (Result, error) {
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
		return Result{}, fmt.Errorf("could not retrieve token: %w", err)
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

func authorizeCallbackHandler() (string, error) {
	m := http.NewServeMux()
	s := http.Server{Addr: "localhost:12345", Handler: m, ReadHeaderTimeout: 0}
	m.HandleFunc("/callback", func(w http.ResponseWriter, req *http.Request) {
		defer req.Body.Close()
		if errorCode, ok := req.URL.Query()["error"]; ok {
			log.Fatalf(
				"Could not authorize your device. %s: %s",
				errorCode, req.URL.Query()["error_description"],
			)
		} else {
			authorizationCode := req.URL.Query().Get("code")
			callbackChannel <- authorizationCode
			resp := &http.Request{}
			http.Redirect(w, resp, "https://auth.astronomer.io/device/success", http.StatusFound)
		}
	})
	go func() {
		if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	// Wait for code on channel, or timeout
	authorizationCode := ""
	for authorizationCode == "" {
		select {
		case code := <-callbackChannel:
			authorizationCode = code
		case <-time.After(callbackTimeout):

			err := s.Shutdown(http_context.Background())
			if err != nil {
				fmt.Printf("error: %s", err)
			}
			return "", errors.New("the operation has timed out")
		}
	}
	err := s.Shutdown(http_context.Background())
	if err != nil {
		fmt.Printf("error: %s", err)
	}

	// return code
	return authorizationCode, nil
}

func getUserEmail(c config.Context) (string, error) { //nolint:gocritic
	// Try to get the user's email from the config first
	email := c.UserEmail

	// If email is in config, use that and don't prompt, else prompt for email
	if email == "" {
		userEmail = input.Text("Please enter your account email: ")
	} else {
		userEmail = email
		fmt.Printf("Logging in with saved user %s\n", userEmail)
	}

	return userEmail, err
}

func (a *Authenticator) authDeviceLogin(c config.Context, authConfig astro.AuthConfig, shouldDisplayLoginLink bool, domain, auth0OrgID string) (Result, error) { //nolint:gocritic
	// try to get UserEmail from config first
	userEmail, err := getUserEmail(c)
	if err != nil {
		return Result{}, err
	}

	if userEmail == "" {
		userEmail = input.Text("Please enter your account email: ")
	}

	if (auth0OrgID == "") && authConfig.AuthFlow != AuthFlowIdentityFirst {
		auth0OrgID, err = a.orgChecker(domain)
		if err != nil {
			log.Fatalf("Something went wrong! Try again or contact Astronomer Support")
		}
	}

	// Generate PKCE verifier and challenge
	token := make([]byte, 32)                            //nolint:gomnd
	r := rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec
	r.Read(token)
	verifier := util.Base64URLEncode(token)
	hash32 := sha256.Sum256([]byte(verifier)) // Sum256 returns a [32]byte
	hash := hash32[:]
	challenge := util.Base64URLEncode(hash)
	spinnerMessage := "Waiting for login to complete in browser"
	var res Result

	authorizeURL := fmt.Sprintf(
		"%sauthorize?audience=%s&client_id=%s&redirect_uri=%s&login_hint=%s&scope=openid profile email offline_access&response_type=code&response_mode=query&code_challenge=%s&code_challenge_method=S256",
		authConfig.DomainURL,
		authConfig.Audience,
		authConfig.ClientID,
		redirectURI,
		userEmail,
		challenge,
	)
	// trigger organization specific login only when auth0OrgID is provided
	if auth0OrgID != "" {
		authorizeURL = fmt.Sprintf("%s&organization=%s", authorizeURL, auth0OrgID)
	}

	authorizeURL = strings.Replace(authorizeURL, " ", "%20", -1)

	// open browser
	if !shouldDisplayLoginLink {
		fmt.Printf("\n%s to open the browser to log in or %s to quit…", ansi.Green("Press Enter"), ansi.Red("^C"))
		fmt.Scanln()
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
		fmt.Println("\nPlease visit the following link on a device with a browser: " + authorizeURL)
		fmt.Printf("\n")
		authorizationCode, err := a.callbackHandler()
		if err != nil {
			return Result{}, err
		}
		res, err = a.tokenRequester(authConfig, verifier, authorizationCode)
		if err != nil {
			return Result{}, err
		}
	}

	res.UserEmail = userEmail
	return res, nil
}

func switchToLastUsedWorkspace(c *config.Context, workspaces []astro.Workspace) (astro.Workspace, bool, error) {
	if c.LastUsedWorkspace != "" {
		for i := range workspaces {
			if c.LastUsedWorkspace == workspaces[i].ID {
				err := c.SetContextKey("workspace", workspaces[i].ID)
				if err != nil {
					return astro.Workspace{}, false, err
				}
				return workspaces[i], true, nil
			}
		}
	}
	return astro.Workspace{}, false, nil
}

// check client status after a successfully login
func CheckUserSession(c *config.Context, authConfig astro.AuthConfig, client astro.Client, coreClient astrocore.CoreClient, out io.Writer) error {
	// fetch self user based on token
	// we set CreateIfNotExist to true so we always create astro user when a successfully login
	createIfNotExist := true
	selfResp, err := coreClient.GetSelfUserWithResponse(http_context.Background(), &astrocore.GetSelfUserParams{
		CreateIfNotExist: &createIfNotExist,
	})
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(selfResp.HTTPResponse, selfResp.Body)
	if err != nil {
		return err
	}
	activeOrgID := c.Organization
	// we only set activeOrgID base on auth org in org first auth flow
	if authConfig.AuthFlow != AuthFlowIdentityFirst && selfResp.JSON200.OrganizationId != nil {
		// OrganizationId is optional, it may not be returned by getSelf api
		activeOrgID = *selfResp.JSON200.OrganizationId
	}
	// fetch all orgs that the user can access
	orgsResp, err := coreClient.ListOrganizationsWithResponse(http_context.Background())
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(orgsResp.HTTPResponse, orgsResp.Body)
	if err != nil {
		return err
	}
	orgs := *orgsResp.JSON200
	if len(orgs) == 0 {
		return ErrorNoOrganization
	}
	// default to first one in case something crazy happen lol
	activeOrg := orgs[0]
	for i := range orgs {
		if orgs[i].Id == activeOrgID {
			activeOrg = orgs[i]
			break
		}
	}
	err = c.SetOrganizationContext(activeOrg.Id, activeOrg.ShortName)
	if err != nil {
		return err
	}

	workspaces, err := client.ListWorkspaces(activeOrg.Id)
	if err != nil {
		return err
	}

	if len(workspaces) == 1 {
		w := workspaces[0]
		err = c.SetContextKey("workspace", w.ID)
		if err != nil {
			return err
		}
		// update last used workspace ID
		err = c.SetContextKey("last_used_workspace", w.ID)
		if err != nil {
			return err
		}
		fmt.Printf(configSetDefaultWorkspace, w.Label)
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
			err := workspace.Switch("", client, out)
			if err != nil {
				fmt.Print(cliSetWorkspaceExample)
				return err
			}
		} else {
			fmt.Printf(configSetDefaultWorkspace, w.Label)
		}
	}

	fmt.Println(registryAuthSuccessMsg)

	return nil
}

// Login handles authentication to astronomer api and registry
func Login(domain, orgID, token string, client astro.Client, coreClient astrocore.CoreClient, out io.Writer, shouldDisplayLoginLink bool) error {
	var res Result
	domain = domainutil.FormatDomain(domain)
	authConfig, err := FetchDomainAuthConfig(domain)
	if err != nil {
		return err
	}
	// Welcome User
	fmt.Print("\nWelcome to the Astro CLI 🚀\n\n")
	fmt.Print("To learn more about Astro, go to https://docs.astronomer.io\n\n")

	c, _ := context.GetCurrentContext()

	if token == "" {
		res, err = authenticator.authDeviceLogin(c, authConfig, shouldDisplayLoginLink, domain, orgID)
		if err != nil {
			return err
		}
	} else {
		fmt.Println("You are logging into Astro via an OAuth token\nThis token will expire in 24 hours and will not refresh")
		res = Result{
			AccessToken: token,
			ExpiresIn:   86400, //nolint:gomnd
		}
	}

	// Create context if it does not exist
	if domain != "" {
		// Switch context now that we ensured context exists
		err = context.Switch(domain)
		if err != nil {
			return err
		}
	}

	c, err = context.GetCurrentContext()
	if err != nil {
		return err
	}

	err = res.writeToContext(&c)
	if err != nil {
		return err
	}

	err = CheckUserSession(&c, authConfig, client, coreClient, out)
	if err != nil {
		return err
	}

	return nil
}

// Logout logs a user out of the docker registry. Will need to logout of Astro next.
func Logout(domain string, out io.Writer) {
	c, _ := context.GetContext(domain)

	err = c.SetContextKey("token", "")
	if err != nil {
		return
	}
	err = c.SetContextKey("user_email", "")
	if err != nil {
		return
	}

	// remove the current context
	err = config.ResetCurrentContext()
	if err != nil {
		fmt.Fprintln(out, "Failed to reset current context: ", err.Error())
		return
	}

	fmt.Fprintln(out, "Successfully logged out of Astronomer")
}

func FetchDomainAuthConfig(domain string) (astro.AuthConfig, error) {
	var (
		authConfig  astro.AuthConfig
		addr        string
		validDomain bool
	)

	validDomain = context.IsCloudDomain(domain)
	if !validDomain {
		return authConfig, errors.New("Error! Invalid domain. You are attempting to login into Astro. " +
			"Are you trying to authenticate to Astronomer Software? If so, please change your current context with 'astro context switch'")
	}

	addr = domainutil.GetURLToEndpoint("https", domain, authConfigEndpoint)

	ctx := http_context.Background()
	doOptions := &httputil.DoOptions{
		Context: ctx,
		Headers: map[string]string{"x-request-id": "cli-auth-" + cuid.New(), "Content-Type": "application/json; charset=utf-8"},
		Path:    addr,
		Method:  http.MethodGet,
	}
	res, err := httpClient.Do(doOptions)
	if err != nil {
		return authConfig, err
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusOK {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			fmt.Println(err)
		}

		unMarshalErr := json.Unmarshal(body, &authConfig)
		if unMarshalErr != nil {
			return authConfig, fmt.Errorf("cannot decode response: %w", unMarshalErr)
		}
		return authConfig, nil
	}

	return authConfig, errors.New("something went wrong! Try again or contact Astronomer Support")
}
