package astro

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// A flag the command cannot do without, whatever the terminal, is a usage
// error like cobra's own required-flag check: exit 2 and kind usage. It fails
// before anything is asked of the API, so no client is given.
//
// A value the command could ask for at a prompt is not here: that is
// input_required, and it exits 1.
func TestMissingRequiredFlagIsAUsageError(t *testing.T) {
	for _, c := range []struct {
		// path is the command, rest what follows it, and flag the one the
		// message must name.
		path string
		rest []string
		flag string
	}{
		// Each of these names no Deployment, the one flag they cannot run without.
		{"deployment team list", nil, "--deployment"},
		{"deployment team remove", []string{"team-eng"}, "--deployment"},
		{"deployment team add", []string{"team-eng"}, "--deployment"},
		{"deployment team update", []string{"team-eng"}, "--deployment"},
		{"deployment user add", []string{"ada@example.com"}, "--deployment"},
		{"deployment user list", nil, "--deployment"},
		{"deployment user update", []string{"ada@example.com"}, "--deployment"},
		{"deployment user remove", []string{"ada@example.com"}, "--deployment"},
		{"deployment token list", nil, "--deployment"},
		{"deployment token create", nil, "--deployment"},
		{"deployment token update", []string{"token-1"}, "--deployment"},
		{"deployment token rotate", []string{"token-1"}, "--deployment"},
		{"deployment token delete", []string{"token-1"}, "--deployment"},
		{"deployment token add", []string{"token-1"}, "--deployment"},
		{"deployment token remove", []string{"token-1"}, "--deployment"},
		{"deployment token organization-token add", []string{"token-1"}, "--deployment"},
		{"deployment token organization-token update", []string{"token-1"}, "--deployment"},
		{"deployment token organization-token remove", []string{"token-1"}, "--deployment"},
		{"deployment token organization-token list", nil, "--deployment"},
		{"deployment token workspace-token add", []string{"token-1"}, "--deployment"},
		{"deployment token workspace-token update", []string{"token-1"}, "--deployment"},
		{"deployment token workspace-token remove", []string{"token-1"}, "--deployment"},
		{"deployment token workspace-token list", nil, "--deployment"},
		// A flag refused with another, before any request.
		{"deployment create", []string{"--name", "d", "--runtime-version", "12.1.1", "--wait=false", "--wait-time", "1m"}, "--wait-time"},
		{"deployment hibernate", []string{"d", "--wait=false", "--wait-time", "1m"}, "--wait-time"},
		{"deployment wake-up", []string{"d", "--wait=false", "--wait-time", "1m"}, "--wait-time"},
		{"deployment create", []string{"--clone", "d", "--name", "copy", "--wait=false", "--wait-time", "1m"}, "--wait-time"},
		// One of two flags, which cobra cannot say with the wording a bundle has.
		{"deployment bundle create", []string{"--deployment", "dep"}, "--mount-path"},
		{"deployment bundle update", []string{"--deployment", "dep"}, "--mount-path"},
	} {
		t.Run(c.path+" "+strings.Join(c.rest, " "), func(t *testing.T) {
			args := append(strings.Fields(c.path), c.rest...)

			text := execAstroCmd(t, nil, "", newDeploymentRootCmd, args...)
			require.Error(t, text.err)
			assert.Equal(t, cliout.ExitUsage, text.code, "stderr: %s", text.stderr)
			assert.True(t, cliout.IsUsage(text.err))
			assert.Contains(t, text.err.Error(), c.flag)

			js := execAstroCmd(t, nil, "", newDeploymentRootCmd, append(append([]string{}, args...), "-o", "json")...)
			require.Error(t, js.err)
			assert.Equal(t, cliout.ExitUsage, js.code)
			var got errorJSON
			decodeOne(t, js.stdout, &got)
			assert.Equal(t, cliout.ExitUsage, got.Code)
			assert.Equal(t, string(cliout.KindUsage), got.Kind)
			assert.Contains(t, got.Error, c.flag)
			assert.NotContains(t, js.stderr, "Error", "the error is the object alone; a deprecation warning may share stderr")
		})
	}
}

// The Environment Manager's creates refuse a missing --type or --endpoint the
// same way. The object is not there yet, so the set is a create, which the
// flags have to describe whole.
func TestEnvSetMissingCreateFlagIsAUsageError(t *testing.T) {
	ws := "--workspace-id=" + wjWorkspace
	for _, c := range []struct {
		args []string
		flag string
	}{
		{[]string{"connection", "set", "NEW_KEY", ws}, "--type"},
		{[]string{"metrics-export", "set", "NEW_KEY", ws}, "--endpoint"},
		{[]string{"metrics-export", "set", "NEW_KEY", ws, "--endpoint", "https://m"}, "--exporter-type"},
	} {
		t.Run(strings.Join(c.args, " "), func(t *testing.T) {
			r := execEnvJSON(t, wjClient(t), append(c.args, "-o", "json")...)
			require.Error(t, r.err)
			assert.Equal(t, cliout.ExitUsage, r.code, "stdout: %s", r.stdout)
			var got errorJSON
			decodeOne(t, r.stdout, &got)
			assert.Equal(t, string(cliout.KindUsage), got.Kind)
			assert.Contains(t, got.Error, c.flag)
		})
	}
}
