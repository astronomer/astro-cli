package cliouttest

import (
	"flag"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"sync"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// What a tree's commands actually emit, watched at the door.
//
// The goldens pin Go types. They cannot tell you a command still emits the
// type its golden holds: swap the value a command hands to Emit and every
// golden stays green. Emit is the single output path, so recording what
// passes through it closes that for every shape the suite exercises. Each
// tree arms a Watch in its TestMain and ends the run with Finish.
//
// What it checks, exactly:
//
//   - every named struct reaching Emit is pinned, by this tree's goldens or
//     another's (PinnedElsewhere). NOT that a given command emits its own
//     type: swapping one already-pinned type for another passes here.
//   - no anonymous struct reaches Emit, since an anonymous shape cannot be
//     pinned at all.
//   - the watching is still happening: the tally reaches the tree's floor on
//     a whole run, the observer installed at the end is still the one that
//     reports into this tally, and a tree that pins shapes has a floor
//     above 0.
//
// Blind by construction: it sees only what the suite runs.

// Watch is one tree's configuration of the observer.
type Watch struct {
	// Cases is the tree's publishedPayloads.
	Cases []Case
	// PinnedElsewhere names, by type, the shapes that reach Emit in this tree
	// and are pinned by another tree's goldens, with where.
	PinnedElsewhere map[reflect.Type]string
	// NotAPayload names, by the type's String(), what a test pushes through
	// Emit to exercise Emit itself rather than to publish anything, with the
	// reason.
	NotAPayload map[string]string
	// Floor is the tree's minWatchedPayloads: a floor under the tally, not a
	// target. An empty tally reads exactly like a clean one, so without it
	// the observer coming unwired would be silent.
	Floor int
	// File is where the tree's publishedPayloads and minWatchedPayloads
	// live, for the messages.
	File string
}

// tally is what reached Emit during the run.
type tally struct {
	sync.Mutex
	named     map[reflect.Type]bool
	anonymous map[string]bool
	// probed is set when the observer answers a probe meant for this tally.
	probed bool
}

func (t *tally) record(v any) {
	if v == nil {
		return
	}
	pt := PayloadType(reflect.TypeOf(v))
	if pt == nil {
		return
	}
	t.Lock()
	defer t.Unlock()
	if pt.Name() == "" {
		t.anonymous[pt.String()] = true
		return
	}
	t.named[pt] = true
}

// probe is what Armed sends through cliout.EmitObserver to ask whether the
// observer installed there reports into a given tally. It is never a payload:
// the observer answers it and records nothing.
type probe struct{ to *tally }

// Watching is one armed watch: the observer Arm installed and the tally it
// reports into.
type Watching struct {
	w Watch
	t *tally
	// report is where Finish prints the problems: os.Stderr, which a test of
	// Finish replaces.
	report io.Writer
}

// Arm installs the observer for the package run and returns the handle that
// reports on it when the run ends.
func (w Watch) Arm() *Watching {
	t := &tally{named: map[reflect.Type]bool{}, anonymous: map[string]bool{}}
	cliout.EmitObserver = func(v any) {
		if p, ok := v.(probe); ok {
			if p.to == t {
				t.Lock()
				t.probed = true
				t.Unlock()
			}
			return
		}
		t.record(v)
	}
	return &Watching{w: w, t: t, report: os.Stderr}
}

// Armed reports whether cliout.EmitObserver still reports into this watch's
// tally. It asks the installed observer itself, so the observer of another
// Arm, which reflection cannot tell from this one, does not pass for it.
func (h *Watching) Armed() bool {
	obs := cliout.EmitObserver
	if obs == nil {
		return false
	}
	h.t.Lock()
	h.t.probed = false
	h.t.Unlock()
	if !askObserver(obs, probe{to: h.t}) {
		return false
	}
	h.t.Lock()
	defer h.t.Unlock()
	return h.t.probed
}

// askObserver hands the probe to obs, and reports false if obs panicked on
// it: an observer that is not this package's may assert the value it is
// handed is a payload, and the report should say the observer was replaced
// rather than end TestMain in a panic.
func askObserver(obs func(any), p probe) (answered bool) {
	defer func() {
		if recover() != nil {
			answered = false
		}
	}()
	obs(p)
	return true
}

// Problems is what the run leaves to report: an observer replaced during it,
// and the tally's own problems. The floor applies only to a whole run (see
// FilteredRun).
func (h *Watching) Problems() []string {
	var out []string
	if !h.Armed() {
		out = append(out, "the observer was replaced during the run: cliout.EmitObserver no longer\n"+
			"    reports into the tally TestMain armed, so the tally below is of whatever\n"+
			"    ran before that. Restore it where it was replaced, or arm it once.")
	}
	h.t.Lock()
	defer h.t.Unlock()
	floor := h.w.Floor
	if FilteredRun() {
		floor = 0
	}
	return append(out, h.w.Problems(h.t.named, h.t.anonymous, floor)...)
}

// Finish ends a TestMain: it returns the code the package run exits with. A
// failing run keeps its code, since it emitted a partial set and gaps
// reported from it would bury the failure that caused them; so does a run
// rewriting the goldens, which runs only the golden tests. A passing run with
// problems prints them on stderr and fails.
func (h *Watching) Finish(code int) int {
	if code != 0 || Updating() {
		return code
	}
	found := h.Problems()
	if len(found) == 0 {
		return code
	}
	fmt.Fprintln(h.report, "FAIL: what commands emit does not match what is pinned")
	for _, p := range found {
		fmt.Fprintln(h.report, "  "+p)
	}
	return 1
}

// FilteredRun reports whether -run, -skip or -list narrowed the package run,
// whose tally is then partial by definition (a -list run runs no test at
// all). A -run that matches every name (".", ".*") is a whole run.
func FilteredRun() bool {
	for _, name := range []string{"test.skip", "test.list"} {
		if f := flag.Lookup(name); f != nil && f.Value.String() != "" {
			return true
		}
	}
	f := flag.Lookup("test.run")
	if f == nil {
		return false
	}
	switch f.Value.String() {
	case "", ".", ".*", "^.*$", "^.*":
		return false
	}
	return true
}

// Problems is the report over a tally handed in, so the reporting can be
// tested directly: in a clean tree every branch below is silent, which is the
// state that lets a mutation to one of them survive.
func (w Watch) Problems(named map[reflect.Type]bool, anonymous map[string]bool, floor int) []string {
	pinned := map[reflect.Type]bool{}
	for _, c := range w.Cases {
		if t := PayloadType(reflect.TypeOf(c.Value)); t != nil {
			pinned[t] = true
		}
	}

	var out []string
	if len(w.Cases) > 0 && w.Floor == 0 {
		out = append(out, fmt.Sprintf(
			"%s pins %d shapes but its minWatchedPayloads is 0, so the tally has no\n"+
				"    floor and an observer that stopped recording would pass. Set\n"+
				"    minWatchedPayloads to the number of shapes a whole run sees.",
			w.File, len(w.Cases)))
	}
	if len(named) < floor {
		out = append(out, fmt.Sprintf(
			"only %d payload shapes were seen reaching cliout.Renderer.Emit, below the floor of %d.\n"+
				"    Either the observer is no longer wired up, in which case every other\n"+
				"    check here is passing on an empty tally, or command tests stopped\n"+
				"    running. If the drop is real and intended, lower minWatchedPayloads\n"+
				"    in %s and say why.",
			len(named), floor, w.File))
	}
	for t := range named {
		if pinned[t] || w.PinnedElsewhere[t] != "" || w.NotAPayload[t.String()] != "" {
			continue
		}
		out = append(out, fmt.Sprintf(
			"a command passed %s through cliout.Renderer.Emit and no golden pins it.\n"+
				"    A shape that reaches stdout is a contract whether or not anybody\n"+
				"    meant it to be. Add it to publishedPayloads in %s and run\n"+
				"    `make update-schemas`; or, if another tree's golden pins it (the\n"+
				"    error object, cliout.ErrorObject, is the common case), add a\n"+
				"    PinnedElsewhere entry naming that golden to this tree's\n"+
				"    cliouttest.Watch; or, if a test pushes it through Emit only to\n"+
				"    exercise Emit, add a NotAPayload entry to it.", t, w.File))
	}
	for name := range anonymous {
		if w.NotAPayload[name] != "" {
			continue
		}
		out = append(out, fmt.Sprintf(
			"a command passed the anonymous struct %s through cliout.Renderer.Emit.\n"+
				"    An anonymous shape cannot be pinned, so nothing will notice it\n"+
				"    changing. Give it a name and add it to publishedPayloads in %s.", name, w.File))
	}
	sort.Strings(out)
	return out
}
