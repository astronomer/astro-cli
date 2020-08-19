package user

import (
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/input"
)

// Create verifies input before sending a CreateUser API call to houston
func Create(email string, password string, client *houston.Client, out io.Writer) error {
	if len(email) == 0 {
		email = input.InputText("Email: ")
	}

	if !IsValidEmail(email) {
		return errors.New(email + " is an invalid email address")
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

// IsValidEmail checks if the email provided is valid
func IsValidEmail(email string) bool {
	exp := "^[a-zA-Z0-9.!#$%&'*+\\/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$"
	emailRegex := regexp.MustCompile(exp)
	minLen := 3
	maxLen := 254

	if len(email) < minLen && len(email) > maxLen {
		return false
	}
	if !emailRegex.MatchString(email) {
		return false
	}
	parts := strings.Split(email, "@")
	mx, err := net.LookupMX(parts[1])
	if err != nil || len(mx) == 0 {
		return false
	}
	return true
}
