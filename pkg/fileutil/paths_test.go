package fileutil

import (
	"github.com/stretchr/testify/assert"
)

func (s *Suite) TestGetWorkingDir() {
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
		s.Run(tt.name, func() {
			got, err := GetWorkingDir()
			if !tt.errAssertion(s.T(), err) {
				return
			}

			s.Contains(got, tt.want)
		})
	}
}

func (s *Suite) TestGetHomeDir() {
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
		s.Run(tt.name, func() {
			got, err := GetHomeDir()
			if !tt.errAssertion(s.T(), err) {
				return
			}

			s.Contains(got, tt.want)
		})
	}
}

func (s *Suite) TestIsEmptyDir() {
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
		s.Run(tt.name, func() {
			if got := IsEmptyDir(tt.args.path); got != tt.want {
				s.Fail("IsEmptyDir() = %v, want %v", got, tt.want)
			}
		})
	}
}
