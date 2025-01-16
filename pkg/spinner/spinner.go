package spinner

import (
	"time"

	"github.com/astronomer/astro-cli/pkg/ansi"

	"github.com/briandowns/spinner"
)

var spinnerCharSet = spinner.CharSets[14]

const (
	spinnerRefresh = 100 * time.Millisecond
)

func NewSpinner(suffix string) *spinner.Spinner {
	s := spinner.New(spinnerCharSet, spinnerRefresh)
	s.HideCursor = true
	s.Suffix = " " + suffix
	return s
}

func StopWithCheckmark(s *spinner.Spinner, suffix string) *spinner.Spinner {
	s.Start() // Ensure we've started the spinner, so we can stop it. We still want the final checkmark to be displayed.
	s.FinalMSG = ansi.Green("\u2714") + " " + suffix + "\n"
	s.Stop()
	return s
}
