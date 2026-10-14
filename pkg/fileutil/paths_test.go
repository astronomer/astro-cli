package fileutil

import (
	"path/filepath"

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
			want:         string(filepath.Separator),
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
