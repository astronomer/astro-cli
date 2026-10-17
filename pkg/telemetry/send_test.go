package telemetry

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testPayload() TelemetryPayload {
	return TelemetryPayload{
		Source:      "astro-cli",
		Event:       "CLI Command",
		AnonymousID: "anon-123",
		Properties:  map[string]interface{}{"command": "deploy"},
	}
}

func TestSendAuthorization(t *testing.T) {
	tests := []struct {
		name       string
		send       func(url string) (int, error)
		expectAuth string
	}{
		{
			"attaches bearer token when present",
			func(url string) (int, error) { return SendWithToken(testPayload(), url, "tok-abc") },
			"Bearer tok-abc",
		},
		{
			// Logged-out usage still has to be recorded, so an empty token must
			// produce a normal, successful request rather than an error.
			"omits header when token is empty",
			func(url string) (int, error) { return SendWithToken(testPayload(), url, "") },
			"",
		},
		{
			"Send stays anonymous",
			func(url string) (int, error) { return Send(testPayload(), url) },
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotHeader http.Header
			var gotBody []byte

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotHeader = r.Header.Clone()
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				gotBody = body
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			status, err := tt.send(srv.URL)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, status)

			assert.Equal(t, tt.expectAuth, gotHeader.Get("Authorization"))
			assert.Equal(t, "application/json", gotHeader.Get("Content-Type"))

			// The token authenticates the request; it must never be part of the event.
			assert.NotContains(t, string(gotBody), "tok-abc")
		})
	}
}

func TestSenderPayloadOmitsEmptyToken(t *testing.T) {
	body, err := json.Marshal(SenderPayload{
		TelemetryPayload: testPayload(),
		APIURL:           "https://example.test",
	})
	require.NoError(t, err)
	assert.NotContains(t, string(body), "token")
}
