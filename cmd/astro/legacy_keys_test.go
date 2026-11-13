package astro

import (
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/astro/auth"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

const legacyKeysMessage = "ASTRONOMER_KEY_ID and ASTRONOMER_KEY_SECRET (Deployment API keys) are no longer supported: set ASTRO_API_TOKEN to a Deployment API token instead"

// noNetwork makes any request, auth config fetch or login flow a test failure,
// and returns the Astro API client mock that must see no call.
func noNetwork(t *testing.T) *astrov1_mocks.ClientWithResponsesInterface {
	t.Helper()
	prevFetch, prevClient, prevLogin := fetchDomainAuthConfig, client, authLogin
	t.Cleanup(func() { fetchDomainAuthConfig, client, authLogin = prevFetch, prevClient, prevLogin })
	fetchDomainAuthConfig = func(string) (auth.Config, error) {
		t.Error("the auth config was fetched")
		return auth.Config{}, nil
	}
	client = testUtil.NewTestClient(func(r *http.Request) *http.Response {
		t.Errorf("a request went out: %s %s", r.Method, r.URL)
		return &http.Response{StatusCode: http.StatusInternalServerError, Body: http.NoBody, Header: make(http.Header)}
	})
	noLoginFlow(t)
	return new(astrov1_mocks.ClientWithResponsesInterface)
}

func cloudLeaf() *cobra.Command {
	leaf := &cobra.Command{Use: "list", Run: func(*cobra.Command, []string) {}}
	root := &cobra.Command{Use: topLvlCmd}
	group := &cobra.Command{Use: "workspace"}
	group.AddCommand(leaf)
	root.AddCommand(group)
	return leaf
}

// Deployment API keys stopped working in 2024. Both variables set, with no
// API token, refuse before any request or prompt, in both entry points a
// command's login check goes through.
func TestDeploymentAPIKeysAreRefusedBeforeAnyRequest(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"both": {"ASTRONOMER_KEY_ID": "id", "ASTRONOMER_KEY_SECRET": "secret"},
	} {
		for entry, run := range map[string]func(*astrov1_mocks.ClientWithResponsesInterface) error{
			"Setup":       func(mc *astrov1_mocks.ClientWithResponsesInterface) error { return Setup(cloudLeaf(), mc) },
			"EnsureLogin": func(mc *astrov1_mocks.ClientWithResponsesInterface) error { return EnsureLogin(mc) },
		} {
			t.Run(name+" "+entry, func(t *testing.T) {
				testUtil.InitTestConfig(testUtil.CloudPlatform)
				t.Setenv("ASTRO_API_TOKEN", "")
				t.Setenv("ASTRONOMER_KEY_ID", "")
				t.Setenv("ASTRONOMER_KEY_SECRET", "")
				for k, v := range env {
					t.Setenv(k, v)
				}
				mc := noNetwork(t)

				err := run(mc)

				require.ErrorIs(t, err, ErrLegacyAPIKeys)
				assert.Equal(t, legacyKeysMessage, err.Error())
				assert.Empty(t, mc.Calls, "the Astro API was called")
			})
		}
	}
}

// An API token wins over the keys, as it always did: the keys are not read,
// not exchanged and not mentioned, and the token is the run's login.
func TestAnAPITokenWinsOverDeploymentAPIKeys(t *testing.T) {
	home := savedLogin(t, false)
	realTokenParser(t)
	noLoginFlow(t)
	calls := fakeIDP(t)
	token := apiToken(t, "token-and-keys")
	t.Setenv("ASTRO_API_TOKEN", token)
	t.Setenv("ASTRONOMER_KEY_ID", "id")
	t.Setenv("ASTRONOMER_KEY_SECRET", "secret")
	before := snapshot(t, home)
	stderr := captureKeyWarnings(t)

	require.NoError(t, EnsureLogin(orgsClient(nil)))
	require.NoError(t, EnsureLogin(orgsClient(nil)))

	assertUsesEnvironmentToken(t, "Bearer "+token, calls)
	assert.Empty(t, changedFiles(before, snapshot(t, home)))
	assert.Equal(t, "Warning: ASTRONOMER_KEY_ID and ASTRONOMER_KEY_SECRET are set but no longer used (Deployment API keys are not supported); unset them\n", stderr())
}

// A lone variable was ignored by 1.x, and is here: the run uses the saved
// login and warns once, on stderr, naming the variable.
func TestALoneDeploymentAPIKeyVariableIsIgnoredWithAWarning(t *testing.T) {
	for _, name := range []string{"ASTRONOMER_KEY_ID", "ASTRONOMER_KEY_SECRET"} {
		t.Run(name, func(t *testing.T) {
			savedLogin(t, false)
			noNetworkLogin := noNetwork(t)
			t.Setenv(name, "value")
			stderr := captureKeyWarnings(t)

			require.NoError(t, EnsureLogin(noNetworkLogin))
			require.NoError(t, EnsureLogin(noNetworkLogin))

			assert.Equal(t, "Warning: "+name+" is set but no longer used (Deployment API keys are not supported); unset it\n", stderr())
			assert.Empty(t, noNetworkLogin.Calls)
			c, err := config.GetCurrentContext()
			require.NoError(t, err)
			assert.Equal(t, savedToken, c.Token, "the saved login is the run's login")
		})
	}
}

// captureKeyWarnings redirects os.Stderr to a pipe for the test and returns what
// was written to it. It also lets the once-per-run warning fire again.
func captureKeyWarnings(t *testing.T) func() string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	prev := os.Stderr
	os.Stderr = w
	legacyKeysWarned.Store(false)
	var once sync.Once
	var out string
	read := func() string {
		once.Do(func() {
			os.Stderr = prev
			require.NoError(t, w.Close())
			b, err := io.ReadAll(r)
			require.NoError(t, err)
			require.NoError(t, r.Close())
			out = string(b)
		})
		return out
	}
	t.Cleanup(func() {
		read()
		legacyKeysWarned.Store(false)
	})
	return read
}

// A command that needs no login never reads the keys.
func TestDeploymentAPIKeysDoNotAffectCommandsThatNeedNoLogin(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	t.Setenv("ASTRO_API_TOKEN", "")
	t.Setenv("ASTRONOMER_KEY_ID", "id")
	t.Setenv("ASTRONOMER_KEY_SECRET", "secret")
	mc := noNetwork(t)
	stderr := captureKeyWarnings(t)

	run := func(*cobra.Command, []string) {}
	root := &cobra.Command{Use: topLvlCmd}
	annotated := &cobra.Command{Use: "config", Run: run, Annotations: map[string]string{NoLoginAnnotation: "true"}}
	under := &cobra.Command{Use: "get", Run: run}
	annotated.AddCommand(under)
	version := &cobra.Command{Use: "version", Run: run}
	login := &cobra.Command{Use: "login", Run: run}
	root.AddCommand(annotated, version, login)

	for _, cmd := range []*cobra.Command{annotated, under} {
		assert.NoError(t, Setup(cmd, mc), cmd.Name())
	}
	// CalledAs is what Setup reads for the built-ins, so run them.
	for _, cmd := range []*cobra.Command{version, login} {
		root.SetArgs([]string{cmd.Name()})
		root.SetOut(&strings.Builder{})
		root.SetErr(&strings.Builder{})
		cmd.PreRunE = func(c *cobra.Command, _ []string) error { return Setup(c, mc) }
		assert.NoError(t, root.Execute(), cmd.Name())
	}
	assert.Empty(t, mc.Calls)
	assert.Empty(t, stderr(), "no warning on a run that never reaches credential setup")
}
