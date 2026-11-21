package azure

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// An interrupt aborts an upload the server is still reading, rather than
// waiting for it to finish.
func TestUploadEndsWhenItsContextIsCanceled(t *testing.T) {
	received := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(received)
		<-release
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) }) // runs first: lets the handler return

	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-received; cancel() }()

	done := make(chan error, 1)
	go func() {
		_, err := Upload(ctx, srv.URL+"/container/blob?sig=x", strings.NewReader("tarball"))
		done <- err
	}()
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("the upload kept going after its context was canceled")
	}
}
