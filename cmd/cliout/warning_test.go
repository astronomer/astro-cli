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
	if got := WarningLabel(&b); got != "Warning:" {
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
	if got := WarningLabel(os.Stderr); !strings.Contains(got, "\x1b[33m") || !strings.Contains(got, "Warning:") {
		t.Errorf("WarningLabel on stderr = %q, want a yellow Warning:", got)
	}

	t.Setenv("NO_COLOR", "1")
	if got := ErrorLabel(os.Stderr); got != "Error:" {
		t.Errorf("ErrorLabel under NO_COLOR = %q, want plain", got)
	}
	if got := WarningLabel(os.Stderr); got != "Warning:" {
		t.Errorf("WarningLabel under NO_COLOR = %q, want plain", got)
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
