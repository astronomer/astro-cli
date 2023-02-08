package cloud

import (
	http_context "context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	astro "github.com/astronomer/astro-cli/astro-client"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/cloud/auth"
	"github.com/astronomer/astro-cli/cloud/organization"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/httputil"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	authLogin = auth.Login

	client = httputil.NewHTTPClient()
)

const (
	accessTokenExpThreshold = 5 * time.Minute
	topLvlCmd               = "astro"
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

func Setup(cmd *cobra.Command, args []string, client astro.Client, coreClient astrocore.CoreClient) error {
	// If the user is trying to login or logout no need to go through auth setup.
	if cmd.CalledAs() == "login" || cmd.CalledAs() == "logout" {
		return nil
	}

	// If the user is using dev commands no need to go through auth setup.
	if cmd.CalledAs() == "dev" && cmd.Parent().Use == topLvlCmd {
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

	// run auth setup for any command that requires auth
	apiKey, err := checkAPIKeys(client, coreClient, args)
	if err != nil {
		fmt.Println(err)
		fmt.Println("\nThere was an error using API keys, using regular auth instead")
	}
	if apiKey {
		return nil
	}
	err = checkToken(client, coreClient, os.Stdout)
	if err != nil {
		return err
	}
	err = migrateCloudConfig(coreClient)
	if err != nil {
		return err
	}
	return nil
}

func checkToken(client astro.Client, coreClient astrocore.CoreClient, out io.Writer) error {
	c, err := context.GetCurrentContext() // get current context
	if err != nil {
		return err
	}
	expireTime, _ := c.GetExpiresIn()
	// check if user is logged in
	if c.Token == "Bearer " || c.Token == "" || c.Domain == "" {
		// guide the user through the login process if not logged in
		err := authLogin(c.Domain, "", "", client, coreClient, out, false)
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
			err := authLogin(c.Domain, "", "", client, coreClient, out, false)
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
	}
	return nil
}

// isExpired is true if now() + a threshold is after the given date
func isExpired(t time.Time, threshold time.Duration) bool {
	return time.Now().Add(threshold).After(t)
}

// Refresh gets a new access token from the provided refresh token,
// The request is used the default client_id and endpoint for device authentication.
func refresh(refreshToken string, authConfig astro.AuthConfig) (TokenResponse, error) {
	addr := authConfig.DomainURL + "oauth/token"
	data := url.Values{
		"client_id":     {authConfig.ClientID},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}

	client := &http.Client{}

	r, err := http.NewRequestWithContext(http_context.Background(), http.MethodPost, addr, strings.NewReader(data.Encode())) // URL-encoded payload
	if err != nil {
		log.Fatal(err)
		return TokenResponse{}, fmt.Errorf("cannot get a new access token from the refresh token: %w", err)
	}
	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	res, err := client.Do(r)
	if err != nil {
		log.Fatal(err)
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

func checkAPIKeys(astroClient astro.Client, coreClient astrocore.CoreClient, args []string) (bool, error) {
	// check os variables
	astronomerKeyID := os.Getenv("ASTRONOMER_KEY_ID")
	astronomerKeySecret := os.Getenv("ASTRONOMER_KEY_SECRET")
	if astronomerKeyID == "" || astronomerKeySecret == "" {
		return false, nil
	}
	fmt.Println("Using an Astro API key")

	// get authConfig
	c, err := context.GetCurrentContext() // get current context
	if err != nil {
		// set context
		domain := "astronomer.io"
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
		log.Fatal(err)
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
	orgs, err := organization.ListOrganizations(coreClient)
	if err != nil {
		return false, err
	}

	org := orgs[0]
	orgID := org.Id
	orgShortName := org.ShortName

	// If using api keys for virtual runtimes, we dont need to look up for this endpoint
	if !(len(args) > 0 && strings.HasPrefix(args[0], "vr-")) {
		// get workspace ID
		deployments, err := astroClient.ListDeployments(orgID, "")
		if err != nil {
			return false, errors.Wrap(err, astro.AstronomerConnectionErrMsg)
		}
		workspaceID = deployments[0].Workspace.ID

		err = c.SetContextKey("workspace", workspaceID) // c.Workspace
		if err != nil {
			fmt.Println("no workspace set")
		}
	}
	err = c.SetOrganizationContext(orgID, orgShortName)
	if err != nil {
		fmt.Println("no organization context set")
	}
	return true, nil
}
