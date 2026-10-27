package astrosession

import (
	"testing"

	"github.com/golang-jwt/jwt/v4"

	"github.com/astronomer/astro-cli/pkg/util"
)

// twoLogins is a home config with a login on two hosts, each in its own
// organization, with astronomer.io current.
const twoLogins = "context: astronomer.io\n" +
	"contexts:\n" +
	"  astronomer_io:\n" +
	"    domain: astronomer.io\n" +
	"    organization: org-prod\n" +
	"    token: 'Bearer t1'\n" +
	"  astronomer-dev_io:\n" +
	"    domain: astronomer-dev.io\n" +
	"    organization: org-dev\n" +
	"    token: 'Bearer t2'\n"

func TestOrganizationForReadsTheLoginSavedForTheDomain(t *testing.T) {
	writeConfig(t, twoLogins)
	t.Setenv(EnvAPIToken, "")
	cases := []struct {
		domain, want string
	}{
		{"astronomer.io", "org-prod"},
		// The project's host, not the current context's.
		{"astronomer-dev.io", "org-dev"},
		// A project that names no host reads the current context.
		{"", "org-prod"},
		// A host with no login saved is unknown, not the current one's.
		{"nowhere.example", ""},
	}
	for _, c := range cases {
		if got := OrganizationFor(c.domain); got != c.want {
			t.Errorf("OrganizationFor(%q) = %q, want %q", c.domain, got, c.want)
		}
	}
}

func TestOrganizationForWithNoLoginIsUnknown(t *testing.T) {
	writeConfig(t, "")
	t.Setenv(EnvAPIToken, "")
	for _, domain := range []string{"", "astronomer.io"} {
		if got := OrganizationFor(domain); got != "" {
			t.Errorf("OrganizationFor(%q) = %q on a machine with no login, want none", domain, got)
		}
	}
}

// An API token in the environment is the identity a run calls with, so its
// organization is the answer whatever the saved logins say, and a token that
// names none leaves the organization unknown rather than borrowing a saved
// login's.
func TestOrganizationForPrefersTheAPITokensClaims(t *testing.T) {
	writeConfig(t, twoLogins)
	mint := func(permissions ...string) string {
		t.Helper()
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, util.CustomClaims{Permissions: permissions}).SignedString([]byte("k"))
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	cases := []struct {
		name, token, want string
	}{
		{"organization token", mint("organizationId:org-token"), "org-token"},
		{"workspace token", mint("workspaceId:ws", "organizationId:org-token"), "org-token"},
		{"token with the scheme", "Bearer " + mint("organizationId:org-token"), "org-token"},
		{"token naming no organization", mint("deploymentId:dep"), ""},
		{"not a token", "nonsense", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(EnvAPIToken, c.token)
			for _, domain := range []string{"", "astronomer.io", "astronomer-dev.io"} {
				if got := OrganizationFor(domain); got != c.want {
					t.Errorf("OrganizationFor(%q) = %q, want %q", domain, got, c.want)
				}
			}
		})
	}
}
