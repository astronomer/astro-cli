package localenv

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/internal/fsatomic"
)

const gitignoreName = ".gitignore"

// gitignorePerm is the ordinary file mode for .gitignore — it is not secret.
const gitignorePerm = 0o644

// EnvIgnored reports whether the project's .gitignore covers the .env file.
// A missing .gitignore counts as not ignoring it. This backs the `set`
// warning: a project .env that git would track is the failure mode that
// actually leaks secrets.
func EnvIgnored(projectDir string) (bool, error) {
	data, err := os.ReadFile(filepath.Join(projectDir, gitignoreName))
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
	path := filepath.Join(projectDir, gitignoreName)
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
	if err := fsatomic.WriteFile(path, []byte(b.String()), gitignorePerm); err != nil {
		return false, err
	}
	return true, nil
}

// coversEnv reports whether any .gitignore line matches the .env file. It
// recognizes the plain and common wildcard spellings; an exotic pattern that
// happens to match is treated as not covering, so the worst case is a
// harmless extra warning, never a missed one.
func coversEnv(gitignore string) bool {
	for _, line := range strings.Split(gitignore, "\n") {
		switch strings.TrimSpace(line) {
		case ".env", "/.env", "*.env", ".env*", "**/.env":
			return true
		}
	}
	return false
}
