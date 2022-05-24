package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewCfg(t *testing.T) {
	cfg := newCfg("foo", "bar")
	assert.NotNil(t, cfg)
}

func TestGetString(t *testing.T) {
	initTestConfig()
	cfg := newCfg("foo", "0")
	cfg.SetHomeString("1")
	val := cfg.GetString()
	assert.Equal(t, "1", val)

	viperProject.SetConfigFile("test.yaml")
	defer os.Remove("test.yaml")
	cfg.SetProjectString("2")
	val = cfg.GetString()
	assert.Equal(t, "2", val)
}

func TestGetInt(t *testing.T) {
	initTestConfig()
	cfg := newCfg("foo", "0")
	cfg.SetHomeString("1")
	val := cfg.GetInt()
	assert.Equal(t, 1, val)

	viperProject.SetConfigFile("test.yaml")
	defer os.Remove("test.yaml")
	cfg.SetProjectString("2")
	val = cfg.GetInt()
	assert.Equal(t, 2, val)
}

func TestGetBool(t *testing.T) {
	initTestConfig()
	cfg := newCfg("foo", "false")
	cfg.SetHomeString("true")
	val := cfg.GetBool()
	assert.Equal(t, true, val)

	viperProject.SetConfigFile("test.yaml")
	defer os.Remove("test.yaml")
	cfg.SetProjectString("false")
	val = cfg.GetBool()
	assert.Equal(t, false, val)
}
