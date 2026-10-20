package scaffold

import (
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/moby/patternmatcher"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
)

// A Dockerfile moved onto its -base tag runs .astro/install-project.py, and a
// clone without that file does not build. So the project's .gitignore must not
// ignore it, and .astro/ is exactly the kind of directory a .gitignore reaches
// for: it holds per-machine state too.

// helperIgnoreHeader introduces the exception a .gitignore gains for the
// install helper.
const helperIgnoreHeader = "# Run by the Dockerfile's install step. Commit it, so a clone builds.\n"

// planHelperCommittable keeps the install helper a rewritten Dockerfile runs
// out of the project's .gitignore. A rule that ignores the file itself gains
// an exception after it, appended as the per-machine rules are (to the change
// planFiles made to the file, when it made one). A rule that ignores the .astro
// directory as a whole cannot be excepted from, since git does not look inside
// an ignored directory: that returns a note naming the line to change, and the
// rewrite does not happen.
func planHelperCommittable(dir string, cs *Changeset) (blocker string, err error) {
	var data []byte
	planned := slices.IndexFunc(cs.Changes, func(c Change) bool { return c.Path == fileGitignore })
	if planned >= 0 {
		data = cs.Changes[planned].Content
	} else if data, err = readIfPresent(filepath.Join(dir, fileGitignore)); err != nil {
		return "", err
	}
	pm, err := gitignoreMatcher(data)
	if err != nil || !matchesGitignore(pm, airflowrt.InstallHelperPath) {
		return "", nil //nolint:nilerr // patterns git could not read either ignore nothing this can name
	}
	helperDir := path.Dir(airflowrt.InstallHelperPath)
	if matchesGitignore(pm, helperDir) {
		return "Dockerfile: kept FROM its Astro Runtime image rather than moved onto that image's -base tag, because " +
			fileGitignore + " ignores " + helperDir + "/ as a whole, so the " + airflowrt.InstallHelperPath +
			" the moved file runs could not be committed, and a clone would not build. Ignore " + helperDir +
			"/* instead, which still ignores what is in it, and run astro init again", nil
	}
	s := string(data)
	if s != "" && !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	if s != "" {
		s += "\n"
	}
	s += helperIgnoreHeader + "!" + airflowrt.InstallHelperPath + "\n"
	if planned >= 0 {
		cs.Changes[planned].Content = []byte(s)
		return "", nil
	}
	cs.Changes = append(cs.Changes, Change{
		Kind: UpdateFile, Path: fileGitignore, Content: []byte(s),
		Labels: []string{fileGitignore + " (keeps " + airflowrt.InstallHelperPath + ", which the build runs, committed)"},
	})
	return "", nil
}

// gitignoreMatcher reads a .gitignore's patterns into a matcher with git's
// meaning: a pattern with no slash but a trailing one matches at any depth,
// one with a slash is anchored to the root, and a trailing slash, which limits
// a pattern to directories, is dropped, since every path this asks about is
// the one kind or the other already.
func gitignoreMatcher(data []byte) (*patternmatcher.PatternMatcher, error) {
	var patterns []string
	for _, line := range strings.Split(string(data), "\n") {
		p := strings.TrimRight(strings.TrimSuffix(line, "\r"), " ")
		if p == "" || strings.HasPrefix(p, "#") {
			continue
		}
		negate := strings.HasPrefix(p, "!")
		p = strings.TrimSuffix(strings.TrimPrefix(p, "!"), "/")
		if strings.Contains(p, "/") {
			p = strings.TrimPrefix(p, "/")
		} else {
			p = "**/" + p
		}
		if negate {
			p = "!" + p
		}
		patterns = append(patterns, p)
	}
	return patternmatcher.New(patterns)
}

// matchesGitignore reports whether pm ignores p, slash-separated and relative
// to the root, itself or through a directory above it.
func matchesGitignore(pm *patternmatcher.PatternMatcher, p string) bool {
	ok, err := pm.MatchesOrParentMatches(p)
	return err == nil && ok
}
