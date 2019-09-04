package cmd

import (
	"testing"
)

func TestValidateInvalidRole(t *testing.T) {
	err := validateRole("role")
	if err != nil && err.Error() != "please use one of: admin, editor, viewer" {
		t.Errorf("%s", err)
	}
}

func TestValidateValidRole(t *testing.T) {
	err := validateRole("admin")
	if err != nil {
		t.Errorf("%s", err)
	}
}