package user

import (
	"errors"

	"github.com/astronomerio/astro-cli/config"
	"github.com/astronomerio/astro-cli/houston"
	"github.com/astronomerio/astro-cli/pkg/httputil"
	"github.com/astronomerio/astro-cli/pkg/input"
)

var (
	HTTP = httputil.NewHTTPClient()
)

// CreateUser verifies input before sending a CreateUser API call to houston
func CreateUser(skipVerify bool, emailIn string) error {
	API := houston.NewHoustonClient(HTTP)
	email := emailIn

	if len(emailIn) == 0 {
		email = input.InputText("Email: ")
	}

	password, _ := input.InputPassword("Password: ")

	if !skipVerify {
		passwordVerify, _ := input.InputPassword("Re-enter Password: ")
		if password != passwordVerify {
			return errors.New("Passwords do not match, try again")
		}
	}

	status, houstonErr := API.CreateUser(email, password)
	if houstonErr != nil {
		return houstonErr
	}

	config.CFG.CloudAPIToken.SetProjectString(status.Token)
	return nil
}
