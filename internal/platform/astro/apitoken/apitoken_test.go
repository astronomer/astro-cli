package apitoken

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
)

func ptr[T any](v T) *T { return &v }

func TestFromAPI(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	end := created.Add(24 * time.Hour)
	api := &astrov1.ApiToken{
		Id: "t1", Name: "one", Description: "about one", Scope: astrov1.ApiTokenScopeWORKSPACE,
		CreatedAt: created, EndAt: &end, ExpiryPeriodInDays: ptr(1),
		Token: ptr("secret"),
	}

	t.Run("the creator is a person's full name, else the token that created it", func(t *testing.T) {
		api.CreatedBy = &astrov1.BasicSubjectProfile{FullName: ptr("Ada"), ApiTokenName: ptr("ci")}
		assert.Equal(t, "Ada", FromAPI(api, "").CreatedBy)
		api.CreatedBy = &astrov1.BasicSubjectProfile{ApiTokenName: ptr("ci")}
		assert.Equal(t, "ci", FromAPI(api, "").CreatedBy)
		api.CreatedBy = nil
		assert.Empty(t, FromAPI(api, "").CreatedBy)
	})

	t.Run("no secret unless asked for", func(t *testing.T) {
		tok := FromAPI(api, "WORKSPACE_MEMBER")
		assert.Equal(t, Token{
			ID: "t1", Name: "one", Description: "about one", Scope: "WORKSPACE", Role: "WORKSPACE_MEMBER",
			CreatedAt: created, ExpiresAt: &end, ExpiryPeriodInDays: ptr(1),
		}, tok)
		assert.Equal(t, "secret", WithSecret(tok, api).Token)
	})

	t.Run("the lifetime it was created with is not published", func(t *testing.T) {
		b, err := json.Marshal(FromAPI(api, ""))
		require.NoError(t, err)
		var keys map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(b, &keys))
		assert.NotContains(t, keys, "expiry_period_in_days")
		assert.NotContains(t, keys, "ExpiryPeriodInDays")
	})
}

func TestAllFromAPIIsNeverNil(t *testing.T) {
	got := AllFromAPI(nil, func(*astrov1.ApiToken) string { return "" })
	assert.NotNil(t, got)
	assert.Empty(t, got)
}

func TestRoles(t *testing.T) {
	ws := astrov1.ApiTokenRoleEntityTypeWORKSPACE
	dep := astrov1.ApiTokenRoleEntityTypeDEPLOYMENT
	tok := &astrov1.ApiToken{Roles: &[]astrov1.ApiTokenRole{
		{EntityType: ws, EntityId: "w1", Role: "WORKSPACE_MEMBER"},
		{EntityType: dep, EntityId: "w1", Role: "DEPLOYMENT_ADMIN"},
	}}

	t.Run("RoleOn matches the type and the id", func(t *testing.T) {
		assert.Equal(t, "WORKSPACE_MEMBER", RoleOn(tok, ws, "w1"))
		assert.Equal(t, "DEPLOYMENT_ADMIN", RoleOn(tok, dep, "w1"))
		assert.Empty(t, RoleOn(tok, ws, "w2"))
		assert.Empty(t, RoleOn(&astrov1.ApiToken{}, ws, "w1"), "a token with no roles")
	})

	t.Run("WithRole replaces, adds and removes one entry", func(t *testing.T) {
		replaced := WithRole(Roles(tok), ws, "w1", "WORKSPACE_OWNER")
		assert.Equal(t, []astrov1.ApiTokenRole{
			{EntityType: dep, EntityId: "w1", Role: "DEPLOYMENT_ADMIN"},
			{EntityType: ws, EntityId: "w1", Role: "WORKSPACE_OWNER"},
		}, replaced)
		added := WithRole(Roles(tok), ws, "w2", "WORKSPACE_MEMBER")
		assert.Len(t, added, 3)
		removed := WithRole(Roles(tok), ws, "w1", "")
		assert.Equal(t, []astrov1.ApiTokenRole{{EntityType: dep, EntityId: "w1", Role: "DEPLOYMENT_ADMIN"}}, removed)
		assert.NotNil(t, WithRole(nil, ws, "w1", ""), "an empty role set is sent as [], not null")
	})
}

func TestPick(t *testing.T) {
	tokens := []astrov1.ApiToken{{Id: "a", Name: "one"}, {Id: "b", Name: "two"}}
	roleOf := func(t *astrov1.ApiToken) string { return "role-" + t.Id }

	t.Run("offers each token with its role, and returns the one chosen", func(t *testing.T) {
		var offered []Token
		got, err := Pick(func(heading string, ts []Token) (int, error) {
			assert.Equal(t, "which?", heading)
			offered = ts
			return 1, nil
		}, "which?", tokens, roleOf)
		require.NoError(t, err)
		assert.Equal(t, "b", got.Id)
		require.Len(t, offered, 2)
		assert.Equal(t, "role-a", offered[0].Role)
	})

	t.Run("a refusal is the error", func(t *testing.T) {
		refused := errors.New("cannot ask")
		_, err := Pick(func(string, []Token) (int, error) { return 0, refused }, "", tokens, roleOf)
		assert.ErrorIs(t, err, refused)
	})

	t.Run("out of range is an error, not a panic", func(t *testing.T) {
		_, err := Pick(func(string, []Token) (int, error) { return 2, nil }, "", tokens, roleOf)
		assert.Error(t, err)
	})
}

func TestTimeAgo(t *testing.T) {
	now := time.Now()
	assert.Equal(t, "Just now", TimeAgo(now))
	assert.Equal(t, "30 minutes ago", TimeAgo(now.Add(-30*time.Minute)))
	assert.Equal(t, "5 hours ago", TimeAgo(now.Add(-5*time.Hour)))
	assert.Equal(t, "10 days ago", TimeAgo(now.Add(-10*24*time.Hour)))
}
