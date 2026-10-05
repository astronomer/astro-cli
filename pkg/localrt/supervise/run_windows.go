//go:build windows

package supervise

import "errors"

// Run is a stub: standalone mode (and with it the supervisor) is not
// supported on Windows in the MVP (docs/architecture.md).
func Run(_ []string) error {
	return errors.New("the local Airflow supervisor is not supported on Windows")
}
