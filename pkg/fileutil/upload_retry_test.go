package fileutil

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A try that fails before its request is sent has already read the file once;
// the next one still sends all of it, not an empty file.
func TestUploadFileRetryAfterARequestErrorSendsTheWholeFile(t *testing.T) {
	var got []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	calls := 0
	prev := newRequestWithContext
	t.Cleanup(func() { newRequestWithContext = prev })
	newRequestWithContext = func(ctx context.Context, method, url string, body io.Reader) (*http.Request, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("could not build the request")
		}
		return http.NewRequestWithContext(ctx, method, url, body)
	}

	file := filepath.Join(t.TempDir(), "dags.tar.gz")
	require.NoError(t, os.WriteFile(file, []byte("the dags tarball"), 0o600))

	err := UploadFile(&UploadFileArguments{
		FilePath:            file,
		TargetURL:           srv.URL,
		FormFileFieldName:   "file",
		Headers:             map[string]string{},
		MaxTries:            2,
		InitialDelayInMS:    1,
		BackoffFactor:       2,
		RetryDisplayMessage: "uploading",
		Out:                 io.Discard,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
	assert.Contains(t, string(got), "the dags tarball")
}
