//go:build !unix

package testenv

import "io/fs"

// ownedByMe: Windows keeps a directory's owner in an ACL, and its temporary
// directory is already per user, so there is nothing to compare.
func ownedByMe(fs.FileInfo) bool { return true }
