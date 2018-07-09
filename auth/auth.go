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
	registry := config.RegistryUrl()
	token := config.CFG.CloudAPIToken.GetProjectString()

	dockerErr := docker.ExecLogin(registry, "user", token)
	if dockerErr != nil {
		fmt.Println(dockerErr)
		os.Exit(1)
	}

	fmt.Printf(messages.REGISTRY_AUTH_SUCCESS, registry)

	return nil
}

// Login handles authentication to houston and registry
func Login(oAuthOnly bool) error {
	token := ""
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

	config.CFG.CloudAPIToken.SetProjectString(token)
	registryAuth()

	return nil
}

// Logout logs a user out of the docker registry. Will need to logout of Houston next.
func Logout() {
	config.CFG.CloudAPIToken.SetProjectString("")
}
