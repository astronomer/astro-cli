package cliout

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
)

// Held, each warning and note prints once, at the Flush after the run's
// output, in the order found.
func TestRecapHoldsEachForTheFlush(t *testing.T) {
	var buf bytes.Buffer
	r := NewRecap(&buf, true)
	r.Warn("early")
	r.Note("a note")
	fmt.Fprintln(&buf, "step 1/9")
	r.Warn("late")
	r.Flush()
	want := "step 1/9\nWarning: early\nNote: a note\nWarning: late\n"
	if buf.String() != want {
		t.Errorf("printed\n%s\nwant\n%s", buf.String(), want)
	}
	r.Flush()
	if buf.String() != want {
		t.Errorf("a second Flush reprinted:\n%s", buf.String())
	}
}

// A Flush before a question prints what was found so far; the next prints
// only what came after.
func TestRecapFlushBeforeAQuestionIsNotRepeated(t *testing.T) {
	var buf bytes.Buffer
	r := NewRecap(&buf, true)
	r.Warn("before the question")
	r.Flush()
	fmt.Fprintln(&buf, "Deploy to which deployment?")
	r.Warn("after")
	r.Flush()
	want := "Warning: before the question\nDeploy to which deployment?\nWarning: after\n"
	if buf.String() != want {
		t.Errorf("printed\n%s\nwant\n%s", buf.String(), want)
	}
}

// Not holding (json), each prints once, as found.
func TestRecapWithoutHoldPrintsAsFound(t *testing.T) {
	var buf bytes.Buffer
	r := NewRecap(&buf, false)
	r.Warn("early")
	fmt.Fprintln(&buf, "step 1/9")
	r.Flush()
	if want := "Warning: early\nstep 1/9\n"; buf.String() != want {
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
		go func() { defer wg.Done(); r.Flush() }()
	}
	wg.Wait()
	r.Flush()
	if n != 8 {
		t.Errorf("shown %d times, want each warning once", n)
	}
}
