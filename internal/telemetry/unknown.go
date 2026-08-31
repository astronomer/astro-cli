package telemetry

import (
	"regexp"

	"github.com/spf13/cobra"
)

// EventUnknownCommand is the event type for a command the CLI does not have.
// It is kept apart from EventCommandExecution so that a run which did nothing
// is never counted as a run which did something.
const EventUnknownCommand = "CLI Unknown Command"

// RedactedCommand stands in for a word that does not look like a command name,
// so that the event is still counted without carrying what the user typed.
const RedactedCommand = "<redacted>"

// maxCommandLength is the longest word we will report. The longest command the
// CLI has is `organization-token`, at 18, so this leaves room for a guess at a
// longer name while dropping the opaque strings a secret tends to be.
const maxCommandLength = 30

// commandPattern describes what a command name may look like: a lower-case
// letter, then lower-case letters, digits, dashes or underscores. Every command
// the CLI has fits this. A token or a path does not.
var commandPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

// TrackUnknownCommand sends an event for a word the CLI has no command for.
// parent is the command it was typed under, and suggestion is the nearest
// command name, or "" when there is none.
func TrackUnknownCommand(parent *cobra.Command, word, suggestion string) {
	if !IsEnabled() || isTestRun() {
		return
	}

	showFirstRunNotice()

	properties := buildCommandProperties(parent)
	if properties["command"] == "" {
		delete(properties, "command")
	}
	properties["unknown_command"] = unknownCommandPath(parent, word)
	if suggestion != "" {
		properties["suggestion"] = suggestion
	}

	track(EventUnknownCommand, properties)
}

// unknownCommandPath joins the parent path and the unknown word, so that
// `astro dev restrt` reads as "dev restrt". The parent path is made of command
// names the CLI defines. Only the unknown word comes from the user, so only
// the unknown word is redacted.
func unknownCommandPath(parent *cobra.Command, word string) string {
	if len(word) > maxCommandLength || !commandPattern.MatchString(word) {
		word = RedactedCommand
	}
	path := GetCommandPath(parent)
	if path == "" {
		return word
	}
	return path + " " + word
}
