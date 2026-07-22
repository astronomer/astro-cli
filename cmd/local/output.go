package local

import (
	"encoding/json"
	"errors"
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

// wrapErrorOutput makes every leaf under cmd honor --output json on its failure
// path: a failed command emits one JSON error object on stdout instead of only
// cobra's plaintext "Error:" on stderr. Applied once over the whole v2 tree so
// every command shares the behavior. Streaming commands still emit their own
// NDJSON; this adds the terminal error object when the command returns an error.
func wrapErrorOutput(d Deps, cmd *cobra.Command) {
	for _, sub := range cmd.Commands() {
		wrapErrorOutput(d, sub)
	}
	inner := cmd.RunE
	if inner == nil {
		return
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		err := inner(cmd, args)
		if err == nil {
			return nil
		}
		// The command already wrote a richer JSON object (a plan-build failure's
		// structured payload); carry the exit non-zero without a second object.
		var shown errJSONShown
		if errors.As(err, &shown) {
			cmd.SilenceErrors = true
			return err
		}
		// A command that carries its own exit code has already rendered its
		// result (check's NDJSON summary, for one) and main turns the code into
		// the exit status; keep cobra from printing "Error: exit code N" over it.
		var exit *ExitError
		if errors.As(err, &exit) {
			cmd.SilenceErrors = true
			return err
		}
		if cmdOutputFormat(cmd) == FormatJSON {
			emitJSONError(d.Stdout, err)
			// The object is on stdout; silence cobra so json mode stays a single
			// object and nothing lands on stderr.
			cmd.SilenceErrors = true
		}
		return err
	}
}

// cmdOutputFormat reads the resolved --output value off cmd, defaulting to text
// when the flag is absent (the dev stub parses it itself) or unparseable.
func cmdOutputFormat(cmd *cobra.Command) Format {
	f := cmd.Flags().Lookup("output")
	if f == nil {
		return FormatText
	}
	format, err := ParseFormat(f.Value.String())
	if err != nil {
		return FormatText
	}
	return format
}

// errJSONShown marks an error whose command already wrote its own JSON object
// to stdout, so wrapErrorOutput does not add the generic one on top.
type errJSONShown struct{ err error }

func (e errJSONShown) Error() string { return e.err.Error() }
func (e errJSONShown) Unwrap() error { return e.err }

// emitJSONError writes the single JSON error object a failed command reports in
// json mode.
func emitJSONError(w io.Writer, err error) {
	//nolint:errcheck // the command already failed; a write error changes nothing
	json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
		Code  int    `json:"code"`
	}{Error: err.Error(), Code: 1})
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
