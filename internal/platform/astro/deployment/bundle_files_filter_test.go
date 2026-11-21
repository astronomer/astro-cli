package deployment

import (
	"os"
	"path/filepath"
)

func writeLocalFiles(s *Suite, dir string, files map[string]string) {
	for name, content := range files {
		s.NoError(os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755))
		s.NoError(os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	}
}

func (s *Suite) TestLocalFileFilterGitignores() {
	s.Run("applies .gitignore files in kept directories and the repository's info/exclude", func() {
		repo := s.T().TempDir()
		writeLocalFiles(s, repo, map[string]string{".git/info/exclude": "*.tmp\n", "sub/.gitignore": "*.log\n"})
		filter, err := newLocalFileFilter(repo, BundleFilterOptions{})
		s.Require().NoError(err)

		s.True(filter.excluded("a.tmp"))
		s.True(filter.excluded("sub/a.log"))
		s.False(filter.excluded("a.log"))
	})

	s.Run("skips .git where it is a file, as in a git worktree", func() {
		fromDir := s.T().TempDir()
		writeLocalFiles(s, fromDir, map[string]string{".git": "gitdir: /elsewhere\n", "a.py": "a"})
		filter, err := newLocalFileFilter(fromDir, BundleFilterOptions{})
		s.Require().NoError(err)
		files, _, err := localFiles(fromDir, filter)
		s.NoError(err)
		s.Equal([]string{"a.py"}, files)
	})

	s.Run("never reads a directory the filter skips", func() {
		fromDir := s.T().TempDir()
		writeLocalFiles(s, fromDir, map[string]string{"a.py": "a", "node_modules/locked/x.js": "x"})
		locked := filepath.Join(fromDir, "node_modules", "locked")
		s.NoError(os.Chmod(locked, 0o000))
		s.T().Cleanup(func() { _ = os.Chmod(locked, 0o755) })

		filter, err := newLocalFileFilter(fromDir, BundleFilterOptions{})
		s.Require().NoError(err)
		files, _, err := localFiles(fromDir, filter)
		s.NoError(err)
		s.Equal([]string{"a.py"}, files)
	})
}

func (s *Suite) TestLocalFileFilterIncludes() {
	s.Run("keeps a file inside a skipped directory only for an --include that names a path through it", func() {
		fromDir := s.T().TempDir()
		writeLocalFiles(s, fromDir, map[string]string{"a.py": "a", "node_modules/x.py": "x", "node_modules/pkg/y.py": "y", "node_modules/other/z.py": "z"})
		filter, err := newLocalFileFilter(fromDir, BundleFilterOptions{Includes: []string{"*.py", "node_modules/pkg/"}})
		s.Require().NoError(err)

		files, skipped, err := localFiles(fromDir, filter)
		s.NoError(err)
		s.ElementsMatch([]string{"a.py", "node_modules/pkg/y.py"}, files)
		s.Equal(2, skipped)
		s.True(filter.excluded("node_modules/other/z.py"))
		s.False(filter.excluded("node_modules/pkg/y.py"))
	})

	s.Run("still skips an excluded directory that no --include names", func() {
		fromDir := s.T().TempDir()
		writeLocalFiles(s, fromDir, map[string]string{"a.py": "a", "node_modules/x.py": "x"})
		filter, err := newLocalFileFilter(fromDir, BundleFilterOptions{Includes: []string{"*.py"}})
		s.Require().NoError(err)

		s.True(filter.skipDir(filepath.Join(fromDir, "node_modules"), "node_modules"))
		s.True(filter.excluded("node_modules/x.py"))
		s.False(filter.excluded("a.py"))
	})
}
