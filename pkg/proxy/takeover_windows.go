//go:build windows

package proxy

// takeOverFromV1 does nothing on Windows: the takeover identifies the process
// holding a port by its command line and environment, which it reads only on
// unix. A 1.x proxy holding the port leaves a Windows daemon on its fallback
// port instead.
func (d *Daemon) takeOverFromV1(string) {}
