package otto

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// checkAnswering is a MembershipCheck that records the organization it was
// asked about and gives the answer it was built with.
func checkAnswering(user string, notMember bool, asked *string) MembershipCheck {
	return func(_ context.Context, org string) (string, bool) {
		*asked = org
		return user, notMember
	}
}

func TestVerifyOrgMembership(t *testing.T) {
	t.Run("a member goes ahead and nothing is printed", func(t *testing.T) {
		var asked string
		var out bytes.Buffer
		err := verifyOrgMembership(&Config{Token: "t", Organization: "org-a", loginOrganization: "org-a"}, checkAnswering("", false, &asked), &out)
		assert.NoError(t, err)
		assert.Equal(t, "org-a", asked)
		assert.Empty(t, out.String())
	})

	sources := []struct {
		name  string
		cfg   *Config
		env   string
		org   string
		where string
		fix   string
	}{
		{
			name: "the login's organization", cfg: &Config{Token: "t", Organization: "org-a", loginOrganization: "org-a"},
			org: "org-a", where: "selected for this login", fix: "astro organization switch",
		},
		{
			name: "the project's organization", cfg: &Config{Token: "t", Organization: "org-p", loginOrganization: "org-a"},
			org: "org-p", where: "[tool.astro] organization", fix: "Run Otto outside this project",
		},
		{
			name: "an inherited ASTRO_ORGANIZATION", cfg: &Config{Token: "t"}, env: "stale-org",
			org: "stale-org", where: "ASTRO_ORGANIZATION in your environment", fix: "unset ASTRO_ORGANIZATION",
		},
	}
	for _, tc := range sources {
		t.Run("a non-member of "+tc.name+" is stopped and told why", func(t *testing.T) {
			t.Setenv("ASTRO_ORGANIZATION", tc.env)
			var asked string
			var out bytes.Buffer
			err := verifyOrgMembership(tc.cfg, checkAnswering("b@example.com", true, &asked), &out)
			assert.ErrorIs(t, err, ErrNotOrgMember)
			assert.Equal(t, tc.org, asked)
			msg := out.String()
			assert.Contains(t, msg, "b@example.com isn't a member of organization "+tc.org)
			assert.Contains(t, msg, tc.where)
			assert.Contains(t, msg, tc.fix)
			assert.Contains(t, msg, "astro login")
		})
	}

	never := func(t *testing.T) MembershipCheck {
		return func(context.Context, string) (string, bool) {
			t.Error("the check ran")
			return "", true
		}
	}
	t.Run("no organization skips the check", func(t *testing.T) {
		t.Setenv("ASTRO_ORGANIZATION", "")
		assert.NoError(t, verifyOrgMembership(&Config{Token: "t"}, never(t), &bytes.Buffer{}))
	})
	t.Run("no token skips the check", func(t *testing.T) {
		assert.NoError(t, verifyOrgMembership(&Config{Organization: "org-a"}, never(t), &bytes.Buffer{}))
	})
	t.Run("no check skips it", func(t *testing.T) {
		assert.NoError(t, verifyOrgMembership(&Config{Token: "t", Organization: "org-a"}, nil, &bytes.Buffer{}))
	})
	t.Run("the check runs under a deadline", func(t *testing.T) {
		check := func(ctx context.Context, _ string) (string, bool) {
			_, ok := ctx.Deadline()
			assert.True(t, ok, "no deadline on the check's context")
			return "", false
		}
		assert.NoError(t, verifyOrgMembership(&Config{Token: "t", Organization: "org-a"}, check, &bytes.Buffer{}))
	})
}
