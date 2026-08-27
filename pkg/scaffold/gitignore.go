// The project's .gitignore, which this package both writes (for a new project)
// and heals (for one it adopted). It moved here from internal/localenv with its
// two callers: EnsureEnvIgnored had exactly one, in this package, and EnvIgnored
// exactly one, in cmd/local/env.go. Keeping the pair together is what avoids a
// second copy of coversEnv, which is the only part with a rule in it worth
// getting wrong.

package scaffold

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// EnvIgnored reports whether the project's .gitignore covers the .env file.
// A missing .gitignore counts as not ignoring it. This backs the `set`
// warning: a project .env that git would track is the failure mode that
// actually leaks secrets.
func EnvIgnored(projectDir string) (bool, error) {
	data, err := os.ReadFile(filepath.Join(projectDir, fileGitignore))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return coversEnv(string(data)), nil
}

// EnsureEnvIgnored makes the project's .gitignore cover .env, appending a
// rule when an existing file omits it. A missing .gitignore is left alone —
// `astro init` writes one from its template — so this only heals an imported
// or hand-made project. added reports whether a rule was appended.
func EnsureEnvIgnored(projectDir string) (added bool, err error) {
	path := filepath.Join(projectDir, fileGitignore)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if coversEnv(string(data)) {
		return false, nil
	}
	s := string(data)
	var b strings.Builder
	b.WriteString(s)
	if s != "" && !strings.HasSuffix(s, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString("\n# Local env values (astro local env)\n.env\n")
	// Written in place rather than through a temp file and a rename, for the
	// same reason Run writes the manifest that way: this file is the user's, and
	// a rename replaces it — dropping its mode, writing through a read-only bit
	// that says don't, and turning a symlinked .gitignore into a regular file.
	// This function only ever runs when the file already exists, so os.WriteFile
	// truncates in place and the mode is the user's, not filePerm.
	if err := os.WriteFile(path, []byte(b.String()), filePerm); err != nil {
		return false, err
	}
	return true, nil
}

// coversEnv reports whether a .gitignore leaves the .env file ignored. It
// recognizes the plain and common wildcard spellings; an exotic pattern that
// happens to match is treated as not covering, so the worst case is a harmless
// extra warning, never a missed one.
//
// That promise is the whole point of the function — a missed warning is how a
// credential reaches a commit — and two details are what keep it. Both were
// wrong when this moved here from internal/localenv.
//
// LAST MATCH WINS, and negation counts. git applies patterns in order and a
// later `!.env` re-includes the file, so returning true at the first match read
// `.env` followed by `!.env` as ignored when git tracks it. That is the
// false-negative direction the doc above rules out, reached by the one gitignore
// feature specifically designed to undo an earlier rule.
//
// LEADING WHITESPACE IS PART OF THE PATTERN. git strips trailing whitespace
// unless it is escaped, and keeps leading whitespace, so `  .env` is a pattern
// for a file whose name begins with two spaces. Trimming both ends read that as
// covering `.env`, which it does not.
func coversEnv(gitignore string) bool {
	covered := false
	for _, line := range strings.Split(gitignore, "\n") {
		// Trailing only: \r as well, so a CRLF file is not read as a set of
		// patterns that all end in a carriage return.
		line = strings.TrimRight(line, " \t\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		negated := strings.HasPrefix(line, "!")
		switch strings.TrimPrefix(line, "!") {
		case ".env", "/.env", "*.env", ".env*", "**/.env":
			covered = !negated
		}
	}
	return covered
}
