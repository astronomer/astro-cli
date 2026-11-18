package telemetry

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
)

func TestTrackSetupFailure(t *testing.T) {
	initTestConfig(t)
	require.NoError(t, config.CFG.TelemetryNoticeShown.SetHomeString(noticeVersion))
	root := &cobra.Command{Use: "astro"}
	deploy := &cobra.Command{Use: "deploy"}
	hidden := &cobra.Command{Use: "gone", Hidden: true}
	tel := &cobra.Command{Use: "telemetry"}
	root.AddCommand(deploy, hidden, tel)

	sent, restore := CaptureEventsForTesting()
	t.Cleanup(restore)

	TrackSetupFailure(deploy, "unauthenticated")
	TrackSetupFailure(deploy, "")
	TrackSetupFailure(hidden, "unauthenticated")
	TrackSetupFailure(tel, "unauthenticated")
	TrackSetupFailure(root, "unauthenticated")

	events := sent()
	require.Len(t, events, 2, "a hidden command, the telemetry commands and the root send nothing")
	assert.Equal(t, EventCommandSetupFailed, events[0].Event)
	assert.Equal(t, "deploy", events[0].Properties["command"])
	assert.Equal(t, "unauthenticated", events[0].Properties["error_kind"])
	assert.NotContains(t, events[1].Properties, "error_kind", "a failure with no kind names none")
	for _, ev := range events {
		assert.Empty(t, ev.Token, "sent anonymously")
	}
}

// A setup failure on a first run sends nothing and writes nothing: the notice
// it would show is a config write, which the failing run must not make.
func TestTrackSetupFailureLeavesAFirstRunAlone(t *testing.T) {
	initTestConfig(t)
	root := &cobra.Command{Use: "astro"}
	deploy := &cobra.Command{Use: "deploy"}
	root.AddCommand(deploy)
	sent, restore := CaptureEventsForTesting()
	t.Cleanup(restore)

	TrackSetupFailure(deploy, "unauthenticated")

	assert.Empty(t, sent())
	assert.False(t, noticeShown(), "the notice was not marked shown")
}
