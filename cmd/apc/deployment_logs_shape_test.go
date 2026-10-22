package apc

import (
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
)

// logsOf answers a request for component's logs with one record, at
// second, saying component.
func logsOf(api *mocks.ClientInterface, component, second string) {
	api.On("ListDeploymentLogs", mock.MatchedBy(func(r houston.ListDeploymentLogsRequest) bool {
		return r.DeploymentID == "dep-1" && r.Component == component
	})).Return([]houston.DeploymentLog{{CreatedAt: "2026-01-01T00:00:" + second + "Z", Log: component + " line"}}, nil).Once()
}

// noTriggererClient is newAPCClient on a platform with no triggerer.
func noTriggererClient() *mocks.ClientInterface {
	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{Flags: houston.FeatureFlags{AstroRuntimeEnabled: true}}, nil).Maybe()
	api.On("GetPlatformVersion", mock.Anything).Return("1.0.0", nil).Maybe()
	return api
}

// astro deployment logs on APC is shaped as Astro's: flags pick the
// components, every one when none does, and several are merged by time.
func TestDeploymentLogsPicksComponentsAsAstroDoes(t *testing.T) {
	t.Run("none named is every component, merged oldest first", func(t *testing.T) {
		api := newAPCClient()
		logsOf(api, "webserver", "04")
		logsOf(api, "scheduler", "01")
		logsOf(api, "worker", "03")
		logsOf(api, "triggerer", "02")

		run := runAPC(t, api, "", "deployment", "logs", "dep-1", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got []string
		for _, e := range decodeLines[logEntryJSON](t, run.stdout) {
			got = append(got, e.Component)
		}
		assert.Equal(t, []string{"scheduler", "triggerer", "worker", "webserver"}, got)
		api.AssertExpectations(t)
	})

	t.Run("several in text name their component, as Astro's name their source", func(t *testing.T) {
		api := newAPCClient()
		logsOf(api, "scheduler", "01")
		logsOf(api, "worker", "02")

		run := runAPC(t, api, "", "deployment", "logs", "dep-1", "--scheduler", "--workers")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		assert.Equal(t, "scheduler line scheduler\nworker line worker\n", run.stdout)
		api.AssertExpectations(t)
	})

	t.Run("one in text is the lines alone, as 1.x printed them", func(t *testing.T) {
		api := newAPCClient()
		logsOf(api, "webserver", "01")

		run := runAPC(t, api, "", "deployment", "logs", "dep-1", "--webserver")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		assert.Equal(t, "webserver line\n", run.stdout)
		api.AssertExpectations(t)
	})

	t.Run("--component takes Houston's names and the flag's, once each", func(t *testing.T) {
		api := newAPCClient()
		logsOf(api, "scheduler", "01")
		logsOf(api, "worker", "02")

		run := runAPC(t, api, "", "deployment", "logs", "dep-1", "--component", "Workers,scheduler", "--component", "worker", "--workers", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		assert.Len(t, decodeLines[logEntryJSON](t, run.stdout), 2)
		api.AssertExpectations(t)
	})

	t.Run("--component skips empty names, as a trailing comma leaves", func(t *testing.T) {
		api := newAPCClient()
		logsOf(api, "scheduler", "01")

		run := runAPC(t, api, "", "deployment", "logs", "dep-1", "--component", ",scheduler, ", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		assert.Len(t, decodeLines[logEntryJSON](t, run.stdout), 1, "the scheduler alone, not every component")
		api.AssertExpectations(t)
	})

	t.Run("an unknown --component is a usage error, before any request", func(t *testing.T) {
		api := newAPCClient()
		run := runAPC(t, api, "", "deployment", "logs", "dep-1", "--component", "ApiServer")
		assert.Equal(t, 2, run.code)
		require.Error(t, run.err)
		assert.Contains(t, run.err.Error(), `--component "ApiServer" is not a component this platform logs: use webserver, scheduler, worker or workers, triggerer`)
		api.AssertNotCalled(t, "ListDeploymentLogs", mock.Anything)
	})

	// Taken for a Deployment id, a component would be asked about as one.
	t.Run("a component where the Deployment goes is refused, naming its flag", func(t *testing.T) {
		for _, args := range [][]string{{"triggerer", "dep-1"}, {"triggerer"}, {"Triggerers", "dep-1"}} {
			api := newAPCClient()
			run := runAPC(t, api, "", append([]string{"deployment", "logs"}, args...)...)
			assert.Equal(t, 2, run.code, "%v", args)
			require.Error(t, run.err, "%v", args)
			assert.Contains(t, run.err.Error(), "names a component, not a Deployment: pick it with --triggerer", "%v", args)
			api.AssertNotCalled(t, "ListDeploymentLogs", mock.Anything)
		}
	})

	t.Run("with no triggerer, none named is the rest, and the triggerer is refused", func(t *testing.T) {
		api := noTriggererClient()
		logsOf(api, "webserver", "01")
		logsOf(api, "scheduler", "02")
		logsOf(api, "worker", "03")
		run := runAPC(t, api, "", "deployment", "logs", "dep-1", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		assert.Len(t, decodeLines[logEntryJSON](t, run.stdout), 3)
		api.AssertExpectations(t)

		run = runAPC(t, noTriggererClient(), "", "deployment", "logs", "dep-1", "--component", "triggerer")
		assert.Equal(t, 2, run.code)
		run = runAPC(t, noTriggererClient(), "", "deployment", "logs", "dep-1", "--triggerer")
		assert.Equal(t, 2, run.code, "no --triggerer flag on a platform with no triggerer")
	})

	t.Run("no Deployment is a usage error", func(t *testing.T) {
		run := runAPC(t, newAPCClient(), "", "deployment", "logs")
		assert.Equal(t, 2, run.code)
		require.Error(t, run.err)
		assert.Contains(t, run.err.Error(), "name the Deployment")
	})

	t.Run("help shows the flags, not 1.x's subcommands", func(t *testing.T) {
		run := runAPC(t, newAPCClient(), "", "deployment", "logs", "--help")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		assert.Contains(t, run.stdout, "--scheduler")
		assert.Contains(t, run.stdout, "--component")
		assert.Contains(t, run.stdout, "(webserver, scheduler, worker or workers, triggerer)", "Houston's name beside the flag's")
		assert.NotContains(t, run.stdout, "Available Commands")
	})
}

// Houston streams one component's logs per subscription, so --follow takes
// exactly one.
func TestDeploymentLogsFollowsOneComponent(t *testing.T) {
	prev := subscribeLogs
	t.Cleanup(func() { subscribeLogs = prev })
	var followed []string
	subscribeLogs = func(_, component, _ string, _ time.Duration, _ io.Writer, onLog func(houston.DeploymentLog) error) error {
		followed = append(followed, component)
		return onLog(houston.DeploymentLog{CreatedAt: "t", Log: "a line\n"})
	}

	for _, args := range [][]string{
		{"--follow"},
		{"--follow", "--scheduler", "--workers"},
		{"--follow", "--component", "scheduler,worker"},
	} {
		run := runAPC(t, newAPCClient(), "", append([]string{"deployment", "logs", "dep-1"}, args...)...)
		assert.Equal(t, 2, run.code, "%v", args)
		require.Error(t, run.err, "%v", args)
		assert.Contains(t, run.err.Error(), "--follow streams one component's logs", "%v", args)
	}
	assert.Empty(t, followed, "a refused follow subscribes to nothing")

	run := runAPC(t, newAPCClient(), "", "deployment", "logs", "dep-1", "--follow", "--workers")
	require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
	assert.Equal(t, []string{"worker"}, followed)
	assert.Equal(t, "a line\n", run.stdout)
}
