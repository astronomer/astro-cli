package scaffold

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/moby/patternmatcher"
	"github.com/moby/patternmatcher/ignorefile"
)

// WalkContext walks the paths under dir/sub that a build context of dir would
// carry under the ignore rules in ignore (a .dockerignore's text), parents
// before children and in lexical order, calling visit with each path relative
// to dir (in the OS's separators) and its entry. sub is "" for the whole
// context, or a slash-separated directory under dir; one that does not exist
// is walked as empty.
//
// A directory the rules leave out is not entered, unless a "!" rule could
// match something beneath it, so a virtualenv or an unreadable directory the
// rules exclude costs nothing and fails nothing. Symlinks are visited, never
// followed, as docker copies them.
func WalkContext(dir, sub, ignore string, visit func(rel string, d fs.DirEntry) error) error {
	patterns, err := ignorefile.ReadAll(strings.NewReader(ignore))
	if err != nil {
		return fmt.Errorf("reading the ignore rules: %w", err)
	}
	pm, err := patternmatcher.New(patterns)
	if err != nil {
		return fmt.Errorf("reading the ignore rules: %w", err)
	}
	root := filepath.Join(dir, filepath.FromSlash(sub))
	if _, err := os.Lstat(root); os.IsNotExist(err) {
		return nil
	}
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return walkErr
		}
		excluded, err := pm.MatchesOrParentMatches(rel)
		if err != nil {
			return err
		}
		if excluded {
			// An excluded directory entered because a "!" rule could match
			// beneath it has to be readable, as it has to be for docker.
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() && !exclusionBeneath(pm, rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		return visit(rel, d)
	})
}

// exclusionBeneath reports whether a "!" rule could match a path below the
// directory rel, which is the one reason to enter a directory the rules leave
// out.
func exclusionBeneath(pm *patternmatcher.PatternMatcher, rel string) bool {
	dirSegs := strings.Split(filepath.ToSlash(rel), "/")
	for _, p := range pm.Patterns() {
		if p.Exclusion() && couldMatchBeneath(strings.Split(filepath.ToSlash(p.String()), "/"), dirSegs) {
			return true
		}
	}
	return false
}

// couldMatchBeneath reports whether a pattern, split into segments, could
// match a path below the directory dirSegs: every directory segment matches
// the pattern's segment at the same depth, and the pattern goes on deeper, or
// a "**" is reached first.
func couldMatchBeneath(patSegs, dirSegs []string) bool {
	for i, seg := range dirSegs {
		if i >= len(patSegs) {
			return false
		}
		if strings.Contains(patSegs[i], "**") {
			return true
		}
		if ok, err := path.Match(patSegs[i], seg); err != nil || !ok {
			return false
		}
	}
	return len(patSegs) > len(dirSegs)
}

// IgnoreFor is the text of the ignore file a build of dockerfile reads, dir
// being the project root and dockerfile the manifest's [tool.astro]
// dockerfile: <dockerfile>.dockerignore when it exists, else .dockerignore,
// and "" when there is neither.
func IgnoreFor(dir, dockerfile string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, dockerignorePath(dir, dockerfile)))
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return string(data), nil
}

// DagFiles counts the DAG files under dir/dags, as the 1.x CLI counted them
// (.py files at any depth): onDisk is how many there are, and shipped how many
// the ignore rules in ignore leave in a build context of dir. A dags/ that
// does not exist has none.
func DagFiles(dir, ignore string) (onDisk, shipped int, err error) {
	count := func(n *int) func(string, fs.DirEntry) error {
		return func(rel string, d fs.DirEntry) error {
			if !d.IsDir() && strings.HasSuffix(rel, ".py") {
				*n++
			}
			return nil
		}
	}
	if err := WalkContext(dir, "dags", "", count(&onDisk)); err != nil {
		return 0, 0, err
	}
	if err := WalkContext(dir, "dags", ignore, count(&shipped)); err != nil {
		return 0, 0, err
	}
	return onDisk, shipped, nil
}
