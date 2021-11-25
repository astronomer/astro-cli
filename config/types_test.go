package config

import (
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

func TestNewCfg(t *testing.T) {
	c := newCfg("foo", "bar")
	assert.NotNil(t, c)
}

func Test_cfg_GetString(t *testing.T) {
	type fields struct {
		Path    string
		Default string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "basic test case",
			fields: fields{
				Path:    "foo",
				Default: "bar",
			},
			want: "bar",
		},
	}
	for _, tt := range tests {
		fs := afero.NewOsFs()
		InitConfig(fs)
		t.Run(tt.name, func(t *testing.T) {
			c := cfg{
				Path:    tt.fields.Path,
				Default: tt.fields.Default,
			}
			if got := c.GetString(); got != tt.want {
				t.Errorf("cfg.GetString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_cfg_GetBool(t *testing.T) {
	type fields struct {
		Path    string
		Default string
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{
			name: "basic test case",
			fields: fields{
				Path:    "foo",
				Default: "bar",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		fs := afero.NewOsFs()
		InitConfig(fs)
		t.Run(tt.name, func(t *testing.T) {
			c := cfg{
				Path:    tt.fields.Path,
				Default: tt.fields.Default,
			}
			if got := c.GetBool(); got != tt.want {
				t.Errorf("cfg.GetString() = %v, want %v", got, tt.want)
			}
		})
	}
}
