package auth

import (
	"fmt"
	"os"

	"github.com/astronomerio/astro-cli/config"
	"github.com/astronomerio/astro-cli/docker"
	"github.com/astronomerio/astro-cli/houston"
	"github.com/astronomerio/astro-cli/messages"
	"github.com/astronomerio/astro-cli/pkg/httputil"
	"github.com/astronomerio/astro-cli/pkg/input"
)

var (
	http = httputil.NewHTTPClient()
	api  = houston.NewHoustonClient(http)
)

// basicAuth handles authentication with the houston api
func basicAuth(username string) string {
	password, _ := input.InputPassword(messages.INPUT_PASSWORD)

	token, err := api.CreateBasicToken(username, password)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	return token.Token.Value
}

func getWorkspaceByLabel(label string) *houston.Workspace {
	workspaces, err := api.GetWorkspaceAll()
	if err != nil {
		return nil
	}
	for _, ws := range workspaces {
		if ws.Label == label {
			return &ws
		}
	}

	return nil
}

// oAuth handles oAuth with houston api
func oAuth(oAuthUrl string) string {
	fmt.Println("\n" + messages.HOUSTON_OAUTH_REDIRECT)
	fmt.Println(oAuthUrl + "\n")
	authSecret := input.InputText(messages.INPUT_OAUTH_TOKEN)

	token, err := api.CreateOAuthToken(authSecret)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	return token.Token.Value
}

// registryAuth authenticates with the private registry
func registryAuth() error {
	c, err := config.GetCurrentCluster()
	if err != nil {
		return err
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
func Login(domain string, oAuthOnly bool) error {
	var token string
	c, err := config.GetCluster(domain)
	if err != nil {
		return err
	}

	c.SwitchCluster()

	authConfig, err := api.GetAuthConfig()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	username := ""
	if !oAuthOnly {
		username = input.InputText(messages.INPUT_USERNAME)
	}

	if len(username) == 0 {
		if authConfig.GoogleEnabled {
			token = oAuth(authConfig.OauthUrl)
		} else {
			fmt.Println(messages.HOUSTON_OAUTH_DISABLED)
			os.Exit(1)
		}
	} else {
		if authConfig.LocalEnabled {
			token = basicAuth(username)
		} else {
			fmt.Println(messages.HOUSTON_BASIC_AUTH_DISABLED)
		}
	}

	c.SetClusterKey("token", token)

	// Attempt to set projectworkspace if there is only one workspace
	workspaces, err := api.GetWorkspaceAll()
	if err != nil {
		return nil
	}

	if len(workspaces) == 1 {
		w := workspaces[0]
		c.SetClusterKey("workspace", w.Uuid)
		fmt.Printf(messages.CONFIG_SET_DEFAULT_WORKSPACE, w.Label, w.Uuid)
	} else {
		fmt.Printf(messages.CLI_SET_WORKSPACE_EXAMPLE)
	}

	err = registryAuth()
	if err != nil {
		fmt.Printf(messages.REGISTRY_AUTH_FAIL)
	}

	return nil
}

// Logout logs a user out of the docker registry. Will need to logout of Houston next.
func Logout(domain string) {
	c, _ := config.GetCluster(domain)

	c.SetClusterKey("token", "")
}
