package vaultenv

import (
	"testing"

	"github.com/astronomer/astro-cli/internal/envresolve"
	"github.com/astronomer/astro-cli/internal/localenv"
)

// Text names each store for what it is, the three vaults alike, and drops a
// workspace's id but keeps why it could not be read. JSON keeps the source
// labels themselves, so this is the one place they are translated.
func TestSourceLabel(t *testing.T) {
	for source, want := range map[string]string{
		localenv.SourceProject:                  ".env",
		SourceProject:                           "project vault",
		SourceGlobal:                            "global vault",
		localenv.SourceShell:                    "shell",
		envresolve.SourceDefault:                "default",
		localenv.SourceAbsent:                   "",
		"workspace":                             "cloud vault",
		localenv.WorkspaceSource("cmws"):        "cloud vault",
		"workspace (unavailable: no route)":     "cloud vault (unavailable: no route)",
		"something a newer build might call it": "something a newer build might call it",
	} {
		if got := SourceLabel(source); got != want {
			t.Errorf("SourceLabel(%q) = %q, want %q", source, got, want)
		}
	}
}
