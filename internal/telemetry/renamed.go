package telemetry

import (
	"slices"

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
	// inert are 1.x flags v2 still accepts that do nothing now, by name.
	inert []string
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
	queueRenamed(cmd, renamed, current, nil)
}

// RecordInertFlags notes the 1.x flags cmd was given that v2 accepts and
// ignores (--clean-output), by name as typed and nothing else, so we know when
// nobody passes one any more and it can go. It rides the renamed-flag event
// (RecordRenamedFlags), as the property inert_flags.
func RecordInertFlags(cmd *cobra.Command, inert []string) {
	queueRenamed(cmd, nil, nil, inert)
}

func queueRenamed(cmd *cobra.Command, renamed, current, inert []string) {
	if len(renamed) == 0 && len(inert) == 0 {
		return
	}
	// A run that gives a deprecated command its hidden flag aliases records
	// twice (the flags and the command's own path, in whichever order the
	// argument checks run): one event holds both.
	if q := queuedRenamedFlags; q != nil && q.cmd == cmd {
		renamed, current = append(slices.Clone(q.renamed), renamed...), append(slices.Clone(q.current), current...)
		inert = append(slices.Clone(q.inert), inert...)
	}
	queuedRenamedFlags = &renamedFlags{cmd: cmd, renamed: renamed, current: current, inert: inert}
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
	if len(q.renamed) > 0 {
		properties["renamed_flags"] = q.renamed
		properties["current_flags"] = q.current
	}
	if len(q.inert) > 0 {
		properties["inert_flags"] = q.inert
	}
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
