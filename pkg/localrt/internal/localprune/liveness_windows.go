//go:build windows

package localprune

// defaultGroupAlive is a stub: standalone mode is macOS/Linux only in the
// MVP (docs/architecture.md), so no standalone record exists on Windows.
func defaultGroupAlive(pgid int) bool {
	return false
}
