package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// The api group's own refusals are reported like any other command's: an
// unknown flag on `astro api` itself is cobra's error on stderr, exit 2. The
// group used to silence its errors, and through cliout.Execute that printed
// nothing at all.
func TestTheAPIGroupsRefusalsAreReported(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--bogus"}, "Error: unknown flag: --bogus"},
		{[]string{"-o", "json"}, "Error: unknown shorthand flag: 'o' in -o"},
	} {
		stdout, stderr, code := runAPI(t, tc.args...)
		assert.Equal(t, cliout.ExitUsage, code, tc.args)
		assert.Contains(t, stderr, tc.want, tc.args)
		assert.Empty(t, stdout, tc.args)
	}
}

// An unknown subcommand is refused, naming it: a usage error on stderr,
// exit 2, rather than the group's help with exit 0.
func TestAnUnknownAPISubcommandIsRefused(t *testing.T) {
	stdout, stderr, code := runAPI(t, "clod")
	assert.Equal(t, cliout.ExitUsage, code)
	assert.Contains(t, stderr, `unknown command "clod"`)
	assert.Empty(t, stdout)
}

// A request the API refuses prints the API's error body on stdout and exits 1,
// with no "Error:" line after it: the body already said what went wrong.
func TestARefusedRequestPrintsItsBodyOnce(t *testing.T) {
	isolateSpecCache(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"detail":"no such provider"}`))
	}))
	t.Cleanup(srv.Close)

	stdout, stderr, code := runAPI(t, "registry", "/providers/nope.json", "--registry-url", srv.URL)
	assert.Equal(t, cliout.ExitFailure, code)
	assert.Contains(t, stdout, "no such provider")
	assert.Equal(t, 1, strings.Count(stdout, "no such provider"))
	assert.NotContains(t, stderr, "Error:", "the refusal was reported twice")
}

// emptyRefusal serves a 502 with no body, except for an Airflow version
// request, answered as countingVersion.
func emptyRefusal(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/version") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":"` + countingVersion + `"}`))
			return
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// A refusal with no body has presented nothing, so the run says what failed,
// once, on stderr, and exits 1: a single request, and a page of a paginated
// one.
func TestARefusalWithNoBodyIsReportedOnce(t *testing.T) {
	isolateSpecCache(t)
	seedAirflowSpecCache(t, countingVersion, registrySpec)
	url := emptyRefusal(t)
	for _, args := range [][]string{
		{"registry", "/providers.json", "--registry-url", url},
		{"airflow", "/api/v2/dags", "--url", url, "--paginate"},
	} {
		stdout, stderr, code := runAPI(t, args...)
		assert.Equal(t, cliout.ExitFailure, code, args)
		assert.Empty(t, stdout, args)
		assert.Equal(t, 1, strings.Count(stderr, "Error: API request failed with status 502"), "%v: %q", args, stderr)
	}
}

// refusedResponse is the rule all three refusal sites share (a single
// request, a --paginate page, and an Airflow reached through its own
// transport): only a JSON body counts as presented. It is printed formatted
// on stdout and the error is silent. Anything else is not a result: a
// non-blank body goes to stderr with a newline, and the RequestError, which
// carries the status, is left for the run to report.
func TestRefusedResponse(t *testing.T) {
	for _, tc := range []struct {
		name           string
		body           string
		stdout, stderr string
		presented      bool
	}{
		{"empty", "", "", "", false},
		{"a newline", "\n", "", "", false},
		{"a CRLF", "\r\n", "", "", false},
		{"an HTML gateway page", "<html><body>502 Bad Gateway</body></html>", "", "<html><body>502 Bad Gateway</body></html>\n", false},
		{"plain text", "upstream connect error\n\n", "", "upstream connect error\n", false},
		{"JSON", `{"detail":"nope"}`, "{\n  \"detail\": \"nope\"\n}\n", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr strings.Builder
			err := refusedResponse(&stdout, &stderr, http.StatusBadGateway, []byte(tc.body))
			assert.Equal(t, tc.stdout, stdout.String())
			assert.Equal(t, tc.stderr, stderr.String())
			assert.EqualError(t, err, "API request failed with status 502")

			var silent *SilentError
			var reqErr *RequestError
			if tc.presented {
				require.ErrorAs(t, err, &silent)
				assert.Equal(t, http.StatusBadGateway, silent.StatusCode)
				assert.NotErrorAs(t, err, &reqErr)
				return
			}
			require.ErrorAs(t, err, &reqErr)
			assert.Equal(t, http.StatusBadGateway, reqErr.StatusCode)
			assert.NotErrorAs(t, err, &silent)
		})
	}
}
