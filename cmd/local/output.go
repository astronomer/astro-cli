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
	// labels ("variable", "airflow-variable") back into a section. Empty for
	// every other event.
	Section string `json:"section,omitempty"`
	Key     string `json:"key,omitempty"`
	Reason  string `json:"reason,omitempty"`
	// AlreadyStopped marks the state event of a stop that found nothing
	// running.
	AlreadyStopped bool `json:"already_stopped,omitempty"`
	// Watch, Files, Install, Removed and Reasons carry a --watch event: the
	// localrt.WatchEventKind, the files the edit covered, the requirements
	// installed and dropped, and the codes of what forced a restart. Empty
	// for every other event.
	Watch   string           `json:"watch,omitempty"`
	Files   []string         `json:"files,omitempty"`
	Install []string         `json:"install,omitempty"`
	Removed []string         `json:"removed,omitempty"`
	Reasons []localrt.Reason `json:"reasons,omitempty"`
}

// callbacks bridges localrt progress into the renderer. A start's write errors
// are dropped: callbacks have no error channel, a start is a change that
// finishes whatever happens to its output, and a broken pipe surfaces on the
// command's own final write.
func (c *cli) callbacks(r cliout.Renderer) localrt.Callbacks {
	return c.callbacksFailing(r, nil)
}

// callbacksFailing is callbacks for a loop that must end when its output
// cannot be written (--watch): fail is called with the write error.
func (c *cli) callbacksFailing(r cliout.Renderer, fail func(error)) localrt.Callbacks {
	// In text the progress of a start goes to stderr, with the other notes: it
	// comes before the work it reports on, and stdout carries the final status
	// alone. In json the events are the stream on stdout.
	progress := textWriter(r)
	return localrt.Callbacks{
		OnState: func(s localrt.State, err error) {
			e := event{Event: "state", State: s}
			if err != nil {
				e.Error = err.Error()
			}
			werr := r.EmitEvent(e, func(io.Writer) error {
				if err != nil {
					_, werr := fmt.Fprintf(progress, "airflow: %s (%s)\n", s, err)
					return werr
				}
				_, werr := fmt.Fprintf(progress, "airflow: %s\n", s)
				return werr
			})
			if werr != nil && fail != nil {
				fail(werr)
			}
		},
		OnLine: func(l localrt.LogLine) {
			if werr := r.EmitEvent(logEvent(l), func(io.Writer) error {
				return renderLogLine(progress, l)
			}); werr != nil && fail != nil {
				fail(werr)
			}
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
