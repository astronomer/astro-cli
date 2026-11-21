package envresolve

import (
	"strings"
	"testing"
)

// skipping is a Provider holding keys that are not env-var names.
type skipping struct{ keys []string }

func (skipping) Lookup(string) (string, bool) { return "", false }
func (skipping) Label() string                { return "workspace" }
func (s skipping) SkippedKeys() []string      { return s.keys }

// The keys a Workspace holds that are not names are shown as text in the
// note: what such a key holds is printed, not acted on by the terminal.
func TestSkippedNoteShowsKeysAsText(t *testing.T) {
	note := SkippedNote(skipping{keys: []string{"bad key\x1b]0;title\x07", "x\x1b[2Jy"}}, "ws\x1b[2J1")
	if strings.Contains(note, "\x1b") {
		t.Errorf("note carries an escape: %q", note)
	}
	if !strings.Contains(note, "Workspace ws1 holds bad key, xy,") {
		t.Errorf("note = %q", note)
	}
}
