package uv

import (
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
	if line := lastLine(e.Stderr); line != "" {
		msg += ": " + line
	}
	return msg
}

func (e *CommandError) Unwrap() error { return e.Err }

// globalFlagValues are the global flags that take a separate value, so the
// word following one of them is not the subcommand either.
var globalFlagValues = map[string]bool{"--color": true}

// verb is the uv subcommand, skipping the global flags the runner prepends and
// any value they carry.
func (e *CommandError) verb() string {
	for i := 0; i < len(e.Args); i++ {
		a := e.Args[i]
		if !strings.HasPrefix(a, "-") {
			return a
		}
		if globalFlagValues[a] {
			i++
		}
	}
	return strings.Join(e.Args, " ")
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
