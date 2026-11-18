package astrosession

import (
	"context"
	"errors"
	"testing"
	"time"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func TestTokenIdentityReadsTheClaims(t *testing.T) {
	for _, tt := range []struct {
		name        string
		permissions []string
		want        Identity
	}{
		{"organization", []string{"organizationId:org"}, Identity{Organization: "org"}},
		{"workspace", []string{"organizationId:org", "workspaceId:ws"}, Identity{Organization: "org", Workspace: "ws"}},
		{"deployment", []string{"organizationId:org", "workspaceId:ws", "deploymentId:dep"}, Identity{Organization: "org", Workspace: "ws", Deployment: "dep"}},
		{"several different workspaces are none", []string{"organizationId:org", "workspaceId:a", "workspaceId:b"}, Identity{Organization: "org"}},
		{"the same workspace twice is one", []string{"organizationId:org", "workspaceId:a", "workspaceId:a"}, Identity{Organization: "org", Workspace: "a"}},
		{"several deployments are none", []string{"organizationId:org", "deploymentId:a", "deploymentId:b"}, Identity{Organization: "org"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			expiry := time.Now().Add(time.Hour).Truncate(time.Second)
			token := testUtil.MintAPIToken(t, expiry, tt.permissions...)
			t.Setenv(EnvAPIToken, "Bearer "+token)
			got, ok, err := TokenIdentity()
			if err != nil || !ok {
				t.Fatalf("ok, err = %v, %v", ok, err)
			}
			if !got.ExpiresAt.Equal(expiry) {
				t.Errorf("expiry %v, want %v", got.ExpiresAt, expiry)
			}
			got.ExpiresAt = time.Time{}
			tt.want.Credential = token
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestTokenIdentityWithNoTokenIsNoTokenRun(t *testing.T) {
	for _, v := range []string{"", "Bearer ", " bearer"} {
		t.Setenv(EnvAPIToken, v)
		if _, ok, err := TokenIdentity(); ok || err != nil {
			t.Errorf("%s=%q: ok, err = %v, %v", EnvAPIToken, v, ok, err)
		}
	}
}

// A token that is malformed, expired or without permissions fails every
// reader of the session, as setup fails the platform tree, and the saved login
// is not read instead.
func TestUnusableTokensFailEveryReaderOfTheSession(t *testing.T) {
	testUtil.MockKeyring(t)
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	for _, tt := range []struct {
		name  string
		token string
		want  error
	}{
		{"malformed", "ci-token", ErrAPITokenMalformed},
		{"expired", testUtil.MintAPIToken(t, time.Now().Add(-time.Minute), "organizationId:org"), ErrAPITokenExpired},
		{"no permissions", testUtil.MintAPIToken(t, time.Now().Add(time.Hour)), ErrNotAPIToken},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(EnvAPIToken, tt.token)
			if _, err := Bearer(context.Background()); !errors.Is(err, tt.want) || !errors.Is(err, ErrInvalidToken) {
				t.Errorf("Bearer: %v", err)
			}
			if _, err := BearerFor(context.Background(), "astronomer.io"); !errors.Is(err, tt.want) {
				t.Errorf("BearerFor: %v", err)
			}
			if _, err := Login("astronomer.io"); !errors.Is(err, tt.want) {
				t.Errorf("Login: %v", err)
			}
		})
	}
}

// Login under a token is the token's own: its credential, and the
// organization and Workspace its claims name, whatever the saved login holds.
func TestLoginUnderATokenIsTheTokensOwn(t *testing.T) {
	testUtil.MockKeyring(t)
	testUtil.InitTestConfig(testUtil.CloudPlatform) // a saved login with its own token, organization and Workspace
	token := testUtil.MintAPIToken(t, time.Now().Add(time.Hour), "organizationId:org-claim", "workspaceId:ws-claim")
	t.Setenv(EnvAPIToken, token)
	got, err := Login("astronomer.io")
	if err != nil {
		t.Fatal(err)
	}
	if got.Token != "Bearer "+token || got.Organization != "org-claim" || got.Workspace != "ws-claim" || got.Domain != "astronomer.io" {
		t.Errorf("login = %+v", got)
	}
}

// Without any overlay on the context, and with no saved login at all, a token's
// claim is still the run's Workspace.
func TestResolveReadsTheClaimsWithNoContextAtAll(t *testing.T) {
	testUtil.MockKeyring(t)
	testUtil.InitTestConfig(testUtil.Initial)
	t.Setenv(EnvAPIToken, testUtil.MintAPIToken(t, time.Now().Add(time.Hour), "organizationId:org", "workspaceId:ws-claim"))
	got := Resolve(Request{LoginError: true})
	if got.Workspace != "ws-claim" || got.WorkspaceSource != SourceToken || got.Require() != nil {
		t.Errorf("got %q from %v", got.Workspace, got.WorkspaceSource)
	}
}

// With the saved login's Workspace on the context and nothing applied, an
// Organization token still has none.
func TestResolveUnderAnOrganizationTokenIgnoresTheSavedWorkspace(t *testing.T) {
	testUtil.MockKeyring(t)
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	t.Setenv(EnvAPIToken, testUtil.MintAPIToken(t, time.Now().Add(time.Hour), "organizationId:org"))
	for _, domain := range []string{"", "astronomer.io"} {
		got := Resolve(Request{Domain: domain})
		if got.Workspace != "" || got.WorkspaceSource != SourceNone {
			t.Errorf("domain %q: got %q from %v", domain, got.Workspace, got.WorkspaceSource)
		}
		if !errors.Is(got.Missing(), ErrOrgTokenNoWorkspace) {
			t.Errorf("domain %q: Missing() = %v", domain, got.Missing())
		}
	}
}

// An unusable token that finds no Workspace is the run's error, not an
// Organization token that "selects none".
func TestMissingWorkspaceUnderAnUnusableTokenNamesTheToken(t *testing.T) {
	t.Setenv(EnvAPIToken, testUtil.MintAPIToken(t, time.Now().Add(-time.Hour), "organizationId:org", "workspaceId:ws"))
	err := Resolve(Request{Advice: ByFlag}).Missing()
	if !errors.Is(err, ErrAPITokenExpired) {
		t.Fatalf("got %v, want the expired-token error", err)
	}
}
