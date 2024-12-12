package ansi

import (
	"time"

	"github.com/briandowns/spinner"
	"github.com/pkg/errors"
)

const (
	spinnerTextEllipsis = "…"
	spinnerTextDone     = ""
	spinnerTextFailed   = "failed"

	spinnerColor = "cyan"
)

func Waiting(fn func() error) error {
	return loading("", "", "", fn)
}

func Spinner(text string, fn func() error) error {
	initialMsg := text + spinnerTextEllipsis + " "
	doneMsg := spinnerTextDone + "\n"
	failMsg := spinnerTextFailed + "\n"

	return loading(initialMsg, doneMsg, failMsg, fn)
}

func loading(initialMsg, doneMsg, failMsg string, fn func() error) error {
	done := make(chan struct{})
	errc := make(chan error)
	go func() {
		defer close(done)

		spinnerSet := []string{"●   ", "●   ", " ●  ", "  ● ", "    ●", "  ● ", " ●  "}
		s := spinner.New(spinnerSet, 100*time.Millisecond, spinner.WithWriter(Messages)) //nolint:mnd
		s.Prefix = initialMsg
		s.FinalMSG = doneMsg
		s.HideCursor = true
		s.Writer = Messages

		if err := s.Color(spinnerColor, "bold"); err != nil {
			panic(errors.Wrap(err, "failed setting spinner color"))
		}

		s.Start()
		err := <-errc
		if err != nil {
			s.FinalMSG = failMsg
		}

		s.Stop()
	}()

	err := fn()
	errc <- err
	<-done
	return err
}
