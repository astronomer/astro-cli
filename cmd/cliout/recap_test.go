package cliout

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
)

// A warning is shown when found and repeated at the end once progress output
// has followed it; one found after the last progress output, and one held
// for the end, print once.
func TestRecapShowsWhenFoundAndRepeatsWhatProgressBuried(t *testing.T) {
	var buf bytes.Buffer
	r := NewRecap(&buf, true)
	r.Warn("early")
	r.Note("a note")
	fmt.Fprintln(&buf, "step 1/9")
	r.Progress()
	r.Warn("late")
	r.Later("after the work")
	r.End()
	want := "Warning: early\nNote: a note\nstep 1/9\nWarning: late\n" +
		"Warning: early\nNote: a note\nWarning: after the work\n"
	if buf.String() != want {
		t.Errorf("printed\n%s\nwant\n%s", buf.String(), want)
	}
	r.End()
	if buf.String() != want {
		t.Errorf("a second End reprinted:\n%s", buf.String())
	}
}

// Without repeat (json), each prints once: when found, or at the end.
func TestRecapWithoutRepeatPrintsEachOnce(t *testing.T) {
	var buf bytes.Buffer
	r := NewRecap(&buf, false)
	r.Warn("early")
	r.Progress()
	r.Later("after the work")
	r.End()
	if want := "Warning: early\nWarning: after the work\n"; buf.String() != want {
		t.Errorf("printed %q, want %q", buf.String(), want)
	}
}

// A build streams from goroutines of its own; the recap takes that.
func TestRecapIsSafeForConcurrentUse(t *testing.T) {
	var mu sync.Mutex
	n := 0
	r := NewRecapFunc(func(bool, string) { mu.Lock(); n++; mu.Unlock() }, true)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(2)
		go func() { defer wg.Done(); r.Warn("w") }()
		go func() { defer wg.Done(); r.Progress() }()
	}
	wg.Wait()
	r.End()
	if n < 8 || n > 16 {
		t.Errorf("shown %d times, want each warning once or twice", n)
	}
}
