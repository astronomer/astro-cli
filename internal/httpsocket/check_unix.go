//go:build !windows

package httpsocket

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// currentUID is the user the socket has to belong to. A variable so a test
// can play another user.
var currentUID = os.Getuid

// checkSocket refuses a socket the current user does not own, or one in a
// directory where another user could replace it: one writable by others
// without the sticky bit. Ownership is checked on the socket itself, after
// following symlinks, and the directory is the one that socket is in.
//
// A path that does not exist passes, so the dial reports it the usual way.
func checkSocket(path string) error {
	resolved, err := filepath.EvalSymlinks(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return err
	}
	uid := currentUID()
	if st, ok := info.Sys().(*syscall.Stat_t); ok && int(st.Uid) != uid {
		return fmt.Errorf("the socket is owned by uid %d, not you (uid %d); refusing to send requests to it", st.Uid, uid)
	}
	dir, err := os.Stat(filepath.Dir(resolved))
	if err != nil {
		return err
	}
	if mode := dir.Mode(); mode.Perm()&0o002 != 0 && mode&fs.ModeSticky == 0 {
		return fmt.Errorf("the socket is in %s, which any user can write to; refusing to send requests to it", filepath.Dir(resolved))
	}
	return nil
}
