package telemetry

import (
	"sync"

	sharedtel "github.com/astronomer/astro-cli/pkg/telemetry"
)

// SentEvent is one event a test captured instead of sending.
type SentEvent struct {
	Event      string
	Properties map[string]interface{}
	// Token is the credential the event would have carried, "" for anonymous.
	Token string
}

// CaptureEventsForTesting makes this process, a test binary, track events as
// the CLI does and hands them to the returned function instead of sending
// them. restore puts the real behavior back (a test binary sends nothing).
// It is test support that other packages' tests import, so it cannot live in
// a _test.go file.
func CaptureEventsForTesting() (sent func() []SentEvent, restore func()) {
	var (
		mu     sync.Mutex
		events []SentEvent
	)
	prevTest, prevDispatch := isTestRunFn, dispatch
	isTestRunFn = func() bool { return false }
	dispatch = func(payload sharedtel.TelemetryPayload, _, token string) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, SentEvent{Event: payload.Event, Properties: payload.Properties, Token: token})
	}
	return func() []SentEvent {
			mu.Lock()
			defer mu.Unlock()
			return append([]SentEvent(nil), events...)
		}, func() {
			isTestRunFn, dispatch = prevTest, prevDispatch
		}
}
