//go:build !windows

package localprune

import "syscall"

// defaultGroupAlive reports whether any process in the group is reachable,
// signaling the whole group (negative pgid) the way the standalone engine
// does. Signal 0 delivers nothing; it only tests reachability.
func defaultGroupAlive(pgid int) bool {
	if pgid <= 0 {
		return false
	}
	return syscall.Kill(-pgid, 0) == nil
}
