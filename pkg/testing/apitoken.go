package testing

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"

	"github.com/astronomer/astro-cli/pkg/util"
)

// UseAPIToken sets ASTRO_API_TOKEN for the test to an unsigned token whose
// permissions are given, as the claims of a Workspace, Deployment or
// Organization token read ("workspaceId:ws", "deploymentId:dep", ...). The CLI
// only reads a token's claims, never checks its signature.
func UseAPIToken(t *testing.T, permissions ...string) {
	t.Helper()
	claims := util.CustomClaims{
		Permissions:      permissions,
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("minting a test API token: %v", err)
	}
	t.Setenv("ASTRO_API_TOKEN", token)
}
