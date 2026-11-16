package testing

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"

	"github.com/astronomer/astro-cli/pkg/util"
)

// MintAPIToken returns an unsigned API token with the given permissions
// ("organizationId:org", "workspaceId:ws", "deploymentId:dep", ...) that
// expires at expiresAt, never when it is the zero time. Unlike UseAPIToken it
// sets nothing: a test that needs an expired token, or two tokens, or to set
// the variable itself, mints it here. The CLI only reads a token's claims, so
// the signature is not one.
func MintAPIToken(t *testing.T, expiresAt time.Time, permissions ...string) string {
	t.Helper()
	claims := util.CustomClaims{Permissions: permissions}
	if !expiresAt.IsZero() {
		claims.ExpiresAt = jwt.NewNumericDate(expiresAt)
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("minting a test API token: %v", err)
	}
	return token
}
