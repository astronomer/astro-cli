package telemetry

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"time"

	segment "github.com/segmentio/analytics-go/v3"
)

const (
	// sendTimeout is the maximum time to wait for the telemetry to be sent
	sendTimeout = 5 * time.Second
)

// SendEvent reads a JSON payload from stdin and sends it to Segment
// This is meant to be called from the hidden _telemetry-send command
func SendEvent() error {
	// Read payload from stdin
	payloadBytes, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}

	var payload TelemetryPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return err
	}

	// Create Segment client with timeout
	client, err := segment.NewWithConfig(payload.WriteKey, segment.Config{
		BatchSize: 1,
	})
	if err != nil {
		return err
	}
	defer client.Close()

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()

	// Send the track event
	err = client.Enqueue(segment.Track{
		AnonymousId: payload.AnonymousID,
		Event:       payload.Event,
		Properties:  payload.Properties,
		Context: &segment.Context{
			Extra: map[string]interface{}{
				"direct": true,
			},
		},
	})
	if err != nil {
		return err
	}

	// Wait for the event to be sent or timeout
	done := make(chan error, 1)
	go func() {
		done <- client.Close()
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
