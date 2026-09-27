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

// localIgnoreRules are the per-machine files Astro tools write into a project,
// all but one under .astro/. gitignoreTemplate lists them for a new project, and
// planIgnoreRules adds whichever are missing to a .gitignore the project already
// had. Each holds something true of one machine and wrong, or secret, on any
// other:
//
//   - .astro/standalone/ is a standalone Airflow's AIRFLOW_HOME
//     (airflowrt.StandaloneDir): its SQLite database, its logs, and the
//     generated admin password.
//   - .astro/worktrees/ holds the git worktrees Astro Desktop nests inside a
//     project, each pointing back at this clone by absolute path.
//   - .astro/*.local.yaml and .astro/*.local.yml cover the standalone af CLI's
//     instance file, config.local.yaml: a local URL derived from this machine's
//     directory name and proxy port, the local admin credentials, and the
//     current instance.
//   - .astro/otto/*.local.json covers Otto's per-machine permissions scope,
//     permissions.local.json. Its sibling permissions.json is meant to be
//     committed, and so is extensions.json, which is why .astro/otto/ is not
//     ignored as a whole.
//   - .astro/otto/mcp.json carries Astro Desktop's loopback MCP address and a
//     bearer token for it. Otto's MCP loader fixes the name, so the *.local.json
//     rule cannot cover it.
//   - plugins/fix_local_executor_pickle.py is the LocalExecutor pickling fix the
//     standalone engine drops into plugins/ for Airflow 2 on macOS
//     (airflowrt.AF2PickleFixPlugin). It is machine-specific and recreated on
//     the next start whenever it is missing, so it has no business in the repo.
//     Only that file: the rest of plugins/ is the project's own code.
//
// Nothing ignores .astro/ itself: .astro/config.yaml holds deployment links and
// .astro/memory/ is Otto's shared project memory, and both are committed.
var localIgnoreRules = []string{
	".astro/standalone/",
	".astro/worktrees/",
	".astro/*.local.yaml",
	".astro/*.local.yml",
	".astro/otto/*.local.json",
	".astro/otto/mcp.json",
	pickleFixRule,
}

// pickleFixRule is the one per-machine rule outside .astro/, and the one that
// carries a comment of its own, because nothing in its path says a tool wrote it.
const pickleFixRule = "plugins/fix_local_executor_pickle.py"

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
// O3 requires a preview for.
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
