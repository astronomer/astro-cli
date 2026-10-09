# AGENTS.md

Notes for coding agents working in this repository. Read [CONTRIBUTING.md](CONTRIBUTING.md) for setup and [docs/architecture.md](docs/architecture.md) for where new code goes.

## How to test and lint

- `make test` runs the unit tests of the root module. Use the target, not your own `go test ./...`: it adds `-race`, `-shuffle=on`, `-count=1` and the coverage flags, and a failure often needs them to reproduce.
- `make lint` runs golangci-lint and `make fmt` runs gofumpt, both through prek, at the versions [prek.toml](prek.toml) pins. Both need `prek` on PATH; without it they fail before doing anything, and the per-module commands below are the way through.
- Each `pkg/*` directory with its own `go.mod` is a separate module, and `make test` and `make lint` do not descend into it. After a change under `pkg/*`, run `make test-submodules` and `make lint-submodules`, which cover every one of them. The list in `LINT_SUBMODULES` is not maintained by memory — `TestEveryPkgSubmoduleIsLinted` in `internal/archlint` fails on a `pkg/*/go.mod` missing from it.
- `make lint-goos` lints for the platforms the host run did not cover. This is the one most easily forgotten and the one CI will catch: a `//go:build !windows` file has a paired stub that must gain the same methods, and a linter that cannot see a file reports success on it. Run it after the other two — on its own it skips the host platform, so it is not a whole lint.
- `make deadcode` fails on any root-module function that no path from `main()` reaches on linux or windows — the exported orphans `unused` cannot see, because it stops at the package boundary. Test callers do not count, so code only its own tests call is dead: delete it with its tests, or move a helper tests still need into a `_test.go` file. Deleting one finding often exposes another; re-run until clean. It skips the `pkg/*` sub-modules, which have callers outside this repo (Astro Desktop), so a sub-module function the CLI never calls is not necessarily dead.
- `make update-schemas` regenerates every output golden: the `--output json` shapes under `cmd/local/testdata/schema`, `cmd/astro/testdata/schema`, `cmd/testdata/schema`, `cmd/apc/testdata/schema` and `cmd/api/testdata/schema`, and `astro deployment inspect`'s bytes. Read the diff before committing it: those shapes are what scripts, agents and Astro Desktop parse.

Before pushing anything that touches per-GOOS files — today `pkg/airflowrt`, `pkg/fsatomic`, `pkg/localrt` and `pkg/proxy` among the sub-modules, plus `pkg/ansi` and `pkg/input` inside the root one — cross-compile. Two commands reproduce the whole `test-windows` build step in seconds, and the root `./...` does not descend into `pkg/*` or `e2e/`, so run them per module too:

```bash
GOOS=windows go build ./...
GOOS=windows go vet ./...
```

## The e2e suite

`e2e/` is its own module behind the `e2e` build tag, driving the built binary as a subprocess. It replaced the Python suite that used to live in `integration-test/`.

Cost differs by three orders of magnitude across its cases, so each declares a tier and a run picks a **ceiling** with `ASTRO_E2E_MAX_TIER` — a ceiling, not a selection. Anything above it skips with a message naming the variable rather than failing for want of a tool.

| tier | needs | where it runs |
|---|---|---|
| 0 | nothing — temp dirs only | every PR, Linux and Windows |
| 1 | uv (a real Airflow in a venv, no Airflow process) | every PR, Linux |
| 2 | a real Airflow running | nightly; Unix only |
| 3 | Docker | nightly; Unix only |
| 4 | cloud credentials | not written |

```bash
make test-e2e                      # tier 0, the default
make test-e2e ASTRO_E2E_MAX_TIER=1 # through a real venv
make test-e2e ASTRO_E2E_MAX_TIER=2 # through a real Airflow
make test-e2e ASTRO_E2E_MAX_TIER=3 # through a real Airflow in Docker
make lint-e2e                      # the e2e module, with --build-tags e2e
```

Every uv the suite runs, the CLI's own included, resolves as of a fixed date: `pinnedExcludeNewer` in `e2e/excludenewer_test.go`, handed over as `UV_EXCLUDE_NEWER`. A weekly workflow opens a PR moving it; a case that needs something published after it (a new Airflow series, most often) moves it with `scripts/bump-e2e-exclude-newer.sh`. The Astronomer build of Airflow that `astro init` writes into `[tool.uv]` is picked as of the same date, from the upload times written in the text of Astronomer's index pages (uv itself reads none there, which is why `astro init` also writes `exclude-newer-package = false` for those packages), and a tier 0 case reads no index at all (`ASTRO_AIRFLOW_INDEX_URL` points nowhere), so it writes none. `ASTRO_E2E_EXCLUDE_NEWER=none` resolves against live PyPI, which is what the nightly's `live-pypi` leg does; a docker-mode build (tier 3) installs inside the image and is not pinned either way.

A test added to `e2e/` carries a build tag of its own, and it has to match the tags of every helper it calls: a case tagged plain `e2e` that reaches for something in a `//go:build e2e && !windows` file compiles locally and fails `test-windows`, which builds the module at tier 0. `GOOS=windows go vet -tags e2e ./...` from `e2e/` catches it.

The isolation contract is the part worth getting right, and `e2e/doc.go` states it: `XDG_CACHE_HOME`, `ASTRO_HOME` and `HOME` are three separate levers and none implies the others. Run the CLI only through `newProject`, or a case writes into the developer's own state.

## Things worth knowing before you claim something works

- The harness sets `ASTRO_TELEMETRY_DISABLED=1`, which also suppresses the config writes a first run would make. A case asserting "this leaves nothing behind" can pass because the harness disabled the thing it meant to check.
- `gofumpt` and `go test` passing is not the same as CI passing. The lint job is the one with the opinions.
- Tier 3 leaves nothing behind only because its cases clean up after themselves. Docker's storage is outside all three isolation levers, so nothing else will remove it, and there is more of it than the obvious:
  - the metadata volume, which a plain `astro local stop` keeps by design, and the compose network beside it;
  - the built image, ~1.3 GB. NOT only when a Dockerfile is declared: `imagebuild` runs the base as-is only when the project adds no dependencies AND no OS packages, so declaring either one tags `astro-local/<project>` too;
  - the two tags `astro package astro` writes, `astro-package/<name>:<runtime>-<hash>` and a moving `:latest`. `astro local reset --yes` does not touch these — it knows the local build's tag — so a case that packages removes them itself;
  - the image `astro deploy` builds before it pushes, `astro-deploy/<dir>-<hash>:<dags|nodags|deps>-<random>`, unique per deploy, which stays behind when the deploy fails after its build. A case that deploys removes it itself (by repository, since the tag is random). A deploy or package that ships the project also tags an intermediate `<image>:<tag>-deps` for a moment, under the same repository.

  Use `astro local reset --yes`, not `stop --clean`: stop refuses when no runtime is recorded, which is every case that has already stopped, and reset derives the compose project from the path instead.
- The suite censuses itself, so most of that is caught for you — but know what the census covers before you rely on it. `e2e/leakcensus_test.go` snapshots before and after the run and fails on anything new along five axes: compose containers, volumes and networks whose names start `astro-`, images under `astro-local/`, `astro-package/` and `astro-deploy/`, and proxy daemons still serving from the binary that run built. Two things it cannot do: an image that a rebuild left untagged matches no repository filter and is invisible to it, and it will blame you for an astro project started in another terminal mid-run, or for a second suite run overlapping this one.
- A start waits five minutes for Airflow by default. `ASTRO_LOCAL_HEALTH_TIMEOUT` changes that — `10m` for a slow link or a cold image pull, something short to test what a start that never becomes healthy leaves behind. It takes a Go duration, so it needs a unit: a bare `600` is rejected, with a warning.
