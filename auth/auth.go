package auth

import (
	"github.com/astronomerio/astro-cli/docker"
)

// Login logs a user into the docker registry. Will need to login to Houston next.
func Login() {
	docker.Exec("login", docker.CloudRegistry)
}

// Logout logs a user out of the docker registry. Will need to logout of Houston next.
func Logout() {
	docker.Exec("logout", docker.CloudRegistry)
}
