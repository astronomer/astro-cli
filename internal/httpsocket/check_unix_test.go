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

// playAnotherUser makes the socket's owner (this process's user) look like
// someone else to checkSocket.
func playAnotherUser(t *testing.T) {
	t.Helper()
	me := currentUID
	t.Cleanup(func() { currentUID = me })
	currentUID = func() int { return me() + 1 }
}

// A socket in a directory only its owner can write to is accepted whoever owns
// it: nobody else could have put it there. This is the container case, where a
// mounted socket keeps the host's numeric owner.
func TestSocketOwnedByAnotherUserInAPrivateDirectoryIsAccepted(t *testing.T) {
	path, got := listen(t)
	playAnotherUser(t)
	rt, err := NewRouter(path, DefaultHosts, &directRecorder{}, nil)
	require.NoError(t, err)
	require.NoError(t, do(t, rt, "https://api.astronomer.io/v1/x"))
	<-got
}

// In a shared directory with the sticky bit, such as /tmp, another user could
// have created the socket first. Whoever owns the socket receives every
// request in plaintext, token included, so one that is not this user's is
// refused before a byte is sent.
func TestSocketOwnedByAnotherUserInASharedDirectoryIsRefused(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("run as root, the socket is root's, which is trusted in any directory")
	}
	path, got := listen(t)
	require.NoError(t, os.Chmod(filepath.Dir(path), 0o777|fs.ModeSticky))

	rt, err := NewRouter(path, DefaultHosts, &directRecorder{}, nil)
	require.NoError(t, err)
	require.NoError(t, do(t, rt, "https://api.astronomer.io/v1/x"), "this user's own socket in a shared directory")
	<-got

	playAnotherUser(t)
	calls := 0
	rt, err = NewRouter(path, DefaultHosts, &directRecorder{}, func() { calls++ })
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

// Without the sticky bit, any user can replace the socket between the check
// and the dial, so it is refused whoever owns it.
func TestSocketInAWorldWritableDirectoryIsRefused(t *testing.T) {
	path, _ := listen(t)
	require.NoError(t, os.Chmod(filepath.Dir(path), 0o777))
	rt, err := NewRouter(path, DefaultHosts, &directRecorder{}, nil)
	require.NoError(t, err)
	err = do(t, rt, "https://api.astronomer.io/v1/x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "any user can write to")
}

// A symlink is judged by the socket it leads to and that socket's directory,
// not by where the link itself lives.
func TestSocketThroughASymlinkIsCheckedAtItsTarget(t *testing.T) {
	path, got := listen(t)
	linkDir, err := os.MkdirTemp("", "hl")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(linkDir) })
	link := filepath.Join(linkDir, "link")
	require.NoError(t, os.Symlink(path, link))

	rt, err := NewRouter(link, DefaultHosts, &directRecorder{}, nil)
	require.NoError(t, err)
	require.NoError(t, do(t, rt, "https://api.astronomer.io/v1/x"))
	<-got

	require.NoError(t, os.Chmod(filepath.Dir(path), 0o777))
	err = do(t, rt, "https://api.astronomer.io/v1/x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), filepath.Dir(path))
}

func TestOwnerTrusted(t *testing.T) {
	assert.True(t, ownerTrusted(501, 501), "this user's own socket")
	assert.True(t, ownerTrusted(0, 501), "root's socket")
	assert.False(t, ownerTrusted(502, 501), "another user's socket")
	assert.True(t, ownerTrusted(0, 0), "root's socket, as root")
}
