package cliout

import (
	"bytes"
	"testing"
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
