package localenv

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowenv"
	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/fsatomic"
)

// ErrMultilineValue reports a value with a newline, which a dotenv line
// cannot hold. Rejecting it up front keeps a set from corrupting the file.
var ErrMultilineValue = errors.New("value contains a newline, which a .env file cannot store")

// readMap parses a dotenv file into key -> value, decoding exactly the way
// the runtime does (airflowrt.LoadEnvFile: skip blanks and #-comments, split
// on the first '=', strip matching surrounding quotes). A missing file is an
// empty map, not an error — an unset value is simply absent. On a duplicate
// key the last line wins, matching dotenv loaders.
func readMap(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if i := strings.IndexByte(t, '='); i >= 0 {
			out[strings.TrimSpace(t[:i])] = airflowrt.StripQuotes(strings.TrimSpace(t[i+1:]))
		}
	}
	return out, nil
}

// mergeSet writes key=value into the dotenv file at path, replacing an
// existing assignment in place and appending a new key at the end. Every
// other line — comments, blanks, hand-typed entries — is preserved verbatim:
// the file is edited, never rewritten from a model, so a hand-edited .env
// survives a set. The file is created 0600 and written atomically.
func mergeSet(path, key, value string) error {
	if strings.ContainsAny(value, "\r\n") {
		return ErrMultilineValue
	}
	lines, trailingNewline, err := readLines(path)
	if err != nil {
		return err
	}
	assignment := key + "=" + formatValue(value)
	found := false
	for i, line := range lines {
		if lineKey(line) == key {
			lines[i] = assignment
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, assignment)
	}
	return writeLines(path, lines, trailingNewline || !found)
}

// mergeDelete removes every assignment of key from the file, preserving all
// other lines. ok is false when the file held no such key.
func mergeDelete(path, key string) (ok bool, err error) {
	lines, trailingNewline, err := readLines(path)
	if err != nil {
		return false, err
	}
	kept := lines[:0:0]
	for _, line := range lines {
		if lineKey(line) == key {
			ok = true
			continue
		}
		kept = append(kept, line)
	}
	if !ok {
		return false, nil
	}
	if len(kept) == 0 {
		// The file is now empty of content; leave an empty file rather than
		// removing it, so its 0600 mode and existence are stable.
		return true, writeLines(path, nil, false)
	}
	return true, writeLines(path, kept, trailingNewline)
}

// readLines returns the file's lines with no line ever holding a trailing
// newline, plus whether the file ended in a newline. A missing file is no
// lines. It never creates the file.
func readLines(path string) (lines []string, trailingNewline bool, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if len(data) == 0 {
		return nil, false, nil
	}
	s := string(data)
	trailingNewline = strings.HasSuffix(s, "\n")
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n"), trailingNewline, nil
}

// writeLines writes the lines back atomically at 0600, creating the parent
// directory when needed. addTrailingNewline controls whether the file ends
// in a newline (a freshly appended key always gets one).
func writeLines(path string, lines []string, addTrailingNewline bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { //nolint:mnd // standard directory mode
		return err
	}
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
	}
	if len(lines) > 0 && addTrailingNewline {
		b.WriteByte('\n')
	}
	return fsatomic.WriteFile(path, []byte(b.String()), filePerm)
}

// lineKey returns the assignment key a line declares, or "" for a blank line,
// a comment, or a line with no '='. Leading and trailing space around the key
// is ignored, matching readMap.
func lineKey(line string) string {
	t := strings.TrimSpace(line)
	if t == "" || strings.HasPrefix(t, "#") {
		return ""
	}
	i := strings.IndexByte(t, '=')
	if i < 0 {
		return ""
	}
	return strings.TrimSpace(t[:i])
}

// formatValue renders a value as the right-hand side of a dotenv line so
// readMap (and airflowrt.LoadEnvFile) decode it back unchanged. A value that
// would otherwise be ambiguous — whitespace, a '#', a surrounding quote — is
// wrapped in single quotes, which the loader strips without unescaping; a
// value already holding a single quote falls back to double quotes.
func formatValue(v string) string {
	if v == "" || !needsQuoting(v) {
		return v
	}
	if !strings.Contains(v, "'") {
		return "'" + v + "'"
	}
	if !strings.Contains(v, `"`) {
		return `"` + v + `"`
	}
	// Both quote kinds present (rare for a connection JSON or token): single
	// quotes still bound the value; an interior single quote is a documented
	// MVP limit, not a corruption of neighboring lines.
	return "'" + v + "'"
}

func needsQuoting(v string) bool {
	if v != strings.TrimSpace(v) {
		return true
	}
	return strings.ContainsAny(v, " \t\"'#")
}

// EnvKeyFor maps a kind and name to the Airflow env-var name the value is
// stored under. ok is false when the name cannot be expressed as that env
// var (pkg/airflowenv's rules).
func EnvKeyFor(kind Kind, name string) (string, bool) {
	switch kind {
	case KindEnv:
		if !airflowenv.ValidEnvKey(name) {
			return "", false
		}
		return name, true
	case KindVar:
		if !airflowenv.ValidVarKey(name) {
			return "", false
		}
		return airflowenv.EnvKeyForVarKey(name), true
	case KindConn:
		if !airflowenv.ValidConnID(name) {
			return "", false
		}
		return airflowenv.EnvKeyForConnID(name), true
	default:
		return "", false
	}
}
