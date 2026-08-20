package localdocker

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestComponentName(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC) }

	l := parseLogLine("api-server-3  | 2026-07-21T10:00:00Z msg", now)
	assert.Equal(t, "api-server", l.Component)
	assert.Equal(t, "msg", l.Text)

	// No replica suffix: the name stays intact.
	l = parseLogLine("db-migration  | 2026-07-21T10:00:00Z done", now)
	assert.Equal(t, "db-migration", l.Component)

	// No timestamp: arrival time stands in.
	l = parseLogLine("scheduler-1  | plain text", now)
	assert.Equal(t, "scheduler", l.Component)
	assert.Equal(t, "plain text", l.Text)
	assert.Equal(t, now(), l.Time)
}
