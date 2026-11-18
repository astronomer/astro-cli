//go:build windows

package httpsocket

// checkSocket does nothing on Windows: a socket there carries no Unix owner,
// and access is governed by the directory's ACL.
func checkSocket(string) error { return nil }
