package uv

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrNotFound reports that no uv binary was discovered. The wrapping error
// from New says where was searched and how to fix it.
var ErrNotFound = errors.New("uv not found")

// VersionError reports a discovered uv older than MinVersion.
type VersionError struct {
	Bin     string
	Version string
	Min     string
}

func (e *VersionError) Error() string {
	return fmt.Sprintf("uv %s at %s is older than the minimum supported %s; upgrade uv", e.Version, e.Bin, e.Min)
}

// interruptedError reports a uv invocation cut short by its context.
//
// It exists because the summary was wrong for this case and could not be made
// right: uv is killed partway through, so what it wrote is a half-finished
// progress line rather than a reason, and the summariser — correctly, with no
// diagnostic to find — falls back to that line. "uv sync failed (exit -1):
// Creating virtual environment at: .venv" describes a failure that did not
// happen and blames a step that was going fine.
//
// Canceled and DeadlineExceeded are told apart, because they are not the same
// event to the person reading: one is somebody pressing Ctrl-C, the other is a
// bound the caller set being reached, and "interrupted" is only true of the
// first. An embedder wrapping a sync in context.WithTimeout gets a message
// that says so.
//
// The CommandError is kept rather than replaced. A cancel can land while uv is
// in the middle of a real failure, and a caller that reaches for the exit code
// or the captured stderr — cmd/local/preflight.go asks errors.As for a
// *ResolutionError to build a constraint conflict — should still find it.
// Unwrap returns both, so errors.Is(err, context.Canceled) and
// errors.As(err, &cmdErr) are both true.
type interruptedError struct {
	// Op is the uv verb that was running.
	Op string
	// Err is context.Canceled or context.DeadlineExceeded.
	Err error
	// Cmd is what the invocation itself reported, kept for a caller that
	// wants the exit code or the stderr behind it.
	Cmd *CommandError
}

func (e *interruptedError) Error() string {
	if errors.Is(e.Err, context.DeadlineExceeded) {
		return "uv " + e.Op + " ran out of time"
	}
	return "uv " + e.Op + " was interrupted"
}

// Unwrap returns both causes: the context's error and the command's own.
func (e *interruptedError) Unwrap() []error {
	if e.Cmd == nil {
		return []error{e.Err}
	}
	return []error{e.Err, e.Cmd}
}

// verb is the uv subcommand in an argument list, skipping the global flags the
// runner prepends and any value they carry.
func verb(args []string) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			return a
		}
		if globalFlagValues[a] {
			i++
		}
	}
	return strings.Join(args, " ")
}

// CommandError reports a uv invocation that failed. Stderr always carries
// the (bounded) captured stderr, whatever the failure shape.
type CommandError struct {
	// Args is the uv argument list, including any --no-config.
	Args []string
	// ExitCode is the process exit code, or -1 when it never ran.
	ExitCode int
	// Stderr is the captured stderr tail.
	Stderr string
	// Err is the underlying exec error.
	Err error
}

func (e *CommandError) Error() string {
	msg := fmt.Sprintf("uv %s failed (exit %d)", e.verb(), e.ExitCode)
	if line := summarize(e.Stderr); line != "" {
		msg += ": " + line
	}
	return msg
}

func (e *CommandError) Unwrap() error { return e.Err }

// globalFlagValues are the global flags that take a separate value, so the
// word following one of them is not the subcommand either.
var globalFlagValues = map[string]bool{"--color": true}

func (e *CommandError) verb() string { return verb(e.Args) }

// summarize picks the part of uv's stderr worth putting on one line.
//
// uv fails in two shapes and the last line is the wrong answer for both. A
// build failure ends with "hint: This usually indicates a problem with the
// package or the build environment.", wrapped — so the last line was the word
// "environment." on its own, and the cause sat forty lines up. A missing
// interpreter ends with a hint too, one line below the message that says what
// was actually missing.
//
// The old last-line behavior stays as the floor: something is still printed
// for stderr in a shape this does not recognize, which is what every shape
// got before.
//
// uv's own "error:" line is preferred over a diagnostic block. The two never
// appear together in uv's own output — each captured fixture has one or the
// other — so stderr carrying both did not all come from uv. That happens: for
// `uv run` the capture is the caller's PROGRAM's stderr (see command), and a
// Python CLI that draws boxes emits the same glyphs a uv diagnostic does. When
// something in the pipe claims both shapes, the "error:" line is the one uv
// certainly wrote.
func summarize(stderr string) string {
	if e := errorLine(stderr); e != "" {
		return e
	}
	if d := diagnostic(stderr); d != "" {
		return d
	}
	return lastLine(stderr)
}

// joinCauses renders a message and what caused it as one sentence, without
// doubling a colon a part already ends with — uv writes plenty that do, from
// "No solution found when resolving dependencies:" to "Failed to parse
// `pyproject.toml` during settings discovery:".
//
// One colon, not a run of them: a part ending "a:b::" keeps the colon that
// belongs to its own text.
func joinCauses(parts []string) string {
	trimmed := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed = append(trimmed, strings.TrimSuffix(strings.TrimSpace(part), ":"))
	}
	return strings.Join(trimmed, ": ")
}

// diagnostic renders uv's error block — the "×" headline naming what failed
// and the "├─▶"/"╰─▶" causes under it — as one line.
//
// Long lines wrap, the headline's continuations marked with "│" and a cause's
// with plain indentation:
//
//	× Failed to build `capture @
//	│ file:///home/dev/project`
//	├─▶ The build backend returned an error
//	╰─▶ Call to `setuptools.build_meta:__legacy__.build_wheel` failed (exit
//	    status: 1)
//
// which is one sentence broken over five lines, and reads as
// "Failed to build `capture @ file:///home/dev/project`: The build backend
// returned an error: Call to `setuptools...` failed (exit status: 1)".
//
// Empty when there is no such block, which is how summarize knows to try the
// next shape.
func diagnostic(stderr string) string {
	var parts, current []string
	flush := func() {
		if len(current) > 0 {
			parts = append(parts, strings.Join(current, " "))
			current = nil
		}
	}
	started := false
scan:
	for _, line := range strings.Split(stderr, "\n") {
		// Every line of the block is indented, the marker included. An
		// unindented line is therefore not part of it, and is the boundary to
		// use for prose the block never terminated: only a blank line, a hint
		// and a help are otherwise treated as the end, and output interleaved
		// on the same descriptor has none of those.
		indented := line != strings.TrimLeft(line, " \t")
		text := strings.TrimSpace(line)
		// A nested block carries the outer one's bar down its left edge, so
		// the bar comes off before anything else is decided. Reading it as a
		// continuation instead would put "├─▶" in the middle of a sentence.
		if rest, found := strings.CutPrefix(text, "│"); found {
			text, indented = strings.TrimSpace(rest), true
		}
		switch {
		case strings.HasPrefix(text, "× "):
			flush()
			started = true
			current = []string{strings.TrimSpace(strings.TrimPrefix(text, "× "))}
		case !started:
			// Progress uv printed before it failed: "Using CPython 3.13.13",
			// "Creating virtual environment at: .venv".
		case strings.HasPrefix(text, "├─▶"), strings.HasPrefix(text, "╰─▶"):
			flush()
			_, after, _ := strings.Cut(text, "▶")
			current = []string{strings.TrimSpace(after)}
		case text == "", strings.HasPrefix(text, "hint:"), strings.HasPrefix(text, "help:"), !indented:
			// The block ends at the first blank line. Everything after it is
			// the build backend's own output — a Python traceback, in the case
			// this was written for — which is in Stderr for whoever wants it
			// and is not a summary of anything.
			break scan
		default:
			current = append(current, text)
		}
	}
	flush()
	return joinCauses(parts)
}

// errorLine renders uv's plainer failure: an "error:" line, with the
// "Caused by:" beneath it when there is one.
//
//	warning: Failed to parse `pyproject.toml` during settings discovery:
//	error: Failed to parse: `pyproject.toml`
//	  Caused by: TOML parse error at line 1, column 9
//
// A warning may say almost the same thing as the error that follows it, as
// above, and is skipped: only "error:" opens a message.
//
// A cause belongs to whatever is above it, so the causes read are the ones
// directly under the error and the run stops at the first line that is not
// one. Collecting every cause in the capture instead read a second error's
// cause as the first one's, and read a cause under a TRAILING warning as the
// error's — which this comment already said it would not do, while the code
// only avoided it for warnings that came first.
//
// Both spellings of a cause are read. Newer uv writes "Caused by:"; the
// version on CI writes "cause:", and nothing makes the two agree:
//
//	error: No solution found when resolving dependencies
//	  cause: Because astro-e2e-no-such-package was not found in the package
//
// Knowing only the newer spelling is not a smaller feature, it is a wrong
// answer — the headline alone says a solve failed without saying what could
// not be solved, which is the whole of what the reader needs. Found by the
// e2e case that asserts a failed start names the package, running against an
// older uv than the one these fixtures were captured from.
//
// Every cause under the error is kept, joined the same way the diagnostic
// block joins its "╰─▶" chain: uv is a Rust program and its error chains can
// be more than one link deep, and dropping all but one link means picking
// between the general end of the chain and the specific end. Keeping them
// reads as the chain it is.
//
// The FIRST error opens the message. No uv output captured here prints two,
// so that is a choice rather than an observation — made this way because
// where a tool does print several, the later ones are usually a tally
// ("2 packages failed") and the first is the one saying what broke.
func errorLine(stderr string) string {
	var message string
	var causes []string
	for _, line := range strings.Split(stderr, "\n") {
		text := strings.TrimSpace(line)
		if message == "" {
			if strings.HasPrefix(text, "error:") {
				message = text
			}
			continue
		}
		cause, found := causeText(text)
		if !found {
			break
		}
		causes = append(causes, cause)
	}
	if message == "" {
		return ""
	}
	return joinCauses(append([]string{message}, causes...))
}

// causeMarkers are uv's two spellings of "and the reason is", newest first.
var causeMarkers = []string{"Caused by:", "cause:"}

// causeText is the reason on this line, and whether the line carried one.
func causeText(line string) (string, bool) {
	for _, marker := range causeMarkers {
		if after, found := strings.CutPrefix(line, marker); found {
			return strings.TrimSpace(after), true
		}
	}
	return "", false
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}

// ResolutionError reports that uv's resolver found no set of versions
// satisfying the project's requirements. The structured fields are parsed
// best-effort from uv's prose output; a parse miss leaves them empty and
// the error degrades to the plain CommandError rendering — Stderr always
// carries everything uv said either way.
type ResolutionError struct {
	// Op is the uv verb that failed: "lock", "sync", or "run".
	Op string
	// Packages are the distinct package names in the conflict, in order
	// of first appearance.
	Packages []string
	// Constraints are the distinct requirement expressions in the
	// conflict, e.g. "flask>=2.2.1,<2.3", in order of first appearance.
	Constraints []string
	// Summary is the solver's explanation joined to one line; "" when
	// parsing missed.
	Summary string
	// Stderr is the raw captured stderr, always present.
	Stderr string
	// Err is the underlying *CommandError.
	Err error
}

func (e *ResolutionError) Error() string {
	if e.Summary != "" {
		return fmt.Sprintf("uv %s: no solution: %s", e.Op, e.Summary)
	}
	return e.Err.Error()
}

func (e *ResolutionError) Unwrap() error { return e.Err }

// noSolutionMarker is the stable phrase uv prints for solver failures.
const noSolutionMarker = "No solution found"

// asResolution upgrades a CommandError whose stderr shows a solver failure
// into a *ResolutionError; every other error passes through untouched.
func asResolution(op string, err error) error {
	var cmdErr *CommandError
	if err == nil || !errors.As(err, &cmdErr) || !strings.Contains(cmdErr.Stderr, noSolutionMarker) {
		return err
	}
	re := parseResolution(cmdErr.Stderr)
	re.Op = op
	re.Stderr = cmdErr.Stderr
	re.Err = cmdErr
	return re
}

// requirementRe matches a PEP 508 package name directly followed by a
// version specifier list, e.g. "flask>=2.2.1,<2.3" — the shape uv's solver
// prose uses for requirements.
var requirementRe = regexp.MustCompile(
	`([A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?)` +
		`((?:===|==|~=|!=|>=|<=|>|<)[0-9][^\s,]*(?:,(?:===|==|~=|!=|>=|<=|>|<)[0-9][^\s,]*)*)`)

// parseResolution extracts what it can from uv's "No solution found" prose.
func parseResolution(stderr string) *ResolutionError {
	re := &ResolutionError{Summary: extractSummary(stderr)}
	seenPkg := map[string]bool{}
	seenConstraint := map[string]bool{}
	for _, m := range requirementRe.FindAllStringSubmatch(re.Summary, -1) {
		name, spec := m[1], strings.TrimRight(m[2], ".")
		if constraint := name + spec; !seenConstraint[constraint] {
			seenConstraint[constraint] = true
			re.Constraints = append(re.Constraints, constraint)
		}
		if key := normalizeName(name); !seenPkg[key] {
			seenPkg[key] = true
			re.Packages = append(re.Packages, name)
		}
	}
	return re
}

// extractSummary pulls the solver's explanation — the block uv renders
// after the "╰─▶" arrow — and joins its wrapped lines into one, stopping at
// a blank line or a hint. Returns "" when the shape is not there.
//
// Deliberately narrower than diagnostic, which reads the same arrows: this
// returns the explanation ALONE because parseResolution runs a requirement
// regex over it to fill Packages and Constraints, and a headline folded in
// would be scanned for requirements too. The cost is that ResolutionError's
// message omits what diagnostic keeps — on the no-solution fixture, which
// dependency group failed to solve ("for split (markers: ...)") — so the two
// are not interchangeable and neither is redundant.
func extractSummary(stderr string) string {
	var parts []string
	inBlock := false
	for _, line := range strings.Split(stderr, "\n") {
		if !inBlock {
			if _, after, found := strings.Cut(line, "╰─▶"); found {
				inBlock = true
				parts = append(parts, strings.TrimSpace(after))
			}
			continue
		}
		text := strings.TrimSpace(line)
		if text == "" || strings.HasPrefix(text, "hint:") || strings.HasPrefix(text, "help:") {
			break
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, " ")
}

// normalizeName is PEP 503 name normalization, so "Flask", "flask" and
// "flask_wtf"/"flask-wtf" dedupe.
func normalizeName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "_", "-")
	return strings.ReplaceAll(name, ".", "-")
}
