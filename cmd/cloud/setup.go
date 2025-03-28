package cloud

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

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	"github.com/astronomer/astro-cli/cloud/auth"
	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/cloud/organization"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/logger"
	"github.com/astronomer/astro-cli/pkg/util"
	"github.com/golang-jwt/jwt/v4"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	authLogin          = auth.Login
	defaultDomain      = "astronomer.io"
	client             = httputil.NewHTTPClient()
	isDeploymentFile   = false
	parseAPIToken      = util.ParseAPIToken
	errNotAPIToken     = errors.New("the API token given does not appear to be an Astro API Token")
	errExpiredAPIToken = errors.New("the API token given has expired")
)

const (
	accessTokenExpThreshold = 5 * time.Minute
	topLvlCmd               = "astro"
	deploymentCmd           = "deployment"
)

type TokenResponse struct {
	AccessToken      string  `json:"access_token"`
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

//nolint:gocognit
func Setup(cmd *cobra.Command, platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient) error {
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

	// Check for APITokens before API keys or refresh tokens
	apiToken, err := checkAPIToken(isDeploymentFile, platformCoreClient)
	if err != nil {
		return err
	}
	if apiToken {
		return nil
	}

	// run auth setup for any command that requires auth
	apiKey, err := checkAPIKeys(platformCoreClient, isDeploymentFile)
	if err != nil {
		return err
	}
	if apiKey {
		return nil
	}
	err = checkToken(coreClient, platformCoreClient, os.Stdout)
	if err != nil {
		return err
	}

	return nil
}

func checkToken(coreClient astrocore.CoreClient, platformCoreClient astroplatformcore.CoreClient, out io.Writer) error {
	c, err := context.GetCurrentContext() // get current context
	if err != nil {
		return err
	}
	expireTime, _ := c.GetExpiresIn()
	// check if user is logged in
	if c.Token == "Bearer " || c.Token == "" || c.Domain == "" {
		// guide the user through the login process if not logged in
		err := authLogin(c.Domain, "", coreClient, platformCoreClient, out, false)
		if err != nil {
			return err
		}

		return nil
	} else if isExpired(expireTime, accessTokenExpThreshold) {
		authConfig, err := auth.FetchDomainAuthConfig(c.Domain)
		if err != nil {
			return err
		}
		res, err := refresh(c.RefreshToken, authConfig)
		if err != nil {
			// guide the user through the login process if refresh doesn't work
			err := authLogin(c.Domain, "", coreClient, platformCoreClient, out, false)
			if err != nil {
				return err
			}
		}
		// persist the updated context with the renewed access token
		err = c.SetContextKey("token", "Bearer "+res.AccessToken)
		if err != nil {
			return err
		}
		err = c.SetExpiresIn(res.ExpiresIn)
		if err != nil {
			return err
		}
		err = c.SetContextKey("workspace", c.Workspace)
		if err != nil {
			return err
		}
		err = c.SetContextKey("workspace", c.LastUsedWorkspace)
		if err != nil {
			return err
		}
		err = c.SetContextKey("organization", c.Organization)
		if err != nil {
			return err
		}
		err = c.SetContextKey("organization_product", c.OrganizationProduct)
		if err != nil {
			return err
		}
	}
	return nil
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
		logger.Fatal(err)
		return TokenResponse{}, fmt.Errorf("cannot get a new access token from the refresh token: %w", err)
	}
	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	res, err := client.Do(r)
	if err != nil {
		logger.Fatal(err)
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

func checkAPIKeys(platformCoreClient astroplatformcore.CoreClient, isDeploymentFile bool) (bool, error) {
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

	// get authConfig
	c, err := context.GetCurrentContext() // get current context
	if err != nil {
		// set context
		var domain string
		if domain = os.Getenv("ASTRO_DOMAIN"); domain == "" {
			domain = defaultDomain
		}
		if !context.Exists(domain) {
			err := context.SetContext(domain)
			if err != nil {
				return false, err
			}
		}

		// Switch context
		err = context.Switch(domain)
		if err != nil {
			return false, err
		}

		c, err = context.GetContext(domain) // get current context
		if err != nil {
			return false, err
		}
	}

	authConfig, err := auth.FetchDomainAuthConfig(c.Domain)
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

	err = c.SetContextKey("token", "Bearer "+tokenRes.AccessToken)
	if err != nil {
		return false, err
	}

	err = c.SetExpiresIn(tokenRes.ExpiresIn)
	if err != nil {
		return false, err
	}
	orgs, err := organization.ListOrganizations(platformCoreClient)
	if err != nil {
		return false, err
	}

	org := orgs[0]
	orgID := org.Id
	orgProduct := fmt.Sprintf("%s", *org.Product) //nolint

	// get workspace ID
	deployments, err := deployment.CoreGetDeployments("", orgID, platformCoreClient)
	if err != nil {
		return false, errors.Wrap(err, organization.AstronomerConnectionErrMsg)
	}
	workspaceID = deployments[0].WorkspaceId

	err = c.SetContextKey("workspace", workspaceID) // c.Workspace
	if err != nil {
		fmt.Println("no workspace set")
	}

	err = c.SetOrganizationContext(orgID, orgProduct)
	if err != nil {
		fmt.Println("no organization context set")
	}
	return true, nil
}

func checkAPIToken(isDeploymentFile bool, platformCoreClient astroplatformcore.CoreClient) (bool, error) {
	// check os variables
	astroAPIToken := os.Getenv("ASTRO_API_TOKEN")
	if astroAPIToken == "" {
		return false, nil
	}
	if !isDeploymentFile {
		fmt.Println("Using an Astro API Token")
	}

	// get authConfig
	c, err := context.GetCurrentContext() // get current context
	if err != nil {
		// set context
		var domain string
		if domain = os.Getenv("ASTRO_DOMAIN"); domain == "" {
			domain = defaultDomain
		}
		if !context.Exists(domain) {
			err := context.SetContext(domain)
			if err != nil {
				return false, err
			}
		}

		// Switch context
		err = context.Switch(domain)
		if err != nil {
			return false, err
		}

		c, err = context.GetContext(domain) // get current context
		if err != nil {
			return false, err
		}
	}

	err = c.SetContextKey("token", "Bearer "+astroAPIToken)
	if err != nil {
		return false, err
	}

	err = c.SetExpiresIn(time.Now().AddDate(1, 0, 0).Unix())
	if err != nil {
		return false, err
	}
	// Parse the token to peek at the custom claims
	claims, err := parseAPIToken(astroAPIToken)
	if err != nil {
		return false, err
	}
	if len(claims.Permissions) == 0 {
		return false, errNotAPIToken
	}
	if claims.ExpiresAt != nil && claims.ExpiresAt.Before(time.Now()) {
		fmt.Printf("The given API Token %s has expired \n", claims.APITokenID)
		return false, errExpiredAPIToken
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

	orgs, err := organization.ListOrganizations(platformCoreClient)
	if err != nil {
		return false, err
	}

	org := orgs[0]
	orgProduct := fmt.Sprintf("%s", *org.Product) //nolint

	if wsID == "" {
		wsID = c.Workspace
	}

	err = c.SetContextKey("workspace", wsID)
	if err != nil {
		fmt.Println("no workspace set")
	}
	err = c.SetOrganizationContext(orgID, orgProduct)
	if err != nil {
		fmt.Println("no organization context set")
	}
	return true, nil
}

func workspaceOrDeploymentIDFlagSet(cmd *cobra.Command) bool {
	wsID, _ := cmd.Flags().GetString("workspace-id")
	depID, _ := cmd.Flags().GetString("deployment-id")
	return wsID != "" || depID != ""
}
