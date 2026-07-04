package runtimes

import (
	"github.com/briandowns/spinner"

	sp "github.com/astronomer/astro-cli/pkg/spinner"
)

// spinnerFeedback adapts astro-cli's pkg/spinner to the container.Feedback
// interface, so the shared runtime logic drives the same CLI UX (the
// "Astro uses containers to run your project…" spinner) it did before the
// logic moved into pkg/container.
type spinnerFeedback struct {
	s *spinner.Spinner
}

func newSpinnerFeedback() *spinnerFeedback {
	return &spinnerFeedback{}
}

// Start creates and starts the spinner with the given suffix message.
func (f *spinnerFeedback) Start(message string) {
	f.s = sp.NewSpinner(message)
	f.s.Start()
}

// Update changes the spinner suffix mid-flight (e.g. download vs. start phases).
func (f *spinnerFeedback) Update(message string) {
	if f.s == nil {
		f.s = sp.NewSpinner(message)
		f.s.Start()
		return
	}
	f.s.Suffix = " " + message
}

// Success stops the spinner with a green checkmark and the given message.
func (f *spinnerFeedback) Success(message string) {
	if f.s == nil {
		f.s = sp.NewSpinner(message)
	}
	sp.StopWithCheckmark(f.s, message)
	f.s = nil
}

// Stop stops the spinner without a checkmark.
func (f *spinnerFeedback) Stop() {
	if f.s != nil {
		f.s.Stop()
		f.s = nil
	}
}
