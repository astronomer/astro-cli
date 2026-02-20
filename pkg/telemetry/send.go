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

// SendEvent reads a JSON payload from stdin and sends it to the telemetry API
// This is meant to be called from the hidden _telemetry-send command
func SendEvent() error {
	// Read payload from stdin
	payloadBytes, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}

	var sp senderPayload
	if err := json.Unmarshal(payloadBytes, &sp); err != nil {
		return err
	}

	// Marshal just the TelemetryPayload for the API request body
	body, err := json.Marshal(sp.TelemetryPayload)
	if err != nil {
		return err
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sp.APIURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telemetry API returned status %d", resp.StatusCode)
	}

	return nil
}
