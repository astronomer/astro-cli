package localenv

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowenv"
	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/fsatomic"
)

// ErrMultilineValue reports a value with a newline, which a set does not
// write. The file can hold one, as a quoted value over several lines, written
// by hand; a set keeps every assignment on one line.
var ErrMultilineValue = errors.New("value contains a newline, which astro local env set does not write; " +
	"put it in .env by hand as a quoted value over several lines")

// readMap parses a dotenv file into key -> value, decoding exactly the way
// the runtime does: airflowrt.ParseDotenv, Docker Compose's rules. A missing
// file is an empty map, not an error — an unset value is simply absent. On a
// duplicate key the last assignment wins, matching dotenv loaders.
func readMap(path string) (map[string]string, error) {
	m, _, err := readMapProblems(path)
	return m, err
}

// readMapProblems is readMap plus the lines the parser could not read, which
// the runtime skips just the same.
func readMapProblems(path string) (map[string]string, []airflowrt.DotenvProblem, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	m, problems := airflowrt.ParseDotenvMap(data)
	return m, problems, nil
}

// entriesOf parses lines as the file they came from, so each entry's line
// span indexes into them.
func entriesOf(lines []string) []airflowrt.DotenvEntry {
	entries, _ := airflowrt.ParseDotenv([]byte(strings.Join(lines, "\n")))
	return entries
}

// mergeSet writes key=value into the dotenv file at path, replacing the
// assignment that wins (the last, as readMap reads it) in place and
// appending a new key at the end. Every other line — comments, blanks,
// hand-typed entries — is preserved verbatim: the file is edited, never
// rewritten from a model, so a hand-edited .env survives a set. An
// assignment whose quoted value spans several lines is replaced whole. The
// file is created 0600 and written atomically.
func mergeSet(path, key, value string) error {
	if strings.ContainsAny(value, "\r\n") {
		return ErrMultilineValue
	}
	lines, trailingNewline, err := readLines(path)
	if err != nil {
		return err
	}
	assignment := key + "=" + formatValue(value)
	var last *airflowrt.DotenvEntry
	entries := entriesOf(lines)
	for i := range entries {
		if entries[i].Key == key {
			last = &entries[i]
		}
	}
	if last == nil {
		lines = append(lines, assignment)
		trailingNewline = true
	} else {
		lines = append(lines[:last.StartLine:last.StartLine], append([]string{assignment}, lines[last.EndLine+1:]...)...)
	}
	// A quote left open earlier in the file can close on the new
	// assignment's, taking it into that value. Refuse rather than report a
	// set that a read, and a start, would not see.
	if got, _ := airflowrt.ParseDotenvMap([]byte(strings.Join(lines, "\n"))); got[key] != value {
		return fmt.Errorf("%s would not read back as set: a quote left open earlier in %s takes it in; "+
			"close that quote and set it again", key, filepath.Base(path))
	}
	return writeLines(path, lines, trailingNewline)
}

// mergeDelete removes every assignment of key from the file, each one whole,
// preserving all other lines. ok is false when the file held no such key.
func mergeDelete(path, key string) (ok bool, err error) {
	lines, trailingNewline, err := readLines(path)
	if err != nil {
		return false, err
	}
	drop := make(map[int]bool)
	for _, e := range entriesOf(lines) {
		if e.Key != key {
			continue
		}
		ok = true
		for ln := e.StartLine; ln <= e.EndLine; ln++ {
			drop[ln] = true
		}
	}
	if !ok {
		return false, nil
	}
	kept := lines[:0:0]
	for i, line := range lines {
		if !drop[i] {
			kept = append(kept, line)
		}
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

// formatValue renders a value as the right-hand side of a dotenv line so
// airflowrt.ParseDotenv decodes it back unchanged. A value that would
// otherwise be ambiguous — whitespace, a '#', a quote, a '$' — is wrapped in
// single quotes, which hold everything literally except a single quote, and a
// backslash right before the closing one. A value with either of those is
// double-quoted instead, with its backslashes, double quotes and dollars
// escaped.
//
// The '$' is quoted although ParseDotenv does no ${VAR} interpolation, because
// every other dotenv reader does (Docker Compose, python-dotenv), and so would
// this one if it ever gained it: a value written bare as pa$word would then
// quietly read as "pa". Quoted, it means the same thing to all of them.
func formatValue(v string) string {
	if v == "" || !needsQuoting(v) {
		return v
	}
	if !strings.Contains(v, "'") && !strings.HasSuffix(v, `\`) {
		return "'" + v + "'"
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`).Replace(v) + `"`
}

func needsQuoting(v string) bool {
	if v != strings.TrimSpace(v) {
		return true
	}
	return strings.ContainsAny(v, " \t\"'#$")
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
