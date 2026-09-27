package scaffold

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeGitignore(t *testing.T, dir, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(content), 0o600))
}

func readGitignore(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	require.NoError(t, err)
	return string(data)
}

// hasLine reports whether want is one of content's lines, independently of the
// code under test. planIgnoreRules decides what to write by calling coversEnv,
// so verifying the result with coversEnv would let a broken predicate agree with
// itself — the assertions below use this instead.
func hasLine(content, want string) bool {
	return slices.Contains(strings.Split(content, "\n"), want)
}

// coversEnv decides whether a project's .env stays ignored, and .env is where
// local secrets live. It had no test anywhere before this package took it over.
//
// The two directions are not symmetric, which is what these pin: a pattern it
// does not understand must read as UNCOVERED, because a redundant warning is
// harmless and a missed one is how a credential reaches a commit.
func TestCoversEnv(t *testing.T) {
	covered := []string{
		".env",
		"/.env",
		"*.env",
		".env*",
		"**/.env",
		".env  ",
		"node_modules\n.env\ndist",
		"# comment\n.env",
		".env\r\ndist",
		// Re-ignored after a negation: last match wins in both directions.
		"!.env\n.env",
	}
	for _, g := range covered {
		assert.Truef(t, coversEnv(g), "coversEnv(%q) = false, want true", g)
	}

	notCovered := []string{
		"",
		"node_modules",
		".envrc",
		"env",
		".env.local",
		"# .env",
		"!.env",
		// git applies patterns in order and `!` re-includes, so this file does
		// NOT ignore .env. Reading it as ignored suppresses the warning that
		// exists to stop a credential being committed.
		".env\n!.env",
		"*.env\n!.env",
		// Leading whitespace is part of a git pattern (only trailing is
		// stripped), so this ignores a file whose name starts with two spaces.
		"  .env",
	}
	for _, g := range notCovered {
		assert.Falsef(t, coversEnv(g), "coversEnv(%q) = true, want false — a pattern this does not understand must read as uncovered", g)
	}
}

func TestEnvIgnored(t *testing.T) {
	t.Run("no .gitignore counts as not ignored", func(t *testing.T) {
		got, err := EnvIgnored(t.TempDir())
		require.NoError(t, err)
		assert.False(t, got, "a project with no .gitignore tracks .env")
	})

	t.Run("reports an existing rule", func(t *testing.T) {
		dir := t.TempDir()
		writeGitignore(t, dir, "dist\n.env\n")
		got, err := EnvIgnored(dir)
		require.NoError(t, err)
		assert.True(t, got)
	})
}

// healIgnores plans the missing rules and performs them, which is what the
// scaffold does in two steps. The behaviors below were EnsureEnvIgnored's before the
// Plan/Apply split took its only caller; they still have to hold.
func healIgnores(t *testing.T, dir string) bool {
	t.Helper()
	c, err := planIgnoreRules(dir)
	require.NoError(t, err)
	if c == nil {
		return false
	}
	require.NoError(t, c.apply(dir))
	return true
}

func TestPlanIgnoreRules(t *testing.T) {
	// A missing .gitignore is left alone on purpose: `astro init` writes one
	// from its template, so creating a second here would fight it. This only
	// heals a project that already has one.
	t.Run("a missing .gitignore is not created", func(t *testing.T) {
		dir := t.TempDir()
		assert.False(t, healIgnores(t, dir), "there was no .gitignore to add to")
		_, statErr := os.Stat(filepath.Join(dir, ".gitignore"))
		assert.Error(t, statErr, "a .gitignore was created; init's template owns that file")
	})

	t.Run("existing rules are left alone", func(t *testing.T) {
		dir := t.TempDir()
		before := "dist\n.env\n" + strings.Join(wantLocalRules, "\n") + "\n"
		writeGitignore(t, dir, before)

		assert.False(t, healIgnores(t, dir))
		assert.Equal(t, before, readGitignore(t, dir), "file rewritten; want it untouched")
	})

	t.Run("a rule is appended when missing", func(t *testing.T) {
		dir := t.TempDir()
		writeGitignore(t, dir, "dist\n")

		require.True(t, healIgnores(t, dir))

		got := readGitignore(t, dir)
		assert.Truef(t, hasLine(got, ".env"), "after appending, .env is not its own line: %q", got)
		assert.Truef(t, hasLine(got, "dist"), "existing content was not preserved: %q", got)
	})

	// A negated .env is not covered, so this is a file that needs healing even
	// though it mentions .env — the case the first version of coversEnv read
	// backwards.
	t.Run("a negated rule is healed", func(t *testing.T) {
		dir := t.TempDir()
		writeGitignore(t, dir, ".env\n!.env\n")

		assert.True(t, healIgnores(t, dir), "a .gitignore that re-includes .env leaves it tracked, so it needs the rule")
		assert.True(t, coversEnv(readGitignore(t, dir)), "after healing, .env is ignored again")
	})

	// A .gitignore with no trailing newline still ends up with both entries on
	// their own lines.
	//
	// Note what this does NOT prove: the HasSuffix guard in planIgnoreRules is
	// cosmetic rather than load-bearing. Removing it leaves this passing, because
	// the appended literal already begins with a newline — the guard only adds
	// the blank separator line. Mutation testing found that. The comment here
	// first claimed the guard stopped the new rule being glued onto the user's
	// last entry, which it does not, because that could never happen.
	t.Run("a file with no trailing newline still gets separate lines", func(t *testing.T) {
		dir := t.TempDir()
		writeGitignore(t, dir, "dist")

		require.True(t, healIgnores(t, dir))

		got := readGitignore(t, dir)
		for _, line := range []string{"dist", ".env"} {
			assert.Truef(t, hasLine(got, line), "%q is not its own line in %q", line, got)
		}
	})

	// The scaffold's own template has to cover .env, and nothing else asserts it.
	// The heal used to be skipped whenever the template was being written, which
	// made every new project's protection depend on this string — silently.
	t.Run("the scaffolded template covers .env", func(t *testing.T) {
		assert.True(t, coversEnv(gitignoreTemplate), "a freshly scaffolded project would track .env")
	})

	// The file being healed is the user's, so it keeps its mode and its identity
	// — see planIgnoreRules for why this package writes in place rather than
	// through a rename.
	//
	// Unix only, because the assertion has no meaning anywhere else: Windows has
	// no Unix permission bits, and Go reports 0666 for any writable file and 0444
	// for a read-only one. Asserting 0600 there fails on the platform rather than
	// on the behavior, which is what it did — 0x180 wanted, 0x1b6 seen.
	t.Run("the user's file keeps its mode", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("file modes are not Unix permission bits on Windows")
		}
		dir := t.TempDir()
		path := filepath.Join(dir, ".gitignore")
		require.NoError(t, os.WriteFile(path, []byte("dist\n"), 0o600))

		require.True(t, healIgnores(t, dir))

		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "healing republished the file with a different mode")
	})
}

// wantLocalRules is the per-machine list spelled out independently of
// localIgnoreRules, so the assertions below cannot agree with a list that lost
// an entry.
var wantLocalRules = []string{
	".astro/standalone/",
	".astro/worktrees/",
	".astro/*.local.yaml",
	".astro/*.local.yml",
	".astro/otto/*.local.json",
	".astro/otto/mcp.json",
	"plugins/fix_local_executor_pickle.py",
}

func TestLocalIgnoreRulesMatchTheTemplate(t *testing.T) {
	assert.Equal(t, wantLocalRules, localIgnoreRules, "the heal list and the expected list disagree")
	for _, rule := range wantLocalRules {
		assert.Truef(t, hasLine(gitignoreTemplate, rule), "the scaffolded .gitignore does not list %q", rule)
	}
	assert.Truef(t, strings.Contains(gitignoreTemplate, localIgnoreHeader),
		"the template's comment above the per-machine rules differs from the one an append writes")
	// A note goes directly above its rule, the same in the template as in an
	// append.
	for rule, note := range localIgnoreNotes {
		assert.Truef(t, strings.Contains(gitignoreTemplate, note+rule+"\n"),
			"the template does not carry the note for %q directly above it", rule)
	}
	// The pickle fix is ignored by name; plugins/ itself is the project's code.
	for _, blanket := range []string{"plugins", "plugins/", "/plugins/", "plugins/*", "plugins/*.py"} {
		assert.Falsef(t, hasLine(gitignoreTemplate, blanket), "the template ignores %q, which hides the project's plugins", blanket)
	}
	// Ignoring .astro/ as a whole would take .astro/config.yaml, the committed
	// deployment links, with it.
	for _, blanket := range []string{".astro", ".astro/", "/.astro", "/.astro/", ".astro/*", ".astro/otto/"} {
		assert.Falsef(t, hasLine(gitignoreTemplate, blanket), "the template ignores %q, which hides shared files", blanket)
	}
}

func TestHasIgnoreRule(t *testing.T) {
	const rule = ".astro/otto/mcp.json"
	for _, g := range []string{
		rule,
		"/" + rule,
		rule + "  ",
		rule + "\r",
		"dist\n" + rule + "\nbuild",
		"!" + rule + "\n" + rule,
	} {
		assert.Truef(t, hasIgnoreRule(g, rule), "hasIgnoreRule(%q) = false, want true", g)
	}
	for _, g := range []string{
		"",
		"# " + rule,
		"  " + rule,
		".astro/otto/mcp.json.bak",
		rule + "\n!" + rule,
		// Broader patterns are not recognized; the rule is appended beside them,
		// which is redundant rather than wrong.
		".astro/",
	} {
		assert.Falsef(t, hasIgnoreRule(g, rule), "hasIgnoreRule(%q) = true, want false", g)
	}
}

func TestPlanIgnoreRulesAddsThePerMachineRules(t *testing.T) {
	t.Run("a file that covers .env gets only the per-machine rules", func(t *testing.T) {
		dir := t.TempDir()
		writeGitignore(t, dir, "dist\n.env\n")

		c, err := planIgnoreRules(dir)
		require.NoError(t, err)
		require.NotNil(t, c, "the per-machine rules were not planned")
		assert.Equal(t, []string{".gitignore (added the per-machine rules)"}, c.Labels)
		require.NoError(t, c.apply(dir))

		got := readGitignore(t, dir)
		assert.True(t, strings.HasPrefix(got, "dist\n.env\n"), "existing content was not kept: %q", got)
		assert.Equal(t, 1, strings.Count(got, "\n.env\n"), ".env was appended again: %q", got)
		for _, rule := range wantLocalRules {
			assert.Truef(t, hasLine(got, rule), "%q is not its own line in %q", rule, got)
		}
	})

	t.Run("a file with neither gets both, reported separately", func(t *testing.T) {
		dir := t.TempDir()
		writeGitignore(t, dir, "dist\n")

		c, err := planIgnoreRules(dir)
		require.NoError(t, err)
		require.NotNil(t, c)
		assert.Equal(t, []string{
			".gitignore (added the .env rule)",
			".gitignore (added the per-machine rules)",
		}, c.Labels)
	})

	t.Run("only the missing rules are appended", func(t *testing.T) {
		dir := t.TempDir()
		writeGitignore(t, dir, ".env\n.astro/*.local.yaml\n/.astro/otto/mcp.json\n")

		require.True(t, healIgnores(t, dir))

		lines := strings.Split(readGitignore(t, dir), "\n")
		count := func(want string) int {
			n := 0
			for _, l := range lines {
				if l == want {
					n++
				}
			}
			return n
		}
		assert.Equal(t, 1, count(".astro/*.local.yaml"), "a rule already there was appended again")
		assert.Equal(t, 0, count(".astro/otto/mcp.json"), "an anchored spelling of the rule was not recognized")
		for _, rule := range []string{".astro/standalone/", ".astro/worktrees/", ".astro/*.local.yml", ".astro/otto/*.local.json", "plugins/fix_local_executor_pickle.py"} {
			assert.Equalf(t, 1, count(rule), "missing rule %q was not appended exactly once", rule)
		}
		assert.Contains(t, readGitignore(t, dir),
			"# Airflow 2 on macOS: the standalone engine regenerates this plugin.\nplugins/fix_local_executor_pickle.py\n",
			"the pickle fix rule was appended without the note that says what writes it")
	})

	t.Run("a negated rule is healed", func(t *testing.T) {
		dir := t.TempDir()
		writeGitignore(t, dir, ".env\n"+strings.Join(wantLocalRules, "\n")+"\n!.astro/otto/mcp.json\n")

		require.True(t, healIgnores(t, dir), "a .gitignore that re-includes mcp.json leaves it tracked")
		assert.True(t, strings.HasSuffix(readGitignore(t, dir), "\n.astro/otto/mcp.json\n"))
	})
}

// What git itself makes of the scaffolded .gitignore, on both paths that leave
// one behind: the template for a new project, and the heal for a project that
// had its own. Patterns are easy to get subtly wrong (a directory rule, an
// anchor, a glob that reaches too far), and git is the only judge that counts.
func TestGitIgnoresThePerMachineFiles(t *testing.T) {
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not on PATH")
	}

	ignored := []string{
		".env",
		".astro/standalone/airflow.db",
		".astro/standalone/standalone_admin_password.txt",
		".astro/worktrees/feature-x/README.md",
		".astro/config.local.yaml",
		".astro/config.local.yml",
		".astro/otto/permissions.local.json",
		".astro/otto/mcp.json",
		"plugins/fix_local_executor_pickle.py",
	}
	shared := []string{
		".astro/config.yaml",
		".astro/env.schema.yaml",
		".astro/otto/permissions.json",
		".astro/otto/extensions.json",
		".astro/memory/MEMORY.md",
		"pyproject.toml",
		// plugins/ is the project's code; only the pickle fix is ignored.
		"plugins/__init__.py",
		"plugins/my_operator.py",
		"plugins/helpers/fix_local_executor_pickle.py",
	}

	for _, tc := range []struct {
		name     string
		existing string
	}{
		{"a new project", ""},
		{"a project with its own .gitignore", "dist\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.existing != "" {
				writeGitignore(t, dir, tc.existing)
			}
			_, err := Run(dir, Options{})
			require.NoError(t, err)

			// No global or system config: a developer's core.excludesFile would
			// otherwise decide the "not ignored" half.
			empty := filepath.Join(t.TempDir(), "gitconfig")
			require.NoError(t, os.WriteFile(empty, nil, 0o600))
			git := func(args ...string) *exec.Cmd {
				cmd := exec.Command(gitBin, args...)
				cmd.Dir = dir
				cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+empty, "GIT_CONFIG_NOSYSTEM=1")
				return cmd
			}
			out, err := git("init", "-q").CombinedOutput()
			require.NoErrorf(t, err, "git init: %s", out)

			for _, rel := range append(append([]string{}, ignored...), shared...) {
				path := filepath.Join(dir, filepath.FromSlash(rel))
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
				if _, statErr := os.Stat(path); statErr != nil {
					require.NoError(t, os.WriteFile(path, []byte("x\n"), 0o600))
				}
			}

			// check-ignore exits 0 for an ignored path, 1 for one that is not,
			// and anything else on an error, which must fail rather than read
			// as either answer.
			checkIgnore := func(rel string) bool {
				out, err := git("check-ignore", "-q", "--", rel).CombinedOutput()
				if err == nil {
					return true
				}
				var exit *exec.ExitError
				require.ErrorAsf(t, err, &exit, "git check-ignore %s: %s", rel, out)
				require.Equalf(t, 1, exit.ExitCode(), "git check-ignore %s: %s", rel, out)
				return false
			}
			for _, rel := range ignored {
				assert.Truef(t, checkIgnore(rel), "git would track %s, a per-machine file", rel)
			}
			for _, rel := range shared {
				assert.Falsef(t, checkIgnore(rel), "git would ignore %s, which is shared and must stay committed", rel)
			}
		})
	}
}
