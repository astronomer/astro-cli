package telemetry

import (
	"github.com/spf13/cobra"
)

// EventRenamedFlag is the event type for a run that passed a flag by the
// spelling 1.x had for it, which v2 still accepts as a hidden alias. It is
// kept apart from EventCommandExecution, which the same run sends too.
const EventRenamedFlag = "CLI Renamed Flag"

// TrackRenamedFlag sends an event for a 1.x spelling given to cmd: flag is
// the alias as named (--workspace-id), and current the flag it stands for
// (--workspace). The events are what tell us when nobody passes an alias any
// more, and it can go.
//
// A command that skips the pre-run (the core tree, which leaves no config/
// state behind on a machine that has none) sends it only on a machine that
// has already shown the first-run notice, as TrackRemovedCommand does, since
// showing the notice is a write. Every other command sends it whenever its
// own command event would go, CI runners that start fresh among them, which
// is where most scripts run.
func TrackRenamedFlag(cmd *cobra.Command, flag, current string) {
	if GetCommandPath(cmd) == "" {
		return
	}
	if cmd.Annotations[SkipPreRunAnnotation] == "true" && !noticeShown() {
		return
	}
	if !canTrack(cmd) {
		return
	}

	properties := buildCommandProperties(cmd)
	properties["renamed_flag"] = flag
	properties["current_flag"] = current
	track(EventRenamedFlag, properties)
}
