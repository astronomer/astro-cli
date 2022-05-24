package config

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContextGetSoftwareAPIURL(t *testing.T) {
	initTestConfig()
	CFG.LocalHouston.SetHomeString("http://localhost/v1")
	CFG.CloudAPIProtocol.SetHomeString("https")
	CFG.CloudAPIPort.SetHomeString("8080")
	type fields struct {
		Domain string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name:   "basic localhost case",
			fields: fields{Domain: "localhost"},
			want:   "http://localhost/v1",
		},
		{
			name:   "basic cloud case",
			fields: fields{Domain: "dev.astro.io"},
			want:   "https://houston.dev.astro.io:8080/v1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Context{
				Domain: tt.fields.Domain,
			}
			if got := c.GetSoftwareAPIURL(); got != tt.want {
				t.Errorf("Context.GetSoftwareAPIURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContextGetSoftwareAppURL(t *testing.T) {
	initTestConfig()
	CFG.LocalHouston.SetHomeString("http://localhost/v1")
	CFG.CloudAPIProtocol.SetHomeString("https")
	type fields struct {
		Domain string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name:   "basic localhost case",
			fields: fields{Domain: "localhost"},
			want:   "http://localhost/v1",
		},
		{
			name:   "basic cloud case",
			fields: fields{Domain: "dev.astro.io"},
			want:   "https://app.dev.astro.io",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Context{
				Domain: tt.fields.Domain,
			}
			if got := c.GetSoftwareAppURL(); got != tt.want {
				t.Errorf("Context.GetSoftwareAppURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContextGetSoftwareWebsocketURL(t *testing.T) {
	initTestConfig()
	CFG.LocalHouston.SetHomeString("http://localhost/v1")
	CFG.CloudAPIPort.SetHomeString("8080")
	type fields struct {
		Domain string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name:   "basic localhost case",
			fields: fields{Domain: "localhost"},
			want:   "http://localhost/v1",
		},
		{
			name:   "basic cloud case",
			fields: fields{Domain: "dev.astro.io"},
			want:   "wss://houston.dev.astro.io:8080/ws",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Context{
				Domain: tt.fields.Domain,
			}
			if got := c.GetSoftwareWebsocketURL(); got != tt.want {
				t.Errorf("Context.GetSoftwareWebsocketURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPrintCurrentSoftwareContext(t *testing.T) {
	initTestConfig()
	ctx := Context{Domain: "localhost"}
	ctx.SetContext()
	ctx.SwitchContext()
	buf := new(bytes.Buffer)
	err := PrintCurrentSoftwareContext(buf)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "localhost")
}
