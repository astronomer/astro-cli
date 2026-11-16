package testenv

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
)

// uvPythonDir is where the test binaries share uv's managed Pythons: a
// directory per user under the OS temporary directory, which is neither the
// developer's home nor any test's.
//
// uv downloads a managed interpreter (tens of MB) into UV_PYTHON_INSTALL_DIR,
// by default ~/.local/share/uv/python, and the tests under cmd/local ask for
// one. With HOME in a directory each binary removes, they would download it in
// every package on every run; pointing at the developer's real directory would
// write into their home, which no test may. This is a plain directory uv owns
// and locks, so concurrent test binaries are safe, and it lasts until the OS
// clears its temporary directory. uv's download cache needs no such setting:
// the CLI pins UV_CACHE_DIR to its own directory under the (isolated) cache
// home.
func uvPythonDir() string {
	return filepath.Join(os.TempDir(), "astro-test-uv-"+strconv.Itoa(os.Getuid()), "python")
}

// prepareUVDir makes the shared directory, and refuses one it did not make or
// that someone else could have changed. uv runs the interpreters in it, so a
// directory another user created first, left open, or replaced with a symlink
// would let them choose what runs. Each of the two levels is made with Mkdir,
// so that a race is lost to EEXIST rather than won by following a link, and is
// then Lstat'ed: it must be a real directory, owned by this user, closed to
// everyone else.
func prepareUVDir() (string, error) {
	python := uvPythonDir()
	for _, dir := range []string{filepath.Dir(python), python} {
		if err := os.Mkdir(dir, dirPerm); err != nil && !errors.Is(err, fs.ErrExist) {
			return "", err
		}
		info, err := os.Lstat(dir)
		if err != nil {
			return "", err
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			return "", fmt.Errorf("%s is a symlink", dir)
		case !info.IsDir():
			return "", fmt.Errorf("%s is not a directory", dir)
		case !ownedByMe(info):
			return "", fmt.Errorf("%s is owned by someone else", dir)
		case runtime.GOOS != goosWindows && info.Mode().Perm()&0o077 != 0:
			return "", fmt.Errorf("%s is open to other users (mode %v)", dir, info.Mode().Perm())
		}
	}
	return python, nil
}

// goosWindows is runtime.GOOS on Windows, where modes and owners differ.
const goosWindows = "windows"
