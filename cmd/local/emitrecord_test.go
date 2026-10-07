package local

import (
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/cmd/cliout/cliouttest"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

// What a command actually emits, watched at the door by cliouttest.Watch,
// which says what it checks and what it cannot see.
//
// The three guards over this contract fail in different directions: the
// watch sees only what the suite runs, and a payload behind an untested path
// is not recorded, silently. The static TestEveryJSONPayloadTypeIsPinned
// over-reports instead. Neither is a superset of the other, which is why
// both stay.

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

// emitWatch is this package's configuration of the observer TestMain arms.
// The error object every tree's failures publish is pinned here, so nothing
// is pinned elsewhere.
func emitWatch() cliouttest.Watch {
	cases := make([]cliouttest.Case, len(publishedPayloads))
	for i, c := range publishedPayloads {
		cases[i] = c.shared()
	}
	return cliouttest.Watch{
		Cases:       cases,
		NotAPayload: notAPayload,
		Floor:       minWatchedPayloads,
		File:        "cmd/local/schema_cases_test.go",
	}
}

// TestMain arms the observer for the package run and checks the tally
// afterwards. A single test cannot: the point is what every OTHER test
// emitted.
func TestMain(m *testing.M) {
	watching := emitWatch().Arm()
	os.Exit(watching.Finish(m.Run()))
}

// This package's cases reach the watch through schemaCase.shared(): a
// payload pinned here must read as pinned to it.
func TestAPinnedPayloadIsPinnedToTheWatch(t *testing.T) {
	assert.Empty(t, emitWatch().Problems(map[reflect.Type]bool{reflect.TypeOf(localrt.Status{}): true}, nil, 0))
}
