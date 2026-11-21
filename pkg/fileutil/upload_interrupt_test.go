package fileutil

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An interrupt ends an upload the server is stalling on at once, and is not
// retried: one try, then the cancellation, however many tries were allowed.
func TestUploadFileEndsOnInterruptWithoutRetrying(t *testing.T) {
	var hits atomic.Int32
	received := make(chan struct{}, 8)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hits.Add(1)
		received <- struct{}{}
		<-release
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) }) // runs first: lets the handler return

	file := filepath.Join(t.TempDir(), "dags.tar.gz")
	require.NoError(t, os.WriteFile(file, []byte("dags"), 0o600))

	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-received; cancel() }()

	done := make(chan error, 1)
	go func() {
		done <- UploadFile(&UploadFileArguments{
			Context:             ctx,
			FilePath:            file,
			TargetURL:           srv.URL,
			FormFileFieldName:   "file",
			Headers:             map[string]string{},
			MaxTries:            8,
			InitialDelayInMS:    60_000,
			BackoffFactor:       2,
			RetryDisplayMessage: "uploading",
			Out:                 &discard{},
		})
	}()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(10 * time.Second):
		t.Fatal("the upload kept going after the interrupt")
	}
	assert.Equal(t, int32(1), hits.Load())
}

type discard struct{}

func (*discard) Write(p []byte) (int, error) { return len(p), nil }
