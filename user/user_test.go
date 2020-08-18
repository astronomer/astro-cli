package user

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidEmail(t *testing.T) {
	email := "steve@apple.com"
	actual := IsValidEmail(email)
	expected := true

	assert.Equal(t, actual, expected)
}
