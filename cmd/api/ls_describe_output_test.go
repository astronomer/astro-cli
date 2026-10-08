package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// registrySpec is a registry OpenAPI document with one endpoint.
const registrySpec = `{"openapi":"3.0.0","info":{"title":"Registry","version":"1"},"paths":{` +
	`"/providers.json":{"get":{"operationId":"listProviders","tags":["Providers"],"summary":"List providers",` +
	`"responses":{"200":{"description":"ok","content":{"application/json":{"schema":{"type":"object",` +
	`"properties":{"providers":{"type":"array","items":{"type":"string"}}}}}}}}}}}}`

// countingVersion is the Airflow version countingServer reports.
const countingVersion = "3.0.3"

// countingServer counts the requests it gets. It answers an Airflow version
// request as an Airflow countingVersion would, and anything else with
// registrySpec.
func countingServer(t *testing.T) (url string, hits *atomic.Int32) {
	t.Helper()
	hits = new(atomic.Int32)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/version") {
			_, _ = w.Write([]byte(`{"version":"` + countingVersion + `"}`))
			return
		}
		_, _ = w.Write([]byte(registrySpec))
	}))
	t.Cleanup(srv.Close)
	return srv.URL, hits
}

// runAPI runs `astro api args...` the way main does (cliout.Execute) and
// returns what reached stdout and stderr, and the exit status.
func runAPI(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	return runAPIHooked(t, nil, args...)
}

// runAPIHooked is runAPI under a root whose pre-run, the one that refreshes
// the token and records telemetry in the real CLI, counts into preRuns.
func runAPIHooked(t *testing.T, preRuns *atomic.Int32, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errOut bytes.Buffer
	root := &cobra.Command{Use: "astro"}
	if preRuns != nil {
		root.PersistentPreRunE = func(*cobra.Command, []string) error {
			preRuns.Add(1)
			return nil
		}
	}
	root.AddCommand(NewAPICmdWithOutput(&out))
	root.SetOut(&errOut)
	root.SetErr(&errOut)
	ctx := context.Background()
	err := cliout.Execute(ctx, root, append([]string{"api"}, args...), &out, nil)
	return out.String(), errOut.String(), cliout.ExitCode(ctx, err)
}

// apiFamily is one `astro api` family and the flags that point its spec at a
// test server, so a test can count what it fetched.
type apiFamily struct {
	name string
	at   []string
}

// familiesAt points every family at url. cloud needs a cloud context to get
// as far as fetching, which initTestConfig gives it. airflow asks the server
// its version and reads that version's published spec, seeded here so
// nothing is fetched from GitHub.
func familiesAt(t *testing.T, url string) []apiFamily {
	t.Helper()
	initTestConfig(t)
	seedAirflowSpecCache(t, countingVersion, registrySpec)
	return []apiFamily{
		{"airflow", []string{"--url", url}},
		{"cloud", []string{"--spec-url", url}},
		{"registry", []string{"--registry-url", url}},
	}
}

// The servers the refusal tests count are real: each family, run without a
// refusal, does fetch from it, so hits staying at zero there means something.
func TestEveryFamilyFetchesFromTheCountedServer(t *testing.T) {
	url, hits := countingServer(t)
	for _, family := range familiesAt(t, url) {
		t.Run(family.name, func(t *testing.T) {
			before := hits.Load()
			_, stderr, code := runAPI(t, append([]string{family.name, "ls", "-o", "json"}, family.at...)...)
			require.Zero(t, code, stderr)
			assert.Greater(t, hits.Load(), before, "%s ls never reached the server", family.name)
		})
	}
}

// assertRefusedFirst runs args and checks the run failed as a usage error with
// want, before the root's pre-run and before any fetch: under -o json as the
// one error object, in text as cobra's error on stderr with nothing on stdout.
func assertRefusedFirst(t *testing.T, hits *atomic.Int32, want string, asJSON bool, args ...string) {
	t.Helper()
	var preRuns atomic.Int32
	before := hits.Load()
	stdout, stderr, code := runAPIHooked(t, &preRuns, args...)
	assert.Equal(t, cliout.ExitUsage, code)
	assert.Zero(t, preRuns.Load(), "the root's pre-run ran before the refusal")
	assert.Equal(t, before, hits.Load(), "a refused run fetched a spec")
	if asJSON {
		var obj cliout.ErrorObject
		require.NoError(t, json.Unmarshal([]byte(stdout), &obj), stdout)
		assert.Equal(t, want, obj.Error)
		assert.Equal(t, cliout.ExitUsage, obj.Code)
		assert.Empty(t, stderr)
		return
	}
	assert.Empty(t, stdout, "nothing may run before the refusal")
	assert.Contains(t, stderr, "Error: "+want)
}

// --json shipped in 1.x on every family's ls and describe. v2 replaced it with
// -o json and tombstoned it: passing it is a usage error naming the
// replacement, before anything is fetched, under -o json as the one error
// object.
func TestJSONFlagIsTombstoned(t *testing.T) {
	url, hits := countingServer(t)
	for _, family := range familiesAt(t, url) {
		for _, sub := range [][]string{{"ls"}, {"describe", "listProviders"}} {
			for _, asJSON := range []bool{false, true} {
				args := append(append([]string{family.name}, sub...), family.at...)
				args = append(args, "--json")
				if asJSON {
					args = append(args, "-o", "json")
				}
				t.Run(strings.Join(args, " "), func(t *testing.T) {
					assertRefusedFirst(t, hits, cliout.ErrJSONFlagRemoved, asJSON, args...)
				})
			}
		}
	}
}

// A format ls and describe do not offer is refused the same way, before the
// root's pre-run (token refresh, telemetry) and before any spec is fetched.
func TestUnknownOutputFormatIsRefusedFirst(t *testing.T) {
	url, hits := countingServer(t)
	for _, family := range familiesAt(t, url) {
		for _, sub := range [][]string{{"ls"}, {"describe", "listProviders"}} {
			args := append(append([]string{family.name}, sub...), family.at...)
			args = append(args, "-o", "yaml")
			t.Run(strings.Join(args, " "), func(t *testing.T) {
				assertRefusedFirst(t, hits, `unknown output format "yaml" (supported: text, json)`, false, args...)
			})
		}
	}
}

// The tombstone is not offered: help names -o, not --json.
func TestJSONFlagIsHiddenFromHelp(t *testing.T) {
	apiCmd := NewAPICmdWithOutput(new(bytes.Buffer))
	for _, path := range [][]string{
		{"airflow", "ls"},
		{"airflow", "describe"},
		{"cloud", "ls"},
		{"cloud", "describe"},
		{"registry", "ls"},
		{"registry", "describe"},
	} {
		cmd, _, err := apiCmd.Find(path)
		require.NoError(t, err)
		require.NotNil(t, cmd.Flag("json"), "%v has no --json tombstone", path)
		assert.True(t, cmd.Flag("json").Hidden, "%v shows --json", path)
		assert.NotContains(t, cmd.UsageString(), "--json", "%v", path)
		assert.Contains(t, cmd.UsageString(), "-o, --output", "%v", path)
	}
}

// ls and describe publish through cliout: -o json is the listing and the
// schemas, compact when not on a terminal. A format they do not offer is
// TestUnknownOutputFormatIsRefusedFirst's.
func TestRegistryLsAndDescribeTakeOutput(t *testing.T) {
	isolateSpecCache(t)
	url, _ := countingServer(t)

	stdout, stderr, code := runAPI(t, "registry", "ls", "--registry-url", url, "-o", "json")
	require.Zero(t, code, stderr)
	rows := decodeEndpoints(t, []byte(stdout))
	require.Len(t, rows, 1)
	assert.Equal(t, "listProviders", rows[0]["operation_id"])
	assert.Equal(t, 1, strings.Count(stdout, "\n"), "piped json is one line: %q", stdout)

	stdout, stderr, code = runAPI(t, "registry", "describe", "listProviders", "--registry-url", url, "-o", "json")
	require.Zero(t, code, stderr)
	eps := decodeEndpoints(t, []byte(stdout))
	require.Len(t, eps, 1)
	assert.Equal(t, "/providers.json", eps[0]["path"])
	assert.Equal(t, 1, strings.Count(stdout, "\n"), "piped json is one line: %q", stdout)

	stdout, _, code = runAPI(t, "registry", "ls", "--registry-url", url)
	require.Zero(t, code)
	assert.Contains(t, stdout, "/providers.json")
	assert.Contains(t, stdout, "1 endpoint")
}

// Text builds no payload: ls and describe hand Emit a cliout.Lazy, which is
// resolved, and reaches the observer, only under -o json. Resolving every
// $ref is work the text rendering never needs.
func TestTextBuildsNoPayload(t *testing.T) {
	isolateSpecCache(t)
	url, _ := countingServer(t)
	prev := cliout.EmitObserver
	var emitted []any
	cliout.EmitObserver = func(v any) {
		emitted = append(emitted, v)
		if prev != nil {
			prev(v)
		}
	}
	t.Cleanup(func() { cliout.EmitObserver = prev })

	for _, sub := range [][]string{{"ls"}, {"describe", "listProviders"}} {
		emitted = nil
		_, stderr, code := runAPI(t, append(append([]string{"registry"}, sub...), "--registry-url", url)...)
		require.Zero(t, code, stderr)
		assert.Empty(t, emitted, "text %v built a payload", sub)

		_, stderr, code = runAPI(t, append(append([]string{"registry"}, sub...), "--registry-url", url, "-o", "json")...)
		require.Zero(t, code, stderr)
		require.Len(t, emitted, 1, "json %v", sub)
		_, unresolved := emitted[0].(cliout.Lazy)
		assert.False(t, unresolved, "json %v published the builder, not what it builds", sub)
	}
}
