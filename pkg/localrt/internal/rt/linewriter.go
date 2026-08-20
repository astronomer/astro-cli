package rt

import (
	"bytes"
	"strings"
)

// LineWriter is an io.Writer that splits its input into lines and hands each
// complete one to Emit. Subprocesses write whole lines, but nothing guarantees
// write boundaries, so partial lines are buffered.
//
// Lives here rather than beside the engines because it exists to feed
// Callbacks.OnLine, and because every consumer of this contract that shells out
// to a subprocess needs it — including pkg/imagebuild, which is a separate
// module and so cannot reach an engine-private copy.
type LineWriter struct {
	buf  bytes.Buffer
	Emit func(string)
}

func (w *LineWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	for {
		line, err := w.buf.ReadString('\n')
		if err != nil {
			// Partial line: put it back and wait for the rest.
			w.buf.WriteString(line)
			break
		}
		if line = strings.TrimRight(line, "\r\n"); line != "" {
			w.Emit(line)
		}
	}
	return len(p), nil
}

// Flush emits any trailing line that arrived without a newline.
func (w *LineWriter) Flush() {
	if rest := strings.TrimRight(w.buf.String(), "\r\n"); rest != "" {
		w.Emit(rest)
	}
	w.buf.Reset()
}

// OnState reports a state transition if the caller asked for them. Callbacks
// fields are optional, so every emit site would otherwise repeat this nil check.
func OnState(cb Callbacks, s State, err error) {
	if cb.OnState != nil {
		cb.OnState(s, err)
	}
}
