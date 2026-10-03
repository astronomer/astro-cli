//go:build windows

package secrets

import "os"

// tightenable is false on Windows: access is governed by ACLs, which the
// profile directory's inheritance already restricts to the user, and os.Chmod
// only toggles the read-only attribute.
const tightenable = false

// ownedByCurrentUser accepts every directory on Windows, where ownership is an
// ACL question os.FileInfo does not answer. The profile directory the vault
// lives under is already private to its user.
func ownedByCurrentUser(os.FileInfo) bool { return true }
