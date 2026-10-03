package astrosession

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/astroauth"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func TestBearerReadsTheCurrentContext(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	token, err := Bearer(context.Background())
	if err != nil {
		t.Fatalf("bearer: %v", err)
	}
	// The token travels as the config stores it; airflowapi.BearerToken is what
	// normalizes the scheme away, so nothing here re-implements that.
	if token != "token" {
		t.Fatalf("token = %q, want the stored value", token)
	}
}

func TestBearerWithNoLoginNamesTheFix(t *testing.T) {
	testUtil.InitTestConfig(testUtil.Initial) // no context at all
	_, err := Bearer(context.Background())
	if err == nil {
		t.Fatal("a logged-out machine produced a token")
	}
	if !strings.Contains(err.Error(), "not logged in") || !strings.Contains(err.Error(), "astro login") {
		t.Fatalf("err = %v, want the logged-out message and its fix", err)
	}
}

// TestBearerTreatsASchemeWithNoTokenAsLoggedOut covers how a context that has
// been logged out of actually spells itself: the scheme survives the logout,
// the credential does not.
func TestBearerTreatsASchemeWithNoTokenAsLoggedOut(t *testing.T) {
	writeConfig(t, "context: astronomer.io\ncontexts:\n  astronomer_io:\n    domain: astronomer.io\n    token: 'Bearer '\n")
	if _, err := Bearer(context.Background()); err == nil || !strings.Contains(err.Error(), "not logged in") {
		t.Fatalf("err = %v, want the logged-out message", err)
	}
}

// writeConfig points config/ at an in-memory home holding the given YAML, the
// same way pkg/testing does for its own fixtures.
func writeConfig(t *testing.T, yaml string) {
	t.Helper()
	fs := afero.NewMemMapFs()
	if err := afero.WriteFile(fs, config.HomeConfigFile, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	config.InitConfig(fs)
}

// prodLogin stores a login for astronomer.io beside the current context, which
// InitTestConfig(LocalPlatform) points at localhost.
func prodLogin(t *testing.T, token, refreshToken string) config.Context {
	t.Helper()
	t.Setenv(EnvAPIToken, "")
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	prod := config.Context{Domain: "astronomer.io"}
	for key, value := range map[string]string{"domain": "astronomer.io", "token": token, "refreshtoken": refreshToken} {
		if err := prod.SetContextKey(key, value); err != nil {
			t.Fatal(err)
		}
	}
	return prod
}

func stubRefresh(t *testing.T, refresh func(domain, refreshToken string) (*astroauth.TokenResponse, error)) {
	t.Helper()
	restore := refreshLogin
	t.Cleanup(func() { refreshLogin = restore })
	refreshLogin = refresh
}

func assertCurrentIsLocalhost(t *testing.T) {
	t.Helper()
	current, err := config.GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if current.Domain != "localhost" {
		t.Fatalf("current context = %q, want it left on localhost", current.Domain)
	}
}

func TestBearerForReadsTheDomainsLoginNotTheCurrentContext(t *testing.T) {
	prod := prodLogin(t, "Bearer prod", "")
	if err := prod.SetExpiresIn(3600); err != nil {
		t.Fatal(err)
	}
	token, err := BearerFor(context.Background(), "astronomer.io")
	if err != nil {
		t.Fatalf("bearer: %v", err)
	}
	if token != "Bearer prod" {
		t.Fatalf("token = %q, want the astronomer.io login", token)
	}
	assertCurrentIsLocalhost(t)
}

func TestBearerForPrefersTheAPIToken(t *testing.T) {
	prodLogin(t, "Bearer prod", "")
	t.Setenv(EnvAPIToken, "ci-token")
	token, err := BearerFor(context.Background(), "astronomer.io")
	if err != nil || token != "ci-token" {
		t.Fatalf("token, err = %q, %v; want %s", token, err, EnvAPIToken)
	}
}

func TestBearerForWithNoDomainReadsTheCurrentContext(t *testing.T) {
	t.Setenv(EnvAPIToken, "")
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	token, err := BearerFor(context.Background(), "")
	if err != nil || token != "token" {
		t.Fatalf("token, err = %q, %v; want the current context's token", token, err)
	}
}

// A stale login for the project's domain is refreshed, saved under that
// domain, and used, without moving the current context.
func TestBearerForRefreshesAStaleLoginUnderItsOwnDomain(t *testing.T) {
	prod := prodLogin(t, "Bearer old", "refresh-1")
	stubRefresh(t, func(domain, refreshToken string) (*astroauth.TokenResponse, error) {
		if domain != "astronomer.io" || refreshToken != "refresh-1" {
			t.Errorf("refreshed %q with %q", domain, refreshToken)
		}
		return &astroauth.TokenResponse{AccessToken: "new", RefreshToken: "refresh-2", ExpiresIn: 3600}, nil
	})

	token, err := BearerFor(context.Background(), "astronomer.io")
	if err != nil {
		t.Fatalf("bearer: %v", err)
	}
	if token != "Bearer new" {
		t.Fatalf("token = %q, want the refreshed one", token)
	}
	saved, err := prod.GetContext()
	if err != nil {
		t.Fatal(err)
	}
	if saved.Token != "Bearer new" || saved.RefreshToken != "refresh-2" {
		t.Fatalf("saved token %q, refresh %q; want the refreshed pair", saved.Token, saved.RefreshToken)
	}
	assertCurrentIsLocalhost(t)
}

// A refresh renews a login every host on the tenant shares, so it goes to all
// of them whole: a sibling left with its old refresh token and email would call
// as one user while it renews as another. A host on another tenant keeps its own.
func TestBearerForSharesARefreshedLoginAcrossItsTenant(t *testing.T) {
	prod := prodLogin(t, "Bearer old", "refresh-1")
	if err := prod.SetContextKey("user_email", "x@astronomer.test"); err != nil {
		t.Fatal(err)
	}
	sibling := config.Context{Domain: "pr1111.astronomer-dev.io"}
	other := config.Context{Domain: "astronomer-stage.io"}
	for _, c := range []struct {
		ctx                  config.Context
		authDomain, clientID string
	}{
		{prod, "https://auth.example.com/", "client-id"},
		{sibling, "https://auth.example.com/", "client-id"},
		{other, "https://auth.astronomer-stage.io/", "stage-client"},
	} {
		if err := c.ctx.SetAuthTenant(c.authDomain, c.clientID); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []config.Context{sibling, other} {
		for key, value := range map[string]string{"domain": c.Domain, "token": "Bearer theirs", "refreshtoken": "their-refresh", "user_email": "y@astronomer.test"} {
			if err := c.SetContextKey(key, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	stubRefresh(t, func(string, string) (*astroauth.TokenResponse, error) {
		return &astroauth.TokenResponse{AccessToken: "new", ExpiresIn: 3600}, nil
	})

	if _, err := BearerFor(context.Background(), "astronomer.io"); err != nil {
		t.Fatalf("bearer: %v", err)
	}

	got, err := sibling.GetContext()
	if err != nil {
		t.Fatal(err)
	}
	if got.Token != "Bearer new" || got.RefreshToken != "refresh-1" || got.UserEmail != "x@astronomer.test" {
		t.Errorf("sibling = %q / %q / %q, want the whole refreshed login", got.Token, got.RefreshToken, got.UserEmail)
	}
	got, err = other.GetContext()
	if err != nil {
		t.Fatal(err)
	}
	if got.Token != "Bearer theirs" || got.RefreshToken != "their-refresh" {
		t.Errorf("a host on another tenant changed: %q / %q", got.Token, got.RefreshToken)
	}
	assertCurrentIsLocalhost(t)
}

func TestBearerForNamesTheDomainWhenARefreshFails(t *testing.T) {
	prodLogin(t, "Bearer old", "refresh-1")
	stubRefresh(t, func(string, string) (*astroauth.TokenResponse, error) { return nil, errors.New("invalid_grant") })
	_, err := BearerFor(context.Background(), "astronomer.io")
	want := "your astronomer.io session expired. Log in again with `astro login astronomer.io`"
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
}

func TestBearerForNamesTheDomainOfAnExpiredLoginWithNoRefreshToken(t *testing.T) {
	prod := prodLogin(t, "Bearer old", "")
	if err := prod.SetExpiresIn(-60); err != nil {
		t.Fatal(err)
	}
	_, err := BearerFor(context.Background(), "astronomer.io")
	if err == nil || !strings.HasPrefix(err.Error(), "your astronomer.io session expired") {
		t.Fatalf("err = %v, want the astronomer.io session named", err)
	}
}

// `astro logout` clears the token and keeps the refresh token, so a refresh
// here would log the user back in.
func TestBearerForDoesNotRefreshALoggedOutLogin(t *testing.T) {
	prodLogin(t, "", "refresh-1")
	stubRefresh(t, func(string, string) (*astroauth.TokenResponse, error) {
		t.Error("refreshed a login the user logged out of")
		return &astroauth.TokenResponse{AccessToken: "new"}, nil
	})
	_, err := BearerFor(context.Background(), "astronomer.io")
	if err == nil || !strings.HasPrefix(err.Error(), "not logged in to astronomer.io") {
		t.Fatalf("err = %v, want the astronomer.io login named as missing", err)
	}
}

func TestBearerForNamesTheDomainWithNoLogin(t *testing.T) {
	t.Setenv(EnvAPIToken, "")
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	_, err := BearerFor(context.Background(), "astronomer-stage.io")
	want := "not logged in to astronomer-stage.io. Log in with `astro login astronomer-stage.io`"
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
}

func TestRejectedBlamesTheAPITokenWhenOneIsSet(t *testing.T) {
	t.Setenv(EnvAPIToken, "ci-token")

	err := Rejected("astronomer-dev.io")

	if !strings.Contains(err.Error(), "astronomer-dev.io rejected the token in "+EnvAPIToken) {
		t.Fatalf("got %q", err)
	}
}

func TestRejectedWithNoAPITokenIsAnExpiredLogin(t *testing.T) {
	t.Setenv(EnvAPIToken, "")

	if got, want := Rejected("astronomer-dev.io").Error(), ExpiredOn("astronomer-dev.io").Error(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if !errors.Is(Rejected(""), errExpired) {
		t.Fatalf("with no domain, got %q", Rejected(""))
	}
}
