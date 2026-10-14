package proxy

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseProcArgs(t *testing.T) {
	raw := binary.LittleEndian.AppendUint32(nil, 3)
	raw = append(raw, "/opt/homebrew/bin/astro\x00\x00\x00\x00astro\x00dev\x00proxy\x00HOME=/Users/me\x00ASTRO_HOME=/tmp/a\x00\x00ignored\x00"...)

	args, env, ok := parseProcArgs(raw)
	assert.True(t, ok)
	assert.Equal(t, []string{"astro", "dev", "proxy"}, args)
	assert.Equal(t, []string{"HOME=/Users/me", "ASTRO_HOME=/tmp/a"}, env)

	_, _, ok = parseProcArgs(raw[:2])
	assert.False(t, ok, "a buffer too short for argc")
	_, _, ok = parseProcArgs(binary.LittleEndian.AppendUint32(nil, 5))
	assert.False(t, ok, "a buffer with no executable path")
}
