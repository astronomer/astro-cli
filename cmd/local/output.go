package local

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/pkg/localrt"
)

// Format selects how a command renders its results.
type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
)

// ParseFormat validates an --output flag value.
func ParseFormat(s string) (Format, error) {
	switch Format(s) {
	case FormatText, FormatJSON:
		return Format(s), nil
	default:
		return "", fmt.Errorf("unknown output format %q (supported: text, json)", s)
	}
}

// Renderer writes command results. Emit is the single output path for every
// v2 command: json mode encodes the value as one line — so repeated calls
// on a streaming surface form NDJSON — and text mode runs the text renderer
// over the same value. Human output is a rendering of the same data, never
// a separate code path.
type Renderer struct {
	Format Format
	Out    io.Writer
}

// Emit writes v. In text mode it calls text, which must render v and
// nothing else.
func (r Renderer) Emit(v any, text func(w io.Writer) error) error {
	if r.Format == FormatJSON {
		return json.NewEncoder(r.Out).Encode(v)
	}
	return text(r.Out)
}

// addOutputFlag registers the shared --output flag on cmd's persistent
// flags, so one registration covers a whole command family.
func addOutputFlag(cmd *cobra.Command, target *string) {
	cmd.PersistentFlags().StringVarP(target, "output", "o", string(FormatText), "Output format: text or json")
}

// event is one progress update on a streaming surface (start, logs). In
// json mode each event is one NDJSON line.
type event struct {
	Event     string        `json:"event"`
	State     localrt.State `json:"state,omitempty"`
	Component string        `json:"component,omitempty"`
	Time      string        `json:"time,omitempty"`
	Text      string        `json:"text,omitempty"`
	Error     string        `json:"error,omitempty"`
}

// callbacks bridges localrt progress into the renderer. Write errors are
// dropped: callbacks have no error channel, and a broken pipe surfaces on
// the command's own final write.
func (c *cli) callbacks(r Renderer) localrt.Callbacks {
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
