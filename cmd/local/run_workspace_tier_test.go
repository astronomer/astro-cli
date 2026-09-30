//go:build !windows

package local

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"

	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// A run that asks for the workspace goes on without it when it is offline,
// with one line saying so, when nothing required needs it.
func TestRunWithWorkspaceOfflineNotes(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	manifest := strings.Replace(workspaceEnvManifest, "DATA_WAREHOUSE_URI = { source = 'workspace' }\n", "", 1)
	d, stderr, _ := stoppedWorkspaceProject(t, manifest)
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("dial tcp: no route to host"))
	d.WorkspaceClients = workspaceClients(mc)
	if err := execute(t, d, "local", "run", "--with-workspace", "pytest"); err != nil {
		t.Fatalf("offline run: %v", err)
	}
	if !strings.Contains(stderr.String(), "workspace cmws not read (offline): starting without its values") {
		t.Errorf("stderr = %q, want the offline note", stderr.String())
	}
}
