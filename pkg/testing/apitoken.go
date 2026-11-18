package testing

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"

	"github.com/astronomer/astro-cli/config"
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
	ApplyAPIToken(t)
}

// ApplyAPIToken applies the ASTRO_API_TOKEN of the test to the current
// context, as the setup that accepts the token does before a command runs: the
// token is the context's login, and its Workspace claim (none for an
// Organization token) the selection, whatever the saved context holds. A run
// that reads the Workspace without that setup is refused the saved one. Call it
// again after anything that reloads the config (InitTestConfig).
func ApplyAPIToken(t *testing.T) {
	t.Helper()
	token := os.Getenv("ASTRO_API_TOKEN")
	claims, err := util.ParseAPIToken(token)
	if err != nil {
		return
	}
	var workspace string
	for _, permission := range claims.Permissions {
		if id, ok := strings.CutPrefix(permission, "workspaceId:"); ok {
			workspace = id
		}
	}
	if restore, ok := config.UseEnvironmentLoginForTesting("Bearer "+token, workspace); ok {
		t.Cleanup(restore)
	}
}
