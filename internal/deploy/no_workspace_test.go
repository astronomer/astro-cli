package deploy

import (
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// A deploy under an API token with no workspace to list from may be given one
// by --workspace or by the project's link, and says so.
func TestUnlinkedTargetWithNoWorkspaceUnderATokenNamesTheProject(t *testing.T) {
	t.Setenv("ASTRO_API_TOKEN", "token")
	_, err := unlinkedTarget(Request{}, nil)
	if err == nil || !strings.Contains(err.Error(), "--workspace, or run in a project") {
		t.Errorf("err = %v", err)
	}

	// Astro Private Cloud takes no Astro token, so its message is its own.
	_, err = unlinkedTarget(Request{Platform: manifest.KindAPC}, nil)
	if err == nil || strings.Contains(err.Error(), "ASTRO_API_TOKEN") {
		t.Errorf("an APC deploy blamed the Astro token: %v", err)
	}
}
