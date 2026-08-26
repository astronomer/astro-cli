//go:build windows

package localrt

// claimSupported refuses on Windows, where a standalone claim could only ever
// publish a record that reads as stopped.
//
// Standalone liveness there is a hardcoded false: there is no process-group
// signal to probe, so localprune's group check returns false unconditionally and
// the standalone engine's ReadStatus is a not-supported stub. A claim would
// therefore succeed, then be invisible to `astro local list`, error out of
// `astro local status`, and never make a start refuse — the exact silent failure
// rt.ExternalRuntime.Pgid warns about, guaranteed rather than merely possible.
//
// Refusing at the door is the honest answer. Docker is the only local mode on
// Windows anyway, and docker mode records itself through Start.
func claimSupported() error { return ErrClaimUnsupported }
