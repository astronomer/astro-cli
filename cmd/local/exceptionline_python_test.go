package local

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// This test asks a real Python for tracebacks and for the answer, instead of
// comparing against text somebody typed into a fixture.
//
// Every other case in check_test.go is captured output, which pins today's
// behavior and cannot notice tomorrow's: a CPython that rewords a traceback
// leaves those tests green and reaches us through a bug report. exceptionLine
// matches on five pieces of Python's rendering — the exception-line shape, the
// ExceptionGroup gutter, the group banner, its sub-exception count, and the two
// chain sentences — so drift is a question of when, not whether.
//
// The expected answer is computed by the generator, per run, from the live
// exception OBJECT: its type's qualified name, used to pick out the line
// Python wrote about it. exceptionLine reaches the same answer from the
// rendered TEXT, knowing nothing about the object.
//
// Two routes to one answer from two different inputs is the whole design. An
// expectation that read the text the way the parser does would agree with the
// parser by construction, including when both were wrong; taking it off the
// object instead means the two can genuinely disagree — and when CPython
// rewrites its rendering, the traceback and the expectation move together while
// the parser does not, so the disagreement is the drift. That is the signal a
// captured fixture cannot give, because a captured fixture is a copy of what
// the parser already did.
//
// It skips when no interpreter new enough for ExceptionGroup (3.11+) is
// reachable, so it is silent on a machine that cannot run it and real on the
// ones that can — CI's ubuntu runner ships 3.12, and uv is a prerequisite of
// this CLI anyway.
func TestExceptionLineAgainstRealPython(t *testing.T) {
	name, args := findPython(t)

	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(tracebackGenerator)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("generating tracebacks failed: %v\n%s", err, stderr.String())
	}

	type genCase struct {
		Name      string `json:"name"`
		Traceback string `json:"traceback"`
		Want      string `json:"want"`
	}
	var cases []genCase
	for line := range strings.SplitSeq(strings.TrimSpace(stdout.String()), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var c genCase
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			t.Fatalf("generator emitted a non-JSON line: %v: %q", err, line)
		}
		cases = append(cases, c)
	}
	// A generator that silently produced nothing would pass every assertion
	// below, so the count is checked rather than assumed.
	if len(cases) < 14 {
		t.Fatalf("want at least 14 generated cases, got %d — did the generator fail quietly?\n%s",
			len(cases), stderr.String())
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			got := exceptionLine(c.Traceback)
			if got != c.Want {
				t.Errorf("exceptionLine disagrees with Python about its own traceback\n got:  %q\n want: %q\n\ntraceback:\n%s",
					got, c.Want, c.Traceback)
			}
			// Whatever we return has to be a line Python actually wrote, read
			// through the same gutter stripping the parser uses. This catches a
			// rewording that leaves us returning the banner, a frame, or
			// something assembled rather than quoted.
			if !isLineOf(got, c.Traceback) {
				t.Errorf("returned a line that is not in the traceback: %q", got)
			}
			if strings.HasPrefix(got, "Traceback (most recent call last)") {
				t.Errorf("returned the banner rather than an exception: %q", got)
			}
		})
	}
}

// isLineOf reports whether s is one of the traceback's own lines, compared the
// way exceptionLine reads them.
func isLineOf(s, traceback string) bool {
	for line := range strings.SplitSeq(traceback, "\n") {
		if body, _ := stripGutter(strings.TrimRight(line, "\r")); body == s {
			return true
		}
	}
	return false
}

// findPython returns a command that runs a Python new enough to have
// ExceptionGroup, or skips the test.
//
// The interpreter on PATH first, then uv — which this CLI already requires for
// standalone mode, so a developer machine that cannot run this test is one that
// cannot run `astro local start` either.
func findPython(t *testing.T) (name string, args []string) {
	t.Helper()
	for _, candidate := range []string{"python3", "python"} {
		path, err := exec.LookPath(candidate)
		if err != nil {
			continue
		}
		if hasExceptionGroup(path) {
			return path, []string{"-"}
		}
	}
	if path, err := exec.LookPath("uv"); err == nil {
		return path, []string{"run", "--python", "3.12", "--no-project", "python", "-"}
	}
	t.Skip("no Python 3.11+ and no uv: cannot generate real tracebacks")
	return "", nil
}

// hasExceptionGroup reports whether an interpreter is new enough to raise one
// (3.11+), asked by trying rather than by parsing a version string.
func hasExceptionGroup(python string) bool {
	return exec.Command(python, "-c", "ExceptionGroup").Run() == nil
}

// tracebackGenerator prints one JSON object per line: a real traceback and the
// line that names its exception, both from Python.
//
// Two cases declare an expectation of their own, and they are exactly the two
// places our policy departs from Python's summary — worth reading as the
// clearest statement of that policy anywhere in the tree:
//
//   - A group of ONE reports its sub-exception. "ExceptionGroup: eg (1
//     sub-exception)" names a container; "ValueError: 1" names what broke.
//   - An outer group of one wrapping a group of SEVERAL reports the inner one,
//     for the same reason applied twice: the outer says nothing the inner does
//     not say better, and the inner has a count worth printing.
const tracebackGenerator = `
import json
import sys
import traceback


def type_name(exc):
    t = type(exc)
    if t.__module__ in ("builtins", "__main__"):
        return t.__qualname__
    return t.__module__ + "." + t.__qualname__


def exception_line(exc):
    # The line Python wrote that names THIS exception's type, found by the name
    # off the object rather than by position. Position does not work: the
    # exception line is last for a plain error, first for a multi-line message
    # like SQLAlchemy's, and last again for a SyntaxError whose rendering puts
    # the source and caret above it. Nor can it be built from the type and
    # str(exc), since CPython renders SyntaxError specially.
    #
    # Starting from the type off the OBJECT is what keeps this independent of
    # the Go parser it is checked against. Matching the text the way that parser
    # does would agree with it by construction, including when both are wrong.
    # format_exception_only yields CHUNKS, not lines: a message running to
    # several lines arrives as one element with newlines inside it. The reported
    # line is the first physical line of the chunk that names the type, because
    # a table row is one line and the rest of the message is detail the frames
    # carry. That rule is the contract, not a restatement of the parser: the
    # chunk is found by the type name off the object, and only then trimmed.
    name = type_name(exc)
    chunks = [c.rstrip("\n") for c in traceback.format_exception_only(exc)]
    notes = getattr(exc, "__notes__", None) or []
    if notes:
        chunks = chunks[: len(chunks) - len(notes)]
    for chunk in reversed(chunks):
        if chunk == name or chunk.startswith(name + ":"):
            return chunk.split("\n")[0]
    return chunks[-1].split("\n")[0]


def emit(name, exc, want=None):
    print(json.dumps({
        "name": name,
        "traceback": "".join(traceback.format_exception(exc)),
        "want": want if want is not None else exception_line(exc),
    }))


def caught(fn):
    try:
        fn()
    except BaseException as e:
        return e
    raise AssertionError("expected a raise: " + name)


def raises(exc):
    def go():
        raise exc
    return go


def plain():
    import definitely_not_a_real_module_xyz  # noqa: F401


def syntax_err():
    compile("def f(:\n    pass\n", "<gen>", "exec")


def chained():
    try:
        raise ValueError("handled")
    except ValueError:
        raise RuntimeError("gave up")


def caused():
    try:
        raise ValueError("root")
    except ValueError as e:
        raise RuntimeError("wrapped") from e


def noted():
    e = ValueError("the real problem")
    e.add_note("a note, which is not the exception line")
    raise e


def two_groups():
    try:
        raise ExceptionGroup("first", [ValueError(1), TypeError(2)])
    except BaseException:
        raise ExceptionGroup("second", [KeyError(3), IndexError(4), OSError(5)])


def group_then_plain():
    try:
        raise ExceptionGroup("handled", [ValueError(1), TypeError(2)])
    except BaseException:
        raise RuntimeError("what actually failed")


# A dotted type name, which is what the exceptions this product actually meets
# look like: airflow.exceptions.AirflowTaskTimeout,
# airflow.exceptions.AirflowDagDuplicatedIdException. Every builtin above
# renders bare, so without this the generated set cannot notice a rule that
# stops accepting dots.
class ProviderError(Exception):
    pass


ProviderError.__module__ = "acme.providers.warehouse"


# A message whose own text runs to several lines, the last of them a URL. This
# is SQLAlchemy's shape, and the reason a "last line of the traceback" rule was
# wrong: it reported the URL as the exception.
class BackendError(Exception):
    pass


BackendError.__module__ = "sqlalchemy.exc"


# A message with an indented detail block under it — pydantic's shape, where a
# positional rule returned a bare field name.
class ValidationError(Exception):
    pass


ValidationError.__module__ = "pydantic_core"


emit("plain import error", caught(plain))
emit("syntax error", caught(syntax_err))
emit("dotted type name", caught(raises(ProviderError("the warehouse refused the connection"))))
emit("multi-line message ending in a url", caught(raises(BackendError(
    "(sqlite3.OperationalError) no such table: dag\n"
    "[SQL: SELECT dag.dag_id FROM dag]\n"
    "(Background on this error at: https://sqlalche.me/e/20/e3q8)"))))
emit("multi-line message ending in a bare url", caught(raises(BackendError(
    "could not reach the warehouse\n"
    "retrying will not help\n"
    "https://example.com/docs/errors#e123"))))
emit("message with an indented detail block", caught(raises(ValidationError(
    "2 validation errors for Settings\ndb_url\n  Field required\napi_key\n  Field required"))))
emit("chained traceback", caught(chained))
emit("explicit cause", caught(caused))
emit("exception with a note", caught(noted))
emit("group of three", caught(raises(
    ExceptionGroup("eg", [ValueError(1), TypeError(2), KeyError(3)]))))
emit("chain of two groups", caught(two_groups))
emit("handled group then a plain failure", caught(group_then_plain))

one = caught(raises(ExceptionGroup("eg", [ValueError(1)])))
emit("group of one", one, want=exception_line(one.exceptions[0]))

nested = caught(raises(
    ExceptionGroup("outer", [ExceptionGroup("inner", [ValueError(1), TypeError(2)])])))
emit("group of one wrapping a group of several", nested,
     want=exception_line(nested.exceptions[0]))

sys.stdout.flush()
`
