package fileutil

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

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

// IsHomeDir reports whether dir is the home directory GetHomeDir finds,
// however it is spelled. config.IsHomeDir is the same test against
// config.HomePath, which tests move; this one is for the packages that may not
// read config/.
func IsHomeDir(dir string) bool {
	home, err := GetHomeDir()
	return err == nil && SamePath(dir, home)
}

// SamePath reports whether a and b name the same file or directory. When both
// can be read that is os.SameFile, which sees through symlinks and through the
// case differences a case-insensitive file system ignores. When either cannot,
// it compares the two absolute paths, with what symlinks resolve resolved,
// ignoring case on Windows. An empty path names nothing.
func SamePath(a, b string) bool {
	return SamePathAs(b)(a)
}

// SamePathAs is SamePath against a fixed b, read once rather than on every
// comparison, for a caller that asks the same question of many paths.
func SamePathAs(b string) func(a string) bool {
	if b == "" {
		return func(string) bool { return false }
	}
	bInfo, bErr := os.Stat(b)
	resolvedB := sync.OnceValue(func() string { return resolvePath(b) })
	return func(a string) bool {
		if a == "" {
			return false
		}
		if bErr == nil {
			if aInfo, err := os.Stat(a); err == nil {
				return os.SameFile(aInfo, bInfo)
			}
		}
		ra, rb := resolvePath(a), resolvedB()
		if runtime.GOOS == "windows" {
			return strings.EqualFold(ra, rb)
		}
		return ra == rb
	}
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
// true are passed over without asking match: neither is ever the project
// above another. An error from match ends the walk and is returned.
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

// NearestReadableDir is NearestDir for a walk that takes a directory match
// cannot read for no match: an error from match passes that directory over
// rather than ending the walk, since nothing says it is a project. It returns
// "" when no directory matches.
func NearestReadableDir(dir string, skip func(string) bool, match func(string) (bool, error)) string {
	found, err := NearestDir(dir, skip, func(d string) (bool, error) {
		ok, err := match(d)
		return ok && err == nil, nil
	})
	if err != nil {
		return ""
	}
	return found
}
