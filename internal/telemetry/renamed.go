package telemetry

import (
	"github.com/spf13/cobra"
)

// EventRenamedFlag is the event type for a run that passed flags by the
// spellings 1.x had for them, which v2 still accepts as hidden aliases. It is
// kept apart from EventCommandExecution, which the same run sends too.
const EventRenamedFlag = "CLI Renamed Flag"

// renamedFlags are the 1.x spellings a run gave, queued by RecordRenamedFlags
// until the command's own event goes.
type renamedFlags struct {
	cmd              *cobra.Command
	renamed, current []string
}

// queuedRenamedFlags is what this run queued; a process runs one command.
var queuedRenamedFlags *renamedFlags

// RecordRenamedFlags notes the 1.x spellings cmd was given, renamed (as
// named: --workspace-id) beside current, the flags they stand for
// (--workspace), once the run has accepted them. The events are what tell us
// when nobody passes an alias any more, and it can go.
//
// The event goes with the command's own (TrackCommand), so a run the pre-run
// stops, for want of a login say, sends neither, and all of a run's aliases
// go in one event. A command that skips the pre-run (the core tree, which
// leaves no config/ state behind on a machine that has none) never reaches
// TrackCommand, so it sends at once, and only on a machine that has already
// shown the first-run notice, as TrackRemovedCommand does, since showing the
// notice is a write.
func RecordRenamedFlags(cmd *cobra.Command, renamed, current []string) {
	if len(renamed) == 0 {
		return
	}
	queuedRenamedFlags = &renamedFlags{cmd: cmd, renamed: renamed, current: current}
	if cmd.Annotations[SkipPreRunAnnotation] == "true" {
		if !noticeShown() {
			queuedRenamedFlags = nil
			return
		}
		sendRenamedFlags(cmd)
	}
}

// sendRenamedFlags sends the event RecordRenamedFlags queued for cmd, if any,
// and empties the queue.
func sendRenamedFlags(cmd *cobra.Command) {
	q := takeRenamedFlags(cmd)
	if q == nil || GetCommandPath(cmd) == "" || !canTrack(cmd) {
		return
	}
	properties := buildCommandProperties(cmd)
	properties["renamed_flags"] = q.renamed
	properties["current_flags"] = q.current
	track(EventRenamedFlag, properties)
}

// takeRenamedFlags empties the queue, returning what it held for cmd.
func takeRenamedFlags(cmd *cobra.Command) *renamedFlags {
	q := queuedRenamedFlags
	queuedRenamedFlags = nil
	if q == nil || q.cmd != cmd {
		return nil
	}
	return q
}
