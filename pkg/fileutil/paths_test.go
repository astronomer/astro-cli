package fileutil

import (
	"strings"
	"testing"
)

func TestGetWorkingDir(t *testing.T) {
	tests := []struct {
		name    string
		want    string
		wantErr bool
	}{
		{
			name:    "basic case",
			want:    "fileutil",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetWorkingDir()
			if (err != nil) != tt.wantErr {
				t.Errorf("GetWorkingDir() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("GetWorkingDir() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetHomeDir(t *testing.T) {
	tests := []struct {
		name    string
		want    string
		wantErr bool
	}{
		{
			name:    "basic case",
			want:    "/",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetHomeDir()
			if (err != nil) != tt.wantErr {
				t.Errorf("GetHomeDir() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("GetHomeDir() = %v, want %v", got, tt.want)
			}
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
