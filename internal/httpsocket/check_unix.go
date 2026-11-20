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

// checkSocket refuses a socket another user could have put in place: one in a
// directory any user can write to without the sticky bit, where it could be
// swapped, or one in a shared directory with the sticky bit (/tmp) that is
// owned by someone other than this user or root. Symlinks are followed, and
// the socket they lead to and its directory are what is checked.
//
// A socket in a directory nobody else can write to passes whoever owns it.
// Nobody else could have created it there, and its owner is often not this
// user for a good reason: a socket mounted into a container or a sandbox
// keeps the host's numeric owner, which inside is another user or nobody.
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
	dirPath := filepath.Dir(resolved)
	dir, err := os.Stat(dirPath)
	if err != nil {
		return err
	}
	mode := dir.Mode()
	if mode.Perm()&0o002 == 0 {
		return nil
	}
	if mode&fs.ModeSticky == 0 {
		return fmt.Errorf("the socket is in %s, which any user can write to; refusing to send requests to it", dirPath)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	if uid := currentUID(); !ownerTrusted(int(st.Uid), uid) {
		return fmt.Errorf("the socket is in the shared directory %s and owned by uid %d, not by you (uid %d); refusing to send requests to it", dirPath, st.Uid, uid)
	}
	return nil
}

// ownerTrusted reports whether a socket owned by owner, in a shared directory,
// may carry requests for user me: one of their own, or root's, since root
// could read the requests anyway.
func ownerTrusted(owner, me int) bool {
	return owner == me || owner == 0
}
