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

// captureRequest runs fn against a test server and returns the request the
// server saw, so assertions can be made on headers and body together.
func captureRequest(t *testing.T, fn func(url string)) (header http.Header, body []byte) {
	t.Helper()

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

	fn(srv.URL)
	return gotHeader, gotBody
}

func TestSendWithTokenSetsAuthorizationHeader(t *testing.T) {
	header, body := captureRequest(t, func(url string) {
		status, err := SendWithToken(testPayload(), url, "tok-abc")
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, status)
	})

	assert.Equal(t, "Bearer tok-abc", header.Get("Authorization"))
	assert.Equal(t, "application/json", header.Get("Content-Type"))

	// The token authenticates the request; it must never be part of the event.
	assert.NotContains(t, string(body), "tok-abc")
}

func TestSendWithTokenOmitsHeaderWhenTokenEmpty(t *testing.T) {
	// Logged-out users are the population these events exist to measure, so an
	// empty token must still produce a normal, successful request.
	header, _ := captureRequest(t, func(url string) {
		status, err := SendWithToken(testPayload(), url, "")
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, status)
	})

	assert.Empty(t, header.Get("Authorization"))
}

func TestSendStaysAnonymous(t *testing.T) {
	// Send is the compatibility entry point for callers outside this repo.
	header, _ := captureRequest(t, func(url string) {
		_, err := Send(testPayload(), url)
		assert.NoError(t, err)
	})

	assert.Empty(t, header.Get("Authorization"))
}

// TestSenderPayloadWireFormat pins the JSON contract between the CLI's
// spawnTelemetrySender and this package's SendEvent. The two structs are
// declared separately, in different modules, and a tag mismatch fails silently —
// the field just arrives empty. This test encodes the keys the CLI actually
// writes, so a rename on either side breaks the build here rather than quietly
// dropping the token in production.
func TestSenderPayloadWireFormat(t *testing.T) {
	const wire = `{
		"source": "astro-cli",
		"event": "CLI Command",
		"anonymousId": "anon-123",
		"properties": {"command": "deploy"},
		"api_url": "https://example.test/v1alpha1/telemetry",
		"token": "tok-abc"
	}`

	var sp senderPayload
	require.NoError(t, json.Unmarshal([]byte(wire), &sp))

	assert.Equal(t, "astro-cli", sp.Source)
	assert.Equal(t, "CLI Command", sp.Event)
	assert.Equal(t, "anon-123", sp.AnonymousID)
	assert.Equal(t, "https://example.test/v1alpha1/telemetry", sp.APIURL)
	assert.Equal(t, "tok-abc", sp.Token)
}

func TestSenderPayloadOmitsEmptyToken(t *testing.T) {
	body, err := json.Marshal(senderPayload{
		TelemetryPayload: testPayload(),
		APIURL:           "https://example.test",
	})
	require.NoError(t, err)
	assert.NotContains(t, string(body), "token")
}
