package cloud

import (
	"bytes"
	"net/http"
	"os"
	"testing"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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
	workspace1               = astrocore.Workspace{
		Name:                         "test-workspace",
		Description:                  &workspaceTestDescription,
		ApiKeyOnlyDeploymentsDefault: false,
		Id:                           "workspace-id",
		OrganizationId:               "test-org-id",
	}

	workspaces = []astrocore.Workspace{
		workspace1,
	}
	ListWorkspacesResponseOK = astrocore.ListWorkspacesResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.WorkspacesPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
			Workspaces: workspaces,
		},
	}
)

func TestWorkspaceSwitch(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
	mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
	astroCoreClient = mockClient

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
