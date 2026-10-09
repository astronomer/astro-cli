package apc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
	"github.com/astronomer/astro-cli/pkg/logger"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// The workspace family under --output json, and its text, run the way main
// runs them: through cliout.Execute, on the tree AddCmds builds.

// The current context's workspace in testUtil's software config.
const currentWS = "ck05r3bor07h40d02y2hw4n4v"

var (
	wsCurrent = houston.Workspace{ID: currentWS, Label: "airflow", Description: new("desc a"), CreatedAt: "2019-10-16T21:14:22.105Z", UpdatedAt: "2019-10-16T21:14:22.105Z"}
	wsOther   = houston.Workspace{ID: "ckbv8pwbq00wk0760us7ktcgd", Label: "second", Description: new("desc b"), CreatedAt: "2020-01-02T03:04:05.000Z", UpdatedAt: "2020-01-02T03:04:05.000Z"}
	// Houston's description is nullable, and a workspace made outside the
	// CLI may have none.
	wsBare = houston.Workspace{ID: "ckc0j8y1101xo0760or02jdi7", Label: "third"}
)

// wsRun is one run of the APC tree.
type wsRun struct {
	stdout, stderr string
	// processStdout is what reached os.Stdout other than through the out the
	// tree was built with, in a run whose root binds nothing.
	processStdout string
	code          int
	err           error
	// stdinRead is whether anything read stdin: a refused prompt must not.
	stdinRead bool
}

// wsClient is a Houston mock that answers what building the tree asks.
func wsClient() *mocks.ClientInterface {
	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{}, nil).Maybe()
	api.On("GetPlatformVersion", mock.Anything).Return("1.0.0", nil).Maybe()
	return api
}

// runWorkspaceTree builds the APC tree on a file standing in for stdout and
// runs args, with answers on stdin. bindRoot binds the root's own out and
// err, as a test harness would, and points the process's stdout at the same
// file. Without it the root binds nothing, as in production, and the
// process's stdout is a file of its own (processStdout): a result that
// leaves by anything but the out the tree was built with (cmd.OutOrStdout,
// fmt.Print) lands there instead of in stdout.
func runWorkspaceTree(t *testing.T, api houston.ClientInterface, answers string, bindRoot bool, args ...string) wsRun {
	t.Helper()
	return runTree(t, api, answers, bindRoot, nil, args...)
}

// runWorkspaceTreeWith is runWorkspaceTree on a bound root with nothing on
// stdin, calling before once the test config is in place.
func runWorkspaceTreeWith(t *testing.T, api houston.ClientInterface, before func(), args ...string) wsRun {
	t.Helper()
	return runTree(t, api, "", true, before, args...)
}

func runTree(t *testing.T, api houston.ClientInterface, answers string, bindRoot bool, before func(), args ...string) wsRun {
	t.Helper()
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	if before != nil {
		before()
	}
	dir := t.TempDir()
	stdout, err := os.Create(filepath.Join(dir, "stdout"))
	require.NoError(t, err)
	stderr, err := os.Create(filepath.Join(dir, "stderr"))
	require.NoError(t, err)
	processStdout := stdout
	if !bindRoot {
		processStdout, err = os.Create(filepath.Join(dir, "process-stdout"))
		require.NoError(t, err)
	}
	inR, inW, err := os.Pipe()
	require.NoError(t, err)
	_, err = inW.WriteString(answers)
	require.NoError(t, err)
	require.NoError(t, inW.Close())

	prevOut, prevErr, prevIn := os.Stdout, os.Stderr, os.Stdin
	os.Stdout, os.Stderr, os.Stdin = processStdout, stderr, inR
	t.Cleanup(func() { os.Stdout, os.Stderr, os.Stdin = prevOut, prevErr, prevIn })

	root := &cobra.Command{Use: "astro", SilenceErrors: true}
	LoadPlatform(api) // as the root does for a line that runs one of these commands
	root.AddCommand(AddCmds(api, stdout)...)
	if bindRoot {
		root.SetOut(stdout)
		root.SetErr(stderr)
	}
	ctx := context.Background()
	runErr := cliout.Execute(ctx, root, args, stdout, nil)
	os.Stdout, os.Stderr, os.Stdin = prevOut, prevErr, prevIn

	left, _ := io.ReadAll(inR)
	require.NoError(t, stdout.Close())
	require.NoError(t, stderr.Close())
	outBytes, err := os.ReadFile(stdout.Name())
	require.NoError(t, err)
	errBytes, err := os.ReadFile(stderr.Name())
	require.NoError(t, err)
	var processBytes []byte
	if processStdout != stdout {
		require.NoError(t, processStdout.Close())
		processBytes, err = os.ReadFile(processStdout.Name())
		require.NoError(t, err)
	}
	return wsRun{
		stdout:        string(outBytes),
		processStdout: string(processBytes),
		stderr:        string(errBytes),
		code:          cliout.ExitCode(ctx, runErr),
		err:           runErr,
		stdinRead:     len(left) < len(answers),
	}
}

// decodeWorkspaceOutput decodes stdout as exactly one json value into v.
func decodeWorkspaceOutput(t *testing.T, stdout string, v any) {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(stdout))
	require.NoError(t, dec.Decode(v), "stdout:\n%s", stdout)
	var extra json.RawMessage
	require.ErrorIs(t, dec.Decode(&extra), io.EOF, "more than one value on stdout:\n%s", stdout)
}

// wsErrorObject is the failure cliout.Execute publishes under json.
type wsErrorObject struct {
	Error string  `json:"error"`
	Code  int     `json:"code"`
	Kind  *string `json:"kind"`
}

func currentContextWorkspace(t *testing.T) string {
	t.Helper()
	c, err := config.GetCurrentContext()
	require.NoError(t, err)
	return c.Workspace
}

// Text is what each command printed on v2 before it had -o, byte for byte:
// recorded from v2 and compared on 17 runs when the family was converted.
func TestWorkspaceTextIsUnchanged(t *testing.T) {
	const contextTable = " CLUSTER                             WORKSPACE                           \n" +
		" astronomer_dev.com                  ckbv8pwbq00wk0760us7ktcgd           \n"
	for _, tc := range []struct {
		name  string
		setup func(*mocks.ClientInterface)
		args  []string
		want  string
	}{
		{"list highlights the current workspace", func(a *mocks.ClientInterface) {
			a.On("ListWorkspaces", nil).Return([]houston.Workspace{wsCurrent, wsOther, wsBare}, nil)
		}, []string{"workspace", "list"}, " NAME        ID                            \n" +
			"\x1b[1;32m airflow     ck05r3bor07h40d02y2hw4n4v     \x1b[0m\n" +
			" second      ckbv8pwbq00wk0760us7ktcgd     \n" +
			" third       ckc0j8y1101xo0760or02jdi7     \n"},
		{"an empty list is its header", func(a *mocks.ClientInterface) {
			a.On("ListWorkspaces", nil).Return([]houston.Workspace{}, nil)
		}, []string{"workspace", "ls"}, " NAME     ID     \n"},
		{"create", func(a *mocks.ClientInterface) {
			a.On("CreateWorkspace", houston.CreateWorkspaceRequest{Label: "new", Description: "N/A"}).Return(&houston.Workspace{ID: "cknew", Label: "new", Description: new("N/A")}, nil)
		}, []string{"workspace", "create", "-l", "new"}, " NAME     ID        \n new      cknew     \n\n Successfully created workspace\n"},
		{"update", func(a *mocks.ClientInterface) {
			a.On("UpdateWorkspace", houston.UpdateWorkspaceRequest{WorkspaceID: wsOther.ID, Args: map[string]string{"label": "renamed"}}).
				Return(&houston.Workspace{ID: wsOther.ID, Label: "renamed", Description: new("desc b")}, nil)
		}, []string{"workspace", "update", wsOther.ID, "--label", "renamed"}, " NAME        ID                            \n renamed     ckbv8pwbq00wk0760us7ktcgd     \n\n Successfully updated workspace\n"},
		{"delete", func(a *mocks.ClientInterface) {
			a.On("DeleteWorkspace", wsOther.ID).Return(&wsOther, nil)
		}, []string{"workspace", "delete", wsOther.ID}, "\n Successfully deleted workspace\n"},
		{"switch shows the context it left", func(a *mocks.ClientInterface) {
			a.On("ValidateWorkspaceID", wsOther.ID).Return(&wsOther, nil)
		}, []string{"workspace", "switch", wsOther.ID}, contextTable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := wsClient()
			tc.setup(api)
			got := runWorkspaceTree(t, api, "", true, tc.args...)
			require.Equal(t, 0, got.code, "stderr:\n%s", got.stderr)
			assert.Equal(t, tc.want, got.stdout)
			assert.Empty(t, got.stderr)
			api.AssertExpectations(t)
		})
	}
}

func TestWorkspaceListJSON(t *testing.T) {
	t.Run("every workspace, in Houston's order, the current one marked", func(t *testing.T) {
		api := wsClient()
		api.On("ListWorkspaces", nil).Return([]houston.Workspace{wsOther, wsCurrent, wsBare}, nil)

		got := runWorkspaceTree(t, api, "", true, "workspace", "list", "-o", "json")
		require.Equal(t, 0, got.code, "stdout:\n%s", got.stdout)
		var list workspaceListJSON
		decodeWorkspaceOutput(t, got.stdout, &list)
		require.Len(t, list.Workspaces, 3)
		ids := []string{list.Workspaces[0].ID, list.Workspaces[1].ID, list.Workspaces[2].ID}
		assert.Equal(t, []string{wsOther.ID, currentWS, wsBare.ID}, ids)
		assert.Equal(t, []bool{false, true, false}, []bool{list.Workspaces[0].IsCurrent, list.Workspaces[1].IsCurrent, list.Workspaces[2].IsCurrent})
		assert.Equal(t, "desc b", *list.Workspaces[0].Description)
		assert.Equal(t, "2020-01-02T03:04:05.000Z", *list.Workspaces[0].CreatedAt)
		assert.Empty(t, got.stderr)
	})
	t.Run("what Houston gave no value for is null, not empty", func(t *testing.T) {
		api := wsClient()
		api.On("ListWorkspaces", nil).Return([]houston.Workspace{wsBare}, nil)

		got := runWorkspaceTree(t, api, "", true, "workspace", "list", "-o", "json")
		require.Equal(t, 0, got.code)
		assert.Contains(t, got.stdout, `"description":null`)
		assert.Contains(t, got.stdout, `"created_at":null`)
		assert.Contains(t, got.stdout, `"updated_at":null`)
	})
	t.Run("none is an empty list", func(t *testing.T) {
		api := wsClient()
		api.On("ListWorkspaces", nil).Return([]houston.Workspace{}, nil)

		got := runWorkspaceTree(t, api, "", true, "workspace", "list", "-o", "json")
		require.Equal(t, 0, got.code)
		assert.Equal(t, `{"workspaces":[]}`+"\n", got.stdout)
	})
	t.Run("a failure is the error object, exit 1", func(t *testing.T) {
		api := wsClient()
		api.On("ListWorkspaces", nil).Return(nil, errors.New("boom"))

		got := runWorkspaceTree(t, api, "", true, "workspace", "list", "-o", "json")
		assert.Equal(t, 1, got.code)
		var e wsErrorObject
		decodeWorkspaceOutput(t, got.stdout, &e)
		assert.Equal(t, "boom", e.Error)
		assert.Equal(t, 1, e.Code)
		assert.Empty(t, got.stderr)
	})
}

func TestWorkspaceCreateAndUpdateJSON(t *testing.T) {
	t.Run("create returns the workspace Houston stored", func(t *testing.T) {
		api := wsClient()
		created := houston.Workspace{ID: "cknew", Label: "new", Description: new("d"), CreatedAt: "2026-10-07T00:00:00.000Z", UpdatedAt: "2026-10-07T00:00:00.000Z"}
		api.On("CreateWorkspace", houston.CreateWorkspaceRequest{Label: "new", Description: "d"}).Return(&created, nil)

		got := runWorkspaceTree(t, api, "", true, "workspace", "create", "--label", "new", "--description", "d", "-o", "json")
		require.Equal(t, 0, got.code, "stdout:\n%s", got.stdout)
		var w workspaceJSON
		decodeWorkspaceOutput(t, got.stdout, &w)
		assert.Equal(t, "cknew", w.ID)
		assert.Equal(t, "new", w.Label)
		assert.Equal(t, "d", *w.Description)
		assert.Equal(t, created.CreatedAt, *w.CreatedAt)
		assert.False(t, w.IsCurrent, "creating does not switch")
		assert.Empty(t, got.stderr)
	})
	t.Run("update returns the workspace as it now is", func(t *testing.T) {
		api := wsClient()
		api.On("UpdateWorkspace", houston.UpdateWorkspaceRequest{WorkspaceID: currentWS, Args: map[string]string{"description": "new words"}}).
			Return(&houston.Workspace{ID: currentWS, Label: "airflow", Description: new("new words")}, nil)

		got := runWorkspaceTree(t, api, "", true, "workspace", "update", currentWS, "-d", "new words", "-o", "json")
		require.Equal(t, 0, got.code, "stdout:\n%s", got.stdout)
		var w workspaceJSON
		decodeWorkspaceOutput(t, got.stdout, &w)
		assert.Equal(t, currentWS, w.ID)
		assert.Equal(t, "new words", *w.Description)
		assert.True(t, w.IsCurrent)
	})
	t.Run("update with nothing to change is refused before Houston is asked", func(t *testing.T) {
		api := wsClient()
		got := runWorkspaceTree(t, api, "", true, "workspace", "update", currentWS, "-o", "json")
		assert.Equal(t, 1, got.code)
		var e wsErrorObject
		decodeWorkspaceOutput(t, got.stdout, &e)
		assert.Equal(t, errUpdateWorkspaceInvalidArgs.Error(), e.Error)
		api.AssertNotCalled(t, "UpdateWorkspace", mock.Anything)
	})
}

func TestWorkspaceDeleteJSON(t *testing.T) {
	t.Run("what it removed", func(t *testing.T) {
		api := wsClient()
		// Houston answers with the record it removed: id, label and description.
		api.On("DeleteWorkspace", wsOther.ID).Return(&houston.Workspace{ID: wsOther.ID, Label: wsOther.Label, Description: wsOther.Description}, nil)

		got := runWorkspaceTree(t, api, "", true, "workspace", "delete", wsOther.ID, "-o", "json")
		require.Equal(t, 0, got.code, "stdout:\n%s", got.stdout)
		var r workspaceRemovalJSON
		decodeWorkspaceOutput(t, got.stdout, &r)
		assert.Equal(t, wsOther.ID, r.WorkspaceID)
		assert.Equal(t, "second", *r.Label)
		assert.Equal(t, "deleted", r.Action)
		assert.Empty(t, got.stderr)
	})
	t.Run("a workspace with Deployments is refused, exit 1", func(t *testing.T) {
		api := wsClient()
		// Houston refuses rather than deleting.
		api.On("DeleteWorkspace", wsOther.ID).Return(nil, errors.New("You must first deprovision all deployments before you can delete your workspace."))

		got := runWorkspaceTree(t, api, "", true, "workspace", "delete", wsOther.ID, "-o", "json")
		assert.Equal(t, 1, got.code)
		var e wsErrorObject
		decodeWorkspaceOutput(t, got.stdout, &e)
		assert.Contains(t, e.Error, "deprovision all deployments")
	})
}

func TestWorkspaceSwitchJSON(t *testing.T) {
	t.Run("by ID: the workspace now current", func(t *testing.T) {
		api := wsClient()
		api.On("ValidateWorkspaceID", wsOther.ID).Return(&wsOther, nil)

		got := runWorkspaceTree(t, api, "", true, "workspace", "switch", wsOther.ID, "-o", "json")
		require.Equal(t, 0, got.code, "stdout:\n%s", got.stdout)
		var w workspaceJSON
		decodeWorkspaceOutput(t, got.stdout, &w)
		assert.Equal(t, wsOther.ID, w.ID)
		assert.Equal(t, "second", w.Label)
		assert.True(t, w.IsCurrent)
		assert.Equal(t, wsOther.ID, currentContextWorkspace(t))
		assert.Empty(t, got.stderr)
	})
	t.Run("an ID Houston does not know is a failure, and switches nothing", func(t *testing.T) {
		api := wsClient()
		api.On("ValidateWorkspaceID", "nope").Return(nil, houston.ErrWorkspaceNotFound{})

		got := runWorkspaceTree(t, api, "", true, "workspace", "switch", "nope", "-o", "json")
		assert.Equal(t, 1, got.code)
		var e wsErrorObject
		decodeWorkspaceOutput(t, got.stdout, &e)
		assert.Contains(t, e.Error, "workspace id is not valid")
		assert.Equal(t, currentWS, currentContextWorkspace(t))
	})
	// Without an ID the switch asks, paged or not; under json it refuses,
	// naming the argument that answers it, before it fetches anything.
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"unpaged", []string{"workspace", "switch", "-o", "json"}},
		{"paged", []string{"workspace", "switch", "--paginated", "--page-size", "2", "-o", "json"}},
	} {
		t.Run("without an ID, "+tc.name+", it refuses", func(t *testing.T) {
			api := wsClient()
			got := runWorkspaceTree(t, api, "1\n", true, tc.args...)
			assert.Equal(t, 1, got.code)
			var e wsErrorObject
			decodeWorkspaceOutput(t, got.stdout, &e)
			require.NotNil(t, e.Kind)
			assert.Equal(t, "input_required", *e.Kind)
			assert.Contains(t, e.Error, "the workspace ID as an argument")
			assert.False(t, got.stdinRead, "a refused question reads nothing")
			assert.Empty(t, got.stderr, "nor draws the table")
			api.AssertNotCalled(t, "ListWorkspaces", mock.Anything)
			api.AssertNotCalled(t, "PaginatedListWorkspaces", mock.Anything)
			assert.Equal(t, currentWS, currentContextWorkspace(t))
		})
	}
	t.Run("quitting the paged picker in text switches nothing, exit 0", func(t *testing.T) {
		api := wsClient()
		api.On("PaginatedListWorkspaces", houston.PaginatedListWorkspaceRequest{PageSize: 2, PageNumber: 0}).Return([]houston.Workspace{wsCurrent, wsOther}, nil)

		got := runWorkspaceTree(t, api, "q\n", true, "workspace", "switch", "-p", "-s", "2")
		assert.Equal(t, 0, got.code)
		assert.Empty(t, got.stdout)
		assert.Contains(t, got.stderr, "n. next q. quit", "the picker is drawn on stderr")
		assert.Equal(t, currentWS, currentContextWorkspace(t))
	})
}

// Under --output json the result is on stdout and nothing of it on stderr,
// in the tree production builds, whose root binds no writer: each command
// renders to the out the tree was built with.
func TestWorkspaceJSONReachesStdoutUnderAnUnboundRoot(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*mocks.ClientInterface)
		args  []string
	}{
		{"list", func(a *mocks.ClientInterface) {
			a.On("ListWorkspaces", nil).Return([]houston.Workspace{wsOther}, nil)
		}, []string{"workspace", "list", "-o", "json"}},
		{"create", func(a *mocks.ClientInterface) {
			a.On("CreateWorkspace", mock.Anything).Return(&wsOther, nil)
		}, []string{"workspace", "create", "-l", "second", "-o", "json"}},
		{"update", func(a *mocks.ClientInterface) {
			a.On("UpdateWorkspace", mock.Anything).Return(&wsOther, nil)
		}, []string{"workspace", "update", wsOther.ID, "-l", "second", "-o", "json"}},
		{"delete", func(a *mocks.ClientInterface) {
			a.On("DeleteWorkspace", wsOther.ID).Return(&wsOther, nil)
		}, []string{"workspace", "delete", wsOther.ID, "-o", "json"}},
		{"switch", func(a *mocks.ClientInterface) {
			a.On("ValidateWorkspaceID", wsOther.ID).Return(&wsOther, nil)
		}, []string{"workspace", "switch", wsOther.ID, "-o", "json"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := wsClient()
			tc.setup(api)
			got := runWorkspaceTree(t, api, "", false, tc.args...)
			require.Equal(t, 0, got.code, "stderr:\n%s", got.stderr)
			var v map[string]any
			decodeWorkspaceOutput(t, got.stdout, &v)
			assert.Contains(t, got.stdout, wsOther.ID)
			assert.Empty(t, got.processStdout, "the result left by the process's stdout, not the tree's out")
			assert.Empty(t, got.stderr)
		})
	}
}

// An unreadable current context fails the list, text or json, as it did on
// v2: the list marks the current workspace, and has nothing to read it from.
func TestWorkspaceListFailsWithoutAContext(t *testing.T) {
	for _, args := range [][]string{{"workspace", "list"}, {"workspace", "list", "-o", "json"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			api := wsClient()
			api.On("ListWorkspaces", nil).Return([]houston.Workspace{wsOther}, nil)
			got := runWorkspaceTreeWith(t, api, func() { require.NoError(t, config.ResetCurrentContext()) }, args...)
			assert.Equal(t, 1, got.code)
			assert.NotContains(t, got.stdout, wsOther.ID, "no list without a context")
		})
	}
}

// A description Houston holds as "" is "", and one it does not have is null:
// the two are not told apart by the null rule's empty-is-absent.
func TestWorkspaceEmptyDescriptionIsNotNull(t *testing.T) {
	api := wsClient()
	api.On("ListWorkspaces", nil).Return([]houston.Workspace{
		{ID: "a", Label: "empty", Description: new("")},
		{ID: "b", Label: "none"},
	}, nil)
	got := runWorkspaceTree(t, api, "", true, "workspace", "list", "-o", "json")
	require.Equal(t, 0, got.code, got.stdout)
	var list workspaceListJSON
	decodeWorkspaceOutput(t, got.stdout, &list)
	require.Len(t, list.Workspaces, 2)
	require.NotNil(t, list.Workspaces[0].Description)
	assert.Empty(t, *list.Workspaces[0].Description)
	assert.Nil(t, list.Workspaces[1].Description)
}

// A page size past the cap is noted on stderr in text, before the picker; a
// run under json that will refuse to pick prints the refusal and nothing
// else.
func TestWorkspaceSwitchPageSizeNote(t *testing.T) {
	const note = "Page size cannot be more than 100"
	// The note goes through the logger, which the root points at stderr;
	// here it writes to logs.
	logs := new(bytes.Buffer)
	logger.SetOutput(logs)
	level := logger.GetLevel()
	logger.SetLevel(logrus.WarnLevel)
	t.Cleanup(func() {
		logger.SetOutput(os.Stderr)
		logger.SetLevel(level)
	})
	t.Run("text: noted", func(t *testing.T) {
		logs.Reset()
		api := wsClient()
		api.On("PaginatedListWorkspaces", houston.PaginatedListWorkspaceRequest{PageSize: 100, PageNumber: 0}).Return([]houston.Workspace{wsCurrent}, nil)
		got := runWorkspaceTree(t, api, "q\n", true, "workspace", "switch", "-p", "-s", "500")
		assert.Equal(t, 0, got.code)
		assert.Contains(t, logs.String(), note)
	})
	t.Run("json: only the refusal", func(t *testing.T) {
		logs.Reset()
		api := wsClient()
		got := runWorkspaceTree(t, api, "", true, "workspace", "switch", "-p", "-s", "500", "-o", "json")
		assert.Equal(t, 1, got.code)
		assert.Contains(t, got.stdout, "input_required")
		assert.Empty(t, got.stderr)
		assert.Empty(t, logs.String(), "no note before the refusal")
	})
}
