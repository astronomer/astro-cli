//go:build !windows

package imagebuild

import (
	"os"
	"syscall"
)

// ownedByUser reports whether the user running this owns the file info
// describes.
func ownedByUser(info os.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == os.Getuid()
}
