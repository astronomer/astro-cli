//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// unreadableLogin is what the CLI prints when it reads a saved login and the
// vault will not give it back (config/login.go).
const unreadableLogin = "Could not read your saved Astro login"

// Deciding which platform the current context is on, which every command
// does before cobra has seen argv, reads the context and never its login: a
// command that needs no login never reaches the keyring or the vault.
//
// Whether the vault was read is made visible without a keyring: the context's
// login is in the vault (the config's token is the "Bearer " placeholder), and
// the vault's directory is a file, so reading the login fails at once, before
// any keyring call, and the CLI says so on stderr. A command that reads no
// login says nothing. The control, `astro auth token`, reads it.
//
// Tier 0: everything is under the project's own home.
func TestDecidingThePlatformReadsNoLogin(t *testing.T) {
	tier(t, 0)

	for _, platform := range []string{"cloud", "software"} {
		t.Run(platform, func(t *testing.T) {
			p := newProject(t)
			writeHomeContext(t, p, platform, "    token: \"Bearer \"\n")
			// The vault's directory, as a file: pkg/secrets refuses it before
			// it asks the keyring anything.
			if err := os.WriteFile(filepath.Join(p.home, ".astro", "secrets"), nil, 0o600); err != nil {
				t.Fatal(err)
			}

			for _, args := range [][]string{{"version"}, {"init", "--name", "reads-no-login"}} {
				r := runIn(t, p, args...).requireSuccess()
				if strings.Contains(r.Stderr, unreadableLogin) {
					t.Errorf("astro %s read the login:\n%s", strings.Join(args, " "), r.output())
				}
			}

			if platform != "cloud" {
				return
			}
			// The control: a command that reads the login reports it cannot.
			r := runIn(t, p, "auth", "token")
			if !strings.Contains(r.Stderr, unreadableLogin) {
				t.Errorf("astro auth token did not report the unreadable login, so this case cannot see a read:\n%s", r.output())
			}
		})
	}
}
