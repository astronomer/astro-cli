package env

import (
	"fmt"
	"io"
	"os"

	"github.com/subosito/gotenv"
)

// ParseDotenvFile reads a dotenv-formatted file and returns its key/value pairs.
// Pass "-" as the path to read from stdin. Delegates parsing to subosito/gotenv,
// which handles quoted values, multi-line values, escape sequences (\n, \r, \t,
// \$, \", \\), comments, blank lines, and a leading `export ` prefix. The
// returned map round-trips with output produced by WriteVarList(FormatDotenv).
//
// Keys must follow POSIX shell variable naming (alphanumerics + underscore,
// not starting with a digit). Platform objects with non-POSIX keys (e.g.
// Airflow variables containing `-` or `.`) cannot be bulk-imported via this
// path; use single-key create instead.
//
// Duplicate keys in the source file silently keep the last occurrence;
// callers should warn the user before overwriting.
func ParseDotenvFile(path string) (map[string]string, error) {
	src, name := dotenvSource(path)
	if src == nil {
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("opening %s: %w", path, err)
		}
		defer f.Close()
		src = f
	}
	parsed, err := gotenv.StrictParse(src)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", name, err)
	}
	return parsed, nil
}

// dotenvSource returns (os.Stdin, "<stdin>") when path is "-"; otherwise
// (nil, path) and the caller opens the file.
func dotenvSource(path string) (reader io.Reader, displayName string) {
	if path == "-" {
		return os.Stdin, "<stdin>"
	}
	return nil, path
}
