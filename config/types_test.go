package config

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewCfg(t *testing.T) {
	cfg := newCfg("foo", "bar")
	assert.NotNil(t, cfg)
}