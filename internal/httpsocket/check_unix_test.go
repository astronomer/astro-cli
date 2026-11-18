//go:build !windows

package httpsocket

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Whoever owns the socket receives every request in plaintext, token
// included, so a socket that belongs to someone else is refused before a byte
// is sent.
func TestSocketOwnedByAnotherUserIsRefused(t *testing.T) {
	path, got := listen(t)
	me := currentUID
	t.Cleanup(func() { currentUID = me })
	currentUID = func() int { return me() + 1 }

	calls := 0
	rt, err := NewRouter(path, DefaultHosts, &directRecorder{}, func() { calls++ })
	require.NoError(t, err)
	err = do(t, rt, "https://api.astronomer.io/v1/x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "owned by uid")
	assert.Contains(t, err.Error(), SocketEnv)
	assert.Zero(t, calls)
	select {
	case s := <-got:
		t.Errorf("the socket saw a request for %s", s.host)
	default:
	}
}

// A socket in a directory any user can write to, without the sticky bit, can
// be replaced by another user between the check and the dial.
func TestSocketInAWorldWritableDirectoryIsRefused(t *testing.T) {
	path, _ := listen(t)
	dir := filepath.Dir(path)
	require.NoError(t, os.Chmod(dir, 0o777))
	rt, err := NewRouter(path, DefaultHosts, &directRecorder{}, nil)
	require.NoError(t, err)
	err = do(t, rt, "https://api.astronomer.io/v1/x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "any user can write to")

	// The sticky bit (as on /tmp) stops others replacing a file they do not
	// own, so the same directory with it set is fine.
	require.NoError(t, os.Chmod(dir, 0o777|fs.ModeSticky))
	require.NoError(t, do(t, rt, "https://api.astronomer.io/v1/x"))
}

// A symlink to the socket is judged by the socket it points at.
func TestSocketThroughASymlinkIsCheckedAtItsTarget(t *testing.T) {
	path, got := listen(t)
	link := filepath.Join(filepath.Dir(path), "link")
	require.NoError(t, os.Symlink(path, link))
	rt, err := NewRouter(link, DefaultHosts, &directRecorder{}, nil)
	require.NoError(t, err)
	require.NoError(t, do(t, rt, "https://api.astronomer.io/v1/x"))
	<-got

	me := currentUID
	t.Cleanup(func() { currentUID = me })
	currentUID = func() int { return me() + 1 }
	require.Error(t, do(t, rt, "https://api.astronomer.io/v1/x"))
}
