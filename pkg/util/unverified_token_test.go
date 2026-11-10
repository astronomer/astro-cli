package util

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The CLI reads an API token's claims without checking its signature, and a
// broker on ASTRO_HTTP_UNIX_SOCKET depends on that: it can hand the CLI a
// stand-in token that carries the real token's claims, signed by nothing the
// CLI knows, and swap in the real one upstream. A change that starts verifying
// signatures here breaks that setup, so it should be a deliberate one.
func TestParseAPITokenDoesNotVerifyTheSignature(t *testing.T) {
	exp := jwt.NewNumericDate(time.Now().Add(time.Hour))
	claims := CustomClaims{
		Permissions: []string{"workspaceId:ws", "organizationId:org"},
		APITokenID:  "tok",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: exp,
		},
	}
	placeholder, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("a key the CLI never sees"))
	require.NoError(t, err)

	got, err := ParseAPIToken(placeholder)
	require.NoError(t, err)
	assert.Equal(t, claims.Permissions, got.Permissions)
	assert.Equal(t, "tok", got.APITokenID)
	assert.True(t, exp.Equal(got.ExpiresAt.Time))
}
