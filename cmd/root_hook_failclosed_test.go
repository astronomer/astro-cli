package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	astroCmd "github.com/astronomer/astro-cli/cmd/astro"
	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/astro/auth"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/telemetry"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/logger"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/astronomer/astro-cli/pkg/util"
)

// recordingAPI is a fake Astro API that answers 401 and remembers the
// Authorization header of every request it was sent.
type recordingAPI struct {
	*httptest.Server
	mu    sync.Mutex
	auths []string
}

func newRecordingAPI(t *testing.T) *recordingAPI {
	t.Helper()
	api := &recordingAPI{}
	api.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api.mu.Lock()
		api.auths = append(api.auths, r.Header.Get("Authorization"))
		api.mu.Unlock()
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(api.Close)
	return api
}

func (a *recordingAPI) requests() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.auths...)
}

// rootRun is one command line run through a real root, hooks and all.
type rootRun struct {
	stdout, stderr string
	err            error
	code           int
}

// runRoot runs args through a real cloud root under the output contract, with
// the process's stdout and stderr captured.
func runRoot(t *testing.T, args ...string) rootRun {
	t.Helper()
	level := logger.GetLevel()
	t.Cleanup(func() { logger.SetLevel(level) })
	var run rootRun
	run.stdout, run.stderr = captureProcessOutput(t, func(out io.Writer) {
		root := newRootCmd(&rootOptions{
			platform:      cloudPlatform,
			loggedIn:      true,
			houstonClient: stubHoustonAt(t, "1.0.0"),
			out:           out,
		})
		ctx := t.Context()
		run.err = cliout.Execute(ctx, root, args, out, problemKinds)
		run.code = cliout.ExitCode(ctx, run.err)
	})
	return run
}

// oneErrorDocument decodes stdout as exactly one JSON error object.
func oneErrorDocument(t *testing.T, stdout string) (doc struct {
	Error string `json:"error"`
	Code  int    `json:"code"`
	Kind  string `json:"kind"`
},
) {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(stdout))
	require.NoError(t, dec.Decode(&doc), "stdout: %q", stdout)
	assert.False(t, dec.More(), "more than one json value on stdout: %q", stdout)
	return doc
}

// savedLoginToken is the access token of the login these tests save, which no
// request may carry when the run is not that login's.
const savedLoginToken = "Bearer saved-login-access-token"

// useLocalAPI points the cloud root at api, logged in as a saved login whose
// access token is saved and good for an hour, with a saved Workspace.
func useLocalAPI(t *testing.T, api *recordingAPI) {
	t.Helper()
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Cleanup(func() { testUtil.InitTestConfig(testUtil.CloudPlatform) })
	require.NoError(t, config.CFG.AstroLocalCoreURL.SetHomeString(api.URL))
	require.NoError(t, config.CFG.UpgradeMessage.SetHomeString("false"))
	t.Setenv("ASTRO_TELEMETRY_DISABLED", "1")
	t.Setenv("ASTRO_API_TOKEN", "")
	c, err := config.GetCurrentContext()
	require.NoError(t, err)
	require.NoError(t, c.SetContextKey("token", savedLoginToken))
	require.NoError(t, c.SetContextKey("workspace", "saved-workspace"))
	require.NoError(t, c.SetContextKey("organization", "saved-org"))
	require.NoError(t, c.SetExpiresIn(int64(time.Hour/time.Second)))
}

func mintJWT(t *testing.T, permissions ...string) string {
	t.Helper()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, util.CustomClaims{
		Permissions:      permissions,
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
	}).SignedString([]byte("k"))
	require.NoError(t, err)
	return token
}

// A token with no permissions in ASTRO_API_TOKEN is the credential the script
// supplied, and the one that failed: the run does not become the saved login.
// With a saved login present, neither command makes a request with its
// credentials, and `astro auth token` does not print it.
func TestNoPermissionsTokenNeverFallsBackToTheSavedLogin(t *testing.T) {
	for _, args := range [][]string{
		{"auth", "token", "-o", "json"},
		{"workspace", "list", "-o", "json"},
		{"deployment", "list", "-o", "json"},
	} {
		t.Run(strings.Join(args[:2], " "), func(t *testing.T) {
			api := newRecordingAPI(t)
			useLocalAPI(t, api)
			t.Setenv("ASTRO_API_TOKEN", mintJWT(t))

			run := runRoot(t, args...)

			require.Error(t, run.err)
			require.ErrorIs(t, run.err, astroCmd.ErrNotAPIToken)
			assert.Equal(t, 1, run.code)
			doc := oneErrorDocument(t, run.stdout)
			assert.Equal(t, string(KindUnauthenticated), doc.Kind)
			assert.Equal(t, 1, doc.Code)
			assert.Equal(t, "ASTRO_API_TOKEN is not an Astro API token (it has no permissions): set a valid token, or unset it to use your login", doc.Error)
			assert.NotContains(t, run.stdout, "saved-login-access-token", "the saved login's token must not be printed")
			assert.Empty(t, api.requests(), "no request may carry the saved login's credentials")
			assert.Empty(t, run.stderr)
		})
	}
}

// The control for the test above: without the variable, the same commands run
// as the saved login, and the fake API sees its credentials. It is what makes
// "no request" in that test mean something.
func TestSavedLoginIsUsedWhenNoTokenIsGiven(t *testing.T) {
	api := newRecordingAPI(t)
	useLocalAPI(t, api)
	run := runRoot(t, "workspace", "list", "-o", "json")
	require.Error(t, run.err, "the fake API answers 401")
	assert.Contains(t, api.requests(), savedLoginToken)
}

// Each hard failure of Astro's setup stops the command with its message,
// its kind and its exit status, and under --output json writes one document on
// stdout and nothing on stderr.
func TestSetupFailuresAreOneJSONDocument(t *testing.T) {
	origSetup := astroSetup
	t.Cleanup(func() { astroSetup = origSetup })
	transport := &url.Error{Op: "Get", URL: "http://api.invalid/", Err: &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}}

	cases := []struct {
		name string
		err  error
		kind cliout.ProblemKind
		code int
		text string
	}{
		{"a token with no permissions", astroCmd.ErrNotAPIToken, KindUnauthenticated, 1, "has no permissions"},
		{"an expired token", astroCmd.ErrAPITokenExpired, KindUnauthenticated, 1, "API Token is expired"},
		{"a malformed token", astroCmd.ErrAPITokenMalformed, KindUnauthenticated, 1, "API Token is invalid or malformed"},
		{"a login the run may not start", fmt.Errorf("%w; stdin is not a terminal", auth.ErrLoginNeeded), KindUnauthenticated, 1, "this command needs you to log in"},
		{"a login that expired and could not be renewed", fmt.Errorf("%w, run 'astro login' to log in again: refused", astroCmd.ErrLoginRenewal), KindUnauthenticated, 1, "could not renew your Astro login"},
		{"no context", config.ErrGetHomeString, KindUnauthenticated, 1, "no context set"},
		{"a config file that did not parse", fmt.Errorf("%w: /h/config.yaml did not parse", astroCmd.ErrConfigUnreadable), KindUnauthenticated, 1, "/h/config.yaml did not parse"},
		{"an organization lookup that never got an answer", fmt.Errorf("%w: %w", astroCmd.ErrAPITokenOrganization, transport), KindAPIUnavailable, 1, "cannot look up the Organization"},
		{"an interrupt", input.ErrInterrupted, "", 130, "interrupted"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := newRecordingAPI(t)
			useLocalAPI(t, api)
			astroSetup = func(*cobra.Command, astrov1.APIClient) error { return tc.err }

			run := runRoot(t, "workspace", "list", "-o", "json")

			require.ErrorIs(t, run.err, tc.err)
			assert.Equal(t, tc.code, run.code)
			doc := oneErrorDocument(t, run.stdout)
			assert.Equal(t, string(tc.kind), doc.Kind)
			assert.Equal(t, tc.code, doc.Code)
			assert.Contains(t, doc.Error, tc.text)
			assert.Empty(t, run.stderr, "stderr carries only warnings")
			assert.Empty(t, api.requests(), "the command must not have run")
		})
	}
}

// A real corrupt home config: the run fails naming the file, as one document.
// The config's own startup warning ("Error reading config") is printed while it
// loads, before any command runs, and is not part of the run.
func TestCorruptConfigFailsTheCommandAsOneDocument(t *testing.T) {
	t.Setenv("ASTRO_API_TOKEN", "")
	fs := afero.NewMemMapFs()
	require.NoError(t, afero.WriteFile(fs, config.HomeConfigFile, []byte("context: [unterminated\n  : :"), 0o600))
	_, _ = captureProcessOutput(t, func(io.Writer) { config.InitConfig(fs) })
	t.Cleanup(func() { testUtil.InitTestConfig(testUtil.CloudPlatform) })

	// `auth token` reads the login itself (it renews the one it prints), and
	// gives the same answer as the commands that go through the root's setup.
	for _, args := range [][]string{{"deployment", "list", "-o", "json"}, {"auth", "token", "-o", "json"}} {
		t.Run(strings.Join(args[:2], " "), func(t *testing.T) {
			run := runRoot(t, args...)

			require.ErrorIs(t, run.err, astroCmd.ErrConfigUnreadable)
			doc := oneErrorDocument(t, run.stdout)
			assert.Equal(t, string(KindUnauthenticated), doc.Kind)
			assert.Contains(t, doc.Error, config.HomeConfigFile)
			assert.Contains(t, doc.Error, "did not parse")
			assert.Empty(t, run.stderr)
		})
	}
}

// Shell completion runs on every TAB, so it never logs in, refreshes a token,
// calls an API or asks anything, whether or not the person is logged in or
// online: it answers from the command tree alone.
func TestCompletionNeverTouchesTheLoginOrTheNetwork(t *testing.T) {
	origSetup := astroSetup
	t.Cleanup(func() { astroSetup = origSetup })

	for _, request := range []string{cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd} {
		for _, state := range []string{"logged out", "a garbage token", "an expired login", "offline"} {
			t.Run(request+" "+state, func(t *testing.T) {
				api := newRecordingAPI(t)
				useLocalAPI(t, api)
				switch state {
				case "logged out":
					c, err := config.GetCurrentContext()
					require.NoError(t, err)
					require.NoError(t, c.SetContextKey("token", ""))
				case "a garbage token":
					t.Setenv("ASTRO_API_TOKEN", "garbage")
				case "an expired login":
					c, err := config.GetCurrentContext()
					require.NoError(t, err)
					require.NoError(t, c.SetExpiresIn(-3600))
					require.NoError(t, c.SetContextKey("refreshtoken", "saved-refresh"))
				case "offline":
					api.Close()
				}
				var setups int
				astroSetup = func(*cobra.Command, astrov1.APIClient) error {
					setups++
					return errors.New("setup ran for a completion")
				}

				run := runRoot(t, request, "workspace", "")

				require.NoError(t, run.err)
				assert.Zero(t, setups, "completion must not run Astro's setup")
				assert.Contains(t, run.stdout, "list", "the subcommands of workspace are offered")
				// Cobra's own note of the directive it ended with is the one line
				// stderr may hold: no prompt, no login, no log.
				for _, line := range strings.Split(strings.TrimSpace(run.stderr), "\n") {
					assert.True(t, line == "" || strings.HasPrefix(line, "Completion ended with directive:"), "stderr: %q", run.stderr)
				}
				assert.Empty(t, api.requests())
			})
		}
	}
}

// Completion skipping setup is not skipping the rest: an ordinary command in
// the same states still goes through it.
func TestOrdinaryCommandsStillRunSetup(t *testing.T) {
	origSetup := astroSetup
	t.Cleanup(func() { astroSetup = origSetup })
	api := newRecordingAPI(t)
	useLocalAPI(t, api)
	var setups int
	astroSetup = func(*cobra.Command, astrov1.APIClient) error {
		setups++
		return errors.New("stop")
	}
	run := runRoot(t, "workspace", "list", "-o", "json")
	require.Error(t, run.err)
	assert.Equal(t, 1, setups)
}

// A command the root's setup stopped is still recorded: one event, of its own
// kind, carrying the command and the kind of the failure, sent anonymously, and
// nothing that names the credential, the domain or the message. With telemetry
// off nothing is sent.
func TestSetupFailureIsTrackedWithItsKind(t *testing.T) {
	telemetryOn := func(t *testing.T, api *recordingAPI) func() []telemetry.SentEvent {
		useLocalAPI(t, api)
		t.Setenv("ASTRO_TELEMETRY_DISABLED", "")
		require.NoError(t, config.CFG.TelemetryEnabled.SetHomeString("true"))
		require.NoError(t, config.CFG.TelemetryNoticeShown.SetHomeString("3"))
		sent, restore := telemetry.CaptureEventsForTesting()
		t.Cleanup(restore)
		return sent
	}

	t.Run("a token with no permissions", func(t *testing.T) {
		api := newRecordingAPI(t)
		sent := telemetryOn(t, api)
		token := mintJWT(t)
		t.Setenv("ASTRO_API_TOKEN", token)

		run := runRoot(t, "workspace", "list", "-o", "json")

		require.ErrorIs(t, run.err, astroCmd.ErrNotAPIToken)
		events := sent()
		require.Len(t, events, 1, "one event, and not the command's own")
		ev := events[0]
		assert.Equal(t, telemetry.EventCommandSetupFailed, ev.Event)
		assert.Equal(t, "workspace list", ev.Properties["command"])
		assert.Equal(t, string(KindUnauthenticated), ev.Properties["error_kind"])
		assert.Empty(t, ev.Token, "the credential that failed is not sent with the event")
		body, err := json.Marshal(ev)
		require.NoError(t, err)
		for _, secret := range []string{token, "ASTRO_API_TOKEN", "no permissions", savedLoginToken, "saved-workspace", "saved-org", "localhost", api.URL} {
			assert.NotContains(t, string(body), secret)
		}
		assert.Empty(t, api.requests())
	})

	t.Run("the run's own event is sent when setup passes", func(t *testing.T) {
		api := newRecordingAPI(t)
		sent := telemetryOn(t, api)
		runRoot(t, "workspace", "list", "-o", "json") // the fake API answers 401
		events := sent()
		require.Len(t, events, 1)
		assert.Equal(t, telemetry.EventCommandExecution, events[0].Event)
		assert.NotContains(t, events[0].Properties, "error_kind")
	})

	t.Run("telemetry disabled sends nothing", func(t *testing.T) {
		api := newRecordingAPI(t)
		sent := telemetryOn(t, api)
		t.Setenv("ASTRO_TELEMETRY_DISABLED", "1")
		t.Setenv("ASTRO_API_TOKEN", mintJWT(t))
		run := runRoot(t, "workspace", "list", "-o", "json")
		require.ErrorIs(t, run.err, astroCmd.ErrNotAPIToken)
		assert.Empty(t, sent())

		t.Setenv("ASTRO_TELEMETRY_DISABLED", "")
		require.NoError(t, config.CFG.TelemetryEnabled.SetHomeString("false"))
		runRoot(t, "workspace", "list", "-o", "json")
		assert.Empty(t, sent())
	})
}
