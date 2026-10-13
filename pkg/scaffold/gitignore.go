// The project's .gitignore, which this package both writes (for a new project)
// and heals (for one it adopted), adding the .env rule and the per-machine
// .astro/ rules when they are missing. It moved here from internal/localenv with its
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

	"github.com/astronomer/astro-cli/pkg/airflowrt"
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

// localIgnoreRules are the per-machine files Astro tools write into a project
// (airflowrt.LocalFileRules, which says what each one is). gitignoreTemplate
// lists them for a new project, and planIgnoreRules adds whichever are missing
// to a .gitignore the project already had. The list lives in airflowrt so the
// image a deploy generates (pkg/imagebuild) leaves out the same files.
var localIgnoreRules = airflowrt.LocalFileRules()

// pickleFixRule is the one per-machine rule outside .astro/, and the one that
// carries a comment of its own, because nothing in its path says a tool wrote it.
const pickleFixRule = airflowrt.PickleFixRule

// localIgnoreNotes are comment lines written directly above a rule, in the
// template and when the rule is appended, keyed by the rule.
var localIgnoreNotes = map[string]string{
	pickleFixRule: "# Airflow 2 on macOS: the standalone engine regenerates this plugin.\n",
}

// localIgnoreHeader introduces localIgnoreRules when they are appended to an
// existing .gitignore. It is the comment gitignoreTemplate puts above them.
const localIgnoreHeader = "# Per-machine files Astro tools write into the project: local Airflow\n" +
	"# state, credentials and tokens. .astro/config.yaml is shared; keep committing it.\n"

// planIgnoreRules works out the change that would make an existing .gitignore
// cover .env and every entry in localIgnoreRules, and returns nil when nothing
// is needed: no file, or every rule already there. It appends only what is
// missing and leaves the lines already there as they are.
//
// A plan rather than a write so the appended bytes can be shown to a person
// before they land: this edits a file the user wrote, which is exactly the case
// that needs a preview.
func planIgnoreRules(projectDir string) (*Change, error) {
	data, err := os.ReadFile(filepath.Join(projectDir, fileGitignore))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s := string(data)

	var add strings.Builder
	var labels []string
	if !coversEnv(s) {
		add.WriteString("\n# Local env values (astro local env)\n.env\n")
		labels = append(labels, fileGitignore+" (added the .env rule)")
	}
	var missing []string
	for _, rule := range localIgnoreRules {
		if !hasIgnoreRule(s, rule) {
			missing = append(missing, rule)
		}
	}
	if len(missing) > 0 {
		add.WriteString("\n" + localIgnoreHeader)
		for _, rule := range missing {
			add.WriteString(localIgnoreNotes[rule] + rule + "\n")
		}
		labels = append(labels, fileGitignore+" (added the per-machine rules)")
	}
	if len(labels) == 0 {
		return nil, nil
	}

	var b strings.Builder
	b.WriteString(s)
	if s != "" && !strings.HasSuffix(s, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString(add.String())
	// UpdateFile rather than CreateFile: the file is the user's, and Apply
	// writes an update in place so it keeps its mode and its identity. A rename
	// would drop both.
	return &Change{
		Kind:    UpdateFile,
		Path:    fileGitignore,
		Content: []byte(b.String()),
		Labels:  labels,
	}, nil
}

// hasIgnoreRule reports whether a .gitignore states rule, as written or
// anchored with a leading slash, without negating it on a later line.
//
// It matches spellings, not what git would ignore, so a broader pattern that
// covers the same files (.astro/, say) reads as missing and the rule is
// appended beside it. That errs the harmless way: a redundant line, never a
// missing one. Whitespace is read the way coversEnv reads it, for the reasons
// given there.
func hasIgnoreRule(gitignore, rule string) bool {
	has := false
	for _, line := range strings.Split(gitignore, "\n") {
		line = strings.TrimRight(line, " \t\r")
		negated := strings.HasPrefix(line, "!")
		line = strings.TrimPrefix(line, "!")
		if line == rule || line == "/"+rule {
			has = !negated
		}
	}
	return has
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
