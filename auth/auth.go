package auth

import (
	"fmt"

	"github.com/astronomerio/astro-cli/config"
	"github.com/astronomerio/astro-cli/docker"
	"github.com/astronomerio/astro-cli/houston"
	"github.com/astronomerio/astro-cli/pkg/httputil"
	"github.com/astronomerio/astro-cli/pkg/input"
)

var (
	HTTP = httputil.NewHTTPClient()
)

// Login logs a user into the docker registry. Will need to login to Houston next.
func Login() {
	registry := config.CFG.RegistryAuthority.GetString()
	username := input.InputText("Username: ")
	password, _ := input.InputPassword("Password: ")

	API := houston.NewHoustonClient(HTTP)

	// authenticate with houston
	token, houstonErr := API.CreateToken(username, password)
	if houstonErr != nil {
		panic(houstonErr)
	} else if token.Success != true {
		fmt.Println(token.Message)
		return
	}

	config.CFG.UserAPIAuthToken.SetProjectString(token.Token)

	//authenticate with registry
	dockerErr := docker.ExecLogin(registry, username, password)
	if dockerErr != nil {
		// Println instead of panic to prevent excessive error logging to stdout on a failed login
		fmt.Println(dockerErr)
		return
	}

	config.CFG.RegistryUser.SetProjectString(username)
	config.CFG.RegistryPassword.SetProjectString(password)
}

// Logout logs a user out of the docker registry. Will need to logout of Houston next.
func Logout() {
	docker.Exec("logout", config.CFG.RegistryAuthority.GetString())
}
