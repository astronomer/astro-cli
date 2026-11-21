package deployment

import "fmt"

// WaitTimeoutError is a wait for a Deployment that ran out of time. The
// Deployment was not touched by that: the work goes on in Astro, so the error
// names it and the command that shows where it stands. It wraps the sentinel
// for the state waited for (ErrTimedOut, ErrTimedOutHibernating), so
// errors.Is still finds it.
type WaitTimeoutError struct {
	Err          error
	DeploymentID string
}

func (e *WaitTimeoutError) Error() string {
	return fmt.Sprintf("%s; Deployment %s continues in Astro, check it with astro deployment inspect %s", e.Err, e.DeploymentID, e.DeploymentID)
}

func (e *WaitTimeoutError) Unwrap() error { return e.Err }

// StoppedWaitingError is a wait for a Deployment that the run's interrupt
// ended. Only the wait stopped: whatever Astro was doing for the Deployment
// continues and was not canceled, which the error says, with the command that
// shows where it stands. It wraps the interrupt, so the run still exits as
// interrupted.
type StoppedWaitingError struct {
	Err          error
	DeploymentID string
}

func (e *StoppedWaitingError) Error() string {
	return fmt.Sprintf("stopped waiting; Deployment %s continues in Astro and was not canceled, check it with astro deployment inspect %s", e.DeploymentID, e.DeploymentID)
}

func (e *StoppedWaitingError) Unwrap() error { return e.Err }
