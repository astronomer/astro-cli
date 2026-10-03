//go:build !windows

package secrets

import (
	"os"
	"syscall"
)

// tightenable reports whether Unix permission bits govern access to the
// directory, so that prepareDir's chmod means something.
const tightenable = true

// ownedByCurrentUser reports whether fi belongs to the account this process
// runs as. A FileInfo without a Stat_t cannot be checked and is accepted.
func ownedByCurrentUser(fi os.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return true
	}
	return int(st.Uid) == os.Getuid()
}
