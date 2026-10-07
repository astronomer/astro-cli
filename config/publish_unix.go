//go:build !windows

package config

import (
	"errors"
	"fmt"
	iofs "io/fs"
	"os"
	"syscall"

	"github.com/spf13/afero"

	"github.com/astronomer/astro-cli/pkg/fsatomic"
)

// The process identity, a file's owner and chown, as variables so a test can
// stand in for root, or for a file another user owns, without either being
// true.
var (
	geteuid   = os.Geteuid
	chownFile = os.Chown
	ownerOf   = statOwner
)

// owner is a file's uid and gid.
type owner struct{ uid, gid int }

func statOwner(info iofs.FileInfo) (owner, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return owner{}, false
	}
	return owner{int(st.Uid), int(st.Gid)}, true
}

// errOwnerNotKept reports that the temp file a save made has a different
// owner from the file it would replace, and this process cannot give it the
// old one.
var errOwnerNotKept = errors.New("the config's owner cannot be kept through a replace")

// publishConfig publishes data as path with mode perm, over existing (nil
// for a new file), keeping the file's owner.
//
// A rename puts a new file in place, and a new file belongs to whoever
// created it, where viper's in-place write kept the owner by rewriting the
// same inode. So the temp file's owner is checked against the file it will
// replace — the temp file itself, rather than a guess from the process's
// ids, because which group a new file gets depends on the directory (BSD
// semantics, a setgid bit) as much as on the process:
//
//   - The same owner, which is nearly always: published by rename.
//   - Different, and this process is root: the temp file is chowned to the
//     old owner before the rename, so the published file never belongs to
//     the wrong user, even for an instant. Without it `sudo astro login` on
//     macOS, where sudo keeps HOME, would leave the user a root-owned 0600
//     config their next `astro` cannot read — initHome marks it unreadable
//     and every later save is refused.
//   - Different, and the owner cannot be restored — the process is not root
//     (a shared project's group-writable .astro/config.yaml, saved by
//     someone other than its owner), or the chown fails (root without
//     CAP_CHOWN in a container, an owner not mapped into a user namespace):
//     the file is written in place, as it always was. A replace would take
//     the file from its owner, whose next save would then fail on a file
//     they can no longer write; failing the save instead would break a write
//     that used to work. Every chown failure is treated so, because none of
//     them makes the in-place write worse than it was before this.
//
// The in-place write brings back the window this file exists to close, for
// a reader that starts during the save. It is confined to a config shared
// between users, where that is how it has always behaved.
//
// When the temp-and-rename is done here rather than in fsatomic, nothing is
// lost: on Unix fsatomic's rename is a plain rename(2), and its retries are
// for Windows.
func publishConfig(path string, data []byte, perm iofs.FileMode, existing iofs.FileInfo) error {
	if existing == nil {
		return fsatomic.WriteFile(path, data, perm)
	}
	want, ok := ownerOf(existing)
	if !ok {
		return fsatomic.WriteFile(path, data, perm)
	}
	err := replaceConfigFile(afero.NewOsFs(), path, data, perm, func(tmp string) error {
		return keepOwner(tmp, want)
	})
	if errors.Is(err, errOwnerNotKept) {
		return writeInPlace(path, data)
	}
	return err
}

// keepOwner gives tmp the owner want, when it does not have it already.
func keepOwner(tmp string, want owner) error {
	info, err := os.Stat(tmp)
	if err != nil {
		return err
	}
	if got, ok := ownerOf(info); ok && got == want {
		return nil
	}
	if geteuid() != 0 {
		return errOwnerNotKept
	}
	if err := chownFile(tmp, want.uid, want.gid); err != nil {
		return fmt.Errorf("%w: %w", errOwnerNotKept, err)
	}
	return nil
}

// writeInPlace rewrites the existing file at path, as viper's WriteConfigAs
// did: same inode, same owner, same mode.
func writeInPlace(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return err
	}
	_, werr := f.Write(data)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	return werr
}
