package localdocker

import (
	"strings"
	"time"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// Compose log parsing, ported from Astro Desktop's runtime/docker_logs.go.
// `compose logs --timestamps` lines look like:
//
//	api-server-1  | 2026-07-21T10:00:00.123456789Z the message
//
// The service prefix becomes the component (replica suffix stripped), the
// timestamp becomes LogLine.Time.

// parseLogLine splits one compose log line into a LogLine. Lines without a
// service prefix (engine banners, compose warnings) get component
// "system"; lines without a parseable timestamp fall back to now.
func parseLogLine(line string, now func() time.Time) rt.LogLine {
	prefix, rest, found := strings.Cut(line, "| ")
	if !found {
		return rt.LogLine{Component: "system", Time: now(), Text: line}
	}
	component := componentName(strings.TrimSpace(prefix))
	ts, text, ok := splitTimestamp(rest)
	if !ok {
		ts = now()
	}
	return rt.LogLine{Component: component, Time: ts, Text: text}
}

// componentName maps a compose container name (service-N) to its service.
func componentName(container string) string {
	if i := strings.LastIndex(container, "-"); i > 0 {
		if suffix := container[i+1:]; suffix != "" && strings.Trim(suffix, "0123456789") == "" {
			return container[:i]
		}
	}
	return container
}

// splitTimestamp peels a leading RFC3339Nano timestamp off a log message.
func splitTimestamp(s string) (time.Time, string, bool) {
	tsText, rest, _ := strings.Cut(s, " ")
	ts, err := time.Parse(time.RFC3339Nano, tsText)
	if err != nil {
		return time.Time{}, s, false
	}
	return ts, rest, true
}
