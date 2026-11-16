package deploy

import (
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/internal/astrosession"
	"github.com/astronomer/astro-cli/pkg/manifest"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// A deploy under an API token with no workspace to list from may be given one
// by --workspace or by the project's link, and says so.
func TestUnlinkedTargetWithNoWorkspaceUnderATokenNamesTheProject(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	t.Setenv("ASTRO_API_TOKEN", "token")
	// The resolver words the error, for what an Astro deploy accepts.
	run := astrosession.Resolve(astrosession.Request{Accepts: astrosession.ByFlag | astrosession.ByProject, NoSaved: ErrNoWorkspace})
	_, err := unlinkedTarget(Request{NoWorkspace: run.Missing()}, nil)
	if err == nil || !strings.Contains(err.Error(), "--workspace, or run in a project") {
		t.Errorf("err = %v", err)
	}

	// Astro Private Cloud takes no Astro token, so its message is its own.
	_, err = unlinkedTarget(Request{Platform: manifest.KindAPC}, nil)
	if err == nil || strings.Contains(err.Error(), "ASTRO_API_TOKEN") {
		t.Errorf("an APC deploy blamed the Astro token: %v", err)
	}
}
