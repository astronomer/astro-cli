//go:build unix

package scaffold

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// A file someone else owns can carry the owner write bit and still be closed to
// this user. The rename that replaces the manifest needs only the directory to
// be writable, so the bit alone would let EditManifest replace it.
//
// The test needs such a file and cannot make one without root, so it borrows
// /etc/hosts: root-owned and 0644 on every Unix this runs on. It is reached
// through a symlink in a temp dir and never written: the check refuses before
// the read, and /etc is not writable to this user either.
func TestEditManifestRefusesAFileThisUserCannotWrite(t *testing.T) {
	const foreign = "/etc/hosts"
	if os.Geteuid() == 0 {
		t.Skip("root may write any file, so there is no foreign file to refuse")
	}
	info, err := os.Stat(foreign)
	if err != nil {
		t.Skipf("%s is not there to borrow: %v", foreign, err)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(st.Uid) == os.Geteuid() || info.Mode().Perm()&0o200 == 0 {
		t.Skipf("%s is not a file another user owns with the owner write bit set", foreign)
	}

	assert.False(t, writable(foreign, info.Mode().Perm()))
	own := filepath.Join(t.TempDir(), "own.toml")
	require.NoError(t, os.WriteFile(own, nil, 0o644))
	assert.True(t, writable(own, 0o644))

	dir := t.TempDir()
	require.NoError(t, os.Symlink(foreign, filepath.Join(dir, manifest.Marker)))
	err = EditManifest(dir, nil, setKey([]string{"tool", "astro", "dockerfile"}, "Dockerfile.dev"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read-only for this user")
}
