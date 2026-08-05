package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
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

// Send posts a TelemetryPayload to the given API URL synchronously, without
// credentials. It returns the HTTP status code and any error encountered.
//
// Kept for callers outside this repo (e.g. Desktop) that have no Astro token.
// Callers that do have one should use SendWithToken so the server can attribute
// the event to an organization.
func Send(payload TelemetryPayload, apiURL string) (int, error) {
	return SendWithToken(payload, apiURL, "")
}

// SendWithToken posts a TelemetryPayload synchronously, attaching the given
// Astro token as a bearer credential when it is non-empty.
//
// An empty token is normal, not an error: users who have never run `astro login`
// are exactly the population these events measure, so the request still goes out
// and simply lands unattributed. The token is never written into the payload
// body — it travels only as a header, so it cannot leak into debug output.
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

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	// Drain body to allow connection reuse by http.DefaultClient
	_, _ = io.Copy(io.Discard, resp.Body)

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

	var sp senderPayload
	if err := json.Unmarshal(payloadBytes, &sp); err != nil {
		return err
	}

	_, err = SendWithToken(sp.TelemetryPayload, sp.APIURL, sp.Token)
	return err
}
