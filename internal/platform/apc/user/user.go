package user

import (
	"errors"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/pkg/input"
)

var (
	errPasswordMismatch = errors.New("passwords do not match")
	errNoUserCreated    = errors.New("the platform answered without the user it created")
)

// Created is the user Create made. Status is Houston's: "pending" until the
// user verifies their email, "active" once they can log in.
type Created struct {
	ID       string
	Username string
	Status   string
	// Email is the address the user was created with, as given.
	Email string
}

// Create verifies input before sending a CreateUser API call to houston.
// Its prompts go to stderr, as every pkg/input prompt does.
func Create(email, password string, client houston.ClientInterface) (Created, error) {
	if email == "" {
		var err error
		email, err = input.Text("Email: ", input.AnsweredBy("--email"))
		if err != nil {
			return Created{}, err
		}
	}
	if password == "" {
		inputPassword, err := input.Password("Password: ", input.AnsweredBy("--password"))
		// Only a refusal stops here; a read that fails falls through to the
		// empty answer, as it always has.
		if input.IsRequired(err) {
			return Created{}, err
		}
		inputPassword2, err := input.Password("Re-enter Password: ", input.AnsweredBy("--password"))
		// Only a refusal stops here; a read that fails falls through to the
		// empty answer, as it always has.
		if input.IsRequired(err) {
			return Created{}, err
		}
		if inputPassword != inputPassword2 {
			return Created{}, errPasswordMismatch
		}
		password = inputPassword
	}

	authUser, err := houston.Call(client.CreateUser)(houston.CreateUserRequest{Email: email, Password: password})
	if err != nil {
		// Houston says why: public sign-ups being off (its message says
		// so), an email already in use, a password it refuses.
		return Created{}, err
	}
	if authUser == nil {
		return Created{}, errNoUserCreated
	}
	// The token Houston returns with the user is a session for the user just
	// created. It goes no further than here.
	return Created{ID: authUser.User.ID, Username: authUser.User.Username, Status: authUser.User.Status, Email: email}, nil
}
