package local

import (
	"flag"
	"fmt"
	"os"
	"reflect"
	"sort"
	"sync"
	"testing"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/cmd/cliout/cliouttest"
)

// What a command actually emits, watched at the door.
//
// The goldens in testdata/schema pin Go types. They cannot tell you a
// command still emits the type its golden holds: swap the value a command
// hands to Emit and every golden stays green, which is the same blind spot
// as a function whose call sites nobody tests. Emit is the single output
// path (TestEmitIsTheOnlyJSONEncoder holds it to that), so recording what
// passes through it closes that for every shape the suite exercises.
//
// What it checks, exactly, because the three guards over this contract fail
// in different directions and it matters which is which:
//
//   - every named struct reaching Emit is pinned. NOT that a given command
//     emits its own type: swapping `astro use` from useResult to some other
//     ALREADY-PINNED type passes here. Binding a surface to a type needs the
//     pair recorded, and the cliout.Renderer does not know which surface it is.
//   - no anonymous struct reaches Emit, since an anonymous shape cannot be
//     pinned at all.
//   - the watching is still happening. Without that, everything above is
//     vacuous — see minWatchedPayloads.
//
// Blind by construction: it sees only what the suite runs, and a payload
// behind an untested path is not recorded, silently. The static
// TestEveryJSONPayloadTypeIsPinned over-reports instead. Neither is a
// superset of the other, which is why both stay.

// minWatchedPayloads is a floor under the tally, not a target.
//
// An empty tally reads exactly like a clean one: report only the extras and
// "the door stopped being watched" is silent. Measured by mutation — moving
// the observer inside Emit's json branch, and replacing it with a func that
// records nothing — both left the package green before this existed.
//
// 36 shapes are recorded today. The floor sits below that so ordinary test
// churn does not trip it, and it is raised when the number rises, never
// lowered without saying why in the commit.
const minWatchedPayloads = 30

// notAPayload names types a test pushes through Emit to exercise Emit
// itself, rather than to publish anything. Keyed by the type's String(),
// with the reason.
//
// The sibling guards each needed one of these — notPublished for the static
// check, //astro:non-output-json for the single-door check — because every
// rule over this contract is a proxy for "is this published" and none of
// them can actually tell. Without it, a test written with a fixture struct
// fails naming a command that does not exist.
var notAPayload = map[string]string{}

var emitted = struct {
	sync.Mutex
	named     map[reflect.Type]bool
	anonymous map[string]bool
}{
	named:     map[reflect.Type]bool{},
	anonymous: map[string]bool{},
}

// TestMain arms the observer for the package run and checks the tally
// afterwards. A single test cannot: the point is what every OTHER test
// emitted.
func TestMain(m *testing.M) {
	cliout.EmitObserver = recordEmitted

	code := m.Run()

	// Only when the suite passed. A failing run emits a partial, arbitrary
	// set, and gaps reported from it would bury the failure that caused them.
	if code == 0 {
		if problems := payloadProblems(filteredRun()); len(problems) > 0 {
			fmt.Fprintln(os.Stderr, "FAIL: what commands emit does not match what is pinned")
			for _, p := range problems {
				fmt.Fprintln(os.Stderr, "  "+p)
			}
			code = 1
		}
	}
	os.Exit(code)
}

func recordEmitted(v any) {
	if v == nil {
		return
	}
	t := cliouttest.PayloadType(reflect.TypeOf(v))
	if t == nil {
		return
	}

	emitted.Lock()
	defer emitted.Unlock()
	if t.Name() == "" {
		emitted.anonymous[t.String()] = true
		return
	}
	emitted.named[t] = true
}

// pinnedShapes is publishedPayloads reduced to the types the tally is keyed
// by, unwrapped the same way — pin something as &scaffold.Result{} and a
// raw reflect.TypeOf would miss it, reporting "add it to publishedPayloads"
// for a type already there.
func pinnedShapes() map[reflect.Type]bool {
	out := map[reflect.Type]bool{}
	for _, c := range publishedPayloads {
		if t := cliouttest.PayloadType(reflect.TypeOf(c.value)); t != nil {
			out[t] = true
		}
	}
	return out
}

func payloadProblems(filtered bool) []string {
	emitted.Lock()
	defer emitted.Unlock()

	// A filtered run has a partial tally by definition, so the floor would
	// fail every `go test -run ...` in this package and teach people to
	// ignore it. The extras are still worth reporting: whatever did run and
	// emitted something unpinned is a real finding.
	floor := minWatchedPayloads
	if filtered {
		floor = 0
	}
	return problemsIn(emitted.named, emitted.anonymous, floor)
}

// filteredRun reports whether -run narrowed the suite.
func filteredRun() bool {
	f := flag.Lookup("test.run")
	return f != nil && f.Value.String() != ""
}

// problemsIn is payloadProblems over a tally handed in, so the reporting can
// be tested directly. It cannot be tested through the real tally: in a clean
// tree every branch below is silent, which is exactly the state that makes a
// mutation to one of them survive.
func problemsIn(named map[reflect.Type]bool, anonymous map[string]bool, floor int) []string {
	pinned := pinnedShapes()

	var out []string
	if len(named) < floor {
		out = append(out, fmt.Sprintf(
			"only %d payload shapes were seen reaching cliout.Renderer.Emit, below the floor of %d.\n"+
				"    Either the observer is no longer wired up — in which case every other\n"+
				"    check here is passing on an empty tally — or a lot of command tests\n"+
				"    stopped running. If the drop is real and intended, lower\n"+
				"    minWatchedPayloads and say why.", len(named), floor))
	}
	for t := range named {
		if pinned[t] || notAPayload[t.String()] != "" {
			continue
		}
		out = append(out, fmt.Sprintf(
			"a command passed %s through cliout.Renderer.Emit and no golden pins it.\n"+
				"    Add it to publishedPayloads and run `make update-schemas`; a shape\n"+
				"    that reaches stdout is a contract whether or not anybody meant it\n"+
				"    to be. If a test is exercising Emit rather than publishing, name\n"+
				"    the type in notAPayload with the reason.", t))
	}
	for name := range anonymous {
		if notAPayload[name] != "" {
			continue
		}
		out = append(out, fmt.Sprintf(
			"a command passed the anonymous struct %s through cliout.Renderer.Emit.\n"+
				"    An anonymous shape cannot be pinned, so nothing will notice it\n"+
				"    changing. Give it a name and add it to publishedPayloads.", name))
	}
	sort.Strings(out)
	return out
}
