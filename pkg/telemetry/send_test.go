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

// The sender fills in os_version when the CLI left it out, and keeps one a
// caller such as Desktop set itself.
func TestAddOSVersion(t *testing.T) {
	props := map[string]interface{}{}
	addOSVersion(props)
	assert.Equal(t, GetOSVersion(), props[osVersionProperty])

	props = map[string]interface{}{osVersionProperty: "Desktop's own"}
	addOSVersion(props)
	assert.Equal(t, "Desktop's own", props[osVersionProperty])

	addOSVersion(nil)
}

// An https telemetry API that redirects to plain http is not followed, so a
// token never goes out unencrypted; an https-to-https redirect still is.
func TestSendDoesNotFollowADowngrade(t *testing.T) {
	plainReached := false
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		plainReached = true
		w.WriteHeader(http.StatusOK)
	}))
	defer plain.Close()

	var tlsAuth string
	tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/downgrade":
			http.Redirect(w, r, plain.URL+"/events", http.StatusTemporaryRedirect)
		case "/moved":
			http.Redirect(w, r, "/events", http.StatusTemporaryRedirect)
		default:
			tlsAuth = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer tlsSrv.Close()

	orig := sendClient
	t.Cleanup(func() { sendClient = orig })
	sendClient = &http.Client{Transport: tlsSrv.Client().Transport, CheckRedirect: refuseDowngrade}

	_, err := SendWithToken(testPayload(), tlsSrv.URL+"/downgrade", "tok-abc")
	require.ErrorIs(t, err, errDowngradeRedirect)
	assert.False(t, plainReached, "the plain-http target was sent the request")

	status, err := SendWithToken(testPayload(), tlsSrv.URL+"/moved", "tok-abc")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "Bearer tok-abc", tlsAuth)
}

func TestSendClientNamesNoTransport(t *testing.T) {
	assert.Nil(t, sendClient.Transport, "a nil Transport reads http.DefaultTransport, where the CLI installs its socket routing")
}
