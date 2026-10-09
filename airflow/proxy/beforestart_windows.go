//go:build windows

package proxy

// beforeStart is nil on Windows: the astro 1.x takeover identifies the process
// holding a port by its command line and environment, which it reads only on
// unix, and the daemon does not run on Windows anyway.
var beforeStart func(port string)
