package config

import (
	"bytes"
	"errors"
	"fmt"
	iofs "io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/afero"
	"github.com/spf13/viper"

	"github.com/astronomer/astro-cli/pkg/fsatomic"
)

// A config file is published whole: written to a temp file beside it and
// renamed over it, so a process reading it sees the file as it was or as it
// is now, never a file halfway through being written.
//
// viper's WriteConfigAs writes in place — it truncates the file and then
// writes it — and only saveConfig takes the config's lock. A process starting
// up reads the home config without it (initHome runs from main, before any
// command), so one that started while another was saving could read the file
// in the instant after the truncate. An empty file is valid YAML: the reader
// saw no contexts at all, and a command that needed one failed with "no
// context set". Otto and its subagents run `astro auth token --force` side by
// side, each of which saves the config, which is where it was seen.
//
// Taking the lock on every read would close the window too, but at the cost
// of every command waiting on every save, and of a reader that cannot take
// the lock (a read-only home) still racing. A rename needs no cooperation
// from the reader.

// writeConfigFile publishes data as file on fsys, keeping the mode the file
// has now, or 0600 for a new one: the home config holds the API token.
//
// On the OS filesystem the write goes through publishConfig, which hands it
// to pkg/fsatomic: that owns the temp-and-rename and, on Windows, waits out a
// reader holding the destination open — there a rename onto an open file
// fails rather than replacing it. Any other afero filesystem is a test's, in
// this one process, with no other process to race; it gets the same
// temp-and-rename through afero, so a test on a MemMapFs exercises the same
// shape as the real thing.
func writeConfigFile(fsys afero.Fs, file string, data []byte) error {
	_, osfs := fsys.(*afero.OsFs)
	target := file
	if osfs {
		target = resolveConfigLink(file)
	}
	perm := filePerm
	existing, err := fsys.Stat(target)
	switch {
	case err == nil:
		perm = existing.Mode().Perm()
		if err := probeWritable(fsys, target); err != nil {
			return err
		}
	case errors.Is(err, iofs.ErrNotExist):
		existing = nil
	default:
		return err
	}
	if osfs {
		return publishConfig(target, data, perm, existing)
	}
	return replaceConfigFile(fsys, target, data, perm, nil)
}

// probeWritable refuses a file its owner has made read-only.
//
// A rename needs write permission on the directory, not on the file it
// replaces, so without this a config chmod'd 0400 would be replaced as
// readily as any other. viper's in-place write opened the file for writing
// and failed there, and a user who made their config read-only meant that.
// The probe opens it the same way, without truncating it, and the error is
// the one that open gave.
func probeWritable(fsys afero.Fs, file string) error {
	f, err := fsys.OpenFile(file, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	return f.Close()
}

// resolveConfigLink returns the file a symlinked config points at.
//
// A rename replaces the directory entry it lands on, so publishing onto a
// symlink would swap a dotfile manager's link for a regular file and leave its
// target holding the old contents. viper's in-place write went through the
// link, and so does this. A link that cannot be resolved — a dangling one, or
// no link at all — is written where it is.
func resolveConfigLink(file string) string {
	if resolved, err := filepath.EvalSymlinks(file); err == nil {
		return resolved
	}
	return file
}

// replaceConfigFile writes data to a temp file beside file, with mode perm,
// and renames it over file. beforeRename, when not nil, runs on the temp
// file's path once it is complete, and a failure from it abandons the write.
func replaceConfigFile(fsys afero.Fs, file string, data []byte, perm iofs.FileMode, beforeRename func(tmp string) error) error {
	tmp, err := afero.TempFile(fsys, filepath.Dir(file), "."+filepath.Base(file)+".*")
	if err != nil {
		return fmt.Errorf("creating temp file for %s: %w", file, err)
	}
	tmpPath := tmp.Name()
	_, werr := tmp.Write(data)
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = fsys.Chmod(tmpPath, perm)
	}
	if werr == nil && beforeRename != nil {
		werr = beforeRename(tmpPath)
	}
	if werr == nil {
		werr = fsys.Rename(tmpPath, file)
	}
	if werr != nil {
		_ = fsys.Remove(tmpPath) //nolint:errcheck // best-effort cleanup; the write error is the one worth reporting
		return fmt.Errorf("writing %s: %w", file, werr)
	}
	return nil
}

// readConfigFile reads the config file v is set to, from fsys.
//
// The read half of writeConfigFile. On Unix a rename swaps the directory entry
// atomically and a reader never notices; on Windows a file that is being
// renamed onto cannot be opened for that instant, and a reader arriving then
// would report a healthy config as unreadable. fsatomic.ReadFile waits that
// out, as it does for every other file the CLI publishes this way.
func readConfigFile(v *viper.Viper, fsys afero.Fs) error {
	file := v.ConfigFileUsed()
	if _, ok := fsys.(*afero.OsFs); !ok || file == "" {
		return v.ReadInConfig()
	}
	data, err := fsatomic.ReadFile(file)
	if err != nil {
		return err
	}
	return v.ReadConfig(bytes.NewReader(data))
}
