package apc

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// A table's success line, to a pipe, is a notice for the person: it goes to
// the command's own stderr (what cmd.SetErr binds), the table alone stays on
// stdout, and nothing reaches the process's stderr.
func TestSuccessNoticeLeavesByTheCommandsStderr(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	api := new(mocks.ClientInterface)
	api.On("AddWorkspaceUser", mock.Anything).Return(&houston.Workspace{Label: "ws-label"}, nil)
	prevFormat := accessOutput
	accessOutput = cliout.FormatText // a flag other tests leave set
	t.Cleanup(func() { accessOutput = prevFormat })
	prevClient, prevRole := houstonClient, workspaceUserWsRole
	houstonClient, workspaceUserWsRole = api, houston.WorkspaceAdminRole
	t.Cleanup(func() { houstonClient, workspaceUserWsRole = prevClient, prevRole })

	process, err := os.Create(filepath.Join(t.TempDir(), "stderr"))
	require.NoError(t, err)
	prevStderr := os.Stderr
	os.Stderr = process
	t.Cleanup(func() { os.Stderr = prevStderr })

	var out, errOut bytes.Buffer
	require.NoError(t, workspaceUserAdd(&out, &errOut))

	os.Stderr = prevStderr
	require.NoError(t, process.Close())
	leaked, err := os.ReadFile(process.Name())
	require.NoError(t, err)
	assert.Contains(t, errOut.String(), "Added", "the success line did not reach the command's stderr")
	assert.NotContains(t, out.String(), "Added", "the success line went to stdout")
	assert.Empty(t, string(leaked), "a notice went to the process's stderr")
}
