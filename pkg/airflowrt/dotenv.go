package airflowrt

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// DotenvEntry is one assignment a .env file makes: the key, its decoded
// value, and the lines it spans (0-based, inclusive), so an editor can
// replace or remove the whole assignment even when a quoted value runs over
// several lines.
type DotenvEntry struct {
	Key       string
	Value     string
	StartLine int
	EndLine   int
}

// DotenvProblem is something ParseDotenv found wrong with a line. The line is
// 1-based, as an editor shows it. Skipped says the line was dropped (parsing
// resumes on the next one, so a bad line costs only itself); otherwise the
// assignment was kept and the problem is with what it holds.
type DotenvProblem struct {
	Line    int
	Reason  string
	Skipped bool
}

// ParseDotenv reads a .env file the way Docker Compose's env_file loader
// does, which is what 1.x handed every .env to, so a file written for 1.x
// means the same thing here:
//
//   - blank lines and lines starting with # are skipped;
//   - an optional leading `export ` is dropped;
//   - the key is everything before the first '=', trimmed;
//   - an unquoted value runs to the end of the line, loses an inline comment
//     starting at " #", and is trimmed;
//   - a single-quoted value is literal and may span lines;
//   - a double-quoted value may span lines and expands the escapes \n, \t,
//     \r, \", \\, \$ and the rest of Go's single-character escapes;
//   - on a duplicate key the last assignment wins (the caller's map does
//     that; every entry is returned in order).
//
// What Compose does and this does not: ${VAR} interpolation. A value holding
// a '$' is passed through as written.
func ParseDotenv(data []byte) ([]DotenvEntry, []DotenvProblem) {
	src := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(src, "\n")
	var entries []DotenvEntry
	var problems []DotenvProblem
	for i := 0; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		key, rest, ok := splitAssignment(t)
		if !ok {
			problems = append(problems, DotenvProblem{Line: i + 1, Reason: "not a KEY=value assignment", Skipped: true})
			continue
		}
		if key == "" || strings.ContainsFunc(key, unicode.IsSpace) {
			problems = append(problems, DotenvProblem{Line: i + 1, Reason: fmt.Sprintf("%q is not a variable name", key), Skipped: true})
			continue
		}
		rest = strings.TrimLeftFunc(rest, unicode.IsSpace)
		if rest == "" || (rest[0] != '"' && rest[0] != '\'') {
			entries = append(entries, DotenvEntry{Key: key, Value: unquotedValue(rest), StartLine: i, EndLine: i})
			continue
		}
		value, end, trailing, closed := quotedValue(rest, lines, i)
		if !closed {
			problems = append(problems, DotenvProblem{
				Line:    i + 1,
				Reason:  fmt.Sprintf("%s's value opens a %c quote that is never closed", key, rest[0]),
				Skipped: true,
			})
			continue
		}
		// A quote closes at the next one of its kind however far on, as
		// Compose reads it, so a stray opening quote takes in the lines after
		// it. That is kept, for 1.x's sake, but an assignment taken in that
		// way is almost never meant, and it vanishes silently: say so.
		for ln := i + 1; ln <= end; ln++ {
			if looksLikeAssignment(lines[ln]) {
				problems = append(problems, DotenvProblem{
					Line: i + 1,
					Reason: fmt.Sprintf("%s's %c-quoted value runs on to line %d, taking in line %d as part of it; "+
						"close the quote on line %d if that is not meant", key, rest[0], end+1, ln+1, i+1),
				})
				break
			}
		}
		if trailing = strings.TrimSpace(trailing); trailing != "" && !strings.HasPrefix(trailing, "#") {
			problems = append(problems, DotenvProblem{
				Line:   end + 1,
				Reason: fmt.Sprintf("%s's value has %q after its closing quote, which is dropped from the value", key, trailing),
			})
		}
		entries = append(entries, DotenvEntry{Key: key, Value: value, StartLine: i, EndLine: end})
		i = end
	}
	return entries, problems
}

// ParseDotenvMap is ParseDotenv folded into a map, last assignment winning.
func ParseDotenvMap(data []byte) (map[string]string, []DotenvProblem) {
	entries, problems := ParseDotenv(data)
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		out[e.Key] = e.Value
	}
	return out, problems
}

// splitAssignment splits a trimmed line into its key (with any `export `
// dropped) and everything after the '='.
func splitAssignment(t string) (key, rest string, ok bool) {
	if after, found := strings.CutPrefix(t, "export"); found && after != "" && unicode.IsSpace(rune(after[0])) {
		t = strings.TrimLeftFunc(after, unicode.IsSpace)
	}
	key, rest, ok = strings.Cut(t, "=")
	return strings.TrimSpace(key), rest, ok
}

var assignmentLine = regexp.MustCompile(`^\s*(?:export\s+)?[A-Za-z_][A-Za-z0-9_.-]*\s*=`)

// looksLikeAssignment reports whether a line, read on its own, would be a
// KEY=value assignment.
func looksLikeAssignment(line string) bool { return assignmentLine.MatchString(line) }

// unquotedValue drops an inline comment (" #", Compose's rule: a '#' with no
// space before it is part of the value) and trims.
func unquotedValue(v string) string {
	v, _, _ = strings.Cut(v, " #")
	return strings.TrimSpace(v)
}

// quotedValue reads a quoted value starting at rest (the remainder of line
// start, beginning with the quote), continuing onto following lines until the
// closing quote. It returns the decoded value, the line the quote closed on,
// what followed the closing quote there, and whether it closed at all. A
// backslash keeps the next character from closing the value in either quote
// style, as Compose's does; only a double-quoted value expands the escapes.
func quotedValue(rest string, lines []string, start int) (value string, end int, trailing string, closed bool) {
	quote := rest[0]
	var b strings.Builder
	escaped := false
	text := rest[1:]
	for ln := start; ln < len(lines); ln++ {
		if ln > start {
			b.WriteByte('\n')
			text = lines[ln]
		}
		for j := 0; j < len(text); j++ {
			c := text[j]
			switch {
			case escaped:
				escaped = false
				b.WriteByte('\\')
				b.WriteByte(c)
			case c == '\\':
				escaped = true
			case c == quote:
				v := b.String()
				if quote == '"' {
					v = expandEscapes(v)
				}
				return v, ln, text[j+1:], true
			default:
				b.WriteByte(c)
			}
		}
		if escaped {
			// A backslash ending a line stays a literal backslash, then the
			// line break.
			escaped = false
			b.WriteByte('\\')
		}
	}
	return "", 0, "", false
}

var escapeSeq = regexp.MustCompile(`\\(?:[abcfnrtv$"\\]|0\d{0,3})`)

// expandEscapes decodes the escapes Compose decodes in a double-quoted value.
// \$ is a literal '$' (Compose's way to stop interpolation, which this does
// not do, so it is simply the character). Anything else stays as written.
func expandEscapes(s string) string {
	return escapeSeq.ReplaceAllStringFunc(s, func(m string) string {
		if m == `\$` {
			return "$"
		}
		esc := m
		if strings.HasPrefix(esc, `\0`) {
			// Compose's octal form: \0123 is Go's \123.
			esc = strings.Replace(esc, `\0`, `\`, 1)
		}
		v, _, _, err := strconv.UnquoteChar(esc, '"')
		if err != nil {
			return m
		}
		return string(v)
	})
}
