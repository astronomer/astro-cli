// Package testenv keeps a test binary's environment from changing what the CLI
// does. It imports nothing but os, so any package's tests, including those of
// packages pkg/testing itself imports, can use it from TestMain.
package testenv

import "os"

// behaviorEnv is every environment variable the CLI reads that changes what
// it does: who it logs in as, where its state lives, which domain it talks to,
// and whether it may prompt. A developer's shell or a CI runner may export any
// of them, and a test must not depend on that.
var behaviorEnv = []string{
	// Credentials and the session they select.
	"ASTRO_API_TOKEN",
	"ASTRONOMER_KEY_ID",
	"ASTRONOMER_KEY_SECRET",
	"ASTRO_DOMAIN",
	"ASTRO_ORGANIZATION",
	// Where the CLI keeps its state.
	"ASTRO_HOME",
	// Whether the run is non-interactive, or reports as a CI system.
	"CI",
	"JENKINS_URL",
	"HUDSON_URL",
	"TF_BUILD",
	"GITHUB_ACTIONS",
	"DEPLOY_ACTION",
	"DEPLOY_ACTION_VERSION",
	// Which release channel it reports, and how it renders.
	"OTTO_CHANNEL",
	"NO_COLOR",
	"CLICOLOR",
}

// Clear unsets the environment variables that change the CLI's
// behavior, for a whole test binary. A package calls it first in its TestMain,
// so its tests run the same under a shell that exports ASTRO_API_TOKEN or
// ASTRO_HOME, and on a CI runner. A test that needs one set sets it itself with
// t.Setenv (or UseAPIToken), which the test restores afterward.
//
// It is for TestMain, where there is no *testing.T to Setenv through; a test
// must never call it.
func Clear() {
	for _, k := range behaviorEnv {
		if err := os.Unsetenv(k); err != nil { //nolint:usetesting // for the whole package run
			panic(err)
		}
	}
}
