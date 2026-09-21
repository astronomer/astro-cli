//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// The suite checks what it left on the machine, because nothing else will.
//
// Every other thing a case creates is reclaimed for it: t.TempDir takes the
// project, and XDG_CACHE_HOME, ASTRO_HOME and HOME take everything the CLI
// writes. Docker's storage is outside all three, so a tier-3 case that forgets
// to tear down leaves a container, a volume, a network or a 1.3 GB image on the
// machine for good — and passes. The proxy daemon is the same shape: a process
// nothing reaps.
//
// What the scoping and the diff buy, exactly:
//
//   - Scoping is by name prefix and by repository. Compose stamps its objects
//     with a project label, but --filter matches that label's value exactly or
//     not at all, and the project name carries a per-path hash — so the label
//     finds candidates and the astro- prefix decides. Without that second step
//     this watches every compose project on the machine, which on a developer's
//     laptop is their own work.
//   - The diff excludes what was already there, and nothing else. An astro
//     project started in another terminal DURING the run, or a second suite run
//     overlapping this one, is new since the snapshot and gets reported. Two
//     concurrent runs will blame each other.
//
// One blind spot: an image that loses its tag to a rebuild becomes <none>:<none>
// and matches no repository filter, so a re-tag orphan is invisible here while
// still pinning its layers.
const (
	// censusTimeout bounds the proxy-daemon query. The docker queries take
	// dockerListTimeout, which the cases share. A census that hangs would turn
	// a passing suite into a timeout with no useful output.
	censusTimeout = 30 * time.Second
	// censusSettle bounds how long the after-snapshot keeps looking while it
	// still sees something. Teardown here is asynchronous by design: compose
	// down returns once it has issued the removals, and the daemon reap outlives
	// the call that triggers it. A single shot taken the instant m.Run returns
	// reports containers that are still Removing.
	censusSettle = 30 * time.Second
	// censusPoll is how long to wait before looking again.
	censusPoll = 2 * time.Second
	// dockerTier is the lowest tier that can make a docker object. Below it the
	// queries can only cost time — and the Windows PR job runs tier 0 on a
	// machine whose docker service is stopped, where each would pay its full
	// timeout to prove nothing.
	dockerTier = 3
)

// proxyServeArg is the hidden subcommand the CLI re-execs itself with to serve
// the proxy.
//
// Spelled here rather than imported: e2e is its own module and deliberately
// requires nothing, which is the point of driving the binary from the outside.
// TestServeSubcommandSpellingIsWhatE2EMatches in cmd/local pins the literal from
// the other side, so that a rename cannot quietly turn this axis into a check
// that finds nothing forever.
const proxyServeArg = "__proxy-serve"

// ours reports whether a compose object's name belongs to this suite.
//
// The engines build the project name as astro-<sanitized base>-<6 hex of the
// path hash>, and compose derives its container, volume and network names from
// that. The prefix is where the suite's objects end and the developer's own
// running stacks begin.
func ours(name string) bool { return strings.HasPrefix(name, "astro-") }

// censusAxis is one question to ask docker.
type censusAxis struct {
	kind string
	args []string
	// keep decides whether a listed line is this suite's. Nil keeps everything,
	// which is right where the docker-side filter is already exact.
	keep func(string) bool
}

// id identifies an axis for coverage, including its arguments: two axes here
// report the same kind.
func (a censusAxis) id() string { return a.kind + ": docker " + strings.Join(a.args, " ") }

func censusAxes() []censusAxis {
	// Spelled out rather than built from a shared prefix slice: appending to one
	// backing array from three places is a bug waiting for whoever widens it.
	const label = "label=com.docker.compose.project"
	return []censusAxis{
		{kind: "container", keep: ours, args: []string{"ps", "-a", "--filter", label, "--format", "{{.Names}}"}},
		{kind: "volume", keep: ours, args: []string{"volume", "ls", "--filter", label, "--format", "{{.Name}}"}},
		{kind: "network", keep: ours, args: []string{"network", "ls", "--filter", label, "--format", "{{.Name}}"}},
		// Both repositories the CLI tags into: the local build's, and the two
		// tags `astro package astro` writes.
		{kind: "image", args: []string{"images", "--format", "{{.Repository}}:{{.Tag}}", "astro-local/*"}},
		{kind: "image", args: []string{"images", "--format", "{{.Repository}}:{{.Tag}}", "astro-package/*"}},
	}
}

// census is one snapshot: what was found, and which axes managed to look.
//
// Coverage is recorded because an axis that errored is not an axis that found
// nothing. A before-snapshot whose image query failed while the engine was still
// starting, diffed against an after where it worked, would report every
// pre-existing astro image as this run's leak — and the mirror case, erroring
// only afterwards, would hide real leaks and pass.
type census struct {
	// found maps "kind name" to the id of the axis that saw it.
	found map[string]string
	// covered holds the ids of the axes that answered.
	covered map[string]bool
}

func newCensus() census {
	return census{found: map[string]string{}, covered: map[string]bool{}}
}

// takeCensus asks docker what is on the machine right now.
//
// Empty, with no error, when docker is unreachable: tiers 0 and 1 do not need it
// and CI runs them on machines without it. An axis that cannot look is left
// uncovered rather than recorded as empty, which is what keeps the diff honest.
func takeCensus(ctx context.Context, ceiling int) census {
	c := newCensus()
	if ceiling < dockerTier {
		return c
	}
	for _, a := range censusAxes() {
		lines, err := dockerLines(ctx, a.args...)
		if err != nil {
			continue
		}
		c.record(a, lines)
	}
	return c
}

// record marks an axis covered and keeps the lines that are this suite's.
//
// Separate from takeCensus so that the deciding can be checked without an
// engine. The maps are reference types, so the value receiver still writes
// through to the caller's census.
func (c census) record(a censusAxis, lines []string) {
	c.covered[a.id()] = true
	for _, line := range lines {
		if a.keep != nil && !a.keep(line) {
			continue
		}
		c.found[a.kind+" "+line] = a.id()
	}
}

// leaks reports what the run added and did not take away, plus any axis that
// cannot be compared. A blind axis is reported rather than skipped quietly,
// since a check that stops checking otherwise looks exactly like a clean run.
func leaks(before, after census) (left, blind []string) {
	for id := range before.covered {
		if !after.covered[id] {
			blind = append(blind, "could not be checked after the run: "+id)
		}
	}
	for id := range after.covered {
		if !before.covered[id] {
			blind = append(blind, "had no before-snapshot to compare against: "+id)
		}
	}
	for name, id := range after.found {
		if !before.covered[id] {
			// Reported as blind above. Calling this a leak would blame the run
			// for everything that was already on the machine.
			continue
		}
		if _, was := before.found[name]; !was {
			left = append(left, name)
		}
	}
	sort.Strings(left)
	sort.Strings(blind)
	return left, blind
}

// reportLeaks prints what was left behind, and says whether the run should fail.
//
// It keeps looking while it still sees something, up to censusSettle, so that
// teardown which has been issued but not finished is not reported as a leak.
func reportLeaks(before census, ceiling int) bool {
	ctx := context.Background()
	deadline := time.Now().Add(censusSettle)
	var left, blind []string
	for {
		left, blind = leaks(before, takeCensus(ctx, ceiling))
		pids, err := daemons(ctx)
		left = append(left, pids...)
		if err != nil {
			blind = append(blind, err.Error())
		}
		sort.Strings(left)
		if len(left) == 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(censusPoll)
	}

	for _, b := range blind {
		fmt.Fprintf(os.Stderr, "\ne2e leak census: %s\n", b)
	}
	if len(left) > 0 {
		fmt.Fprintf(os.Stderr,
			"\ne2e left %d thing(s) on this machine. Docker storage and the proxy daemon are\n"+
				"outside every isolation lever, so these persist until somebody removes them:\n", len(left))
		for _, name := range left {
			fmt.Fprintln(os.Stderr, "  "+name)
		}
	}
	return len(left) > 0 || len(blind) > 0
}
