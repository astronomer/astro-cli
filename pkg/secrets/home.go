package secrets

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	// configDirName is the astro home's directory name, matching config.ConfigDir.
	configDirName = ".astro"
	// dirName is the vault's directory under the astro home.
	dirName = "secrets"
)

// DefaultDir returns the shared vault directory: ~/.astro/secrets.
//
// This exists because the package doc's claim that both tools use "the shared
// value home" was, until this landed, only a claim. Config.Dir is a parameter,
// so two callers agreeing on the location came down to each writing the same
// filepath.Join by hand, against an astro home each resolves through its own
// code — which is how one shared vault quietly becomes two. A tool that
// computes the path itself can drift; one that calls this cannot. Every caller,
// CLI and desktop, should use it rather than joining "secrets" onto a home of
// its own.
//
// # Why ASTRO_HOME does not move the vault
//
// It moves everything else: config.HomeConfigPath is $ASTRO_HOME/.astro when
// that variable is set, and the CLI's own config.yaml goes with it. This
// deliberately does not follow, and the reason is that one of the two tools
// sharing this vault is a GUI application.
//
// A desktop app launched from Finder or the Dock inherits no shell
// environment, so the same user on the same machine resolves ASTRO_HOME one way
// from a terminal and another way from the desktop — and a vault whose location
// depends on how the process was started is a vault that moves out from under
// its own index. The desktop pairs each encrypted value with an index entry
// stored elsewhere; if the value root shifts between launches after a migration
// has already retired the old copy, the credentials are not merely missing,
// they are unrecoverable. Keying off the home directory instead makes the
// location the one thing that cannot vary between two launches by one user.
//
// The cost is that a user who relocates ~/.astro with ASTRO_HOME still finds
// their secrets in the real ~/.astro/secrets. That is worth saying out loud in
// the docs, and it is a much smaller problem than a relocated vault: it is
// visible, it loses nothing, and both tools still agree with each other, which
// is the property this package exists to guarantee.
func DefaultDir() (string, error) {
	home, err := astroHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, dirName), nil
}

// astroHome returns the directory the vault lives under: ~/.astro, from the
// user's home directory and nothing else.
//
// Unexported on purpose. An exported "AstroHome" invites a caller to place a
// sibling under it — and because this deliberately disagrees with
// config.HomeConfigPath whenever ASTRO_HOME is set, that caller would split its
// own state across two trees, which is the drift this package exists to prevent.
// If something outside really needs a vault-adjacent path, give it a name that
// says vault.
func astroHome() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		// Wrapped rather than passed through: the stdlib's "$HOME is not
		// defined" tells a user nothing about why a credential dialog failed,
		// and rt.CacheRoot in this repo sets the convention.
		return "", fmt.Errorf("resolving the astro home: %w", err)
	}
	return filepath.Join(home, configDirName), nil
}
