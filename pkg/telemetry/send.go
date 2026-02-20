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
)

// Send posts a TelemetryPayload to the given API URL synchronously.
// It returns the HTTP status code and any error encountered.
func Send(payload TelemetryPayload, apiURL string) (int, error) {
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

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, fmt.Errorf("telemetry API returned status %d", resp.StatusCode)
	}

	return resp.StatusCode, nil
}

// SendEvent reads a JSON payload from stdin and sends it to the telemetry API.
// This is meant to be called from the hidden _telemetry-send command.
func SendEvent() error {
	payloadBytes, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}

	var sp senderPayload
	if err := json.Unmarshal(payloadBytes, &sp); err != nil {
		return err
	}

	_, err = Send(sp.TelemetryPayload, sp.APIURL)
	return err
}
