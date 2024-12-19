package spinner

import (
	"github.com/briandowns/spinner"
	"time"
)

var spinnerCharSet = spinner.CharSets[14]

const (
	spinnerRefresh = 100 * time.Millisecond
)

func NewSpinner(suffix string) *spinner.Spinner {
	s := spinner.New(spinnerCharSet, spinnerRefresh)
	s.Suffix = suffix
	return s
}
