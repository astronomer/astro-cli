package user

import (
	"errors"
	"fmt"

	"github.com/astronomerio/astro-cli/houston"
	"github.com/astronomerio/astro-cli/pkg/httputil"
	"github.com/astronomerio/astro-cli/pkg/input"
)

var (
	http = httputil.NewHTTPClient()
	api  = houston.NewHoustonClient(http)
)

// Create verifies input before sending a CreateUser API call to houston
func Create(email string) error {
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

	msg := "Successfully created user %s. %s"

	loginMsg := "You may now login to the platform."
	if r.User.Status == "pending" {
		loginMsg = "Check your email for a verification."
	}

	msg = fmt.Sprintf(msg, email, loginMsg)
	fmt.Println(msg)

	return nil
}
