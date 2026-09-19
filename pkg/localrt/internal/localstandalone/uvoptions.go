package localstandalone

// UVOptions are the consumer's preferences for the venv provisioning a start
// does. The cache directory is deliberately absent: it is shared across
// projects and consumers so a Python toolchain downloads once, so the engine
// owns it.
//
// Untagged on purpose. Nothing in it is OS-dependent, and while it lived in the
// Unix implementation the Windows stub carried a hand-synced copy — so adding a
// field compiled everywhere the author looked and broke a build only CI's
// Windows job runs. One definition makes that impossible rather than merely
// remembered.
type UVOptions struct {
	HermeticEnv    bool
	OnCertFallback func()
}
