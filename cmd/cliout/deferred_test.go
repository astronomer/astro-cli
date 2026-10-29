package cliout

import (
	"bytes"
	"testing"
)

func TestDeferredPrintsNothingUntilFlushedThenInOrder(t *testing.T) {
	var buf bytes.Buffer
	d := NewDeferred(&buf, true)
	d.Warn("first")
	d.Note("second")
	d.Warn("third")
	if buf.Len() != 0 {
		t.Fatalf("printed before Flush: %q", buf.String())
	}
	d.Flush()
	const want = "Warning: first\nNote: second\nWarning: third\n"
	if buf.String() != want {
		t.Errorf("Flush printed %q, want %q", buf.String(), want)
	}
	d.Flush()
	if buf.String() != want {
		t.Errorf("a second Flush reprinted: %q", buf.String())
	}
}

func TestDeferredNotHoldingPrintsAsGiven(t *testing.T) {
	var buf bytes.Buffer
	d := NewDeferred(&buf, false)
	d.Warn("first")
	if buf.String() != "Warning: first\n" {
		t.Fatalf("printed %q, want the warning at once", buf.String())
	}
	d.Note("second")
	d.Flush()
	if want := "Warning: first\nNote: second\n"; buf.String() != want {
		t.Errorf("printed %q, want %q", buf.String(), want)
	}
}
