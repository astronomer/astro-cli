package auth

import (
	"errors"
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/software/workspace"

	log "github.com/sirupsen/logrus"
)

const (
	houstonOAuthRedirect     = "Please visit the following URL, authenticate and paste token in next prompt\n"
	inputOAuthToken          = "oAuth Token: " // nolint:gosec // false positive
	inputUsername            = "Username (leave blank for oAuth): "
	inputPassword            = "Password: "
	cliChooseWorkspace       = "Please choose a workspace:"
	cliSetWorkspaceExample   = "\nNo default workspace detected, you can list workspaces with \n\tastro workspace list\nand set your default workspace with \n\tastro workspace switch [WORKSPACEID]\n\n"
	houstonBasicAuthDisabled = "Basic authentication is disabled, conact administrator or defer back to oAuth"
	registryAuthSuccess      = "\nSuccessfully authenticated to %s\n"

	configSetDefaultWorkspace = "\n\"%s\" Workspace found. This is your default Workspace.\n"

	registryAuthFailMsg = "\nFailed to authenticate to the registry. Do you have Docker running?\nYou will not be able to push new images to your Airflow Deployment unless Docker is running.\nIf Docker is running and you are seeing this message, the registry is down or cannot be reached.\n"

	localhostDomain      = "localhost"
	houstonDomain        = "houston"
	localSoftwareDomain  = "localhost.me"
	registryDomainPrefix = "registry."
)

var (
	errOAuthDisabled = errors.New("cannot authenticate, oauth is disabled")

	// this is used to monkey patch the function in order to write unit test cases
	registryHandlerInit = airflow.RegistryHandlerInit
)

// basicAuth handles authentication with the houston api
func basicAuth(username, password string, ctx *config.Context, client houston.ClientInterface) (string, error) {
	if password == "" {
		password, _ = input.Password(inputPassword)
	}

	return client.AuthenticateWithBasicAuth(username, password, ctx)
}

func switchToLastUsedWorkspace(c *config.Context, workspaces []houston.Workspace) bool {
	if c.LastUsedWorkspace == "" {
		return false
	}

	for idx := range workspaces {
		if c.LastUsedWorkspace == workspaces[idx].ID {
			if err := c.SetContextKey("workspace", workspaces[idx].ID); err != nil {
				return false
			}
			return true
		}
	}
	return false
}

// oAuth handles oAuth with houston api
func oAuth(oAuthURL string) string {
	fmt.Printf("\n" + houstonOAuthRedirect + "\n")
	fmt.Println(oAuthURL + "\n")
	return input.Text(inputOAuthToken)
}

// registryAuth authenticates with the private registry
func registryAuth() error {
	c, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	if c.Domain == localhostDomain || c.Domain == houstonDomain || c.Domain == localSoftwareDomain {
		return nil
	}

	registry := registryDomainPrefix + c.Domain
	token := c.Token
	registryHandler, err := registryHandlerInit(registry)
	if err != nil {
		return err
	}
	err = registryHandler.Login("user", token)
	if err != nil {
		return err
	}

	fmt.Printf(registryAuthSuccess, registry)

	return nil
}

// Login handles authentication to houston and registry
func Login(domain string, oAuthOnly bool, username, password string, client houston.ClientInterface, out io.Writer) error {
	var token string
	var err error

	ctx := &config.Context{Domain: domain}
	err = ctx.PrintSoftwareContext(out)
	if err != nil {
		return err
	}

	authConfig, err := client.GetAuthConfig(ctx)
	if err != nil {
		return err
	}

	if username == "" && !oAuthOnly && authConfig.LocalEnabled {
		username = input.Text(inputUsername)
	}

	token, err = getAuthToken(username, password, authConfig, ctx, client)
	if err != nil {
		return err
	}

	// create cluster if no domain specified, else switch cluster
	err = checkClusterDomain(domain)
	if err != nil {
		return err
	}

	c, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	err = c.SetContextKey("token", token)
	if err != nil {
		return err
	}

	workspaces, err := client.ListWorkspaces()
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
		// try to switch to last used workspace in cluster
		isSwitched := switchToLastUsedWorkspace(&c, workspaces)

		if !isSwitched {
			// show switch menu with available workspace IDs
			fmt.Println("\n" + cliChooseWorkspace)
			err := workspace.Switch("", client, out)
			if err != nil {
				fmt.Fprint(out, cliSetWorkspaceExample)
			}
		}
	}

	err = registryAuth()
	if err != nil {
		log.Debugf("There was an error logging into registry: %s", err.Error())

		fmt.Fprint(out, registryAuthFailMsg)
	}

	return nil
}

// Logout removes the locally stored token and reset current context
func Logout(domain string) {
	c, err := context.GetContext(domain)
	if err != nil {
		return
	}

	err = c.SetContextKey("token", "")
	if err != nil {
		return
	}

	// remove the current context
	err = config.ResetCurrentContext()
	if err != nil {
		fmt.Println("Failed to reset current context: ", err.Error())
		return
	}
}

func checkClusterDomain(domain string) error {
	// If no domain specified
	// Create cluster if it does not exist
	if domain != "" {
		if !context.Exists(domain) {
			// Save new context since it did not exists
			err := context.SetContext(domain)
			if err != nil {
				return err
			}
		}

		// Switch context now that we ensured context exists
		err := context.Switch(domain)
		if err != nil {
			return err
		}
	}
	return nil
}

func getAuthToken(username, password string, authConfig *houston.AuthConfig, ctx *config.Context, client houston.ClientInterface) (string, error) {
	var token string
	var err error
	if username == "" {
		if len(authConfig.AuthProviders) > 0 {
			token = oAuth(ctx.GetSoftwareAppURL() + "/token")
		} else {
			return "", errOAuthDisabled
		}
	} else {
		if authConfig.LocalEnabled {
			token, err = basicAuth(username, password, ctx, client)
			if err != nil {
				return "", fmt.Errorf("local auth login failed: %w", err)
			}
		} else {
			fmt.Println(houstonBasicAuthDisabled)
		}
	}
	return token, nil
}
