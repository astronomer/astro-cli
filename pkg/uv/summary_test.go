package uv

import (
	"strings"
	"testing"
)

// What a failing uv invocation says on one line, over stderr uv really wrote.
//
// The fixtures are captured output, not hand-written approximations of it: the
// bug this replaced came from reasoning about the shape of uv's errors rather
// than looking at one, and a fixture somebody composed would have had the same
// blind spot. Each was produced by running `uv --color never sync` against a
// project broken the named way.
func TestSummarizeNamesTheCause(t *testing.T) {
	for _, tc := range []struct {
		name string
		file string
		// want is a phrase from the cause the reader needs.
		want string
		// wasReported is what the last-line rule produced, asserted so this
		// stays a test about an improvement rather than about a string.
		wasReported string
	}{
		{
			name:        "a dependency that is not a PEP 508 requirement",
			file:        "build-failure",
			want:        "The build backend returned an error",
			wasReported: "environment.",
		},
		{
			// The one case where this trades rather than wins. The old rule
			// landed on "unclosed table, expected `]`", which says what is
			// wrong but not where or in which file; this says the file and
			// the position but not the complaint, because the complaint is
			// printed below uv's cause as part of the TOML renderer's own
			// caret diagram and reaching it means parsing that diagram.
			// Taken because the position is what a parse error is looked up
			// by, and the diagram is on screen either way.
			name:        "a pyproject.toml that will not parse",
			file:        "bad-pyproject",
			want:        "Failed to parse: `pyproject.toml`: TOML parse error at line 1, column 9",
			wasReported: "unclosed table, expected `]`",
		},
		{
			name:        "no interpreter for the requested python",
			file:        "no-interpreter",
			want:        "No interpreter found for Python >=3.99",
			wasReported: "hint: uv embeds available Python downloads and may require an update to install new versions. Consider retrying on a newer version of uv.",
		},
		{
			// The shape an older uv writes, which is what CI runs. Kept
			// because knowing only the newer spelling made this case report
			// the headline alone — a solve failed, without saying what could
			// not be solved. The e2e case that asserts a failed start names
			// the package is what caught it.
			name:        "requirements that cannot be solved, on an older uv",
			file:        "no-solution-cause-prefix",
			want:        "astro-e2e-no-such-package-8f3a1c was not found in the package registry",
			wasReported: "cause: Because astro-e2e-no-such-package-8f3a1c was not found in the package registry and your project depends on astro-e2e-no-such-package-8f3a1c==9.9.9, we can conclude that your project's requirements are unsatisfiable.",
		},
		{
			name:        "requirements that cannot be solved",
			file:        "no-solution",
			want:        "we can conclude that your project's requirements are unsatisfiable",
			wasReported: "your project's requirements are unsatisfiable.",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stderr := readFixture(t, tc.file)

			if got := lastLine(stderr); got != tc.wasReported {
				t.Fatalf("the fixture no longer reproduces the old behavior: lastLine = %q, want %q", got, tc.wasReported)
			}
			got := summarize(stderr)
			if !strings.Contains(got, tc.want) {
				t.Errorf("summary does not carry %q:\n  %s", tc.want, got)
			}
			// A hint is uv telling the reader what usually causes this, which
			// is not what happened and is what the last-line rule kept
			// picking.
			if strings.Contains(got, "hint:") {
				t.Errorf("summary is a hint rather than a cause:\n  %s", got)
			}
		})
	}
}

// The rendered error is the one a caller sees, so it is asserted through
// CommandError rather than only through summarize.
//
// The existing rendering case cannot stand in for this: its stderr ends with
// the error line, so last-line and summary agree on it and Error() reverting
// to the old rule would keep it green. This drives stderr where the two
// disagree, which is the only kind that can tell them apart.
func TestCommandErrorReportsTheCauseNotTheLastLine(t *testing.T) {
	err := &CommandError{Args: []string{"sync"}, ExitCode: 1, Stderr: readFixture(t, "build-failure")}

	got := err.Error()
	if !strings.HasPrefix(got, "uv sync failed (exit 1): ") {
		t.Errorf("Error() lost its prefix:\n  %s", got)
	}
	if !strings.Contains(got, "The build backend returned an error") {
		t.Errorf("Error() does not name the cause:\n  %s", got)
	}
	if strings.HasSuffix(got, "environment.") {
		t.Errorf("Error() is still reporting the wrapped tail of the hint:\n  %s", got)
	}
}

// The wrapped block, unwrapped, exactly — the case the change was made for.
//
// Asserted whole rather than by substring because the joining is the thing
// under test: the headline continues onto a "│" line, the innermost cause
// continues onto an indented one, and both have to come back as one sentence
// with the causes in order.
func TestDiagnosticUnwrapsTheBlock(t *testing.T) {
	got := diagnostic(readFixture(t, "build-failure"))
	want := "Failed to build `capture @ file:///home/dev/project`: " +
		"The build backend returned an error: " +
		"Call to `setuptools.build_meta:__legacy__.build_wheel` failed (exit status: 1)"
	if got != want {
		t.Errorf("diagnostic() =\n  %s\nwant\n  %s", got, want)
	}
}

// The block ends at the blank line, and everything past it is the build
// backend's own output — a Python traceback here, ninety lines of it. Pinned
// because the fixture contains that traceback: a parser that ran on would
// summarize setuptools' stack rather than uv's error.
func TestDiagnosticStopsAtTheBackendsOutput(t *testing.T) {
	got := diagnostic(readFixture(t, "build-failure"))
	for _, absent := range []string{"Traceback", "[stdout]", "build_meta.py", "hint:"} {
		if strings.Contains(got, absent) {
			t.Errorf("summary ran past the block and picked up %q:\n  %s", absent, got)
		}
	}
}

// A headline ending in its own colon does not get a second one from the join.
func TestDiagnosticDoesNotDoubleTheColon(t *testing.T) {
	got := diagnostic("  × No solution found when resolving dependencies:\n  ╰─▶ Because nothing works\n")
	if want := "No solution found when resolving dependencies: Because nothing works"; got != want {
		t.Errorf("diagnostic() = %q, want %q", got, want)
	}
}

// A warning may say almost what the error says and must not be mistaken for
// one; a "Caused by:" belongs to the message above it, so one under a warning
// is not the error's cause.
func TestErrorLineTakesTheFirstErrorAndTheCausesUnderIt(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stderr string
		want   string
	}{
		{
			name:   "an error with a cause",
			stderr: "error: Failed to parse: `pyproject.toml`\n  Caused by: TOML parse error at line 1\n",
			want:   "error: Failed to parse: `pyproject.toml`: TOML parse error at line 1",
		},
		{
			name:   "an error with no cause",
			stderr: "error: disk full\n",
			want:   "error: disk full",
		},
		{
			name:   "a warning before the error",
			stderr: "warning: something\nerror: the real problem\n",
			want:   "error: the real problem",
		},
		{
			name:   "a warning that carries its own cause",
			stderr: "warning: could not read it\n  Caused by: not the failure\nerror: the real problem\n",
			want:   "error: the real problem",
		},
		{
			name:   "an error whose cause has its own cause",
			stderr: "error: Failed to prepare\n  Caused by: could not read the wheel\n  Caused by: permission denied\n",
			want:   "error: Failed to prepare: could not read the wheel: permission denied",
		},
		{
			// Which of two errors is reported is a choice; see errorLine.
			// Pinned so that reversing it is a visible decision rather than a
			// silent one.
			name:   "a second error after the first",
			stderr: "error: could not read foo\nerror: 1 package failed\n",
			want:   "error: could not read foo",
		},
		{
			name:   "no error at all",
			stderr: "warning: something\n",
			want:   "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := errorLine(tc.stderr); got != tc.want {
				t.Errorf("errorLine() = %q, want %q", got, tc.want)
			}
		})
	}
}

// Stderr in no shape this knows still says something, which is what every
// shape got before there were shapes.
func TestSummarizeFallsBackToTheLastLine(t *testing.T) {
	if got, want := summarize("something\nuv has never printed\n"), "uv has never printed"; got != want {
		t.Errorf("summarize() = %q, want the last line %q", got, want)
	}
	if got := summarize("   \n\n  "); got != "" {
		t.Errorf("summarize(blank) = %q, want empty", got)
	}
}

// uv's own "error:" line beats a diagnostic block when both are present.
//
// They never co-occur in uv's own output, so stderr carrying both did not all
// come from uv. `uv run` is the case: the capture is then the caller's
// PROGRAM's stderr, and a Python CLI that draws boxes — Rich and Typer both
// do, and Airflow uses Rich — emits the same glyphs a uv diagnostic uses.
// Reading the block first let the child's own framing displace the error uv
// actually reported.
func TestUVsOwnErrorBeatsABlockFromSomewhereElse(t *testing.T) {
	stderr := "Using CPython\n" +
		"× Something the child printed\n" +
		"more child output\n" +
		"\n" +
		"error: uv run failed for the real reason\n"
	if got, want := summarize(stderr), "error: uv run failed for the real reason"; got != want {
		t.Errorf("summarize() = %q, want %q", got, want)
	}
}

// A cause belongs to the error directly above it.
//
// The causes read are the ones immediately under the error, and the run stops
// at the first line that is not one. Collecting every cause in the capture
// instead read a second error's cause as the first one's, and read a cause
// under a trailing warning as the error's — the second of which the function's
// own doc already promised not to do, while the code only avoided it for a
// warning that came first.
func TestCausesBelongToTheErrorAboveThem(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stderr string
		want   string
	}{
		{
			name:   "a second error further down",
			stderr: "error: A failed\n  Caused by: a1\n\nerror: B failed\n  Caused by: b1\n",
			want:   "error: A failed: a1",
		},
		{
			name:   "a warning with its own cause after the error",
			stderr: "error: the real problem\n  Caused by: real cause\nwarning: also this\n  Caused by: not the failure\n",
			want:   "error: the real problem: real cause",
		},
		{
			name:   "the caret diagram uv prints under a parse error",
			stderr: "error: Failed to parse\n  Caused by: TOML parse error at line 1\n  |\n1 | [project\n  cause: not a real one\n",
			want:   "error: Failed to parse: TOML parse error at line 1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := errorLine(tc.stderr); got != tc.want {
				t.Errorf("errorLine() = %q, want %q", got, tc.want)
			}
		})
	}
}

// One colon comes off, not a run of them.
//
// uv writes messages that end in their own colon and the join adds one, so a
// trailing colon is dropped before joining. Dropping the whole run instead ate
// a colon belonging to the text: a headline ending in a colon-terminated token
// lost it and the reader saw different content, not just different
// punctuation.
func TestJoinKeepsAColonThatIsPartOfTheText(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stderr string
		want   string
	}{
		{
			name:   "one trailing colon is the join's",
			stderr: "  × No solution found when resolving dependencies:\n  ╰─▶ Because nothing works\n",
			want:   "No solution found when resolving dependencies: Because nothing works",
		},
		{
			name:   "a second colon belongs to the text",
			stderr: "  × Ratio a:b::\n  ╰─▶ cause\n",
			want:   "Ratio a:b:: cause",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := diagnostic(tc.stderr); got != tc.want {
				t.Errorf("diagnostic() = %q, want %q", got, tc.want)
			}
		})
	}
}

// A line that is not indented is not part of the block.
//
// Only a blank line, a hint and a help otherwise end it, and output
// interleaved onto the same descriptor by something else has none of those —
// so without this the block ran on and glued unrelated prose into the summary.
func TestTheBlockEndsAtAnUnindentedLine(t *testing.T) {
	if got, want := diagnostic("  × head\nnot indented at all\n  ╰─▶ cause\n"), "head"; got != want {
		t.Errorf("diagnostic() = %q, want %q", got, want)
	}
}

// A nested block carries the outer one's bar down its left edge, and the
// arrow beneath it is an arrow rather than prose.
func TestANestedArrowIsNotReadAsText(t *testing.T) {
	if got, want := diagnostic("  × head\n  │ ├─▶ nested cause\n"), "head: nested cause"; got != want {
		t.Errorf("diagnostic() = %q, want %q", got, want)
	}
}
