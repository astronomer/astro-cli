package scaffold

import (
	"fmt"
	"regexp"
	"strings"
)

// Turning a requirements.txt into [project.dependencies] is deterministic and
// lossless on purpose: every requirement is carried verbatim, comments are
// kept, and a line we cannot express as a PEP 621 dependency (a pip option, a
// garbage line) is carried into the manifest as a comment and reported, never
// dropped. No requirement is rewritten — rewriting would be guessing.

// reqKind classifies one logical requirements.txt line.
type reqKind int

const (
	// reqDependency is a requirement carried verbatim into the array.
	reqDependency reqKind = iota
	// reqComment is a full-line comment, kept as a comment in the array.
	reqComment
	// reqCarried is a line that is not a PEP 621 dependency (a pip option
	// like -r/-e/--index-url, or a garbage line): kept as a comment and
	// surfaced as a warning.
	reqCarried
)

// reqLine is one parsed requirements.txt line.
type reqLine struct {
	kind reqKind
	// text is the dependency spec, the comment body, or the carried line.
	text string
	// inline is a trailing comment on a dependency line, without its '#'.
	inline string
}

// airflowPinRe accepts the version shapes tool.astro.airflow allows.
var airflowPinRe = regexp.MustCompile(`^\d+(\.\d+){0,2}$`)

// airflowDist is the core Airflow distribution's normalized (PEP 503) name.
const airflowDist = "apache-airflow"

// parseRequirements turns requirements.txt bytes into classified lines, in
// file order. Blank lines are dropped (the array's own layout replaces them);
// everything with content is kept.
func parseRequirements(data []byte) []reqLine {
	var out []reqLine
	for _, raw := range joinContinuations(strings.Split(string(data), "\n")) {
		line := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "#"):
			out = append(out, reqLine{kind: reqComment, text: strings.TrimSpace(line[1:])})
		default:
			spec, inline := splitInlineComment(line)
			switch {
			case spec == "":
				// The content was only an inline comment.
				out = append(out, reqLine{kind: reqComment, text: inline})
			case isRequirement(spec):
				out = append(out, reqLine{kind: reqDependency, text: spec, inline: inline})
			default:
				// A pip option (-r, -e, --index-url), a bare URL, a --hash pin,
				// or a garbage line: we cannot express it as a PEP 508
				// dependency, so carry it as a comment instead of emitting a
				// requirement uv would reject.
				out = append(out, reqLine{kind: reqCarried, text: line})
			}
		}
	}
	return out
}

// joinContinuations folds pip's backslash line continuations into single
// logical lines before classification.
func joinContinuations(lines []string) []string {
	var out []string
	var pending strings.Builder
	joining := false
	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r")
		if strings.HasSuffix(trimmed, "\\") {
			pending.WriteString(strings.TrimSuffix(trimmed, "\\"))
			joining = true
			continue
		}
		if joining {
			pending.WriteString(trimmed)
			out = append(out, pending.String())
			pending.Reset()
			joining = false
			continue
		}
		out = append(out, trimmed)
	}
	if joining {
		out = append(out, pending.String())
	}
	return out
}

// splitInlineComment separates a requirement from a trailing pip comment. Pip
// starts an inline comment at a '#' preceded by whitespace, so a '#' inside a
// token (a URL fragment) is not a comment.
func splitInlineComment(line string) (spec, comment string) {
	for i := 0; i < len(line); i++ {
		if line[i] == '#' && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t') {
			return strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+1:])
		}
	}
	return strings.TrimSpace(line), ""
}

func startsAlnum(s string) bool {
	if s == "" {
		return false
	}
	c := s[0]
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// isRequirement reports whether spec can be carried verbatim into
// [project.dependencies] as a PEP 508 requirement. It is deliberately strict:
// a direct URL (git+…, http(s)://…), a line carrying a pip --hash pin, or
// anything whose head is not a distribution name followed by a version
// specifier or marker is not a dependency uv can resolve, so parseRequirements
// carries it as a reported comment instead.
func isRequirement(spec string) bool {
	if !startsAlnum(spec) {
		return false
	}
	// --hash is a pip option, not PEP 508; a continuation-folded "pkg==1 --hash"
	// line would otherwise be emitted as an unresolvable requirement.
	if strings.Contains(spec, "--hash") {
		return false
	}
	// A bare URL names no distribution. A named direct reference (pkg @ url)
	// starts with the name, not the scheme, so it still passes below.
	lower := strings.ToLower(spec)
	for _, scheme := range []string{"git+", "http://", "https://"} {
		if strings.HasPrefix(lower, scheme) {
			return false
		}
	}
	name, rest := splitNameSpec(spec)
	return name != "" && validSpecTail(rest)
}

// validSpecTail reports whether rest, the part after a requirement's name and
// extras, is empty or opens with a version specifier, marker, or direct
// reference. Anything else (stray words, an unexpected symbol) means the line
// is not a real requirement.
func validSpecTail(rest string) bool {
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return true
	}
	switch rest[0] {
	case '<', '>', '=', '!', '~', ';', '@', '(':
		return true
	}
	return false
}

// renderDependencies renders the classified lines as a TOML array literal for
// [project.dependencies], preserving comments. It returns the literal and the
// count of real dependencies.
func renderDependencies(lines []reqLine) (literal string, count int) {
	if len(lines) == 0 {
		return "[]", 0
	}
	var b strings.Builder
	b.WriteString("[\n")
	for _, l := range lines {
		switch l.kind {
		case reqDependency:
			b.WriteString("    " + tomlString(l.text) + ",")
			if l.inline != "" {
				b.WriteString("  # " + sanitizeComment(l.inline))
			}
			b.WriteString("\n")
			count++
		case reqComment:
			b.WriteString("    # " + sanitizeComment(l.text) + "\n")
		case reqCarried:
			b.WriteString("    # carried from requirements.txt (not a dependency): " + sanitizeComment(l.text) + "\n")
		}
	}
	b.WriteString("]")
	return b.String(), count
}

// renderPackages renders OS package names as a TOML array literal for
// [tool.astro] packages. An empty list yields "", so the caller omits the
// packages key — a greenfield manifest carries no packages line.
func renderPackages(names []string) string {
	if len(names) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("[\n")
	for _, n := range names {
		b.WriteString("    " + tomlString(n) + ",\n")
	}
	b.WriteString("]")
	return b.String()
}

// carriedWarnings reports the lines that could not become dependencies, so the
// import surfaces them rather than hiding them in a comment.
func carriedWarnings(lines []reqLine) []string {
	var ws []string
	for _, l := range lines {
		if l.kind == reqCarried {
			ws = append(ws, "requirements.txt line kept as a comment, not a dependency: "+l.text)
		}
	}
	return ws
}

// airflowPin returns the version to pin from an apache-airflow requirement,
// when one carries a clean "==" pin. A range, a wildcard, or extra
// specifiers yield no pin: the import falls back to the default with a notice.
func airflowPin(lines []reqLine) (version string, ok bool) {
	for _, l := range lines {
		if l.kind != reqDependency {
			continue
		}
		if v, found := pinFromSpec(l.text); found {
			return v, true
		}
	}
	return "", false
}

func pinFromSpec(spec string) (version string, ok bool) {
	name, rest := specNameSpec(spec)
	if normalizeName(name) != airflowDist {
		return "", false
	}
	if !strings.HasPrefix(rest, "==") {
		return "", false
	}
	v := strings.TrimSpace(rest[2:])
	if strings.ContainsAny(v, ", *") { // more than a single exact pin
		return "", false
	}
	if !airflowPinRe.MatchString(v) {
		return "", false
	}
	return v, true
}

// specNameSpec splits a requirement into its distribution name and the
// specifier, first dropping any environment marker or direct-URL suffix.
func specNameSpec(spec string) (name, rest string) {
	s := spec
	if i := strings.IndexAny(s, ";@"); i >= 0 {
		s = s[:i]
	}
	return splitNameSpec(s)
}

// namesAirflow reports whether a requirement is an apache-airflow distribution,
// however it is pinned — or whether it is pinned at all.
func namesAirflow(spec string) bool {
	name, _ := specNameSpec(spec)
	return normalizeName(name) == airflowDist
}

// hasAirflowDependency reports whether the parsed lines already carry an
// apache-airflow requirement, so the import leaves it verbatim instead of
// adding its own.
func hasAirflowDependency(lines []reqLine) bool {
	for _, l := range lines {
		if l.kind == reqDependency && namesAirflow(l.text) {
			return true
		}
	}
	return false
}

// airflowRequirement is the [project.dependencies] entry that installs the
// Airflow the manifest pins. It mirrors [tool.astro].airflow one-to-one: a
// partial pin ("3", "3.1") becomes a prefix match ("apache-airflow==3.1.*") so
// the project tracks patch releases — the same "resolution to a concrete
// release happens later" the pin itself promises — while a full "3.1.2" pin
// stays exact. This is the one place the dependency string is derived, so the
// greenfield and import paths stay in step.
func airflowRequirement(version string) string {
	if strings.Count(version, ".") < 2 {
		return airflowDist + "==" + version + ".*"
	}
	return airflowDist + "==" + version
}

// splitNameSpec splits a requirement into its distribution name and the rest
// (specifier), dropping any extras group.
func splitNameSpec(s string) (name, rest string) {
	i := 0
	for i < len(s) && isNameByte(s[i]) {
		i++
	}
	name, rest = s[:i], s[i:]
	if strings.HasPrefix(rest, "[") {
		if j := strings.Index(rest, "]"); j >= 0 {
			rest = rest[j+1:]
		}
	}
	return name, strings.TrimSpace(rest)
}

func isNameByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
		c == '.' || c == '_' || c == '-'
}

// normalizeName is PEP 503 name normalization, enough to match apache-airflow
// however it was spelled.
func normalizeName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "_", "-")
	return strings.ReplaceAll(name, ".", "-")
}

// tomlString renders s as a TOML basic (double-quoted) string, so a
// requirement carrying quotes or a marker survives verbatim and still parses.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < ' ' { // other control characters
				fmt.Fprintf(&b, `\u%04X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// sanitizeComment keeps comment text on one TOML line: a comment runs to the
// newline, so any embedded newline would end it early and break the array.
func sanitizeComment(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r", " "), "\n", " ")
}
