package local

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/instancelocate"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// sampleFor is one real error per kind, and the single list every test below
// works from.
//
// One list rather than three. The first version of this file had the kinds
// written out in the const block, again in the table, and again in each test,
// and a test that counted two of those against each other. A seventh kind
// could be declared, mapped and shipped while the test that actually proves
// the predicate matches a wrapped error never ran against it — which is the
// one thing worth proving. Driving every case off the table, and requiring an
// entry here for each row, means a new kind cannot be added without a sample.
func sampleFor(kind cliout.ProblemKind) (error, bool) {
	samples := map[cliout.ProblemKind]error{
		KindNoProject:                &project.NotFoundError{Start: "/somewhere"},
		KindForeignMode:              localrt.ErrForeignMode,
		KindAlreadyRunning:           localrt.ErrAlreadyRunning,
		KindHealthTimeout:            localrt.ErrHealthTimeout,
		KindUnsupportedBase:          localrt.ErrUnsupportedBase,
		KindDatabaseNewerThanAirflow: localrt.ErrDatabaseNewerThanAirflow,
		KindLocked:                   localrt.ErrStartInProgress,
		KindNotRunning:               localrt.ErrNotRunning,
		KindDeploymentHibernating:    &instancelocate.UnavailableError{State: instancelocate.ErrDeploymentHibernating},
		KindDeploymentDeploying:      &instancelocate.UnavailableError{State: instancelocate.ErrDeploymentDeploying},
		KindDeploymentUnhealthy:      &instancelocate.UnavailableError{State: instancelocate.ErrDeploymentUnhealthy},
		KindAirflowUnavailable:       &instancelocate.UnavailableError{State: instancelocate.ErrAirflowUnavailable},
		KindUnsupportedOnPlatform:    &scaffold.Convert1xUnderAPCError{Dir: "/somewhere"},
	}
	s, ok := samples[kind]
	return s, ok
}

// Every kind in the table resolves from a real error, through a wrap.
//
// Through a wrap because that is how they arrive: nothing returns a bare
// sentinel. A start that runs out of time returns the engine's message wrapping
// ErrHealthTimeout, and cmd/local wraps that again to name the environment
// variable. A resolver matching only the bare value would publish no kind for
// any real failure.
func TestEveryProblemKindResolvesThroughAWrap(t *testing.T) {
	for _, p := range ProblemKinds {
		t.Run(string(p.Kind), func(t *testing.T) {
			sample, ok := sampleFor(p.Kind)
			if !ok {
				t.Fatalf("no sample error for %q; add one so this kind is actually exercised", p.Kind)
			}
			wrapped := fmt.Errorf("while starting: %w; and some advice", sample)
			if got := ProblemKinds.Of(wrapped); got != p.Kind {
				t.Errorf("ProblemKinds.Of() = %q, want %q", got, p.Kind)
			}
		})
	}
}

// The order the table is written in is the order it resolves in.
//
// errors.Is walks every branch of a wrap and of an errors.Join, which this tree
// uses, so one error can match more than one row and the first wins. Nothing
// about the code says which order that should be, so tidying the table — to
// match the const block, say — would silently change what a joined failure
// publishes. Pinned here so it cannot.
func TestProblemKindOrderIsPinned(t *testing.T) {
	want := []cliout.ProblemKind{
		KindNoProject,
		KindForeignMode,
		KindAlreadyRunning,
		KindHealthTimeout,
		KindUnsupportedBase,
		KindDatabaseNewerThanAirflow,
		KindLocked,
		KindNotRunning,
		KindDeploymentHibernating,
		KindDeploymentDeploying,
		KindDeploymentUnhealthy,
		KindAirflowUnavailable,
		KindUnsupportedOnPlatform,
	}
	if len(ProblemKinds) != len(want) {
		t.Fatalf("%d kinds in the table, %d pinned here", len(ProblemKinds), len(want))
	}
	for i, p := range ProblemKinds {
		if p.Kind != want[i] {
			t.Errorf("row %d is %q, pinned as %q", i, p.Kind, want[i])
		}
	}
}

// The more specific of two matching kinds wins.
//
// A start refused because the record belongs to the other mode carries both
// complaints; "already running" is true but unhelpful, because stopping and
// starting again will not fix the mode. This is the case the order exists for,
// so it is asserted rather than left to the pin above.
func TestTheMoreSpecificKindWinsWhenBothMatch(t *testing.T) {
	both := errors.Join(localrt.ErrAlreadyRunning, localrt.ErrForeignMode)
	if got := ProblemKinds.Of(both); got != KindForeignMode {
		t.Errorf("ProblemKinds.Of() = %q, want %q — the mode is why a restart will not help", got, KindForeignMode)
	}
}

// Kinds are snake_case, like the kind a check finding already publishes.
//
// A consumer meeting import_error from `astro local check` and healthTimeout
// from a failed start would have to learn that they are the same idea spelled
// two ways. Pinned rather than trusted, because the next kind is added by
// somebody who has not read this file.
func TestProblemKindsAreSpelledLikeCheckFindings(t *testing.T) {
	for _, p := range ProblemKinds {
		k := string(p.Kind)
		if k != strings.ToLower(k) || strings.ContainsAny(k, " -.") {
			t.Errorf("kind %q should be lower snake_case, as check findings are", k)
		}
	}
}

// An unclassified failure publishes no kind at all.
//
// Rather than "unknown", which reads like an answer. A consumer that branches
// on kind should fall through to the prose when there is nothing to branch on,
// and an absent field is the only thing that says so unambiguously.
func TestAnUnclassifiedFailurePublishesNoKind(t *testing.T) {
	if got := ProblemKinds.Of(errors.New("the manifest could not be parsed")); got != "" {
		t.Errorf("ProblemKinds.Of() = %q, want empty", got)
	}
	if got := ProblemKinds.Of(nil); got != "" {
		t.Errorf("ProblemKinds.Of(nil) = %q, want empty", got)
	}
}

// emitJSONError fails a json-mode command with err and reports it the way
// every command's failure is reported — through cliout.Execute, with this
// package's table — so the object asserted on is the one a user would see.
func emitJSONError(w io.Writer, err error) {
	var output cliout.Format
	cmd := &cobra.Command{
		Use:  "fail",
		RunE: func(*cobra.Command, []string) error { return err },
	}
	cliout.AddOutputFlag(cmd, &output)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cliout.Execute(context.Background(), cmd, []string{"-o", "json"}, w, ProblemKinds)
}

// And the kind reaches the object a failed command actually writes.
//
// The resolver being right is worth nothing if the error report does not ask it —
// which is the way wiring fails: nothing breaks, the field is simply never
// there. Asserted on the emitted JSON rather than on the struct, because the
// field name and its omitempty are part of what is being promised.
func TestTheEmittedErrorObjectCarriesTheKind(t *testing.T) {
	t.Run("a classified failure", func(t *testing.T) {
		var out bytes.Buffer
		emitJSONError(&out, fmt.Errorf("a start gave up: %w", localrt.ErrHealthTimeout))

		var got cliout.ErrorObject
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("the emitted object is not JSON: %v\n%s", err, out.String())
		}
		if got.Kind != KindHealthTimeout {
			t.Errorf("kind = %q, want %q\n%s", got.Kind, KindHealthTimeout, out.String())
		}
		if !strings.Contains(got.Error, "a start gave up") {
			t.Errorf("the prose should survive beside the kind, got %q", got.Error)
		}
	})

	t.Run("an unclassified one omits the field entirely", func(t *testing.T) {
		var out bytes.Buffer
		emitJSONError(&out, errors.New("something else went wrong"))

		if strings.Contains(out.String(), "kind") {
			t.Errorf("an unclassified failure should publish no kind key:\n%s", out.String())
		}
	})
}

// The first failure a consumer meets is named.
//
// Running the built binary in an empty directory is how this turned up: every
// other kind presumes you are in a project, and "no pyproject.toml found in X
// or any parent directory" — the thing a script hits before anything else —
// published no kind at all.
func TestNotBeingInAProjectIsNamed(t *testing.T) {
	var out bytes.Buffer
	emitJSONError(&out, &project.NotFoundError{Start: "/tmp/somewhere"})

	var got cliout.ErrorObject
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("the emitted object is not JSON: %v\n%s", err, out.String())
	}
	if got.Kind != KindNoProject {
		t.Errorf("kind = %q, want %q\n%s", got.Kind, KindNoProject, out.String())
	}
	// The directory it searched from is what makes the prose useful, and it
	// has to survive beside the kind rather than be replaced by it.
	if !strings.Contains(got.Error, "/tmp/somewhere") {
		t.Errorf("the message should still name where it looked, got %q", got.Error)
	}
}

// The CLI's own "nothing is running" errors carry the kind too.
//
// The same user-visible condition reached three commands three ways: `stop`
// through Attach's sentinel, the query commands through an unexported prose
// error, and `open` through a fresh fmt.Errorf. Only the first published a
// kind, so a consumer branching on not_running handled `stop` and silently
// missed every query. Asserted on the package's own error value, because the
// wrapping is what was missing and a table of sentinels cannot see it.
func TestTheCLIsOwnNotRunningErrorsAreNamed(t *testing.T) {
	if got := ProblemKinds.Of(errNoLocalAirflow); got != KindNotRunning {
		t.Errorf("the query commands' error resolves to %q, want %q", got, KindNotRunning)
	}
	// open builds its own, naming the state it found.
	opened := fmt.Errorf("%w: local Airflow is %s; run `x` first", localrt.ErrNotRunning, "stopped")
	if got := ProblemKinds.Of(opened); got != KindNotRunning {
		t.Errorf("open's error resolves to %q, want %q", got, KindNotRunning)
	}
}

// A pyproject.toml without [tool.astro] is no astro project either, and says so
// under the same kind as a directory with no pyproject.toml at all.
func TestNoAstroSectionIsNoProject(t *testing.T) {
	err := fmt.Errorf("start: %w", &project.NoAstroSectionError{Dir: "/somewhere"})
	if got := ProblemKinds.Of(err); got != KindNoProject {
		t.Fatalf("ProblemKinds.Of = %q, want %q", got, KindNoProject)
	}
}
