//go:build !race

package input

import (
	"io"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/stretchr/testify/assert"
)

// TestPromptGetConfirmation drives promptui.Select, which has a data race
// inside the third-party library itself: promptui v0.9.0's screenbuf is written
// by both its render loop and readline v1.5.1's ioloop goroutine (see
// select.go:303 vs select.go:395). Nothing in our code can fix it, so this test
// is skipped under -race. Drop the build tag if promptui is ever patched or
// replaced.
func (s *Suite) TestPromptGetConfirmation() {
	runner := GetYesNoSelector(PromptContent{Label: "test label, enter y/n"})
	runner.Keys = &promptui.SelectKeys{Next: promptui.Key{Code: rune('S')}, Prev: promptui.Key{Code: rune('W')}, PageUp: promptui.Key{Code: rune('D')}, PageDown: promptui.Key{Code: rune('A')}}
	tests := []struct {
		name         string
		inputString  string
		want         bool
		errAssertion assert.ErrorAssertionFunc
	}{
		{
			name:         "basic yes case",
			inputString:  "\n",
			want:         true,
			errAssertion: assert.NoError,
		},
		{
			name:         "basic no case",
			inputString:  "S\n",
			want:         false,
			errAssertion: assert.NoError,
		},
		{
			name:         "no input case",
			inputString:  "",
			want:         false,
			errAssertion: assert.Error,
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			runner.Stdin = io.NopCloser(strings.NewReader(tt.inputString))
			got, err := PromptGetConfirmation(runner)
			if !tt.errAssertion(s.T(), err) {
				return
			}

			s.Equal(tt.want, got)
		})
	}
}
