package auth

import (
	"fmt"

	"github.com/astronomerio/astro-cli/cluster"
	"github.com/astronomerio/astro-cli/docker"
	"github.com/astronomerio/astro-cli/houston"
	"github.com/astronomerio/astro-cli/messages"
	"github.com/astronomerio/astro-cli/pkg/httputil"
	"github.com/astronomerio/astro-cli/pkg/input"
	"github.com/pkg/errors"
)

var (
	http = httputil.NewHTTPClient()
	api  = houston.NewHoustonClient(http)
)

// basicAuth handles authentication with the houston api
func basicAuth(username string) (string, error) {
	password, _ := input.InputPassword(messages.INPUT_PASSWORD)

	token, err := api.CreateBasicToken(username, password)
	if err != nil {
		return "", errors.Wrap(err, "failed to fetch local/db auth token")
	}

	return token.Token.Value, nil
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
	return input.InputText(messages.INPUT_OAUTH_TOKEN)
}

// registryAuth authenticates with the private registry
func registryAuth() error {
	c, err := cluster.GetCurrentCluster()
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

	authConfig, err := api.GetAuthConfig()
	if err != nil {
		return errors.Wrap(err, "failed to fetch auth config")
	}

	username := ""
	if !oAuthOnly && authConfig.LocalEnabled {
		username = input.InputText(messages.INPUT_USERNAME)
	}

	if len(username) == 0 {
		if authConfig.GoogleEnabled || authConfig.Auth0Enabled || authConfig.GithubEnabled {
			token = oAuth(c.GetAppURL() + "/login?source=cli")
		} else {
			return errors.New("cannot authenticate, oauth is disabled")
		}
	} else {
		if authConfig.LocalEnabled {
			token, err = basicAuth(username)
			if err != nil {
				return errors.Wrap(err, "local auth login failed")
			}
		} else {
			fmt.Println(messages.HOUSTON_BASIC_AUTH_DISABLED)
		}
	}

	c.SetContextKey("token", token)

	// Attempt to set projectworkspace if there is only one workspace
	workspaces, err := api.GetWorkspaceAll()
	if err != nil {
		return nil
	}

	if len(workspaces) == 1 && len(c.Workspace) == 0 {
		w := workspaces[0]
		c.SetContextKey("workspace", w.Uuid)
		fmt.Printf(messages.CONFIG_SET_DEFAULT_WORKSPACE, w.Label, w.Uuid)
	}

	if len(workspaces) != 1 && len(c.Workspace) == 0 {
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
	c, _ := cluster.GetCluster(domain)

	c.SetContextKey("token", "")
}
