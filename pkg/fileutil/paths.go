package fileutil

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	homedir "github.com/mitchellh/go-homedir"
)

// GetWorkingDir returns the current working directory
func GetWorkingDir() (string, error) {
	return os.Getwd()
}

// GetHomeDir returns the home directory
func GetHomeDir() (string, error) {
	return homedir.Dir()
}

// SamePath reports whether a and b name the same file or directory. When both
// can be read that is os.SameFile, which sees through symlinks and through the
// case differences a case-insensitive file system ignores. When either cannot,
// it compares the two absolute paths, with what symlinks resolve resolved,
// ignoring case on Windows. An empty path names nothing.
func SamePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	aInfo, aErr := os.Stat(a)
	bInfo, bErr := os.Stat(b)
	if aErr == nil && bErr == nil {
		return os.SameFile(aInfo, bInfo)
	}
	ra, rb := resolvePath(a), resolvePath(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(ra, rb)
	}
	return ra == rb
}

func resolvePath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return filepath.Clean(p)
}

// NearestDir returns dir, or the nearest directory above it, for which match
// reports true, and "" when none does. It is the one walk up the tree the
// project checks make. The filesystem root and every directory skip reports
// true are passed over without asking match: neither is ever a project, the
// home directory because its .astro/ holds the CLI's own settings. An error
// from match ends the walk and is returned; a match that treats a directory
// it cannot read as no project says so by returning false and no error.
func NearestDir(dir string, skip func(string) bool, match func(string) (bool, error)) (string, error) {
	d, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		parent := filepath.Dir(d)
		if parent == d {
			return "", nil
		}
		if skip == nil || !skip(d) {
			ok, err := match(d)
			if err != nil {
				return "", err
			}
			if ok {
				return d, nil
			}
		}
		d = parent
	}
}
