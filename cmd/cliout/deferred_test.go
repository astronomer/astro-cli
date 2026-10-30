package cliout

import (
	"bytes"
	"testing"

	"github.com/astronomer/astro-cli/pkg/input"
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

// A question the run asks prints what is held first; what comes after the
// question is held until Flush, and the hook goes once restored.
func TestDeferredFlushesBeforeAQuestion(t *testing.T) {
	defer input.SetGuard(nil)()
	var buf bytes.Buffer
	d := NewDeferred(&buf, true)
	restore := d.FlushBeforeQuestions()
	d.Warn("before")
	if err := input.MayAsk("> "); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "Warning: before\n" {
		t.Fatalf("before the question: %q", buf.String())
	}
	d.Warn("after")
	if buf.String() != "Warning: before\n" {
		t.Fatalf("a warning after the question printed early: %q", buf.String())
	}
	restore()
	if err := input.MayAsk("> "); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "Warning: before\n" {
		t.Fatalf("a restored hook still flushed: %q", buf.String())
	}
	d.Flush()
	if want := "Warning: before\nWarning: after\n"; buf.String() != want {
		t.Errorf("printed %q, want %q", buf.String(), want)
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
