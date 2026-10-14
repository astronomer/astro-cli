//go:build windows

package proxy

// beforeStart is nil on Windows: the astro 1.x takeover identifies the process
// holding a port by its command line and environment, which it reads only on
// unix. A 1.x proxy holding the port leaves a Windows daemon on its fallback
// port instead.
var beforeStart func(port string)
