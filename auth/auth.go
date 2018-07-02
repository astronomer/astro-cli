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
	HTTP = httputil.NewHTTPClient()
	API  = houston.NewHoustonClient(HTTP)
)

// basicAuth handles authentication with the houston api
func basicAuth(username string) string {
	password, _ := input.InputPassword(messages.INPUT_PASSWORD)

	token, err := API.CreateBasicToken(username, password)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	return token.Token.Value
}

// oAuth handles oAuth with houston api
func oAuth(oAuthUrl string) string {
	fmt.Sprintf(messages.HOUSTON_OAUTH_REDIRECT, oAuthUrl)
	authSecret := input.InputText(messages.INPUT_OAUTH_TOKEN)

	token, err := API.CreateOAuthToken(authSecret)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	return token.Token.Value
}

// registryAuth authenticates with the private registry
func registryAuth(username, password string) error {
	registry := config.CFG.RegistryAuthority.GetString()

	dockerErr := docker.ExecLogin(registry, username, password)
	if dockerErr != nil {
		// Println instead of panic to prevent excessive error logging to stdout on a failed login
		fmt.Println(dockerErr)
		os.Exit(1)
	}

	fmt.Printf(messages.REGISTRY_AUTH_SUCCESS, registry)

	return nil
}

// Login logs a user into the docker registry. Will need to login to Houston next.
func Login() {
	token := ""
	authConfig, err := API.GetAuthConfig()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	username := input.InputText(messages.INPUT_USERNAME)
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

	config.CFG.CloudAPIToken.SetProjectString(token)

	// pass successful credentials to config
	// TODO
	// config.CFG.RegistryAuth.SetProjectString(config.EncodeAuth(username, password))
}

// Logout logs a user out of the docker registry. Will need to logout of Houston next.
func Logout() {
	docker.Exec("logout", config.CFG.RegistryAuthority.GetString())
}
