package httpsocket

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// do sends a GET through rt and reports whether it succeeded.
func do(t *testing.T, rt http.RoundTripper, url string) error {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
	require.NoError(t, err)
	resp, err := (&http.Client{Transport: rt}).Do(req)
	if resp != nil {
		resp.Body.Close()
	}
	return err
}

// The notice says a request really went through the socket, so it comes once
// per process, on the first connection that succeeds, and never for a command
// that only talked to other hosts or never reached the socket at all.
func TestNoticeOnceOnFirstConnection(t *testing.T) {
	t.Run("once, however many requests", func(t *testing.T) {
		path, got := listen(t)
		calls := 0
		rt, err := NewRouter(path, DefaultHosts, &directRecorder{}, func() { calls++ })
		require.NoError(t, err)
		for range 3 {
			require.NoError(t, do(t, rt, "https://api.astronomer.io/v1/x"))
			<-got
		}
		assert.Equal(t, 1, calls)
	})

	t.Run("never for hosts that go direct", func(t *testing.T) {
		path, _ := listen(t)
		calls := 0
		rt, err := NewRouter(path, DefaultHosts, &directRecorder{}, func() { calls++ })
		require.NoError(t, err)
		require.NoError(t, do(t, rt, "http://localhost:8080/health"))
		require.NoError(t, do(t, rt, "https://cdn.astronomer.io/x"))
		assert.Zero(t, calls)
	})

	t.Run("never when the socket could not be reached", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "hs")
		require.NoError(t, err)
		t.Cleanup(func() { os.RemoveAll(dir) })
		calls := 0
		rt, err := NewRouter(filepath.Join(dir, "absent"), DefaultHosts, &directRecorder{}, func() { calls++ })
		require.NoError(t, err)
		require.Error(t, do(t, rt, "https://api.astronomer.io/v1/x"))
		assert.Zero(t, calls)
	})

	t.Run("Install hands the notice the resolved path", func(t *testing.T) {
		original := http.DefaultTransport
		t.Cleanup(func() { http.DefaultTransport = original })
		path, got := listen(t)
		var noted []string
		getenv := func(k string) string {
			if k == SocketEnv {
				return path
			}
			return ""
		}
		require.NoError(t, Install(getenv, func(s string) { noted = append(noted, s) }))
		require.NoError(t, do(t, http.DefaultTransport, "https://api.astronomer.io/v1/x"))
		<-got
		assert.Equal(t, []string{path}, noted)
	})
}
