package auth

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"log"
	"encoding/json"

	"github.com/astronomer/astro-cli/cluster"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/docker"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/astrohub"
	"github.com/astronomer/astro-cli/messages"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/workspace"
	"github.com/pkg/errors"
)

type postTokenResponse struct {
	AccessToken  string    `json:"access_token"`
	IdToken 		 string    `json:"id_token"`
	ExpiresIn    int       `json:"expires_in"`
	TokenType    string    `json:"token_type"`
	Scope        string    `json:"scope"`
}

// basicAuth handles authentication with the houston api
func basicAuth(username, password string) (string, error) {
	if password == "" {
		password, _ = input.InputPassword(messages.INPUT_PASSWORD)
	}

	req := astrohub.Request{
		Query:     houston.TokenBasicCreateRequest,
		Variables: map[string]interface{}{"identity": username, "password": password},
	}

	resp, err := req.Do()
	if err != nil {
		return "", err
	}

	return resp.Data.CreateToken.Token.Value, nil
}

// handles authentication with auth0
func authLogin(authConfig *astrohub.AuthConfig, username, password string) (string, error) {
	if password == "" {
		password, _ = input.InputPassword(messages.INPUT_PASSWORD)
	}

	addr := authConfig.DomainUrl + "oauth/token";
	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("username", username)
	form.Set("password", password)
	form.Set("audience", authConfig.Audience)
	form.Set("scope", "openid profile email")
	form.Set("client_id", authConfig.ClientId)

	client := &http.Client{}
	r, err := http.NewRequest("POST", addr, strings.NewReader(form.Encode())) // URL-encoded payload
	if err != nil {
		log.Fatal(err)
	}
	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	res, err := client.Do(r)

	if err != nil {
		log.Fatal(err)
	}
	defer res.Body.Close()

	var decode postTokenResponse
	err = json.NewDecoder(res.Body).Decode(&decode);
	if err != nil {
		return "", fmt.Errorf("unable to decode token response: %s", err)
	}

	return decode.AccessToken, nil
}

func switchToLastUsedWorkspace(c config.Context, workspaces []astrohub.Workspace) bool {
	if c.LastUsedWorkspace != "" {
		for _, w := range workspaces {
			if c.LastUsedWorkspace == w.Id {
				c.SetContextKey("workspace", w.Id)
				return true
			}
		}
	}
	return false
}

// oAuth handles oAuth with houston api
func oAuth(oAuthUrl string) string {
	fmt.Println("\n" + messages.HOUSTON_OAUTH_REDIRECT)
	fmt.Println(oAuthUrl + "\n")
	return input.InputText(messages.INPUT_OAUTH_TOKEN)
}

// registryAuth authenticates with the private registry
func registryAuth() error {
	c, err := cluster.GetCurrentCluster()
	if err != nil {
		return err
	}

	if c.Domain == "localhost" || c.Domain == "houston" {
		return nil
	}

	registry := "registry." + c.Domain
	token := c.Token
	err = docker.ExecLogin(registry, "user", token)
	if err != nil {
		return err
	}

	fmt.Printf(messages.REGISTRY_AUTH_SUCCESS, registry)

	return nil
}

// Login handles authentication to houston and registry
func Login(domain string, oAuthOnly bool, username, password string, client *houston.Client, astrohubClient *astrohub.Client, out io.Writer) error {
	var token string
	var err error

	// If no domain specified
	// Create cluster if it does not exist
	if len(domain) != 0 {
		if !cluster.Exists(domain) {
			// Save new cluster since it did not exists
			err = cluster.SetCluster(domain)
			if err != nil {
				return err
			}
		}

		// Switch cluster now that we ensured cluster exists
		err = cluster.Switch(domain)
		if err != nil {
			return err
		}
	}

	c, err := cluster.GetCurrentCluster()
	if err != nil {
		return err
	}

	acReq := astrohub.Request{
		Query: astrohub.AuthConfigGetRequest,
	}

	acResp, err := acReq.DoWithClient(astrohubClient)
	if err != nil {
		return err
	}

	authConfig := acResp.Data.GetAuthConfig

	if username == "" && !oAuthOnly {
		username = input.InputText(messages.INPUT_USERNAME)
	}

	if len(username) == 0 {
		token = oAuth(c.GetAppURL() + "/token")
	} else {
		token, err = authLogin(authConfig, username, password)
		if token == "" {
			return errors.New("Username/Password is incorrect")
		}
		if err != nil {
			return errors.Wrap(err, "local auth login failed")
		}
	}

	c.SetContextKey("token", token)

	wsReq := astrohub.Request{
		Query: astrohub.WorkspacesGetRequest,
	}

	wsResp, err := wsReq.Do()
	if err != nil {
		return err
	}

	workspaces := wsResp.Data.GetWorkspaces

	if len(workspaces) == 1 {
		w := workspaces[0]
		c.SetContextKey("workspace", w.Id)
		// update last used workspace ID
		c.SetContextKey("last_used_workspace", w.Id)
		fmt.Printf(messages.CONFIG_SET_DEFAULT_WORKSPACE, w.Label, w.Id)
	}

	if len(workspaces) > 1 {
		// try to switch to last used workspace in cluster
		isSwitched := switchToLastUsedWorkspace(c, workspaces)

		if !isSwitched {
			// show switch menu with available workspace IDs
			fmt.Println("\n" + messages.CLI_CHOOSE_WORKSPACE)
			err := workspace.Switch("", client, astrohubClient, out)
			if err != nil {
				fmt.Printf(messages.CLI_SET_WORKSPACE_EXAMPLE)
			}
		}
	}

	err = registryAuth()
	if err != nil {
		fmt.Printf(messages.RegistryAuthFail)
	}

	return nil
}

// Logout logs a user out of the docker registry. Will need to logout of Houston next.
func Logout(domain string) {
	c, _ := cluster.GetCluster(domain)

	c.SetContextKey("token", "")
}
