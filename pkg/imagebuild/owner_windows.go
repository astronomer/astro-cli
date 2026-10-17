//go:build windows

package imagebuild

import "os"

// ownedByUser reports whether the user running this owns the file info
// describes. On Windows the temp dir is the user's own profile directory
// (%LOCALAPPDATA%\Temp), so everything in it is.
func ownedByUser(os.FileInfo) bool { return true }
