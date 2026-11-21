package rt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripEscapesDropsCSIAndOSC(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"plain":      {"no escapes", "no escapes"},
		"csi color":  {"\x1b[33mscheduler\x1b[0m | ok", "scheduler | ok"},
		"osc bel":    {"a\x1b]52;c;aGk=\x07b", "ab"},
		"osc st":     {"a\x1b]0;title\x1b\\b", "ab"},
		"osc link":   {"\x1b]8;;http://x\x07text\x1b]8;;\x07", "text"},
		"dcs string": {"a\x1bPpayload\x1b\\b", "ab"},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, StripEscapes(tc.in))
		})
	}
}
