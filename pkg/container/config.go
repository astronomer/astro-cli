// Package container resolves the host container runtime (Docker, OrbStack, or
// Podman) and manages the Podman "astro-machine" VM, decoupled from astro-cli's
// global config singleton and CLI spinner so it can be imported by other tools
// (e.g. astro-desktop). It is a faithful, dependency-injected port of the logic
// in astro-cli's airflow/runtimes package; the goal is one shared source of
// truth for "which engine, and how to reach it."
package container

// Config supplies the few values the runtime needs, decoupled from astro-cli's
// global config.CFG (viper) singleton. astro-cli's CLI fills this from config.CFG;
// astro-desktop fills it from its own settings. Zero values are valid: an empty
// Binary means auto-detect, and empty machine sizing falls back to Podman's own
// defaults.
type Config struct {
	// Binary optionally pins the engine binary ("docker" or "podman"). Empty
	// means auto-detect by searching $PATH (docker first, then podman). Any other
	// value is ignored and auto-detection runs instead.
	Binary string
	// MachineMemory and MachineCPU size the Podman machine on first init. Empty
	// omits the flag, letting Podman choose its default.
	MachineMemory string
	MachineCPU    string
}

// Feedback receives progress updates from long-running lifecycle operations,
// replacing astro-cli's hard dependency on the CLI spinner. astro-desktop passes
// NoopFeedback for read-only paths (logs/shell); a start flow can wire it to UI.
type Feedback interface {
	Start(message string)
	Update(message string)
	Success(message string)
	Stop()
}

// NoopFeedback is a Feedback that ignores every call. It is the default when a
// nil Feedback is passed to NewManager.
type NoopFeedback struct{}

func (NoopFeedback) Start(string)   {}
func (NoopFeedback) Update(string)  {}
func (NoopFeedback) Success(string) {}
func (NoopFeedback) Stop()          {}
