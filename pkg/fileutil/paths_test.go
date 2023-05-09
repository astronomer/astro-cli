package fileutil

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetWorkingDir(t *testing.T) {
	tests := []struct {
		name         string
		want         string
		errAssertion assert.ErrorAssertionFunc
	}{
		{
			name:         "basic case",
			want:         "fileutil",
			errAssertion: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetWorkingDir()
			if !tt.errAssertion(t, err) {
				return
			}

			assert.Contains(t, got, tt.want)
		})
	}
}

func TestGetHomeDir(t *testing.T) {
	tests := []struct {
		name         string
		want         string
		errAssertion assert.ErrorAssertionFunc
	}{
		{
			name:         "basic case",
			want:         "/",
			errAssertion: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetHomeDir()
			if !tt.errAssertion(t, err) {
				return
			}

			assert.Contains(t, got, tt.want)
		})
	}
}

func TestIsEmptyDir(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "basic case",
			args: args{path: "."},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsEmptyDir(tt.args.path); got != tt.want {
				t.Errorf("IsEmptyDir() = %v, want %v", got, tt.want)
			}
		})
	}
}
