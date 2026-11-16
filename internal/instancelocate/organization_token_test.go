package instancelocate

import (
	"errors"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/internal/astrosession"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// Under ASTRO_API_TOKEN the organization is the token's own claim, on either
// path, and never the saved login's.
func TestOrganizationUnderATokenIsTheClaim(t *testing.T) {
	testUtil.MockKeyring(t)
	testUtil.InitTestConfig(testUtil.LocalPlatform) // a saved login in test-org-id
	t.Setenv(astrosession.EnvAPIToken, testUtil.MintAPIToken(t, time.Now().Add(time.Hour), "organizationId:org-claim"))
	for _, domain := range []string{"", "localhost", "astronomer.io"} {
		if org, err := Organization(domain); err != nil || org != "org-claim" {
			t.Errorf("Organization(%q) = %q, %v; want the token's", domain, org, err)
		}
	}

	t.Setenv(astrosession.EnvAPIToken, testUtil.MintAPIToken(t, time.Now().Add(time.Hour), "workspaceId:ws"))
	if _, err := Organization(""); !errors.Is(err, astrosession.ErrTokenNoOrganization) {
		t.Errorf("a token naming no organization: %v", err)
	}
	t.Setenv(astrosession.EnvAPIToken, testUtil.MintAPIToken(t, time.Now().Add(-time.Hour), "organizationId:org"))
	if _, err := Organization(""); !errors.Is(err, astrosession.ErrAPITokenExpired) {
		t.Errorf("an expired token: %v", err)
	}
}
