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

	ignorefmt "github.com/go-git/go-git/v5/plumbing/format/gitignore"

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
	"# state, credentials and tokens.\n"

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
	if ignoresRootFile(s, uvLockFile) {
		add.WriteString("\n" + uvLockUnignoreNote + "!/" + uvLockFile + "\n")
		labels = append(labels, fileGitignore+" (stopped ignoring uv.lock, which pins what every image installs: commit it)")
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

// ignoreMatcher reads a .gitignore's text with go-git's gitignore rules, which
// are git's own: the last matching pattern wins, a ! pattern re-includes, a
// trailing / matches only a directory, ** spans directories. Only this file is
// read, not .git/info/exclude or a global excludes file: what a teammate's
// clone ignores is what the committed .gitignore says.
//
// Each line loses its trailing whitespace (and a CRLF file's \r) first, as git
// strips it; leading whitespace stays part of the pattern, as in git, so
// "  .env" names a file whose name starts with two spaces.
func ignoreMatcher(content string) ignorefmt.Matcher {
	var patterns []ignorefmt.Pattern
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimRight(line, " \t\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, ignorefmt.ParsePattern(line, nil))
	}
	return ignorefmt.NewMatcher(patterns)
}

// hasIgnoreRule reports whether a .gitignore states rule, as written or
// anchored with a leading slash, and still ignores what it names: a later
// line that re-includes it (!**/*, say) undoes it, as git reads the file.
//
// The spelling check is on purpose. planIgnoreRules appends what this says is
// missing, and a broader pattern that covers the same files (.astro/, say)
// does not make the specific rule present: it reads as missing and is
// appended beside it, a redundant line rather than a missing one.
func hasIgnoreRule(content, rule string) bool {
	stated := false
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimRight(line, " \t\r")
		if line == rule || line == "/"+rule {
			stated = true
		}
	}
	if !stated {
		return false
	}
	parts, isDir := ruleSample(rule)
	return ignoreMatcher(content).Match(parts, isDir)
}

// ruleSample is a path rule names, for asking a matcher whether the rule is
// still in force: its wildcards filled in (.astro/*.local.yaml is
// .astro/x.local.yaml), and a directory rule (.astro/standalone/) asked as
// that directory.
func ruleSample(rule string) (parts []string, isDir bool) {
	rule = strings.TrimPrefix(rule, "/")
	isDir = strings.HasSuffix(rule, "/")
	rule = strings.TrimSuffix(rule, "/")
	rule = strings.NewReplacer("**", "x", "*", "x", "?", "x").Replace(rule)
	return strings.Split(rule, "/"), isDir
}

// coversEnv reports whether a .gitignore leaves the project's .env ignored, as
// git reads the file (ignoreMatcher). A missed answer here is how a credential
// reaches a commit, so it has to be git's answer, not a reading of the common
// spellings: a later !.env, or a broader !**/*, re-includes the file, and an
// unusual pattern that does match it (.e*) covers it.
func coversEnv(content string) bool {
	return ignoreMatcher(content).Match([]string{".env"}, false)
}

// uvLockFile is the lock every image installs from, which has to be committed.
const uvLockFile = "uv.lock"

// uvLockUnignoreNote is the comment above the rule that re-includes uv.lock.
const uvLockUnignoreNote = "# uv.lock pins what every image of this project installs, so it is committed.\n"

// ignoresRootFile reports whether a .gitignore's own rules leave name, a file
// at the project's root, ignored, as git reads the file (ignoreMatcher). Not
// what another ignore file adds.
func ignoresRootFile(content, name string) bool {
	return ignoreMatcher(content).Match([]string{name}, false)
}
