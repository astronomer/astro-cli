package local

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/internal/instancelocate"
	"github.com/astronomer/astro-cli/pkg/airflowapi"
	"github.com/astronomer/astro-cli/pkg/instances"
)

// outageWatch sits in front of an astro link's transport and notes whether
// the latest answer said no Airflow is behind it. A hibernating or
// redeploying Deployment answers every call that way, and the error that
// reaches the command then names an API generation it could not detect
// rather than the reason. A later real answer clears the mark, so a command
// that fails on that answer keeps its own error.
type outageWatch struct {
	next     airflowapi.Transport
	instance instances.Instance
	domain   string
	missing  atomic.Bool
}

func (w *outageWatch) Do(ctx context.Context, req airflowapi.Request) (airflowapi.Response, error) {
	resp, err := w.next.Do(ctx, req)
	if err == nil {
		w.missing.Store(noAirflowBehind(resp.StatusCode))
	}
	return resp, err
}

func noAirflowBehind(status int) bool {
	switch status {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// outageError is a failure Astro could explain. It reads as the explanation
// and unwraps to both it and the failure it explains. A Deployment Astro calls
// healthy explains least, so there the failure is shown too: a gateway
// timeout on one slow request reads the same as an Airflow still starting.
type outageError struct {
	why   error
	cause error
}

func (e *outageError) Error() string {
	if errors.Is(e.why, instancelocate.ErrAirflowUnavailable) {
		return e.why.Error() + "\n" + e.cause.Error()
	}
	return e.why.Error()
}
func (e *outageError) Unwrap() []error { return []error{e.why, e.cause} }

// explainOutage replaces a failed command's error with Astro's account of the
// Deployment when the Airflow behind it was not there to answer. It asks only
// on that path: a command that succeeded, or failed for any other reason,
// never makes the extra call.
func (c *cli) explainOutage(ctx context.Context, err error) error {
	w := c.outage
	if err == nil || w == nil || !w.missing.Load() || c.d.Locator == nil {
		return err
	}
	diagnoser, ok := c.d.Locator(w.domain).(instancelocate.Diagnoser)
	if !ok {
		return err
	}
	why := diagnoser.WhyUnavailable(ctx, w.instance)
	if why == nil {
		return err
	}
	return &outageError{why: why, cause: err}
}

// explainOutages runs every leaf under cmd through explainOutage.
func explainOutages(c *cli, cmd *cobra.Command) {
	for _, sub := range cmd.Commands() {
		explainOutages(c, sub)
	}
	inner := cmd.RunE
	if inner == nil {
		return
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return c.explainOutage(cmd.Context(), inner(cmd, args))
	}
}
