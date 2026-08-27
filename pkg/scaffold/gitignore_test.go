package scaffold

import (
	"os"
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
// code under test. EnsureEnvIgnored decides what to write by calling coversEnv,
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

// healEnvIgnored plans the .env rule and performs it, which is what the scaffold
// does in two steps. The behaviors below were EnsureEnvIgnored's before the
// Plan/Apply split took its only caller; they still have to hold.
func healEnvIgnored(t *testing.T, dir string) bool {
	t.Helper()
	c, err := planEnvIgnored(dir)
	require.NoError(t, err)
	if c == nil {
		return false
	}
	require.NoError(t, c.apply(dir))
	return true
}

func TestPlanEnvIgnored(t *testing.T) {
	// A missing .gitignore is left alone on purpose: `astro init` writes one
	// from its template, so creating a second here would fight it. This only
	// heals a project that already has one.
	t.Run("a missing .gitignore is not created", func(t *testing.T) {
		dir := t.TempDir()
		assert.False(t, healEnvIgnored(t, dir), "there was no .gitignore to add to")
		_, statErr := os.Stat(filepath.Join(dir, ".gitignore"))
		assert.Error(t, statErr, "a .gitignore was created; init's template owns that file")
	})

	t.Run("an existing rule is left alone", func(t *testing.T) {
		dir := t.TempDir()
		const before = "dist\n.env\n"
		writeGitignore(t, dir, before)

		assert.False(t, healEnvIgnored(t, dir))
		assert.Equal(t, before, readGitignore(t, dir), "file rewritten; want it untouched")
	})

	t.Run("a rule is appended when missing", func(t *testing.T) {
		dir := t.TempDir()
		writeGitignore(t, dir, "dist\n")

		require.True(t, healEnvIgnored(t, dir))

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

		assert.True(t, healEnvIgnored(t, dir), "a .gitignore that re-includes .env leaves it tracked, so it needs the rule")
		assert.True(t, coversEnv(readGitignore(t, dir)), "after healing, .env is ignored again")
	})

	// A .gitignore with no trailing newline still ends up with both entries on
	// their own lines.
	//
	// Note what this does NOT prove: the HasSuffix guard in planEnvIgnored is
	// cosmetic rather than load-bearing. Removing it leaves this passing, because
	// the appended literal already begins with a newline — the guard only adds
	// the blank separator line. Mutation testing found that. The comment here
	// first claimed the guard stopped the new rule being glued onto the user's
	// last entry, which it does not, because that could never happen.
	t.Run("a file with no trailing newline still gets separate lines", func(t *testing.T) {
		dir := t.TempDir()
		writeGitignore(t, dir, "dist")

		require.True(t, healEnvIgnored(t, dir))

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
	// — see EnsureEnvIgnored for why this package writes in place rather than
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

		require.True(t, healEnvIgnored(t, dir))

		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "healing republished the file with a different mode")
	})
}
