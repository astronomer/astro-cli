package astro

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/pkg/input"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// spoof is a name or value as another user may have typed it: a screen clear,
// a window title and a line break.
const spoof = "prod\x1b[2J\x1b]0;title\x07\nfake line"

// The Workspace name in the note that says the project chose it is plain
// text on one line.
func TestWorkspaceLabelIsPlain(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("GetWorkspaceWithResponse", mock.Anything, mock.Anything, "ws-1").Return(&astrov1.GetWorkspaceResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.Workspace{Id: "ws-1", Name: spoof},
	}, nil)
	orig := astroV1Client
	astroV1Client = mc
	t.Cleanup(func() { astroV1Client = orig })

	assert.Equal(t, "prod fake line", workspaceLabel(context.Background(), "ws-1", ""))
}

// The notice for a Deployment with no logs names it as plain text, in text
// and under json (where it goes to stderr).
func TestNoLogsNoticeNamesTheDeploymentPlain(t *testing.T) {
	res := &deployment.LogsResult{DeploymentName: spoof, Hours: 1}
	for _, f := range []cliout.Format{cliout.FormatText, cliout.FormatJSON} {
		var out, errOut bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetErr(&errOut)
		require.NoError(t, emitLogs(cmd, cliout.Renderer{Format: f, Out: &out}, res))
		got := out.String() + errOut.String()
		assert.Contains(t, got, "for Deployment prod fake line\n", f)
		assert.NotContains(t, got, "\x1b", f)
	}
}

// Dotenv lines to a terminal show a value's escapes rather than act on them;
// to a file or a pipe they are the bytes a later load reads back.
func TestDotenvToATerminalIsShownNotActedOn(t *testing.T) {
	value := "a\x1b]52;c;aGk=\x07b"
	objs := []astrov1.EnvironmentObject{{ObjectKey: "K", EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: value}}}

	var piped bytes.Buffer
	require.NoError(t, emitDotenv(objs, &piped))
	assert.Contains(t, piped.String(), value, "a pipe gets the value as it is")

	defer input.ForceTerminalWriters()()
	var term bytes.Buffer
	require.NoError(t, emitDotenv(objs, &term))
	assert.Contains(t, term.String(), "K=")
	assert.NotContains(t, term.String(), "\x1b")
}
