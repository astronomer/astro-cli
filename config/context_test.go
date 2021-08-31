package config

import (
	"bytes"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

func TestNewTableOut(t *testing.T) {
	tab := newTableOut()
	assert.NotNil(t, tab)
	assert.Equal(t, []int{36, 36}, tab.Padding)
}

func TestGetCurrentContext(t *testing.T) {
	fs := afero.NewMemMapFs()
	configRaw := []byte(`cloud:
  api:
    port: "443"
    protocol: https
    ws_protocol: wss
local:
  enabled: true
  houston: http://example.com:8871/v1
context: example_com
contexts:
  example_com:
    domain: example.com
    token: token
    last_used_workspace: ck05r3bor07h40d02y2hw4n4v
    workspace: ck05r3bor07h40d02y2hw4n4v
`)
	err := afero.WriteFile(fs, HomeConfigFile, []byte(configRaw), 0777)
	InitConfig(fs)
	ctx, err := GetCurrentContext()
	assert.NoError(t, err)
	assert.Equal(t, "example.com", ctx.Domain)
	assert.Equal(t, "token", ctx.Token)
	assert.Equal(t, "ck05r3bor07h40d02y2hw4n4v", ctx.Workspace)
}

func TestGetCurrentContextError(t *testing.T) {
	fs := afero.NewMemMapFs()
	configRaw := []byte(`cloud:
  api:
    port: "443"
    protocol: https
    ws_protocol: wss
local:
  enabled: true
  houston: http://example.com:8871/v1
`)
	err := afero.WriteFile(fs, HomeConfigFile, []byte(configRaw), 0777)
	InitConfig(fs)
	_, err = GetCurrentContext()
	assert.EqualError(t, err, "No context set, have you authenticated to a cluster?")
}

func TestPrintContext(t *testing.T) {
	fs := afero.NewMemMapFs()
	configRaw := []byte(`cloud:
  api:
    port: "443"
    protocol: https
    ws_protocol: wss
local:
  enabled: true
  houston: http://example.com:8871/v1
context: example_com
contexts:
  example_com:
    domain: example.com
    token: token
    last_used_workspace: ck05r3bor07h40d02y2hw4n4v
    workspace: ck05r3bor07h40d02y2hw4n4v
`)
	err := afero.WriteFile(fs, HomeConfigFile, []byte(configRaw), 0777)
	InitConfig(fs)

	ctx := Context{
		Token:             "token",
		LastUsedWorkspace: "ck05r3bor07h40d02y2hw4n4v",
		Workspace:         "ck05r3bor07h40d02y2hw4n4v",
		Domain:            "example.com",
	}
	buf := new(bytes.Buffer)
	err = ctx.PrintContext(buf)
	assert.NoError(t, err)
	expected := " CLUSTER                             WORKSPACE                           \n example.com                         ck05r3bor07h40d02y2hw4n4v           \n"
	assert.Equal(t, expected, buf.String())
}

func TestPrintContextNA(t *testing.T) {
	fs := afero.NewMemMapFs()
	configRaw := []byte(`cloud:
  api:
    port: "443"
    protocol: https
    ws_protocol: wss
local:
  enabled: true
  houston: http://example.com:8871/v1
context: example_com
contexts:
  example_com:
    domain: example.com
    token: token
    last_used_workspace: ck05r3bor07h40d02y2hw4n4v
    workspace:
`)
	err := afero.WriteFile(fs, HomeConfigFile, []byte(configRaw), 0777)
	InitConfig(fs)

	ctx := Context{
		Token:             "token",
		LastUsedWorkspace: "ck05r3bor07h40d02y2hw4n4v",
		Workspace:         "",
		Domain:            "example.com",
	}
	buf := new(bytes.Buffer)
	err = ctx.PrintContext(buf)
	assert.NoError(t, err)
	expected := " CLUSTER                             WORKSPACE                           \n example.com                         N/A                                 \n"
	assert.Equal(t, expected, buf.String())
}
