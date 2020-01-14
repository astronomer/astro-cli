package user

import (
	"errors"
	"fmt"
	"io"

	"github.com/sjmiller609/astro-cli/houston"
	"github.com/sjmiller609/astro-cli/pkg/input"
)

// Create verifies input before sending a CreateUser API call to houston
func Create(email, password string, client *houston.Client, out io.Writer) error {
	if len(email) == 0 {
		email = input.InputText("Email: ")
	}
	if password == "" {
		inputPassword, _ := input.InputPassword("Password: ")
		inputPassword2, _ := input.InputPassword("Re-enter Password: ")
		if inputPassword != inputPassword2 {
			return errors.New("Passwords do not match")
		}
		password = inputPassword
	}

	req := houston.Request{
		Query:     houston.UserCreateRequest,
		Variables: map[string]interface{}{"email": email, "password": password},
	}

	resp, err := req.DoWithClient(client)
	if err != nil {
		return errors.New("User creation is disabled")
	}

	authUser := resp.Data.CreateUser

	msg := "Successfully created user %s. %s"

	loginMsg := "You may now login to the platform."
	if authUser.User.Status == "pending" {
		loginMsg = "Check your email for a verification."
	}

	_, err = fmt.Fprintln(out, fmt.Sprintf(msg, email, loginMsg))

	return err
}
