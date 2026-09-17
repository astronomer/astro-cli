// Package e2e drives the built astro binary the way a user does: argv in,
// stdout and an exit code out. Nothing here imports the code under test.
//
// # Running it
//
//	make test-e2e-tier0            # the hermetic tier, seconds, no tools needed
//	make test-e2e ASTRO_E2E_MAX_TIER=2
//
// Every test file is behind the `e2e` build tag, so a plain `go test ./...`
// anywhere in the repo will not start CLI processes. This file carries no tag
// on purpose: a package whose every file is excluded by a constraint makes
// `go vet ./...` fail rather than skip.
//
// # Tiers
//
// Cost differs by three orders of magnitude across these cases, so each test
// declares its tier and a run selects a ceiling with ASTRO_E2E_MAX_TIER
// (default 0). Anything above the ceiling skips with a message naming the
// variable, rather than failing for want of a tool.
//
//	0  hermetic — temp dirs only. No uv, no Docker, no network, no Airflow.
//	   Runs on every PR, Linux and Windows.
//	1  needs uv — a real Airflow in a venv, but no Airflow process.
//	2  runs a real Airflow — standalone start/stop, ports, readiness.
//	3  needs Docker.
//	4  needs cloud credentials.
//
// # Isolation contract
//
// This is the part worth getting right, because a test that leaks writes into
// the developer's own state is worse than no test. Three separate levers, none
// of which implies the others:
//
//	XDG_CACHE_HOME  runtime records — what `astro local list` reads
//	ASTRO_HOME      ~/.astro: config, proxy routes, the global env file
//	HOME            the encrypted vault, which resolves through os.UserHomeDir
//	                and so follows neither of the two above
//
// [newProject] sets all three for every command, which is why the only way to
// run the CLI here is through it. Setting ASTRO_HOME alone is the tempting
// mistake: do that and `astro local list` still lists — and writes — the
// developer's real projects.
//
// The one thing no environment variable moves is the OS keyring. Relocating
// HOME keeps the vault's *files* in a temp dir, but a `--secret` write would
// still reach the real Keychain or Secret Service. No case here does that, and
// the first one that needs to has to bring its own opt-in gate: writing
// credentials into the developer's own Keychain is not something a suite may
// decide to do on their behalf.
package e2e
