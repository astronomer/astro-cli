package config

import (
	"errors"
	"testing"

	"github.com/spf13/afero"
)

// SetSharedLogin gives every context on the tenant the whole new login in one
// save, also when the vault will not take it: the login then stays in the
// config for each of them, and no context is left with the new access token
// beside the old refresh token.
func TestSetSharedLoginReachesEveryContextWhenTheVaultFails(t *testing.T) {
	loginHome(t, plaintextConfig+`    auth_domain: https://auth.example/
    auth_client_id: client
  other_io:
    domain: other.io
    token: Bearer other-old
    refreshtoken: other-old-refresh
    auth_domain: https://auth.example/
    auth_client_id: client
`, errors.New("keyring locked"))
	c, err := GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}

	if err := c.SetSharedLogin("Bearer shared-access", "shared-refresh", 3600, "me@example.com"); err != nil {
		t.Fatal(err)
	}

	initHome(afero.NewOsFs())
	for _, domain := range []string{"astronomer.io", "other.io"} {
		got, err := (&Context{Domain: domain}).GetContext()
		if err != nil {
			t.Fatal(err)
		}
		if got.Token != "Bearer shared-access" || got.RefreshToken != "shared-refresh" || got.UserEmail != "me@example.com" {
			t.Errorf("%s: token/refresh/email match = %v/%v/%v, want the shared login", domain,
				got.Token == "Bearer shared-access", got.RefreshToken == "shared-refresh", got.UserEmail == "me@example.com")
		}
		if expires, _ := got.GetExpiresIn(); expires.IsZero() {
			t.Errorf("%s: no expiry was saved", domain)
		}
	}
	if configHolds(t, "other-old-refresh") {
		t.Error("a context still holds the old refresh token")
	}
}
