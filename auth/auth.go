package auth

import (
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/cluster"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/docker"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/logger"
	"github.com/astronomer/astro-cli/messages"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/workspace"
	"github.com/pkg/errors"
)

var newLogger = logger.NewLogger()

// basicAuth handles authentication with the houston api
func basicAuth(username, password string) (string, error) {
	if password == "" {
		password, _ = input.InputPassword(messages.INPUT_PASSWORD)
	}

	req := houston.Request{
		Query:     houston.TokenBasicCreateRequest,
		Variables: map[string]interface{}{"identity": username, "password": password},
	}

	resp, err := req.Do()
	if err != nil {
		return "", err
	}

	return resp.Data.CreateToken.Token.Value, nil
}

func switchToLastUsedWorkspace(c config.Context, workspaces []houston.Workspace) bool {
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
func Login(domain string, oAuthOnly bool, username, password string, client *houston.Client, out io.Writer) error {
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

	acReq := houston.Request{
		Query: houston.AuthConfigGetRequest,
	}

	acResp, err := acReq.Do()
	if err != nil {
		return err
	}
	authConfig := acResp.Data.GetAuthConfig

	if username == "" && !oAuthOnly && authConfig.LocalEnabled {
		username = input.InputText(messages.INPUT_USERNAME)
	}

	if len(username) == 0 {
		if len(authConfig.AuthProviders) > 0 {
			token = oAuth(c.GetAppURL() + "/token")
		} else {
			return errors.New("cannot authenticate, oauth is disabled")
		}
	} else {
		if authConfig.LocalEnabled {
			token, err = basicAuth(username, password)
			if err != nil {
				return errors.Wrap(err, "local auth login failed")
			}
		} else {
			fmt.Println(messages.HOUSTON_BASIC_AUTH_DISABLED)
		}
	}

	c.SetContextKey("token", token)

	wsReq := houston.Request{
		Query: houston.WorkspacesGetRequest,
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
			err := workspace.Switch("", client, out)
			if err != nil {
				fmt.Printf(messages.CLI_SET_WORKSPACE_EXAMPLE)
			}
		}
	}

	err = registryAuth()
	if err != nil {
		newLogger.Debugf("There was an error logging into registry: %s", err.Error())

		fmt.Printf(messages.RegistryAuthFail)
	}

	return nil
}

// Logout logs a user out of the docker registry. Will need to logout of Houston next.
func Logout(domain string) {
	c, _ := cluster.GetCluster(domain)

	c.SetContextKey("token", "")
}
