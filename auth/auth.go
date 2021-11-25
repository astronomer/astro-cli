package auth

import (
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/cluster"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/messages"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/workspace"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// basicAuth handles authentication with the houston api
func basicAuth(username, password string) (string, error) {
	if password == "" {
		password, _ = input.Password(messages.InputPassword)
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
		for i := range workspaces {
			w := workspaces[i]
			if c.LastUsedWorkspace == w.ID {
				if err := c.SetContextKey("workspace", w.ID); err != nil {
					return false
				}
				return true
			}
		}
	}
	return false
}

// oAuth handles oAuth with houston api
func oAuth(oAuthURL string) string {
	fmt.Println("\n" + messages.HoustonOAuthRedirect)
	fmt.Println(oAuthURL + "\n")
	return input.Text(messages.InputOAuthToken)
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
	registryHandler, err := airflow.RegistryHandlerInit(registry)
	if err != nil {
		return err
	}
	err = registryHandler.Login("user", token)
	if err != nil {
		return err
	}

	fmt.Printf(messages.RegistryAuthSuccess, registry)

	return nil
}

// Login handles authentication to houston and registry
func Login(domain string, oAuthOnly bool, username, password string, client *houston.Client, out io.Writer) error {
	var token string
	var err error

	// create cluster if no domain specified, else switch cluster
	err = checkClusterDomain(domain)
	if err != nil {
		return err
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
		username = input.Text(messages.InputUsername)
	}

	token, err = getAuthToken(username, password, authConfig, c)
	if err != nil {
		return err
	}

	err = c.SetContextKey("token", token)
	if err != nil {
		return err
	}

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
		err = c.SetContextKey("workspace", w.ID)
		if err != nil {
			return err
		}
		// update last used workspace ID
		err = c.SetContextKey("last_used_workspace", w.ID)
		if err != nil {
			return err
		}
		fmt.Printf(messages.ConfigSetDefaultWorkspace, w.Label, w.ID)
	}

	if len(workspaces) > 1 {
		// try to switch to last used workspace in cluster
		isSwitched := switchToLastUsedWorkspace(c, workspaces)

		if !isSwitched {
			// show switch menu with available workspace IDs
			fmt.Println("\n" + messages.CLIChooseWorkspace)
			err := workspace.Switch("", client, out)
			if err != nil {
				fmt.Printf(messages.CLISetWorkspaceExample)
			}
		}
	}

	err = registryAuth()
	if err != nil {
		log.Debugf("There was an error logging into registry: %s", err.Error())

		fmt.Printf(messages.RegistryAuthFail)
	}

	return nil
}

// Logout logs a user out of the docker registry. Will need to logout of Houston next.
func Logout(domain string) {
	c, _ := cluster.GetCluster(domain)

	_ = c.SetContextKey("token", "")
}

func checkClusterDomain(domain string) error {
	// If no domain specified
	// Create cluster if it does not exist
	if domain != "" {
		if !cluster.Exists(domain) {
			// Save new cluster since it did not exists
			err := cluster.SetCluster(domain)
			if err != nil {
				return err
			}
		}

		// Switch cluster now that we ensured cluster exists
		err := cluster.Switch(domain)
		if err != nil {
			return err
		}
	}
	return nil
}

func getAuthToken(username, password string, authConfig *houston.AuthConfig, context config.Context) (string, error) {
	var token string
	var err error
	if username == "" {
		if len(authConfig.AuthProviders) > 0 {
			token = oAuth(context.GetAppURL() + "/token")
		} else {
			return "", errors.New("cannot authenticate, oauth is disabled")
		}
	} else {
		if authConfig.LocalEnabled {
			token, err = basicAuth(username, password)
			if err != nil {
				return "", errors.Wrap(err, "local auth login failed")
			}
		} else {
			fmt.Println(messages.HoustonBasicAuthDisabled)
		}
	}
	return token, nil
}
