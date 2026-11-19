package logger

import (
	"bytes"
	"errors"
	"log"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capture sends the log to a buffer at level l for the rest of the test.
func capture(t *testing.T, l slog.Level) *bytes.Buffer {
	t.Helper()
	prev := GetLevel()
	var buf bytes.Buffer
	SetOutput(&buf)
	SetLevel(l)
	t.Cleanup(func() { SetLevel(prev); SetOutput(os.Stderr) })
	return &buf
}

func TestParseLevel(t *testing.T) {
	for name, want := range map[string]slog.Level{
		"debug": slog.LevelDebug, "DEBUG": slog.LevelDebug, "trace": slog.LevelDebug,
		"info": slog.LevelInfo, "Info": slog.LevelInfo,
		"warn": slog.LevelWarn, "warning": slog.LevelWarn,
		"error": slog.LevelError,
		"fatal": LevelOff, "panic": LevelOff,
	} {
		got, err := ParseLevel(name)
		require.NoError(t, err, name)
		assert.Equal(t, want, got, name)
	}
	_, err := ParseLevel("loud")
	assert.EqualError(t, err, `"loud" is not a log level: use debug, info, warn or error`)
}

func TestLinesAreLabeledAndFilteredByLevel(t *testing.T) {
	buf := capture(t, slog.LevelInfo)
	Debugf("not %s", "written")
	Infof("pulled %d images", 3)
	Warnf("disk is %s", "low")
	slog.Error("proxy failed", "err", errors.New("refused"))
	assert.Equal(t, "Info: pulled 3 images\nWarning: disk is low\nError: proxy failed err=refused\n", buf.String())
}

// The sub-modules log with log/slog and a stray standard-log call goes
// through slog's default: both reach this handler, at its level.
func TestSlogAndTheStandardLogShareTheHandler(t *testing.T) {
	buf := capture(t, slog.LevelDebug)
	slog.Debug("proxy listening", "addr", "127.0.0.1:6563")
	log.Print("from the standard log")
	assert.Equal(t, "Debug: proxy listening addr=127.0.0.1:6563\nInfo: from the standard log\n", buf.String())

	buf.Reset()
	SetLevel(slog.LevelWarn)
	slog.Debug("dropped")
	log.Print("dropped too")
	assert.Empty(t, buf.String())
}

func TestAttributesAreQuotedAndGrouped(t *testing.T) {
	buf := capture(t, slog.LevelDebug)
	l := slog.Default().With("project", "/tmp/my project").WithGroup("health")
	l.Debug("probe", "url", "http://localhost:8080/api/v2/monitor/health", "status", 503, "body", "")
	assert.Equal(t,
		`Debug: probe project="/tmp/my project" health.url=http://localhost:8080/api/v2/monitor/health health.status=503 health.body=""`+"\n",
		buf.String())
}

// A line can carry text from elsewhere (an API's error, a container's name), so
// escape sequences and control characters never reach the terminal.
func TestLinesCarryNoTerminalEscapes(t *testing.T) {
	buf := capture(t, slog.LevelDebug)
	Debugf("server said %s", "\x1b]52;c;aGk=\x07ok\x1b[2J")
	slog.Debug("x", "name", "a\x1b[31mb\rc")
	assert.Equal(t, "Debug: server said ok\nDebug: x name=\"ab c\"\n", buf.String())
}

func TestEnabled(t *testing.T) {
	capture(t, slog.LevelInfo)
	assert.False(t, Enabled(slog.LevelDebug))
	assert.True(t, Enabled(slog.LevelInfo))
	SetLevel(LevelOff)
	assert.False(t, Enabled(slog.LevelError))
}
