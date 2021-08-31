package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewCfg(t *testing.T) {
	cfg := newCfg("foo", "bar")
	assert.NotNil(t, cfg)
}
