package user

import (
	"errors"
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/input"
)

var (
	errPasswordMismatch     = errors.New("passwords do not match")
	errUserCreationDisabled = errors.New("user creation is disabled")
)

// Create verifies input before sending a CreateUser API call to houston
func Create(email, password string, client houston.ClientInterface, out io.Writer) error {
	if email == "" {
		email = input.Text("Email: ")
	}
	if password == "" {
		inputPassword, _ := input.Password("Password: ")
		inputPassword2, _ := input.Password("Re-enter Password: ")
		if inputPassword != inputPassword2 {
			return errPasswordMismatch
		}
		password = inputPassword
	}

	authUser, err := houston.Call(client.CreateUser, houston.CreateUserRequest{Email: email, Password: password})
	if err != nil {
		return errUserCreationDisabled
	}

	msg := "Successfully created user %s. %s"

	loginMsg := "You may now login to the platform."
	if authUser.User.Status == "pending" {
		loginMsg = "Check your email for a verification."
	}

	_, err = fmt.Fprintln(out, fmt.Sprintf(msg, email, loginMsg))

	return err
}
