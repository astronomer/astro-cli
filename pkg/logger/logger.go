// Package logger is the CLI's one diagnostic log: the lines --verbosity turns
// on, written to stderr apart from a command's output.
//
// It is built on log/slog and installs its handler as slog's default, so
// there is one level and one place the lines go however a package writes
// them. Code in the root module may use the printf-style helpers here; the
// pkg/* sub-modules (pkg/localrt, pkg/proxy, ...) cannot import this module
// and log with log/slog directly, which reaches the same handler. A stray
// call to the standard log package does too, at info level, because slog's
// default carries it.
//
// The level is set once per run by the root command (cmd.SetupLogging) from
// --verbosity, ASTRO_VERBOSITY or the verbosity setting, in that order, and
// is warn when none says otherwise.
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/input"
)

// LevelOff is above every level a line is written at, so it writes nothing.
// It is what the 1.x names "fatal" and "panic" select: neither was ever a
// level the CLI logged at, so in 1.x too they meant "nothing".
const LevelOff = slog.LevelError + 4

// DefaultLevel is the level of a run that names none.
const DefaultLevel = slog.LevelWarn

var (
	level = func() *slog.LevelVar {
		v := new(slog.LevelVar)
		v.Set(DefaultLevel)
		return v
	}()

	mu  sync.Mutex
	out io.Writer = os.Stderr

	std = slog.New(&handler{})
)

func init() {
	slog.SetDefault(std)
}

// ParseLevel reads a level's name: debug, info, warn or error, in any case.
// It also takes the other names 1.x's --verbosity took, so a script written
// for it keeps working: warning (warn), trace (debug), and fatal and panic
// (LevelOff).
func ParseLevel(name string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "trace", "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	case "fatal", "panic":
		return LevelOff, nil
	}
	return 0, fmt.Errorf("%q is not a log level: use debug, info, warn or error", name)
}

// SetLevel sets the lowest level a line is written at.
func SetLevel(l slog.Level) { level.Set(l) }

// GetLevel is the lowest level a line is written at.
func GetLevel() slog.Level { return level.Level() }

// Enabled reports whether a line at l would be written, for a caller that
// would otherwise do work to build one.
func Enabled(l slog.Level) bool { return l >= level.Level() }

// SetOutput sends every line from here on to w.
func SetOutput(w io.Writer) {
	mu.Lock()
	defer mu.Unlock()
	out = w
}

// Debug writes a debug line made of args, as fmt.Sprint makes one.
func Debug(args ...any) {
	if Enabled(slog.LevelDebug) {
		std.Log(context.Background(), slog.LevelDebug, fmt.Sprint(args...))
	}
}

// Debugf writes a debug line.
func Debugf(format string, args ...any) { logf(slog.LevelDebug, format, args) }

// Infof writes an info line.
func Infof(format string, args ...any) { logf(slog.LevelInfo, format, args) }

// Warnf writes a warning line.
func Warnf(format string, args ...any) { logf(slog.LevelWarn, format, args) }

// logf formats only once it knows the line will be written.
func logf(l slog.Level, format string, args []any) {
	if Enabled(l) {
		std.Log(context.Background(), l, fmt.Sprintf(format, args...))
	}
}

// handler writes a record as one line a person reads: the label the CLI leads
// its own warnings with (ansi.WarningLabel), the message, then the attributes
// as key=value. The time leads the line when it goes to a file rather than a
// terminal (the proxy daemon's log, otto's, a CI log), where it is read later.
//
// Every message and value is printed without escape sequences or control
// characters (pkg/input), because a line can carry text from elsewhere: an
// error from an API, a path, a container's name.
type handler struct {
	// attrs are the attributes WithAttrs added, already rendered with a
	// leading space.
	attrs string
	// group qualifies the keys of attributes added after WithGroup.
	group string
}

func (h *handler) Enabled(_ context.Context, l slog.Level) bool { return Enabled(l) }

func (h *handler) Handle(_ context.Context, r slog.Record) error { //nolint:gocritic // hugeParam: slog.Handler's signature
	mu.Lock()
	defer mu.Unlock()
	var b strings.Builder
	if stamped(out) {
		t := r.Time
		if t.IsZero() {
			t = time.Now()
		}
		b.WriteString(t.Format(time.RFC3339))
		b.WriteByte(' ')
	}
	b.WriteString(label(out, r.Level))
	b.WriteByte(' ')
	// A message may run to several lines (a report, a server's error);
	// continuation lines are indented, so none can pass for a line of its own
	// with a label of its choosing.
	b.WriteString(strings.ReplaceAll(strings.TrimRight(input.Lines(r.Message), "\n"), "\n", "\n    "))
	b.WriteString(h.attrs)
	r.Attrs(func(a slog.Attr) bool {
		writeAttr(&b, h.group, a)
		return true
	})
	b.WriteByte('\n')
	_, err := io.WriteString(out, b.String())
	return err
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	var b strings.Builder
	b.WriteString(h.attrs)
	for _, a := range attrs {
		writeAttr(&b, h.group, a)
	}
	return &handler{attrs: b.String(), group: h.group}
}

func (h *handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &handler{attrs: h.attrs, group: qualify(h.group, name)}
}

func writeAttr(b *strings.Builder, group string, a slog.Attr) {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return
	}
	if a.Value.Kind() == slog.KindGroup {
		g := group
		if a.Key != "" {
			g = qualify(group, a.Key)
		}
		for _, ga := range a.Value.Group() {
			writeAttr(b, g, ga)
		}
		return
	}
	b.WriteByte(' ')
	b.WriteString(input.Plain(qualify(group, a.Key)))
	b.WriteByte('=')
	v := input.Plain(a.Value.String())
	if v == "" || strings.ContainsAny(v, " \"=") {
		v = strconv.Quote(v)
	}
	b.WriteString(v)
}

func qualify(group, key string) string {
	if group == "" {
		return key
	}
	return group + "." + key
}

// label leads a line at l, colored as the CLI's other labels are on w.
func label(w io.Writer, l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return ansi.ErrorLabel(w)
	case l >= slog.LevelWarn:
		return ansi.WarningLabel(w)
	case l >= slog.LevelInfo:
		return ansi.InfoLabel(w)
	default:
		return ansi.DebugLabel(w)
	}
}

// stamped reports whether lines to w carry the time: a regular file does (the
// proxy daemon's log, otto's), read later. A terminal, a pipe (stderr piped
// into a CI log, which stamps lines itself) and a test's buffer do not, so a
// warning reads the same however stderr is redirected.
func stamped(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode().IsRegular()
}
