//go:build !darwin

package airflow

// hasSystemProxy is a no-op on non-macOS platforms. The _scproxy hang only
// affects macOS, so we never need to inject NO_PROXY on Linux or Windows.
var hasSystemProxy = func() bool {
	return false
}
