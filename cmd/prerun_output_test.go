package cmd

import (
	"bytes"
	stdcontext "context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v4"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/logger"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/astronomer/astro-cli/pkg/util"
)

// The root's pre-run (logging, the login check, the version check) runs ahead
// of every command, so what it prints lands before the command's own output.
// Under --output json stdout must be the one object and nothing else, so
// the pre-run's notes, its login and its logs go to stderr. These run a real
// root, hooks and all.
func TestPreRunLeavesJSONStdoutAlone(t *testing.T) {
	apiToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, util.CustomClaims{
		Permissions: []string{"workspaceId:ws", "organizationId:org"},
	}).SignedString([]byte("k"))
	require.NoError(t, err)

	cases := []struct {
		name string
		// apiToken puts an ASTRO_API_TOKEN in the environment; without it
		// the context has no login at all.
		apiToken bool
		args     []string
		// ok is whether the command succeeds; each publishes one object
		// either way.
		ok bool
		// stderrHas is a note the pre-run makes, on stderr.
		stderrHas string
		// stderrLacks is one it must not make: no login check ran.
		stderrLacks string
	}{
		// The case that printed "Using an Astro API Token" on stdout ahead
		// of the object: a command the login check does not exempt, which
		// is every cloud command that calls the API.
		{name: "an API token, a command that logs in", apiToken: true, args: []string{"probe", "-o", "json"}, stderrHas: "Using an Astro API Token"},
		// With debug logs on, the pre-run logs the login check's failure.
		{name: "an API token, with debug logs", apiToken: true, args: []string{"probe", "-o", "json", "--verbosity", "debug"}, stderrHas: "level=debug"},
		{name: "an API token, auth token", apiToken: true, args: []string{"auth", "token", "-o", "json"}, ok: true},
		{name: "an API token, telemetry", apiToken: true, args: []string{"telemetry", "-o", "json"}, ok: true, stderrLacks: "Using an Astro API Token"},
		// With no login, a command that logs in reaches the login flow:
		// cmd/astro's TestSetupHandsTheLoginFlowStderr and the auth
		// package's "a browser login asks on stderr and leaves stdout empty"
		// hold that flow to stderr, with its browser stubbed. The commands on this machine's own state need no login, so they
		// run instead of starting one.
		{name: "no login, telemetry", args: []string{"telemetry", "-o", "json"}, ok: true},
		{name: "no login, config get", args: []string{"config", "get", "page_size", "-g", "-o", "json"}, ok: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			t.Cleanup(func() { testUtil.InitTestConfig(testUtil.CloudPlatform) })
			level := logger.GetLevel()
			t.Cleanup(func() { logger.SetLevel(level) })
			// The API, and the login's auth config, answer 401: what the
			// pre-run says on the way is the point, not that it succeeds.
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			}))
			t.Cleanup(api.Close)
			require.NoError(t, config.CFG.LocalCore.SetHomeString(api.URL))
			require.NoError(t, config.CFG.UpgradeMessage.SetHomeString("false"))
			t.Setenv("ASTRO_TELEMETRY_DISABLED", "1")
			t.Setenv("ASTRO_API_TOKEN", "")
			if tc.apiToken {
				t.Setenv("ASTRO_API_TOKEN", apiToken)
			} else {
				c, err := config.GetCurrentContext()
				require.NoError(t, err)
				require.NoError(t, c.SetContextKey("token", ""))
			}

			var runErr error
			stdout, stderr := captureProcessOutput(t, func(out io.Writer) {
				root := newRootCmd(&rootOptions{
					platform:      cloudPlatform,
					loggedIn:      true,
					houstonClient: stubHoustonAt(t, "1.0.0"),
					out:           out,
				})
				var output cliout.Format
				probe := &cobra.Command{Use: "probe", RunE: func(cmd *cobra.Command, _ []string) error {
					return cliout.Renderer{Format: output, Out: out}.Emit(versionOutput{Version: "probe"}, nil)
				}}
				cliout.AddOutputFlag(probe, &output)
				root.AddCommand(probe)
				runErr = cliout.Execute(stdcontext.Background(), root, tc.args, out, problemKinds)
			})
			dec := json.NewDecoder(strings.NewReader(stdout))
			var v map[string]any
			require.NoError(t, dec.Decode(&v), "stdout: %q", stdout)
			assert.False(t, dec.More(), "more than one json value on stdout: %q", stdout)
			if tc.ok {
				assert.NoError(t, runErr, "stdout: %q\nstderr: %q", stdout, stderr)
			}
			if tc.stderrHas != "" {
				assert.Contains(t, stderr, tc.stderrHas)
			}
			if tc.stderrLacks != "" {
				assert.NotContains(t, stderr, tc.stderrLacks)
			}
		})
	}
}

// captureProcessOutput runs fn with the process's stdout and stderr each
// replaced by a pipe, handing fn the stdout pipe for the writers it builds,
// and returns what reached each: the pre-run writes to os.Stdout and
// os.Stderr directly, which no cobra writer catches.
func captureProcessOutput(t *testing.T, fn func(out io.Writer)) (stdout, stderr string) {
	t.Helper()
	read := func(f *os.File, into *bytes.Buffer, done chan<- struct{}) {
		_, _ = io.Copy(into, f) // a pipe closed under it ends the copy
		close(done)
	}
	outR, outW, err := os.Pipe()
	require.NoError(t, err)
	errR, errW, err := os.Pipe()
	require.NoError(t, err)
	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	var outBuf, errBuf bytes.Buffer
	outDone, errDone := make(chan struct{}), make(chan struct{})
	go read(outR, &outBuf, outDone)
	go read(errR, &errBuf, errDone)
	defer func() {
		os.Stdout, os.Stderr = origOut, origErr
		// The root's logging setup points the logger at the stderr it
		// found, which is the pipe; leave it on the real one.
		logger.SetOutput(origErr)
	}()
	fn(outW)
	os.Stdout, os.Stderr = origOut, origErr
	outW.Close()
	errW.Close()
	<-outDone
	<-errDone
	return outBuf.String(), errBuf.String()
}
