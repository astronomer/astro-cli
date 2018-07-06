package user

import (
	"errors"
	"fmt"

	"github.com/astronomerio/astro-cli/houston"
	"github.com/astronomerio/astro-cli/messages"
	"github.com/astronomerio/astro-cli/pkg/httputil"
	"github.com/astronomerio/astro-cli/pkg/input"
)

var (
	http = httputil.NewHTTPClient()
	api  = houston.NewHoustonClient(http)
)

// CreateUser verifies input before sending a CreateUser API call to houston
func CreateUser(email string) error {
	if len(email) == 0 {
		email = input.InputText("Email: ")
	}

	password, _ := input.InputPassword("Password: ")

	passwordVerify, _ := input.InputPassword("Re-enter Password: ")
	if password != passwordVerify {
		return errors.New("Passwords do not match")
	}

	r, err := api.CreateUser(email, password)
	if err != nil {
		return err
	}

	fmt.Printf(messages.HOUSTON_USER_CREATE_SUCCESS, r.User.Uuid, email)

	return nil
}
