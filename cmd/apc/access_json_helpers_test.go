package apc

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
)

// accessRun is one run of the APC tree the way main runs it: through
// cliout.Execute, with stdout and stderr apart and stdin under the test's
// control.
type accessRun struct {
	stdout, stderr string
	code           int
	// stdinRead is whether anything read stdin: a refused prompt must not.
	stdinRead bool
}

// accessWorkspaceID is the Workspace the access tests act on, given by flag.
const accessWorkspaceID = "ws-1"

// newAccessClient is a Houston mock that answers what building the tree asks.
func newAccessClient() *mocks.ClientInterface {
	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{}, nil).Maybe()
	api.On("GetPlatformVersion", mock.Anything).Return("0.34.0", nil).Maybe()
	return api
}

// bindAccessTree builds the APC tree under a root that binds no writer of its
// own, with the tree's out as given: production's shape.
func bindAccessTree(api houston.ClientInterface, out io.Writer) *cobra.Command {
	root := &cobra.Command{Use: "astro", SilenceErrors: true}
	LoadPlatform(api) // as the root does for a line that runs one of these commands
	root.AddCommand(AddCmds(api, out)...)
	return root
}

// runAccess builds the APC tree on a file standing in for stdout, and runs
// args with answers on stdin. The root binds no writer of its own, so a
// result that left by anything but the tree's out, or progress that reached
// stdout, shows up where it went.
// The test sets the config up first (testUtil.InitTestConfig), so it can change it.
func runAccess(t *testing.T, api houston.ClientInterface, answers string, args ...string) accessRun {
	t.Helper()
	dir := t.TempDir()
	stdout, err := os.Create(filepath.Join(dir, "stdout"))
	require.NoError(t, err)
	stderr, err := os.Create(filepath.Join(dir, "stderr"))
	require.NoError(t, err)
	inR, inW, err := os.Pipe()
	require.NoError(t, err)
	_, err = inW.WriteString(answers)
	require.NoError(t, err)
	require.NoError(t, inW.Close())

	prevOut, prevErr, prevIn := os.Stdout, os.Stderr, os.Stdin
	os.Stdout, os.Stderr, os.Stdin = stdout, stderr, inR
	t.Cleanup(func() { os.Stdout, os.Stderr, os.Stdin = prevOut, prevErr, prevIn })

	ctx := context.Background()
	runErr := cliout.Execute(ctx, bindAccessTree(api, stdout), args, stdout, nil)
	os.Stdout, os.Stderr, os.Stdin = prevOut, prevErr, prevIn

	left, _ := io.ReadAll(inR)
	require.NoError(t, stdout.Close())
	require.NoError(t, stderr.Close())
	outBytes, err := os.ReadFile(stdout.Name())
	require.NoError(t, err)
	errBytes, err := os.ReadFile(stderr.Name())
	require.NoError(t, err)
	return accessRun{
		stdout:    string(outBytes),
		stderr:    string(errBytes),
		code:      cliout.ExitCode(ctx, runErr),
		stdinRead: len(left) < len(answers),
	}
}

// decodeAccess decodes stdout as exactly one json value into v.
func decodeAccess(t *testing.T, stdout string, v any) {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(stdout))
	require.NoError(t, dec.Decode(v), "stdout:\n%s", stdout)
	var extra json.RawMessage
	require.ErrorIs(t, dec.Decode(&extra), io.EOF, "more than one value on stdout:\n%s", stdout)
}

// accessError is the failure cliout.Execute publishes under json.
type accessError struct {
	Error string `json:"error"`
	Code  int    `json:"code"`
	Kind  string `json:"kind"`
}

func sp(s string) *string { return &s }
