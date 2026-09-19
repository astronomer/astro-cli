package local

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/pkg/localrt"
)

// The reporting, exercised directly.
//
// It cannot be exercised through the real tally, because in a healthy tree
// every branch is silent — which is the state that lets a mutation to one of
// them survive. A guard whose failure path never runs is a guard nobody has
// tested.

func TestProblemsInReportsAnUnpinnedNamedType(t *testing.T) {
	type notPinnedAnywhere struct {
		A string `json:"a"`
	}
	got := problemsIn(map[reflect.Type]bool{reflect.TypeOf(notPinnedAnywhere{}): true}, nil, 0)

	assert.Len(t, got, 1)
	assert.Contains(t, got[0], "notPinnedAnywhere")
	assert.Contains(t, got[0], "no golden pins it")
}

func TestProblemsInPassesAPinnedType(t *testing.T) {
	// localrt.Status is pinned as local-status, and reaches the tally
	// unwrapped — so pinnedShapes has to normalize the same way.
	got := problemsIn(map[reflect.Type]bool{reflect.TypeOf(localrt.Status{}): true}, nil, 0)
	assert.Empty(t, got)
}

// Pinning a payload by pointer must not read as unpinned. The tally stores
// the unwrapped shape, so the two sides have to agree about normalization or
// the report says "add it to publishedPayloads" about a type already there.
func TestPinnedShapesUnwrapsPointersAndSlices(t *testing.T) {
	shapes := pinnedShapes()
	assert.True(t, shapes[reflect.TypeOf(localrt.Status{})],
		"a pinned payload must be found by its unwrapped shape")
}

func TestProblemsInReportsAnAnonymousStruct(t *testing.T) {
	got := problemsIn(nil, map[string]bool{"struct { A int }": true}, 0)

	assert.Len(t, got, 1)
	assert.Contains(t, got[0], "anonymous")
	assert.Contains(t, got[0], "Give it a name")
}

// The floor is the check that everything else rests on: without it an empty
// tally reads as a clean one.
func TestProblemsInReportsATallyBelowTheFloor(t *testing.T) {
	got := problemsIn(map[reflect.Type]bool{reflect.TypeOf(localrt.Status{}): true}, nil, 5)

	assert.Len(t, got, 1)
	assert.Contains(t, got[0], "below the floor")
	assert.Contains(t, got[0], "no longer wired up")
}

func TestProblemsInIsSilentOnAHealthyTally(t *testing.T) {
	assert.Empty(t, problemsIn(map[reflect.Type]bool{reflect.TypeOf(localrt.Status{}): true}, nil, 0))
}

// The escape hatch the two sibling guards each needed.
func TestProblemsInHonorsNotAPayload(t *testing.T) {
	type fixtureOnly struct {
		A string `json:"a"`
	}
	name := reflect.TypeOf(fixtureOnly{}).String()

	notAPayload[name] = "a test fixture, not a published shape"
	t.Cleanup(func() { delete(notAPayload, name) })

	assert.Empty(t, problemsIn(map[reflect.Type]bool{reflect.TypeOf(fixtureOnly{}): true}, nil, 0))
}

// A self-referential type must not hang the walk. `type Loop []Loop` is
// legal Go and Elem() on it returns Loop forever.
func TestPayloadTypeSurvivesASelfReferentialType(t *testing.T) {
	type Loop []Loop
	assert.Nil(t, payloadType(reflect.TypeOf(Loop{})))
}

// A map is not judged here, and the message says so rather than pretending
// maps are fine.
func TestPayloadTypeSkipsShapesItDoesNotJudge(t *testing.T) {
	assert.Nil(t, payloadType(reflect.TypeOf(map[string]string{})))
	assert.Nil(t, payloadType(reflect.TypeOf("")))
}

// Every problem names what to do about it, because a guard that only says
// "no" costs somebody an afternoon.
func TestEveryProblemSaysWhatToDo(t *testing.T) {
	type notPinnedAnywhere struct {
		A string `json:"a"`
	}
	got := problemsIn(
		map[reflect.Type]bool{reflect.TypeOf(notPinnedAnywhere{}): true},
		map[string]bool{"struct { A int }": true},
		99,
	)
	assert.Len(t, got, 3, "the floor, the named type and the anonymous one")
	for _, p := range got {
		assert.True(t,
			strings.Contains(p, "publishedPayloads") ||
				strings.Contains(p, "minWatchedPayloads"),
			"a problem with no remedy in it: %q", p)
	}
}
