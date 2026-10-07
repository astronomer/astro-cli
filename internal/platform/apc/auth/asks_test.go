package auth

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	houstonMocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// captureProcessOutput returns what f writes to the process's stdout and
// stderr, which it swaps for pipes while f runs.
func captureProcessOutput(t *testing.T, f func()) (stdout, stderr string) {
	t.Helper()
	read := func(r *os.File, done chan<- string) {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}
	outR, outW, err := os.Pipe()
	assert.NoError(t, err)
	errR, errW, err := os.Pipe()
	assert.NoError(t, err)
	previousOut, previousErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	outDone, errDone := make(chan string, 1), make(chan string, 1)
	go read(outR, outDone)
	go read(errR, errDone)
	f()
	os.Stdout, os.Stderr = previousOut, previousErr
	outW.Close()
	errW.Close()
	return <-outDone, <-errDone
}

// The OAuth flow's question (where to get the token, and the prompt for it)
// is on stderr, and stdout stays empty.
func (s *Suite) TestOAuthAsksOnStderr() {
	defer testUtil.MockUserInput(s.T(), "the-token\n")()
	var token string
	var err error
	stdout, stderr := captureProcessOutput(s.T(), func() { token, err = oAuth("https://houston.example.com/oauth") })
	s.NoError(err)
	s.Equal("the-token", token)
	s.Empty(stdout)
	s.Contains(stderr, houstonOAuthRedirect)
	s.Contains(stderr, "https://houston.example.com/oauth")
	s.Contains(stderr, inputOAuthToken)
}

// A login that has to ask which Workspace to use asks on stderr, intro,
// picker and prompt, and stdout stays empty; the context the switch leaves is
// the result, on the command's writer.
func (s *Suite) TestLoginAsksForTheWorkspaceOnStderr() {
	fs := afero.NewMemMapFs()
	s.Require().NoError(afero.WriteFile(fs, config.HomeConfigFile, testUtil.NewTestConfig("localhost"), 0o600))
	config.InitConfig(fs)
	s.Require().NoError(config.CFG.Interactive.SetHomeString("false"))
	s.Require().NoError(config.CFG.PageSize.SetHomeString("100"))
	defer testUtil.MockUserInput(s.T(), "2\n")()

	houstonMock := new(houstonMocks.ClientInterface)
	houstonMock.On("GetAuthConfig", mock.Anything).Return(&houston.AuthConfig{LocalEnabled: true}, nil)
	houstonMock.On("AuthenticateWithBasicAuth", mock.Anything).Return(mockToken, nil)
	houstonMock.On("ListWorkspaces", nil).Return([]houston.Workspace{{ID: "ws-first", Label: "first"}, {ID: "ws-second", Label: "second"}}, nil)
	houstonMock.On("ValidateWorkspaceID", "ws-second").Return(&houston.Workspace{}, nil)
	previous := switchToLastUsedWorkspace
	s.T().Cleanup(func() { switchToLastUsedWorkspace = previous })
	switchToLastUsedWorkspace = func(houston.ClientInterface, *config.Context) bool { return false }

	out := &bytes.Buffer{}
	var err error
	stdout, stderr := captureProcessOutput(s.T(), func() {
		err = Login("localhost", false, "test", "test", "0.30.0", houstonMock, out)
	})
	s.NoError(err)
	s.Empty(stdout)
	s.Contains(stderr, cliChooseWorkspace)
	s.Contains(stderr, "ws-first", "the picker's table")
	s.Contains(out.String(), "ws-second", "the context the switch left")
	s.NotContains(out.String(), "ws-first", "the question is not on the command's writer")
}
