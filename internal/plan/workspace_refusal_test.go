package plan

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/internal/astrosession"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// A project that links a Workspace the run's Workspace token is not for is
// refused only when a Workspace value is read: a declared workspace-source
// name asks Environment Manager, and nothing is sent. A start that declares
// none goes on as it did.
func TestBuildRefusesAWorkspaceTheTokenIsNotForOnlyWhenItReadsIt(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	testUtil.UseAPIToken(t, "organizationId:org", "workspaceId:ws-of-the-token")

	t.Run("a workspace-source value is read", func(t *testing.T) {
		dir := newWorkspaceProject(t, workspaceManifest)
		mc := new(astrov1_mocks.ClientWithResponsesInterface) // any call fails the test
		_, err := Build(dir, Options{WorkspaceProvider: emProvider(mc)})
		var disagree *astrosession.WorkspaceDisagreeError
		if !errors.As(err, &disagree) {
			t.Fatalf("err = %v, want a *WorkspaceDisagreeError", err)
		}
		if !strings.Contains(err.Error(), "the project's link names cmws") {
			t.Errorf("err = %q", err)
		}
		mc.AssertNotCalled(t, "ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("no value needs the workspace", func(t *testing.T) {
		body := strings.Replace(workspaceManifest, "\n[tool.astro.env]\nDATA_WAREHOUSE_URI = { source = 'workspace' }\n", "", 1)
		dir := newWorkspaceProject(t, body)
		mc := new(astrov1_mocks.ClientWithResponsesInterface)
		if _, err := Build(dir, Options{WorkspaceProvider: emProvider(mc)}); err != nil {
			t.Fatalf("Build: %v", err)
		}
		if len(mc.Calls) != 0 {
			t.Fatalf("%d requests were made for a Workspace the token is not for", len(mc.Calls))
		}
	})
}
