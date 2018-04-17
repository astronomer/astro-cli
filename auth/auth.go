package auth

import (
	"fmt"

	"github.com/astronomerio/astro-cli/config"
	"github.com/astronomerio/astro-cli/docker"
	"github.com/astronomerio/astro-cli/utils"
)

// Login logs a user into the docker registry. Will need to login to Houston next.
func Login() {
	registry := config.CFG.RegistryAuthority.GetString()
	username := utils.InputText("Username: ")
	password, _ := utils.InputPassword("Password: ")

	err := docker.ExecLogin(registry, username, password)
	if err != nil {
		// Println instead of panic to prevent excessive error logging to
		// stdout on a failed login
		fmt.Println(err)
		return
	}

	// pass successful credentials to config
	config.CFG.RegistryUser.SetProjectString(username)
	config.CFG.RegistryPassword.SetProjectString(password)
}

// Logout logs a user out of the docker registry. Will need to logout of Houston next.
func Logout() {
	docker.Exec("logout", config.CFG.RegistryAuthority.GetString())
}
