package cliout

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/ansi"
)

// A warning is led by "Warning:". Only stderr's own palette may color it, so
// a writer that is not stderr (a buffer, stdout) always gets plain text.
func TestWarnLeadsWithWarning(t *testing.T) {
	var b bytes.Buffer
	Warn(&b, "the image will carry 1 file that .gitignore ignores")
	if got, want := b.String(), "Warning: the image will carry 1 file that .gitignore ignores\n"; got != want {
		t.Errorf("Warn wrote %q, want %q", got, want)
	}
	if got := ansi.WarningLabel(&b); got != "Warning:" {
		t.Errorf("a writer that is not stderr gets a plain label, got %q", got)
	}
}

// An error is led by "Error:", plain on any writer that is not stderr.
func TestErrorLabelIsPlainOffStderr(t *testing.T) {
	var b bytes.Buffer
	if got := ErrorLabel(&b); got != "Error:" {
		t.Errorf("a writer that is not stderr gets a plain label, got %q", got)
	}
}

// On stderr with color on, Error: is red and Warning: yellow; NO_COLOR turns
// both plain again.
func TestLabelsAreColoredOnStderrUnlessNoColor(t *testing.T) {
	forced := ansi.ForceColors
	ansi.ForceColors = true
	t.Cleanup(func() { ansi.ForceColors = forced })
	t.Setenv("NO_COLOR", "")

	if got := ErrorLabel(os.Stderr); !strings.Contains(got, "\x1b[31m") || !strings.Contains(got, "Error:") {
		t.Errorf("ErrorLabel on stderr = %q, want a red Error:", got)
	}
	if got := ansi.WarningLabel(os.Stderr); !strings.Contains(got, "\x1b[33m") || !strings.Contains(got, "Warning:") {
		t.Errorf("WarningLabel on stderr = %q, want a yellow Warning:", got)
	}

	t.Setenv("NO_COLOR", "1")
	if got := ErrorLabel(os.Stderr); got != "Error:" {
		t.Errorf("ErrorLabel under NO_COLOR = %q, want plain", got)
	}
	if got := ansi.WarningLabel(os.Stderr); got != "Warning:" {
		t.Errorf("WarningLabel under NO_COLOR = %q, want plain", got)
	}
}

// Next: is cyan on a terminal stdout as well as stderr, since it leads a
// command's own output; anything else, and NO_COLOR, gets it plain.
func TestNextLabelColorsOnStdoutAndStderr(t *testing.T) {
	forced := ansi.ForceColors
	ansi.ForceColors = true
	t.Cleanup(func() { ansi.ForceColors = forced })
	t.Setenv("NO_COLOR", "")

	for name, w := range map[string]*os.File{"stdout": os.Stdout, "stderr": os.Stderr} {
		if got := NextLabel(w); !strings.Contains(got, "\x1b[36m") || !strings.Contains(got, "Next:") {
			t.Errorf("NextLabel on %s = %q, want a cyan Next:", name, got)
		}
	}
	var b bytes.Buffer
	if got := NextLabel(&b); got != "Next:" {
		t.Errorf("NextLabel on a buffer = %q, want plain", got)
	}

	t.Setenv("NO_COLOR", "1")
	if got := NextLabel(os.Stdout); got != "Next:" {
		t.Errorf("NextLabel on stdout under NO_COLOR = %q, want plain", got)
	}
}

// Note: is dim and the danger label red on stderr with color on; off stderr,
// or under NO_COLOR, both are plain.
func TestNoteAndDangerLabels(t *testing.T) {
	forced := ansi.ForceColors
	ansi.ForceColors = true
	t.Cleanup(func() { ansi.ForceColors = forced })
	t.Setenv("NO_COLOR", "")

	if got := NoteLabel(os.Stderr); !strings.Contains(got, "\x1b[2m") || !strings.Contains(got, "Note:") {
		t.Errorf("NoteLabel on stderr = %q, want a dim Note:", got)
	}
	if got := DangerLabel(os.Stderr); !strings.Contains(got, "\x1b[31m") || !strings.Contains(got, "WARNING:") {
		t.Errorf("DangerLabel on stderr = %q, want a red WARNING:", got)
	}

	var b strings.Builder
	Note(&b, "the project has uncommitted changes")
	if got := b.String(); got != "Note: the project has uncommitted changes\n" {
		t.Errorf("Note off stderr wrote %q, want a plain Note: line", got)
	}

	t.Setenv("NO_COLOR", "1")
	if got := NoteLabel(os.Stderr); got != "Note:" {
		t.Errorf("NoteLabel under NO_COLOR = %q, want plain", got)
	}
	if got := DangerLabel(os.Stderr); got != "WARNING:" {
		t.Errorf("DangerLabel under NO_COLOR = %q, want plain", got)
	}
}

// A command that set its own ErrPrefix keeps it; cobra's default becomes the
// Error: label.
func TestErrorPrefixKeepsACustomPrefix(t *testing.T) {
	var b bytes.Buffer
	if got := errorPrefixFor("Oops:", &b); got != "Oops:" {
		t.Errorf("a custom prefix = %q, want Oops:", got)
	}
	if got := errorPrefixFor("Error:", &b); got != "Error:" {
		t.Errorf("cobra's default prefix = %q, want Error:", got)
	}
}
