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
	if aInfo, err := os.Stat(a); err == nil {
		if bInfo, err := os.Stat(b); err == nil {
			return os.SameFile(aInfo, bInfo)
		}
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
// project checks make. The filesystem root is passed over without asking
// match: it is never the project above another. An error from match ends the
// walk and is returned.
func NearestDir(dir string, match func(string) (bool, error)) (string, error) {
	d, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		parent := filepath.Dir(d)
		if parent == d {
			return "", nil
		}
		ok, err := match(d)
		if err != nil {
			return "", err
		}
		if ok {
			return d, nil
		}
		d = parent
	}
}

// NearestReadableDir is NearestDir for a walk that takes a directory match
// cannot read for no match: an error from match passes that directory over
// rather than ending the walk, since nothing says it is a project. It returns
// "" when no directory matches.
func NearestReadableDir(dir string, match func(string) (bool, error)) string {
	found, err := NearestDir(dir, func(d string) (bool, error) {
		ok, err := match(d)
		return ok && err == nil, nil
	})
	if err != nil {
		return ""
	}
	return found
}
