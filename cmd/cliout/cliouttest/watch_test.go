package cliouttest

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// The reporting, exercised directly. It cannot be exercised through a real
// tally, because in a healthy tree every branch is silent, which is the state
// that lets a mutation to one of them survive.

type pinnedShape struct {
	A string `json:"a"`
}

type notPinnedAnywhere struct {
	A string `json:"a"`
}

type pinnedInAnotherTree struct {
	A string `json:"a"`
}

func testWatch() Watch {
	return Watch{
		// By pointer, as a tree may pin one: the tally holds the unwrapped
		// shape, so the two sides have to agree about unwrapping.
		Cases:           []Case{{Name: "pinned", Value: &pinnedShape{}}},
		PinnedElsewhere: map[reflect.Type]string{reflect.TypeOf(pinnedInAnotherTree{}): "elsewhere.json"},
		Floor:           1,
		File:            "cmd/x/schema_test.go",
	}
}

func shapes(vs ...any) map[reflect.Type]bool {
	out := map[reflect.Type]bool{}
	for _, v := range vs {
		out[reflect.TypeOf(v)] = true
	}
	return out
}

func TestWatchIsSilentOnAHealthyTally(t *testing.T) {
	assert.Empty(t, testWatch().Problems(shapes(pinnedShape{}, pinnedInAnotherTree{}), nil, 1))
}

func TestWatchReportsAnUnpinnedNamedType(t *testing.T) {
	got := testWatch().Problems(shapes(pinnedShape{}, notPinnedAnywhere{}), nil, 0)
	require.Len(t, got, 1)
	assert.Contains(t, got[0], "notPinnedAnywhere")
	assert.Contains(t, got[0], "no golden pins it")
	// All three ways out: pin it here, say where else it is pinned, or say
	// it is a fixture.
	for _, remedy := range []string{"publishedPayloads", "PinnedElsewhere", "cliout.ErrorObject", "NotAPayload"} {
		assert.Contains(t, got[0], remedy)
	}
}

func TestWatchReportsAnAnonymousStruct(t *testing.T) {
	got := testWatch().Problems(nil, map[string]bool{"struct { A int }": true}, 0)
	require.Len(t, got, 1)
	assert.Contains(t, got[0], "anonymous")
}

func TestWatchReportsATallyBelowTheFloor(t *testing.T) {
	got := testWatch().Problems(shapes(pinnedShape{}), nil, 5)
	require.Len(t, got, 1)
	assert.Contains(t, got[0], "below the floor")
}

// A tree that pins shapes and keeps a floor of 0 has no floor at all.
func TestWatchReportsAZeroFloorUnderPinnedShapes(t *testing.T) {
	w := testWatch()
	w.Floor = 0
	got := w.Problems(shapes(pinnedShape{}), nil, 0)
	require.Len(t, got, 1)
	assert.Contains(t, got[0], "minWatchedPayloads is 0")

	w.Cases = nil
	assert.Empty(t, w.Problems(nil, nil, 0), "a tree that pins nothing yet may have a floor of 0")
}

func TestWatchHonorsNotAPayload(t *testing.T) {
	w := testWatch()
	w.NotAPayload = map[string]string{
		reflect.TypeOf(notPinnedAnywhere{}).String(): "a fixture",
		"struct { A int }":                           "a fixture",
	}
	assert.Empty(t, w.Problems(shapes(pinnedShape{}, notPinnedAnywhere{}), map[string]bool{"struct { A int }": true}, 1))
}

// Every problem names what to do about it.
func TestEveryWatchProblemSaysWhatToDo(t *testing.T) {
	w := testWatch()
	w.Floor = 0
	got := w.Problems(shapes(notPinnedAnywhere{}), map[string]bool{"struct { A int }": true}, 99)
	assert.Len(t, got, 4, "the zero floor, the tally below the floor, the named type and the anonymous one")
	for _, p := range got {
		assert.True(t, strings.Contains(p, "publishedPayloads") || strings.Contains(p, "minWatchedPayloads"),
			"a problem with no remedy in it: %q", p)
	}
}

// keepObserver puts back whatever observer this package's run had.
func keepObserver(t *testing.T) {
	prev := cliout.EmitObserver
	t.Cleanup(func() { cliout.EmitObserver = prev })
}

// The observer is checked when the run ends for still reporting into the
// tally that was armed, so a test that replaces it cannot leave the tally
// quietly empty.
func TestWatchReportsAnObserverReplacedDuringTheRun(t *testing.T) {
	keepObserver(t)

	h := Watch{File: "cmd/x/schema_test.go"}.Arm()
	assert.True(t, h.Armed())
	assert.Empty(t, h.Problems())

	cliout.EmitObserver = func(any) {}
	assert.False(t, h.Armed())
	got := h.Problems()
	require.Len(t, got, 1)
	assert.Contains(t, got[0], "replaced during the run")

	cliout.EmitObserver = nil
	assert.False(t, h.Armed())
}

// A second Arm installs an observer built from the same code, which
// reflection cannot tell from the first; the first watch still sees that its
// tally no longer receives anything.
func TestWatchReportsASecondArm(t *testing.T) {
	keepObserver(t)

	first := Watch{File: "cmd/x/schema_test.go"}.Arm()
	second := Watch{File: "cmd/x/schema_test.go"}.Arm()
	assert.False(t, first.Armed(), "the first watch's tally no longer receives what is emitted")
	assert.True(t, second.Armed())
	assert.Contains(t, strings.Join(first.Problems(), "\n"), "replaced during the run")
}

// A probe is an answer, never a payload: it is not tallied.
func TestArmedRecordsNothing(t *testing.T) {
	keepObserver(t)

	h := testWatch().Arm()
	require.True(t, h.Armed())
	assert.Empty(t, h.t.named)
	assert.Empty(t, h.t.anonymous)
}

func TestWatchTalliesWhatReachesEmit(t *testing.T) {
	keepObserver(t)

	h := testWatch().Arm()
	r := cliout.Renderer{Format: cliout.FormatJSON, Out: new(strings.Builder)}
	require.NoError(t, r.Emit([]notPinnedAnywhere{}, nil))
	got := h.Problems()
	require.Len(t, got, 1, "the floor is met by the one shape, which is unpinned")
	assert.Contains(t, got[0], "notPinnedAnywhere")
}

func TestFinishKeepsAFailingCodeAndFailsOnProblems(t *testing.T) {
	keepObserver(t)

	h := testWatch().Arm()
	var report strings.Builder
	h.report = &report
	r := cliout.Renderer{Format: cliout.FormatJSON, Out: new(strings.Builder)}
	require.NoError(t, r.Emit(notPinnedAnywhere{}, nil))
	assert.Equal(t, 3, h.Finish(3), "a failing run keeps its own code")
	assert.Empty(t, report.String(), "and is not judged")
	assert.Equal(t, 1, h.Finish(0), "a passing run with problems fails")
	assert.Contains(t, report.String(), "FAIL: what commands emit does not match what is pinned")
	assert.Contains(t, report.String(), "notPinnedAnywhere")

	report.Reset()
	t.Setenv(UpdateEnv, "1")
	assert.Equal(t, 0, h.Finish(0), "a run rewriting the goldens is not judged")
	assert.Empty(t, report.String())
}

// An observer that is not this package's may assert what it is handed is a
// payload. The probe must not end TestMain in its panic: the report says the
// observer was replaced.
func TestWatchReportsAnObserverThatPanicsOnTheProbe(t *testing.T) {
	keepObserver(t)

	h := testWatch().Arm()
	cliout.EmitObserver = func(v any) { _ = v.(pinnedShape) }
	assert.False(t, h.Armed())
	assert.Contains(t, strings.Join(h.Problems(), "\n"), "replaced during the run")
}

func TestFilteredRun(t *testing.T) {
	flags := map[string]*flag.Flag{}
	for _, name := range []string{"test.run", "test.skip", "test.list"} {
		f := flag.Lookup(name)
		require.NotNil(t, f, name)
		flags[name] = f
		prev := f.Value.String()
		t.Cleanup(func() { require.NoError(t, f.Value.Set(prev)) })
	}

	for _, c := range []struct {
		run, skip, list string
		filtered        bool
	}{
		{"", "", "", false},
		{".", "", "", false},
		{".*", "", "", false},
		{"TestX", "", "", true},
		{"", "TestX", "", true},
		{".", "TestX", "", true},
		{"", "", ".", true},
	} {
		require.NoError(t, flags["test.run"].Value.Set(c.run))
		require.NoError(t, flags["test.skip"].Value.Set(c.skip))
		require.NoError(t, flags["test.list"].Value.Set(c.list))
		assert.Equal(t, c.filtered, FilteredRun(), "-run %q -skip %q -list %q", c.run, c.skip, c.list)
	}
}

func TestKeyProblemsReportsWhatIsNotSnakeCase(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.json"),
		[]byte(`{"ok_key":1,"Capital":{"nested_ok":[{"camelCase":true}]}}`), 0o600))

	got := KeyProblems(t, dir, true, map[string]string{"a.json: camelCase": "legacy", "a.json: goneKey": "legacy"})
	require.Len(t, got, 2)
	assert.Contains(t, got[0], "a.json: Capital is not snake_case")
	assert.Contains(t, got[1], "a.json: goneKey is excused")
}

// failRecorder is a testing.TB that records a failure instead of ending the
// test, so a test can check that a helper fails.
type failRecorder struct {
	testing.TB
	failed bool
}

type failedNow struct{}

func (f *failRecorder) Helper()               {}
func (f *failRecorder) Errorf(string, ...any) { f.failed = true }
func (f *failRecorder) FailNow()              { f.failed = true; panic(failedNow{}) }

// fails reports whether fn failed the TB it was handed.
func fails(t *testing.T, fn func(tb testing.TB)) bool {
	r := &failRecorder{TB: t}
	func() {
		defer func() {
			if p := recover(); p != nil {
				if _, ok := p.(failedNow); !ok {
					panic(p)
				}
			}
		}()
		fn(r)
	}()
	return r.failed
}

// A missing directory of goldens is the empty tree only when there are no
// cases; with cases it is a wrong path, and both helpers fail on it.
func TestAMissingGoldenDirectoryPassesOnlyWithNoCases(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	cases := []Case{{Name: "a"}}

	assert.False(t, fails(t, func(tb testing.TB) { KeyProblems(tb, missing, false, nil) }))
	assert.False(t, fails(t, func(tb testing.TB) { Orphans(tb, missing, nil) }))
	assert.True(t, fails(t, func(tb testing.TB) { KeyProblems(tb, missing, true, nil) }))
	assert.True(t, fails(t, func(tb testing.TB) { Orphans(tb, missing, cases) }))
}
