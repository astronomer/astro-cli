package config

import (
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/spf13/afero"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestInitHome(t *testing.T) {
	fs := afero.NewMemMapFs()
	configRaw := []byte(`cloud:
  api:
    port: "443"
    protocol: https
    ws_protocol: wss
local:
  enabled: true
  astrohub: http://ASTROHUB_HOST:8871/v1
context: ASTROHUB_HOST
contexts:
  ASTROHUB_HOST:
    domain: ASTROHUB_HOST
    token: token
    last_used_workspace: ck05r3bor07h40d02y2hw4n4v
    workspace: ck05r3bor07h40d02y2hw4n4v
`)
	afero.WriteFile(fs, HomeConfigFile, []byte(configRaw), 0777)
	initHome(fs)
}

func TestInitProject(t *testing.T) {
	fs := afero.NewMemMapFs()
	configRaw := []byte(`cloud:
  api:
    port: "443"
    protocol: https
    ws_protocol: wss
local:
  enabled: true
  houston: http://ASTROHUB_HOST:8871/v1
context: ASTROHUB_HOST
contexts:
  ASTROHUB_HOST:
    domain: ASTROHUB_HOST
    token: token
    last_used_workspace: ck05r3bor07h40d02y2hw4n4v
    workspace: ck05r3bor07h40d02y2hw4n4v
`)
	afero.WriteFile(fs, HomeConfigFile, []byte(configRaw), 0777)
	initProject(fs)
	homeDir, _ := fileutil.GetHomeDir()
	_, err := fs.Stat(homeDir)
	if os.IsNotExist(err) {
		t.Error("home does not exist.\n")
	}
}

func TestIsProjectDir(t *testing.T) {
	homeDir, _ := fileutil.GetHomeDir()
	tests := []struct {
		name string
		in   string
		out  bool
	}{
		{"False", "", false},
		{"HomePath False", homeDir, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := IsProjectDir(tt.in)
			assert.NoError(t, err)
			assert.Equal(t, s, tt.out)
		})
	}
}

func TestConfigExists(t *testing.T) {
	fs := afero.NewMemMapFs()
	configRaw := []byte(`cloud:
  api:
    port: "443"
    protocol: https
    ws_protocol: wss
local:
  enabled: true
  houston: http://ASTROHUB_HOST:8871/v1
context: ASTROHUB_HOST
contexts:
  ASTROHUB_HOST:
    domain: ASTROHUB_HOST
    token: token
    last_used_workspace: ck05r3bor07h40d02y2hw4n4v
    workspace: ck05r3bor07h40d02y2hw4n4v
`)
	afero.WriteFile(fs, HomeConfigFile, []byte(configRaw), 0777)
	initProject(fs)

	viperWOConfig := viper.New()
	tests := []struct {
		name string
		in   *viper.Viper
		out  bool
	}{
		{"exists", viperHome, true},
		{"doesnt exists", viperWOConfig, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := configExists(tt.in)
			assert.Equal(t, actual, tt.out)
		})
	}
}
