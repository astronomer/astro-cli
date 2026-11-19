package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	// sendTimeout is the maximum time to wait for the telemetry to be sent
	sendTimeout = 5 * time.Second
	// maxStdinBytes is the maximum bytes to read from stdin for the telemetry payload
	maxStdinBytes = 64 * 1024 // 64KB
)

// maxRedirects is net/http's own limit, which a CheckRedirect of our own
// replaces and so has to keep.
const maxRedirects = 10

var errDowngradeRedirect = errors.New("telemetry API redirected from https to http; not following it")

// sendClient is the client events go out on. It names no Transport, so it
// reads http.DefaultTransport per request, as http.DefaultClient does, and
// whatever the process installed there still applies. It follows redirects
// as http.DefaultClient would, except from https to plain http: the request
// can carry a bearer token, and a downgrade would send it unencrypted.
var sendClient = &http.Client{CheckRedirect: refuseDowngrade}

func refuseDowngrade(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("stopped after %d redirects", maxRedirects)
	}
	if via[0].URL.Scheme == "https" && req.URL.Scheme != "https" {
		return errDowngradeRedirect
	}
	return nil
}

// Send posts a TelemetryPayload synchronously without credentials, returning the
// HTTP status code. Callers holding an Astro token should use SendWithToken.
func Send(payload TelemetryPayload, apiURL string) (int, error) {
	return SendWithToken(payload, apiURL, "")
}

// SendWithToken posts a TelemetryPayload synchronously, attaching token as a
// bearer credential when it is non-empty.
//
// An empty token is normal, not an error — logged-out usage is still recorded,
// just unattributed. The token is sent as a header only, never serialized into
// the payload body.
func SendWithToken(payload TelemetryPayload, apiURL, token string) (int, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := sendClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	// Drain body to allow connection reuse
	_, _ = io.Copy(io.Discard, resp.Body) //nolint:errcheck // a body that will not drain costs the next request a new connection, nothing this one can act on

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("telemetry API returned status %d", resp.StatusCode)
	}

	return resp.StatusCode, nil
}

// SendEvent reads a JSON payload from stdin and sends it to the telemetry API.
// This is meant to be called from the hidden _telemetry-send command.
func SendEvent() error {
	payloadBytes, err := io.ReadAll(io.LimitReader(os.Stdin, maxStdinBytes))
	if err != nil {
		return err
	}

	var sp SenderPayload
	if err := json.Unmarshal(payloadBytes, &sp); err != nil {
		return err
	}

	addOSVersion(sp.Properties)
	_, err = SendWithToken(sp.TelemetryPayload, sp.APIURL, sp.Token)
	return err
}

// addOSVersion fills in os_version when the caller left it out. The CLI leaves
// it to this process: finding it runs a command (uname, or cmd /c ver), and
// the parent is the process the user is waiting on.
func addOSVersion(properties map[string]interface{}) {
	if properties == nil {
		return
	}
	if _, ok := properties[osVersionProperty]; !ok {
		properties[osVersionProperty] = GetOSVersion()
	}
}
