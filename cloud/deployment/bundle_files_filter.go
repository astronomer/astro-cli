package deployment

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/pkg/errors"
)

// defaultBundleExcludes are the local files a copy or sync into a bundle skips
// unless --no-default-excludes is set, as gitignore patterns.
var defaultBundleExcludes = []string{
	// Version control
	// .git is a file in a git worktree or submodule.
	".git", ".hg/", ".svn/",
	// OS clutter
	".DS_Store", "._*", "Thumbs.db", "desktop.ini",
	// Python
	"__pycache__/", "*.pyc", "*.pyo", ".venv/", ".pytest_cache/", ".mypy_cache/", ".ruff_cache/", ".tox/", "*.egg-info/",
	// Node
	"node_modules/",
	// Editors
	".idea/", ".vscode/", "*.swp", "*~",
	// Secrets
	".env", ".env.*",
}

// BundleFilterOptions say which local files a copy or sync skips.
type BundleFilterOptions struct {
	Excludes          []string
	Includes          []string
	NoDefaultExcludes bool
	NoGitignore       bool
}

func (o BundleFilterOptions) set() bool {
	return len(o.Excludes) > 0 || len(o.Includes) > 0 || o.NoDefaultExcludes || o.NoGitignore
}

// localFileFilter decides which paths under one local directory are excluded.
type localFileFilter struct {
	matcher gitignore.Matcher
	// The matcher's patterns, in rising priority: the default excludes, those of .gitignore files, then --exclude and --include.
	defaultExcludes, gitignores, overrides []gitignore.Pattern
	includes                               gitignore.Matcher
	// pathIncludes matches the --include patterns that name a path through a directory, and includePaths holds them split at each '/'.
	pathIncludes gitignore.Matcher
	includePaths [][]string
	// prefix is the filtered directory below the git repository root that patterns are matched from.
	prefix   []string
	defaults bool
}

func newLocalFileFilter(localDir string, opts BundleFilterOptions) (*localFileFilter, error) {
	absDir, err := filepath.Abs(localDir)
	if err != nil {
		return nil, err
	}
	root := absDir
	var prefix []string
	if !opts.NoGitignore {
		repoRoot, found := gitRepositoryRoot(absDir)
		if found && repoRoot != absDir {
			relPath, err := filepath.Rel(repoRoot, absDir)
			if err != nil {
				return nil, err
			}
			root, prefix = repoRoot, strings.Split(filepath.ToSlash(relPath), "/")
		}
	}

	filter := &localFileFilter{prefix: prefix, defaults: !opts.NoDefaultExcludes}
	if !opts.NoDefaultExcludes {
		for _, p := range defaultBundleExcludes {
			filter.defaultExcludes = append(filter.defaultExcludes, gitignore.ParsePattern(p, prefix))
		}
	}
	for _, p := range opts.Excludes {
		filter.overrides = append(filter.overrides, gitignore.ParsePattern(p, prefix))
	}
	if len(opts.Includes) > 0 {
		var includes, pathIncludes []gitignore.Pattern
		for _, p := range opts.Includes {
			include := gitignore.ParsePattern(p, prefix)
			filter.overrides = append(filter.overrides, gitignore.ParsePattern("!"+p, prefix))
			includes = append(includes, include)
			segments := strings.Trim(p, "/")
			if strings.Contains(segments, "/") {
				pathIncludes = append(pathIncludes, include)
				filter.includePaths = append(filter.includePaths, strings.Split(segments, "/"))
			}
		}
		filter.includes = gitignore.NewMatcher(includes)
		filter.pathIncludes = gitignore.NewMatcher(pathIncludes)
	}
	filter.ignore(nil)
	if opts.NoGitignore {
		return filter, nil
	}

	// In a git worktree or submodule .git is a file, and the repository's info/exclude is elsewhere.
	gitDir, err := os.Stat(filepath.Join(root, ".git"))
	if err == nil && gitDir.IsDir() {
		patterns, err := readIgnoreFile(filepath.Join(root, ".git", "info", "exclude"), nil)
		if err != nil {
			return nil, err
		}
		filter.ignore(patterns)
	}
	for i := range prefix {
		patterns, err := readIgnoreFile(filepath.Join(root, filepath.Join(prefix[:i]...), ".gitignore"), prefix[:i])
		if err != nil {
			return nil, err
		}
		filter.ignore(patterns)
	}
	_, err = os.Stat(absDir)
	if err == nil {
		err = filter.readGitignores(absDir)
		if err != nil {
			return nil, errors.Wrap(err, "failed to read .gitignore")
		}
	}
	return filter, nil
}

// ignore adds the patterns of a .gitignore file to the filter.
func (f *localFileFilter) ignore(patterns []gitignore.Pattern) {
	if len(patterns) == 0 && f.matcher != nil {
		return
	}
	f.gitignores = append(f.gitignores, patterns...)
	f.matcher = gitignore.NewMatcher(slices.Concat(f.defaultExcludes, f.gitignores, f.overrides))
}

// readGitignores adds the patterns of the .gitignore files in localDir and in
// each directory below it that the filter keeps, so a skipped directory is never read.
func (f *localFileFilter) readGitignores(localDir string) error {
	return filepath.WalkDir(localDir, func(dirPath string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.IsDir() {
			return err
		}
		domain := f.prefix
		if dirPath != localDir {
			relPath, err := filepath.Rel(localDir, dirPath)
			if err != nil {
				return err
			}
			relPath = filepath.ToSlash(relPath)
			if entry.Name() == ".git" || f.skipDir(dirPath, relPath) {
				return fs.SkipDir
			}
			domain = f.components(relPath)
		}
		patterns, err := readIgnoreFile(filepath.Join(dirPath, ".gitignore"), domain)
		if err != nil {
			return err
		}
		f.ignore(patterns)
		return nil
	})
}

// gitRepositoryRoot returns the nearest directory at or above dir that holds .git.
func gitRepositoryRoot(dir string) (string, bool) {
	for {
		_, err := os.Stat(filepath.Join(dir, ".git"))
		if err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// readIgnoreFile returns the patterns in a .gitignore or info/exclude file, relative to domain.
func readIgnoreFile(file string, domain []string) ([]gitignore.Pattern, error) {
	content, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return []gitignore.Pattern{}, nil
	}
	if err != nil {
		return nil, err
	}
	var patterns []gitignore.Pattern
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		patterns = append(patterns, gitignore.ParsePattern(line, domain))
	}
	return patterns, nil
}

func (f *localFileFilter) components(relPath string) []string {
	return append(slices.Clone(f.prefix), strings.Split(relPath, "/")...)
}

// excluded reports whether the file at relPath, slash-separated and relative to
// the filtered directory, is excluded. Inside an excluded directory, only an
// --include pattern that names a path through it keeps a file.
func (f *localFileFilter) excluded(relPath string) bool {
	components := f.components(relPath)
	if f.includes != nil && f.insideExcludedDir(relPath) {
		return !f.pathIncludes.Match(components, false)
	}
	return f.matcher.Match(components, false)
}

// insideExcludedDir reports whether a directory above relPath is excluded.
func (f *localFileFilter) insideExcludedDir(relPath string) bool {
	for dir := path.Dir(relPath); dir != "."; dir = path.Dir(dir) {
		if f.matcher.Match(f.components(dir), true) {
			return true
		}
	}
	return false
}

// skipDir reports whether to skip the directory at dirPath without reading it.
func (f *localFileFilter) skipDir(dirPath, relPath string) bool {
	components := f.components(relPath)
	if f.includes != nil && (f.includes.Match(components, true) || f.reachesInto(relPath)) {
		return false
	}
	if f.defaults && isVirtualenv(dirPath) {
		return true
	}
	return f.matcher.Match(components, true)
}

// reachesInto reports whether an --include pattern could match a path inside the directory at relPath.
func (f *localFileFilter) reachesInto(relPath string) bool {
	dir := strings.Split(relPath, "/")
	for _, pattern := range f.includePaths {
		if leadsInto(pattern, dir) {
			return true
		}
	}
	return false
}

// leadsInto reports whether the segments of a pattern could match a path inside dir.
func leadsInto(pattern, dir []string) bool {
	for i, name := range dir {
		if i == len(pattern) {
			return false
		}
		if pattern[i] == "**" {
			return true
		}
		matched, err := path.Match(pattern[i], name)
		if err != nil || !matched {
			return false
		}
	}
	return len(pattern) > len(dir)
}

func isVirtualenv(dirPath string) bool {
	_, err := os.Stat(filepath.Join(dirPath, "pyvenv.cfg"))
	return err == nil
}

// followLocalDir returns dir, or the directory it links to when dir is a
// symbolic link, which a walk of dir would not enter.
func followLocalDir(dir string) (string, error) {
	info, err := os.Lstat(dir)
	if err != nil || info.Mode()&fs.ModeSymlink == 0 {
		return dir, nil
	}
	target, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read %s", dir)
	}
	return target, nil
}

// localFiles returns the slash-separated paths, relative to localDir, of the
// regular files under it that filter keeps, and how many paths it skipped.
func localFiles(localDir string, filter *localFileFilter) (files []string, skipped int, err error) {
	err = filepath.WalkDir(localDir, func(filePath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if filePath == localDir {
			return nil
		}
		relPath, err := filepath.Rel(localDir, filePath)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)
		if entry.IsDir() {
			if filter.skipDir(filePath, relPath) {
				skipped++
				return fs.SkipDir
			}
			return nil
		}
		if filter.excluded(relPath) {
			skipped++
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symbolic link: skip it with --exclude", filePath)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("%s is not a regular file", filePath)
		}
		files = append(files, relPath)
		return nil
	})
	if err != nil {
		return nil, 0, errors.Wrapf(err, "failed to read %s", localDir)
	}
	return files, skipped, nil
}

func printSkipped(out io.Writer, skipped int) {
	if skipped > 0 {
		fmt.Fprintf(out, "Skipped %d paths excluded by default, by .gitignore or by --exclude\n", skipped)
	}
}
