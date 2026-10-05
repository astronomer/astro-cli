package local

import (
	"fmt"
	"io"
	"time"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

// The output contract itself — the formats, the Renderer and its single door,
// the json error object, the exit codes — is shared by the whole CLI and lives
// in cmd/cliout. What is here is cmd/local's own: the progress events its streaming
// surfaces emit.

// event is one progress update on a streaming surface (start, logs). In
// json mode each event is one NDJSON line.
type event struct {
	Event     string        `json:"event"`
	State     localrt.State `json:"state,omitempty"`
	Component string        `json:"component,omitempty"`
	Time      string        `json:"time,omitempty"`
	Text      string        `json:"text,omitempty"`
	Error     string        `json:"error,omitempty"`
	// Section, Key and Reason carry an env-schema warning in machine-readable
	// form beside the prose in Text, so a consumer need not regex the human
	// labels ("env var", "Airflow variable") back into a section. Empty for
	// every other event.
	Section string `json:"section,omitempty"`
	Key     string `json:"key,omitempty"`
	Reason  string `json:"reason,omitempty"`
	// AlreadyStopped marks the state event of a stop that found nothing
	// running.
	AlreadyStopped bool `json:"already_stopped,omitempty"`
}

// callbacks bridges localrt progress into the renderer. Write errors are
// dropped: callbacks have no error channel, and a broken pipe surfaces on
// the command's own final write.
func (c *cli) callbacks(r cliout.Renderer) localrt.Callbacks {
	return localrt.Callbacks{
		OnState: func(s localrt.State, err error) {
			e := event{Event: "state", State: s}
			if err != nil {
				e.Error = err.Error()
			}
			//nolint:errcheck // see the comment above
			r.Emit(e, func(w io.Writer) error {
				if err != nil {
					_, werr := fmt.Fprintf(w, "airflow: %s (%s)\n", s, err)
					return werr
				}
				_, werr := fmt.Fprintf(w, "airflow: %s\n", s)
				return werr
			})
		},
		OnLine: func(l localrt.LogLine) {
			//nolint:errcheck // see the comment above
			r.Emit(logEvent(l), func(w io.Writer) error {
				return renderLogLine(w, l)
			})
		},
	}
}

func logEvent(l localrt.LogLine) event {
	return event{
		Event:     "log",
		Component: l.Component,
		Time:      l.Time.Format(time.RFC3339),
		Text:      l.Text,
	}
}

func renderLogLine(w io.Writer, l localrt.LogLine) error {
	_, err := fmt.Fprintf(w, "%s [%s] %s\n", l.Time.Format(time.RFC3339), l.Component, l.Text)
	return err
}
