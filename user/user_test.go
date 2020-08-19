package user

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidEmail(t *testing.T) {
	// Test a valid email
	email := "steve@apple.com"
	actual := IsValidEmail(email)
	expected := true

	assert.Equal(t, actual, expected)

	// Test a invalid email that is too long
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789@."
	sb := make([]byte, 256)
	for i := range sb {
		sb[i] = chars[rand.Intn(len(chars))]
	}
	email = string(sb)
	actual = IsValidEmail(email)
	expected = false

	assert.Equal(t, actual, expected)

	// Test a invalid email that is too short
	email = "abc"
	actual = IsValidEmail(email)
	expected = false

	assert.Equal(t, actual, expected)

	// Test a invalid email
	email = "this@isnotanemail"
	actual = IsValidEmail(email)
	expected = false

	assert.Equal(t, actual, expected)

	// Test real address without MX records
	email = "testing@test.com"
	actual = IsValidEmail(email)
	expected = false

	assert.Equal(t, actual, expected)
}
