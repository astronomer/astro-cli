package secrets

import (
	"errors"
	"fmt"
	"os"
)

// ErrVaultDirUnsafe reports a vault directory this package will not use: a
// symlink, something other than a directory, or a directory another account
// owns. It wraps ErrKeyringUnavailable because it is the same condition to a
// caller, the vault as a whole cannot be used, and nothing is read from or
// written to it until the directory is fixed by hand.
//
// A symlink is refused rather than followed because following it lets whoever
// can replace the link point both tools at a directory of their choosing: they
// would read entries from it and, worse, write new secrets into it.
var ErrVaultDirUnsafe = fmt.Errorf("%w: the vault directory is unsafe to use", ErrKeyringUnavailable)

// prepareDir checks the vault directory before any operation on it, and
// reports whether it exists. With create, a missing directory is made, owner
// only.
//
// An existing directory readable or writable by group or others is tightened
// to dirPerm rather than refused: that is how an older build or a hand-made
// directory leaves it, and refusing would strand the values with no change in
// what an attacker can do. Every operation checks, not only the first, because
// the directory can be replaced while a long-lived process holds a store.
//
// Only the final path component is checked: a symlinked ~/.astro, which a
// dotfiles manager makes, is the user's arrangement and is followed.
func prepareDir(dir string, create bool) (bool, error) {
	fi, err := os.Lstat(dir) //nolint:gosec // G703: dir is the vault directory the caller owns, DefaultDir in production
	if errors.Is(err, os.ErrNotExist) {
		if !create {
			return false, nil
		}
		if err := os.MkdirAll(dir, dirPerm); err != nil { //nolint:gosec // G703: dir is the vault directory the caller owns, DefaultDir in production
			return false, fmt.Errorf("create secrets dir: %w", err)
		}
		fi, err = os.Lstat(dir) //nolint:gosec // G703: as above
	}
	if err != nil {
		return false, fmt.Errorf("inspect secrets dir: %w", err)
	}
	switch {
	case fi.Mode()&os.ModeSymlink != 0:
		return false, fmt.Errorf("%w: %s is a symlink; replace it with a real directory", ErrVaultDirUnsafe, dir)
	case !fi.IsDir():
		return false, fmt.Errorf("%w: %s is not a directory", ErrVaultDirUnsafe, dir)
	case !ownedByCurrentUser(fi):
		return false, fmt.Errorf("%w: %s is owned by another account", ErrVaultDirUnsafe, dir)
	}
	if tightenable && fi.Mode().Perm()&^dirPerm != 0 {
		if err := os.Chmod(dir, dirPerm); err != nil { //nolint:gosec // G703: as above
			return false, fmt.Errorf("restrict secrets dir permissions: %w", err)
		}
	}
	return true, nil
}
