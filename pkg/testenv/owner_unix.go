//go:build unix

package testenv

import (
	"io/fs"
	"os"
	"syscall"
)

// ownedByMe reports whether info says the file belongs to this user.
func ownedByMe(info fs.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == os.Getuid()
}
