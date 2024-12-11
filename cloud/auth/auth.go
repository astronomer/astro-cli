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

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	"github.com/astronomer/astro-cli/cloud/workspace"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/domainutil"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/util"
)

const (
	AuthFlowCurrent       = "ORG_FIRST"
	AuthFlowIdentityFirst = "IDENTITY_FIRST"

	authConfigEndpoint = "auth-config"

	cliChooseWorkspace     = "Please choose a workspace:"
	cliSetWorkspaceExample = "\nNo default workspace detected, you can list workspaces with \n\tastro workspace list\nand set your default workspace with \n\tastro workspace switch [WORKSPACEID]\n\n"

	configSetDefaultWorkspace = "\n\"%s\" Workspace found. This is your default Workspace.\n"

	registryAuthSuccessMsg = "\nSuccessfully authenticated to Astronomer"
)

var (
	httpClient          = httputil.NewHTTPClient()
	openURL             = browser.OpenURL
	ErrorNoOrganization = errors.New("no organization found. Please contact your Astro Organization Owner to be invited to the organization")
	errEmailNotFound    = errors.New("cannot retrieve email")
)

var (
	err             error
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

// Config holds data related to oAuth and basic authentication
type Config struct {
	ClientID  string `json:"clientId"`
	Audience  string `json:"audience"`
	DomainURL string `json:"domainUrl"`
}

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

func authorizeCallbackHandler() (string, error) {
	m := http.NewServeMux()
	s := http.Server{Addr: callbackServer, Handler: m, ReadHeaderTimeout: 0}
	m.HandleFunc("/callback", func(w http.ResponseWriter, req *http.Request) {
		defer req.Body.Close()
		if errorCode, ok := req.URL.Query()["error"]; ok {
			callbackChannel <- CallbackMessage{errorMessage: fmt.Sprintf("Could not authorize your device. %s: %s",
				errorCode, req.URL.Query()["error_description"])}
			resp := &http.Request{}
			http.Redirect(w, resp, "https://auth.astronomer.io/device/denied", http.StatusFound)
		} else {
			callbackChannel <- CallbackMessage{authorizationCode: req.URL.Query().Get("code")}
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
		case callbackMessage := <-callbackChannel:
			if callbackMessage.errorMessage != "" {
				return "", errors.New(callbackMessage.errorMessage)
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
	err := s.Shutdown(http_context.Background())
	if err != nil {
		fmt.Printf("error: %s", err)
	}

	// return code
	return authorizationCode, nil
}

func (a *Authenticator) authDeviceLogin(authConfig Config, shouldDisplayLoginLink bool) (Result, error) { //nolint:gocritic
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
		"%sauthorize?prompt=login&audience=%s&client_id=%s&redirect_uri=%s&scope=openid profile email offline_access&response_type=code&response_mode=query&code_challenge=%s&code_challenge_method=S256",
		authConfig.DomainURL,
		authConfig.Audience,
		authConfig.ClientID,
		redirectURI,
		challenge,
	)

	authorizeURL = strings.Replace(authorizeURL, " ", "%20", -1)

	// open browser
	if !shouldDisplayLoginLink {
		fmt.Printf("\n%s to open the browser to log in or %s to quit…", ansi.Green("Press Enter"), ansi.Red("^C"))
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

	return res, nil
}

func switchToLastUsedWorkspace(c *config.Context, workspaces []astrocore.Workspace) (astrocore.Workspace, bool, error) {
	if c.LastUsedWorkspace != "" {
		for i := range workspaces {
			if c.LastUsedWorkspace == workspaces[i].Id {
				err := c.SetContextKey("workspace", workspaces[i].Id)
				if err != nil {
					return astrocore.Workspace{}, false, err
				}
				return workspaces[i], true, nil
			}
		}
	}
	return astrocore.Workspace{}, false, nil
}

// check client status after a successfully login
func CheckUserSession(c *config.Context, coreClient astrocore.CoreClient, platformCoreClient astroplatformcore.CoreClient, out io.Writer) error {
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
	// fetch all orgs that the user can access
	organizationListParams := &astroplatformcore.ListOrganizationsParams{}
	orgsResp, err := platformCoreClient.ListOrganizationsWithResponse(http_context.Background(), organizationListParams)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(orgsResp.HTTPResponse, orgsResp.Body)
	if err != nil {
		return err
	}
	orgsPaginated := *orgsResp.JSON200
	orgs := orgsPaginated.Organizations
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

	orgProduct := "HYBRID"
	if activeOrg.Product != nil {
		orgProduct = fmt.Sprintf("%s", *activeOrg.Product) //nolint
	}
	err = c.SetOrganizationContext(activeOrg.Id, orgProduct)
	if err != nil {
		return err
	}
	workspaces, err := workspace.GetWorkspaces(coreClient)
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
			err := workspace.Switch("", coreClient, out)
			if err != nil {
				fmt.Print(cliSetWorkspaceExample)
			}
		} else {
			fmt.Printf(configSetDefaultWorkspace, w.Name)
		}
	}
	return nil
}

// Login handles authentication to astronomer api and registry
func Login(domain, token string, coreClient astrocore.CoreClient, platformCoreClient astroplatformcore.CoreClient, out io.Writer, shouldDisplayLoginLink bool) error {
	var res Result
	domain = domainutil.FormatDomain(domain)
	authConfig, err := FetchDomainAuthConfig(domain)
	if err != nil {
		return err
	}
	// Welcome User
	fmt.Print("\nWelcome to the Astro CLI 🚀\n\n")
	fmt.Print("To learn more about Astro, go to https://astronomer.io/docs\n\n")

	c, _ := context.GetCurrentContext()

	if token == "" {
		res, err = authenticator.authDeviceLogin(authConfig, shouldDisplayLoginLink)
		if err != nil {
			return err
		}
	} else {
		fmt.Print("You are logging into Astro via an OAuth token\nThis token will expire in 1 hour and will not refresh\n\n")
		res = Result{
			AccessToken: token,
			ExpiresIn:   3600, //nolint:gomnd
		}
	}

	// fetch user info base on access token
	userInfo, err := authenticator.userInfoRequester(authConfig, res.AccessToken)
	if err != nil {
		return err
	}
	// set email base on userinfo so it always match with access token
	res.UserEmail = userInfo.Email

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

	fmt.Printf("Logging in as %s\n", ansi.Green(res.UserEmail))

	err = CheckUserSession(&c, coreClient, platformCoreClient, out)
	if err != nil {
		return err
	}

	fmt.Println(registryAuthSuccessMsg)
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

func FetchDomainAuthConfig(domain string) (Config, error) {
	var (
		authConfig  Config
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
