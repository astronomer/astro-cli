package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsGitRepository(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{
			name: "basic case",
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsGitRepository(""))
		})
	}
}

func TestParseGitURL(t *testing.T) {
	tests := []struct {
		urlStr     string
		wantScheme string
		wantHost   string
		wantPath   string
	}{
		{
			urlStr:     "http://example.com/foo/bar",
			wantScheme: "http",
			wantHost:   "example.com",
			wantPath:   "/foo/bar",
		},
		{
			urlStr:     "http://example.com/foo/bar.git",
			wantScheme: "http",
			wantHost:   "example.com",
			wantPath:   "/foo/bar",
		},
		{
			urlStr:     "https://user@example.com/foo/bar",
			wantScheme: "https",
			wantHost:   "example.com",
			wantPath:   "/foo/bar",
		},
		{
			urlStr:     "host:foo/bar",
			wantScheme: "ssh",
			wantHost:   "host",
			wantPath:   "foo/bar",
		},
		{
			urlStr:     "host:/foo/bar",
			wantScheme: "ssh",
			wantHost:   "host",
			wantPath:   "/foo/bar",
		},
		{
			urlStr:     "user@host:/foo/bar",
			wantScheme: "ssh",
			wantHost:   "host",
			wantPath:   "/foo/bar",
		},
		{
			urlStr:     "user@host:/foo/bar.git",
			wantScheme: "ssh",
			wantHost:   "host",
			wantPath:   "/foo/bar",
		},
	}
	for _, tt := range tests {
		t.Run(tt.urlStr, func(t *testing.T) {
			got, err := parseGitURL(tt.urlStr)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantScheme, got.Scheme)
			assert.Equal(t, tt.wantHost, got.Host)
			assert.Equal(t, tt.wantPath, got.Path)
		})
	}
}
