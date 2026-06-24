package cloud

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	astrov1 "github.com/astronomer/astro-cli/astro-client-v1"
	astrov1_mocks "github.com/astronomer/astro-cli/astro-client-v1/mocks"
	"github.com/astronomer/astro-cli/cloud/workspace"
	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func execWorkspaceCmd(args ...string) (string, error) {
	buf := new(bytes.Buffer)
	cmd := newWorkspaceCmd(buf)
	cmd.SetOut(buf)
	cmd.SetArgs(args)
	testUtil.SetupOSArgsForGinkgo()
	_, err := cmd.ExecuteC()
	return buf.String(), err
}

func TestWorkspaceRootCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	buf := new(bytes.Buffer)
	cmd := newWorkspaceCmd(os.Stdout)
	cmd.SetOut(buf)
	testUtil.SetupOSArgsForGinkgo()
	_, err := cmd.ExecuteC()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "workspace")
}

var (
	workspaceTestDescription = "test workspace"
	workspace1               = astrov1.Workspace{
		Name:           "test-workspace",
		Description:    &workspaceTestDescription,
		Id:             "workspace-id",
		OrganizationId: "test-org-id",
	}

	workspaces = []astrov1.Workspace{
		workspace1,
	}
	ListWorkspacesResponseOK = astrov1.ListWorkspacesResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.WorkspacesPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
			Workspaces: workspaces,
		},
	}
)

func TestWorkspaceList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
	mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
	astroV1Client = mockClient

	cmdArgs := []string{"list"}
	resp, err := execWorkspaceCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, "workspace-id")
	assert.Contains(t, resp, "test-workspace")
	mockClient.AssertExpectations(t)
}

func TestWorkspaceListJSON(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
	mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
	astroV1Client = mockClient

	cmdArgs := []string{"list", "--json"}
	resp, err := execWorkspaceCmd(cmdArgs...)
	assert.NoError(t, err)

	var result workspace.WorkspaceList
	assert.NoError(t, json.Unmarshal([]byte(resp), &result))
	assert.Len(t, result.Workspaces, 1)
	assert.Equal(t, "test-workspace", result.Workspaces[0].Name)
	assert.Equal(t, "workspace-id", result.Workspaces[0].ID)
	mockClient.AssertExpectations(t)
}

func TestWorkspaceSwitch(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
	mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
	astroV1Client = mockClient

	// mock os.Stdin
	input := []byte("1")
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Write(input)
	if err != nil {
		t.Error(err)
	}
	w.Close()
	stdin := os.Stdin
	// Restore stdin right after the test.
	defer func() { os.Stdin = stdin }()
	os.Stdin = r

	cmdArgs := []string{"switch"}
	resp, err := execWorkspaceCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, "workspace-id")
	assert.Contains(t, resp, "test-workspace")
	mockClient.AssertExpectations(t)
}

func TestCoalesceWorkspace(t *testing.T) {
	// Save and restore the package-level workspaceID flag var so cases don't bleed.
	origWorkspaceID := workspaceID
	defer func() { workspaceID = origWorkspaceID }()

	// setCurrentWorkspace mutates the current context's persisted workspace, mirroring
	// what `astro workspace switch` (non-empty) or a fresh org-token CI home (empty) leave behind.
	setCurrentWorkspace := func(t *testing.T, ws string) {
		t.Helper()
		ctx, err := config.GetCurrentContext()
		assert.NoError(t, err)
		ctx.Workspace = ws
		assert.NoError(t, ctx.SetContext())
	}

	t.Run("explicit --workspace-id is honored when no current workspace context is set", func(t *testing.T) {
		// Regression: org-scoped API tokens leave the current-workspace context empty.
		// coalesceWorkspace must return the explicit flag instead of erroring out.
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		setCurrentWorkspace(t, "")
		workspaceID = "explicit-workspace-id"

		ws, err := coalesceWorkspace()
		assert.NoError(t, err)
		assert.Equal(t, "explicit-workspace-id", ws)
	})

	t.Run("explicit --workspace-id takes precedence over the current workspace context", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		setCurrentWorkspace(t, "context-workspace-id")
		workspaceID = "explicit-workspace-id"

		ws, err := coalesceWorkspace()
		assert.NoError(t, err)
		assert.Equal(t, "explicit-workspace-id", ws)
	})

	t.Run("falls back to the current workspace context when no flag is provided", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		setCurrentWorkspace(t, "context-workspace-id")
		workspaceID = ""

		ws, err := coalesceWorkspace()
		assert.NoError(t, err)
		assert.Equal(t, "context-workspace-id", ws)
	})

	t.Run("errors when neither a flag nor a current workspace context is available", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		setCurrentWorkspace(t, "")
		workspaceID = ""

		_, err := coalesceWorkspace()
		assert.Error(t, err)
	})
}
