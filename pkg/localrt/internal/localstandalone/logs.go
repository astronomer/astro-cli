//go:build !windows

package localstandalone

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// Standalone log parsing, lifted from Astro Desktop's standalone_logs.go.
// `airflow standalone` prefixes each line with the component that wrote it
// ("scheduler ", "api-server ", ...); the parser peels that off, extracts
// the line's own timestamp when it has one, and strips the metadata that
// makes raw Airflow lines unreadable in a terminal.

var (
	// reTimestamp matches ISO timestamps like 2026-03-16T18:33:51.933149Z.
	reTimestamp = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+(?:Z|[+-]\d{2}:?\d{2})?`)
	// reTimestampShorten rewrites a leading timestamp to HH:MM:SS for display.
	reTimestampShorten = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T(\d{2}:\d{2}:\d{2})\.\d+Z?\s*`)
	// reLogLevel matches log levels in brackets like [info     ].
	reLogLevel = regexp.MustCompile(`\[(info|debug|warning|error|critical)\s*\]\s*`)
	// reModulePath matches airflow module paths like [airflow.dag_processing.manager...].
	reModulePath = regexp.MustCompile(`\s*\[airflow\.[^\]]+\]`)
	// reLoc matches source locations like loc=manager.py:592.
	reLoc = regexp.MustCompile(`\s*loc=\S+`)
	// reUvicorn matches uvicorn access logs: INFO:  127.0.0.1:49553 - "GET /path HTTP/1.1" 200 OK.
	reUvicorn = regexp.MustCompile(`^INFO:\s+[\d.]+:\d+\s+-\s+"(\w+)\s+(\S+)\s+HTTP/[\d.]+"\s+(\d+)\s+\w+$`)
	// reMultiSpace collapses runs of spaces.
	reMultiSpace = regexp.MustCompile(`\s{2,}`)
)

// logComponents are the process names `airflow standalone` multiplexes;
// anything else (uv output, tracebacks, banners) is "system".
var logComponents = []string{"scheduler", "api-server", "triggerer", "dag-processor", "webserver", "standalone"}

// parseLogMeta peels the component prefix off a standalone log line and
// returns the component plus the rest of the line — the message body before
// cleaning. This is the cheap half of parsing: no regex cleaning runs here.
func parseLogMeta(line string) (component, rest string) {
	for _, c := range logComponents {
		if strings.HasPrefix(line, c+" ") {
			return c, strings.TrimPrefix(line, c+" ")
		}
	}
	return "system", line
}

// parseLogLine splits one standalone log line into a rt.LogLine, cleaned
// display Text included. Lines without a parseable timestamp get the zero
// Time; the caller substitutes arrival time where one is needed. cleanLogMessage
// runs several regexes, so callers that only need Component/Time (the log
// filters) use parseLogMeta and let deliverFunc clean the body on the OnLine
// path.
func parseLogLine(line string) rt.LogLine {
	component, rest := parseLogMeta(line)
	return rt.LogLine{
		Component: component,
		Time:      parseLineTime(rest),
		Text:      cleanLogMessage(rest),
	}
}

// parseLineTime extracts the line's own timestamp when it carries one.
func parseLineTime(s string) time.Time {
	m := reTimestamp.FindString(s)
	if m == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999"} {
		if t, err := time.Parse(layout, m); err == nil {
			return t
		}
	}
	return time.Time{}
}

// cleanLogMessage strips verbose metadata from Airflow log lines so each
// fits on one terminal line.
func cleanLogMessage(msg string) string {
	msg = strings.TrimLeft(msg, " ")
	msg = strings.TrimPrefix(msg, "| ")
	msg = strings.TrimPrefix(msg, "|")
	msg = strings.TrimSpace(msg)

	if msg == "" || strings.HasPrefix(msg, "======") || strings.HasPrefix(msg, "-----------") {
		return msg
	}

	// Uvicorn access logs → "GET /api/v2/dags 200".
	if m := reUvicorn.FindStringSubmatch(msg); m != nil {
		return fmt.Sprintf("%s %s %s", m[1], m[2], m[3])
	}

	msg = reTimestampShorten.ReplaceAllString(msg, "$1 ")
	msg = reLogLevel.ReplaceAllString(msg, "")
	msg = reModulePath.ReplaceAllString(msg, "")
	msg = reLoc.ReplaceAllString(msg, "")
	msg = reMultiSpace.ReplaceAllString(msg, " ")
	return strings.TrimSpace(msg)
}

// followPollInterval paces the wait for new log lines (and for the log
// file to first appear) while following.
const followPollInterval = 500 * time.Millisecond

// Logs replays the project's log file into opts.OnLine or opts.Writer and,
// with Follow, keeps streaming until the context ends or the process group
// dies. OnLine receives parsed lines; Writer receives the raw ones.
func (a *airflow) Logs(ctx context.Context, opts rt.LogOptions) error {
	if (opts.Writer == nil) == (opts.OnLine == nil) {
		return errors.New("exactly one of LogOptions.Writer and LogOptions.OnLine must be set")
	}
	stateDir, err := rt.StateDir(a.rec.ProjectPath)
	if err != nil {
		return err
	}
	logPath := filepath.Join(stateDir, logFileName)

	f, err := os.Open(logPath)
	if errors.Is(err, os.ErrNotExist) {
		if !opts.Follow {
			return errors.New("this project has no local Airflow logs yet; `astro local start` creates them")
		}
		if f, err = a.waitForLogFile(ctx, logPath); err != nil || f == nil {
			// A nil file without error is a canceled wait — the normal way
			// a follow ends.
			return err
		}
	} else if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	deliver := deliverFunc(opts, a.eng.now)

	// Backlog first: read to EOF, filter, apply Tail, emit.
	reader := bufio.NewReader(f)
	var backlog []logEntry
	offset := readAvailable(reader, &offsetTracker{}, func(raw string) {
		if entry, ok := filterLine(raw, opts); ok {
			backlog = append(backlog, entry)
		}
	})
	if opts.Tail > 0 && len(backlog) > opts.Tail {
		backlog = backlog[len(backlog)-opts.Tail:]
	}
	for _, entry := range backlog {
		deliver(entry)
	}
	if !opts.Follow {
		return nil
	}

	tracker := &offsetTracker{n: offset}
	for {
		select {
		case <-ctx.Done():
			// A canceled follow is the normal way a log stream ends.
			return nil
		case <-time.After(followPollInterval):
		}
		// The capped writer rewrites the file when it tops out; a shrink
		// means our position is gone, so start over from the top.
		if info, err := f.Stat(); err == nil && info.Size() < tracker.n {
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				return err
			}
			reader.Reset(f)
			tracker.n = 0
		}
		alive := a.eng.groupAlive(a.rec)
		readAvailable(reader, tracker, func(raw string) {
			if entry, ok := filterLine(raw, opts); ok {
				deliver(entry)
			}
		})
		if !alive {
			// The process died; everything it wrote has been drained.
			return nil
		}
	}
}

// logEntry carries a raw line with the cheap half of its parse (component,
// time, and the uncleaned message body). Writer mode emits raw and never
// cleans; OnLine mode cleans the body in deliverFunc.
type logEntry struct {
	raw       string
	component string
	time      time.Time
	body      string
}

// filterLine parses the cheap half of one raw line and applies the component
// and Since filters. It does not clean the message — that regex work is
// deferred to the OnLine delivery path, since Writer mode discards the cleaned
// text. Lines without their own timestamp pass the Since filter: better a few
// extra lines than silently dropping tracebacks.
func filterLine(raw string, opts rt.LogOptions) (logEntry, bool) {
	component, body := parseLogMeta(raw)
	lineTime := parseLineTime(body)
	if len(opts.Components) > 0 {
		found := false
		for _, c := range opts.Components {
			if component == c {
				found = true
				break
			}
		}
		if !found {
			return logEntry{}, false
		}
	}
	if !opts.Since.IsZero() && !lineTime.IsZero() && lineTime.Before(opts.Since) {
		return logEntry{}, false
	}
	return logEntry{raw: raw, component: component, time: lineTime, body: body}, true
}

// deliverFunc builds the emit path once: cleaned lines to OnLine, raw lines
// to Writer, arrival time standing in for lines without their own. The regex
// cleaning runs here so it only touches lines OnLine will actually show.
func deliverFunc(opts rt.LogOptions, now func() time.Time) func(logEntry) {
	return func(entry logEntry) {
		if opts.OnLine != nil {
			line := rt.LogLine{
				Component: entry.component,
				Time:      entry.time,
				Text:      cleanLogMessage(entry.body),
			}
			if line.Time.IsZero() {
				line.Time = now()
			}
			opts.OnLine(line)
			return
		}
		fmt.Fprintln(opts.Writer, entry.raw)
	}
}

// offsetTracker carries the byte position across readAvailable calls so
// truncation (the capped writer rewriting the file) is detectable.
type offsetTracker struct{ n int64 }

// readAvailable consumes lines up to EOF, returning the new byte offset.
// An unterminated fragment at EOF is emitted rather than held back:
// `airflow standalone` writes whole lines, so a fragment only appears at
// shutdown, when nothing more would come to complete it.
func readAvailable(reader *bufio.Reader, tracker *offsetTracker, emit func(string)) int64 {
	for {
		line, err := reader.ReadString('\n')
		tracker.n += int64(len(line))
		if line != "" {
			if trimmed := strings.TrimRight(line, "\r\n"); trimmed != "" {
				emit(trimmed)
			}
		}
		if err != nil {
			return tracker.n
		}
	}
}

// waitForLogFile waits for the log file to first appear (a follow started
// before Airflow ever ran).
func (a *airflow) waitForLogFile(ctx context.Context, logPath string) (*os.File, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, nil
		case <-time.After(followPollInterval):
		}
		f, err := os.Open(logPath)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
}
