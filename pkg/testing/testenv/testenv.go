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
	"ASTRO_DEPLOYMENT",
	"ASTRO_REGISTRY_URL",
	"ASTRO_LOCAL_HEALTH_TIMEOUT",
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
	// Telemetry endpoints and debugging, and golden files rewritten in place.
	"ASTRO_TELEMETRY_API_URL",
	"ASTRO_TELEMETRY_DEBUG",
	"ASTRO_UPDATE_SCHEMAS",
	// Where the runtime catalog, constraints and freeze files come from.
	"ASTRO_RUNTIME_CONSTRAINTS_URL",
	"ASTRO_RUNTIME_FREEZE_URL",
	"ASTRO_RUNTIME_VERSIONS_URL",
	// uv settings the CLI reads. UV_CACHE_DIR is left alone: it only makes
	// the suite faster.
	"UV_EXCLUDE_NEWER",
	"UV_NO_SYNC",
	// The CI systems and agents the CLI detects for telemetry.
	"GITLAB_CI",
	"CIRCLECI",
	"BITBUCKET_BUILD_NUMBER",
	"CODEBUILD_BUILD_ID",
	"TEAMCITY_VERSION",
	"BUILDKITE",
	"CF_BUILD_ID",
	"TRAVIS",
	"OTTO",
	"CLAUDECODE",
	"CLAUDE_CODE_ENTRYPOINT",
	"CURSOR_TRACE_ID",
	"CURSOR_AGENT",
	"AIDER_MODEL",
	"CONTINUE_GLOBAL_DIR",
	"CORTEX_SESSION_ID",
	"GEMINI_CLI",
	"OPENCODE",
	"CODEX_API_KEY",
	"COPILOT_CLI",
	"COPILOT_AGENT",
	"AI_AGENT",
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
