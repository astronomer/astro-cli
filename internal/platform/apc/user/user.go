package user

import (
	"errors"
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/pkg/input"
)

var (
	errPasswordMismatch     = errors.New("passwords do not match")
	errUserCreationDisabled = errors.New("user creation is disabled")
)

// Create verifies input before sending a CreateUser API call to houston
func Create(email, password string, client houston.ClientInterface, out io.Writer) error {
	if email == "" {
		var err error
		email, err = input.Text("Email: ", input.AnsweredBy("--email"))
		if err != nil {
			return err
		}
	}
	if password == "" {
		inputPassword, err := input.Password("Password: ", input.AnsweredBy("--password"))
		// Only a refusal stops here; a read that fails falls through to the
		// empty answer, as it always has.
		if input.IsRequired(err) {
			return err
		}
		inputPassword2, err := input.Password("Re-enter Password: ", input.AnsweredBy("--password"))
		// Only a refusal stops here; a read that fails falls through to the
		// empty answer, as it always has.
		if input.IsRequired(err) {
			return err
		}
		if inputPassword != inputPassword2 {
			return errPasswordMismatch
		}
		password = inputPassword
	}

	authUser, err := houston.Call(client.CreateUser)(houston.CreateUserRequest{Email: email, Password: password})
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
