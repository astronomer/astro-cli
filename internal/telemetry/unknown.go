package telemetry

import (
	"regexp"

	"github.com/spf13/cobra"
)

// EventUnknownCommand is the event type for a command the CLI does not have,
// and EventUnknownFlag for a flag it does not have. Both are kept apart from
// EventCommandExecution so that a run which did nothing is never counted as a
// run which did something.
const (
	EventUnknownCommand = "CLI Unknown Command"
	EventUnknownFlag    = "CLI Unknown Flag"
)

// redacted stands in for a word that does not look like a command or a flag,
// so that the event is still counted without carrying what the user typed.
const redacted = "<redacted>"

// maxCommandLength is the longest word we will report. The longest command the
// CLI has is `organization-token`, at 18, so this leaves room for a guess at a
// longer name while dropping the opaque strings a secret tends to be.
const maxCommandLength = 30

// maxFlagLength leaves room for the two dashes on a long flag.
const maxFlagLength = maxCommandLength + 2

// Every command the CLI has fits commandPattern, and every flag fits
// flagPattern. A token or a path fits neither.
var (
	commandPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
	flagPattern    = regexp.MustCompile(`^--[a-z][a-z0-9_-]*$|^-[a-zA-Z0-9]$`)
)

// TrackUnknownCommand sends an event for a word the CLI has no command for.
// parent is the command it was typed under, and suggestion is the nearest
// command name, or "" when there is none.
func TrackUnknownCommand(parent *cobra.Command, word, suggestion string) {
	if !canTrack(parent) {
		return
	}

	properties := unknownProperties(parent)
	properties["unknown_command"] = unknownCommandPath(parent, word)
	if suggestion != "" {
		properties["suggestion"] = suggestion
	}

	track(EventUnknownCommand, properties)
}

// TrackUnknownFlag sends an event for a flag the CLI has no such flag for.
// cmd is the command it was typed against.
func TrackUnknownFlag(cmd *cobra.Command, flag string) {
	if !canTrack(cmd) {
		return
	}

	properties := unknownProperties(cmd)
	properties["unknown_flag"] = unknownFlagName(flag)

	track(EventUnknownFlag, properties)
}

// unknownProperties builds the properties both events share. A command path is
// empty at the root, where nothing resolved, and an empty property is noise.
func unknownProperties(cmd *cobra.Command) map[string]interface{} {
	properties := buildCommandProperties(cmd)
	if properties["command"] == "" {
		delete(properties, "command")
	}
	return properties
}

// unknownFlagName redacts a flag that does not look like a flag name. The name
// arrives without its value, but the name is still what the user typed.
func unknownFlagName(flag string) string {
	if len(flag) > maxFlagLength || !flagPattern.MatchString(flag) {
		return redacted
	}
	return flag
}

// unknownCommandPath joins the parent path and the unknown word, so that
// `astro dev restrt` reads as "dev restrt". The parent path is made of command
// names the CLI defines. Only the unknown word comes from the user, so only
// the unknown word is redacted.
func unknownCommandPath(parent *cobra.Command, word string) string {
	if len(word) > maxCommandLength || !commandPattern.MatchString(word) {
		word = redacted
	}
	path := GetCommandPath(parent)
	if path == "" {
		return word
	}
	return path + " " + word
}
