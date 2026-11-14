package apc

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
)

// warnings is the deprecation warnings in stderr, each without its label.
func warnings(stderr string) []string {
	var lines []string
	for line := range strings.Lines(stderr) {
		if rest, ok := strings.CutPrefix(line, "Warning: "); ok && strings.Contains(rest, " is deprecated: ") {
			lines = append(lines, strings.TrimSuffix(rest, "\n"))
		}
	}
	return lines
}

// withoutDeprecations is s without the lines that say a spelling is
// deprecated (cliout.WarnDeprecated). A test that spells a flag the way 1.x
// did, for what the flag does and not for the warning, asserts that nothing
// else was said.
func withoutDeprecations(s string) string {
	var kept strings.Builder
	for line := range strings.Lines(s) {
		if strings.HasPrefix(line, "Warning: ") && strings.Contains(line, " is deprecated: ") {
			continue
		}
		kept.WriteString(line)
	}
	return kept.String()
}

// 1.x's astro deployment logs <component>, kept and hidden, is a deprecated
// spelling of astro deployment logs --<component>: it still runs, says so once
// on stderr, naming what was typed (its alias too) and what to type instead,
// and the run's exit code is its own.
func TestAKept1xLogsSubcommandIsDeprecated(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"logs", "scheduler", "dep-1"}, "astro deployment logs scheduler is deprecated: use astro deployment logs --scheduler"},
		{[]string{"logs", "sch", "dep-1"}, "astro deployment logs sch is deprecated: use astro deployment logs --scheduler"},
		{[]string{"logs", "webserver", "dep-1"}, "astro deployment logs webserver is deprecated: use astro deployment logs --webserver"},
		{[]string{"logs", "workers", "dep-1"}, "astro deployment logs workers is deprecated: use astro deployment logs --workers"},
		{[]string{"logs", "worker", "dep-1"}, "astro deployment logs worker is deprecated: use astro deployment logs --workers"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			api := newAPCClient()
			api.On("ListDeploymentLogs", mock.Anything).Return([]houston.DeploymentLog{}, nil).Once()
			run := runAPC(t, api, "", append([]string{"deployment"}, tc.args...)...)
			require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
			assert.Equal(t, []string{tc.want}, warnings(run.stderr))
			assert.NotContains(t, run.stdout, "deprecated")
			api.AssertExpectations(t)
		})
	}

	t.Run("the flag that replaces it says nothing", func(t *testing.T) {
		api := newAPCClient()
		api.On("ListDeploymentLogs", mock.Anything).Return([]houston.DeploymentLog{}, nil)
		run := runAPC(t, api, "", "deployment", "logs", "dep-1", "--scheduler")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		assert.Empty(t, warnings(run.stderr))
	})

	t.Run("a subcommand that fails still exits as it would, with the warning", func(t *testing.T) {
		api := newAPCClient()
		api.On("ListDeploymentLogs", mock.Anything).Return(nil, assert.AnError).Once()
		run := runAPC(t, api, "", "deployment", "logs", "scheduler", "dep-1")
		require.Error(t, run.err)
		assert.NotEqual(t, 0, run.code)
		assert.Len(t, warnings(run.stderr), 1)
	})
}

// A hidden alias warns on a run that succeeds too, once however often it is
// typed, with stdout still the one document under --output json.
func TestADeprecatedAliasWarnsOnASuccessfulJSONRun(t *testing.T) {
	api := newAPCClient()
	api.On("CreateWorkspace", mock.Anything).Return(&houston.Workspace{ID: "ws-1", Label: "alpha"}, nil).Once()
	run := runAPC(t, api, "", "workspace", "create", "--label", "alpha", "--label", "alpha", "-o", "json")
	require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
	assert.Equal(t, []string{"--label is deprecated: use --name"}, warnings(run.stderr))
	var one map[string]any
	dec := json.NewDecoder(strings.NewReader(run.stdout))
	require.NoError(t, dec.Decode(&one), run.stdout)
	assert.False(t, dec.More(), "stdout is not one document: %q", run.stdout)
	assert.NotContains(t, run.stdout, "deprecated")
}
