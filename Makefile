GIT_COMMIT_SHORT=$(shell git rev-parse --short HEAD)
VERSION ?= SNAPSHOT-${GIT_COMMIT_SHORT}
LDFLAGS_VERSION=-X github.com/astronomer/astro-cli/version.CurrVersion=${VERSION}
OUTPUT ?= astro
PWD=$(shell pwd)

generate:
	go generate -x

lint:
	prek run golangci-lint --all-files

# golangci-lint does not descend into a nested module, so a root run has never
# reached the pkg/* sub-modules and each has to be linted on its own. prek runs
# from the repo root whatever directory invokes it, so this calls the linter
# directly — at the version prek.toml pins, which stays the one place that
# version is written down.
GOLANGCI_VERSION=$(shell sed -n 's/.*golangci-lint@\(v[0-9.]*\).*/\1/p' prek.toml | head -1)

# Every pkg/* module, because a root golangci-lint run does not descend into a
# nested one and being named here is the only thing that lints them. Keeping the
# list complete is not left to memory: TestEveryPkgSubmoduleIsLinted in
# internal/archlint reads this variable and fails on a pkg/*/go.mod missing from
# it, and its sibling fails on an entry missing from coreBelowCmd. So a new module
# fails two tests on the commit that adds it, which is the cheapest place to
# find out.
LINT_SUBMODULES=pkg/airflowapi pkg/airflowenv pkg/airflowrt pkg/astroauth pkg/awsauth pkg/checks pkg/connmodel pkg/connwarehouse pkg/container pkg/emfetch pkg/envschema pkg/fsatomic pkg/googleauth pkg/imagebuild pkg/instancelocate pkg/instances pkg/localrt pkg/manifest pkg/platformversions pkg/proxy pkg/runtimeversions pkg/scaffold pkg/secrets pkg/telemetry pkg/uv

lint-submodules:
	@set -e; for mod in ${LINT_SUBMODULES}; do \
		echo "==> $$mod"; \
		(cd $$mod && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_VERSION} run --timeout 5m); \
	done

# The e2e module needs a run of its own for the same reason the pkg/* ones do,
# plus --build-tags: every file in it is behind the `e2e` tag, and a linter that
# cannot see a file reports success on it.
lint-e2e:
	cd e2e && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_VERSION} run --timeout 5m --build-tags e2e

# The platforms every module is linted for.
#
# "A linter that cannot see a file reports success on it" is the reason lint-e2e
# passes --build-tags, and GOOS is a build tag. CI runs on ubuntu, so without
# this every _windows.go and _darwin.go in the tree — the process groups, the
# proxy's locking and pid checks, the machine plumbing — is checked by nobody,
# and a file nothing lints looks exactly like one that is clean.
LINT_GOOS=linux darwin windows

# The root module gets a shorter list, and darwin is the one missing. Analyzing
# it for darwin means typechecking github.com/fsnotify/fsevents, reached through
# airflow/, which is cgo-only on that platform and does not resolve from a linux
# host — which is what CI is. Every sub-module cross-analyzes for darwin fine;
# only the root is blocked, so only the root is trimmed.
#
# What still covers it: a darwin developer's own `make lint` is a native darwin
# run, and a macOS runner would cover it in CI if that is ever worth the minutes.
LINT_GOOS_ROOT=linux windows

# Two things keep this from being 24 modules times 3 platforms, and both are
# about not paying for a conclusion that is already known.
#
# The host platform is skipped, because lint and lint-submodules have just
# linted it. Together the three targets cover every platform whatever machine
# they run on. On its own this one is therefore not a whole lint — run it after
# the other two, the way CI does.
#
# And a module is only linted for a platform that changes what it compiles.
# Nineteen of the twenty-four see a byte-identical file set on every GOOS, so a
# second run can only reach the same answer more slowly; the five that do vary
# are where every finding this target has ever reported came from. Derived from
# `go list` rather than listed by hand, so the first _windows.go in a module
# enrols it without anyone having to notice that it should.
#
# Installed rather than `go run`, unlike every other target here: GOOS has to
# say what is ANALYZED, and `GOOS=windows go run` reads it as what to BUILD —
# it cross-compiles the linter and then fails to exec it. Installing with GOOS
# unset gives a host binary that the loop can then point at each platform. Go
# caches the build, so it is only paid once.
#
# Sequential, deliberately: concurrent golangci-lint processes fail on a shared
# lock with "parallel golangci-lint is running".
lint-goos:
	@set -e; \
	host=$$(go env GOOS); \
	bin=$$(mktemp -d); \
	GOBIN=$$bin go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_VERSION}; \
	fileset() { (cd "$$1" && GOOS=$$2 go list -f '{{.ImportPath}}:{{.GoFiles}}:{{.TestGoFiles}}' ./... 2>/dev/null | sort | tr -d ' \n'); }; \
	for goos in ${LINT_GOOS_ROOT}; do \
		if [ "$$goos" = "$$host" ]; then continue; fi; \
		echo "==> . ($$goos)"; \
		GOOS=$$goos $$bin/golangci-lint run --timeout 10m; \
	done; \
	for mod in ${LINT_SUBMODULES}; do \
		base=$$(fileset $$mod $$host); \
		for goos in ${LINT_GOOS}; do \
			if [ "$$goos" = "$$host" ]; then continue; fi; \
			if [ "$$(fileset $$mod $$goos)" = "$$base" ]; then continue; fi; \
			echo "==> $$mod ($$goos)"; \
			(cd $$mod && GOOS=$$goos $$bin/golangci-lint run --timeout 10m); \
		done; \
	done

# Functions no path from main() reaches, on every platform CI builds for. The
# version matches Astro Desktop's pre-push hook; scripts/deadcode.sh says what
# it leaves out and why.
DEADCODE_VERSION=v0.32.0

deadcode:
	DEADCODE_VERSION=${DEADCODE_VERSION} bash scripts/deadcode.sh

build:
	go build -o ${OUTPUT} -ldflags "${LDFLAGS_VERSION}" main.go

# Phony, build included: a stray ./build file would otherwise let `make install`
# skip the rebuild and ship a stale binary under this commit's version.
.PHONY: install uninstall build

# scripts/install.sh chooses the directory -- see its header for the rule and the
# reasoning. Both targets take:
#
#   make install INSTALL_DIR=/usr/local/bin
#   make install NAME=astro-dev        # sit beside a released astro
#
# NAME is := rather than ?= so that an exported NAME, common in CI images, cannot
# rename the install. A command-line NAME= still wins.
NAME := astro

install: build
	@INSTALL_DIR="${INSTALL_DIR}" NAME="${NAME}" OUTPUT="${OUTPUT}" VERSION="${VERSION}" bash scripts/install.sh install

uninstall:
	@INSTALL_DIR="${INSTALL_DIR}" NAME="${NAME}" bash scripts/install.sh uninstall

# GORACE=atexit_sleep_ms=0: a -race binary sleeps one second before it exits,
# to catch races with C atexit() handlers that Go does not have
# (golang/go#20364). One second per package over ~90 packages was most of this
# target's runtime: 75s to 9s on a warm cache.
test:
	GORACE=atexit_sleep_ms=0 go test -count=1 -race -shuffle=on -timeout=15m -cover -coverprofile=coverage.txt -covermode=atomic ./... -test.v

# The `--output json` payloads are pinned against goldens in
# cmd/local/testdata/schema, cmd/astro/testdata/schema, cmd/testdata/schema,
# cmd/apc/testdata/schema and cmd/api/testdata/schema (`astro api … describe
# -o json`), and `astro deployment inspect`'s bytes against
# cmd/astro/testdata/deployment_inspect.
# A deliberate change to one — a new field, a rename — regenerates them; the
# diff then lands in the PR, which is the point. Read what it writes before
# committing it: these shapes are what scripts, agents and Astro Desktop parse.
# ASTRO_UPDATE_SCHEMAS is the switch (cmd/cliout/cliouttest): an environment
# variable, so one switch reaches every package this one `go test` runs, where
# a custom test flag would fail any package that does not define it.
.PHONY: update-schemas
update-schemas:
	ASTRO_UPDATE_SCHEMAS=1 go test -count=1 ./cmd/ ./cmd/api/ ./cmd/local/ ./cmd/astro/ ./cmd/apc/ -run 'TestPublishedJSONPayloadsKeepTheirShape|TestDeploymentInspectPrintsPinnedBytes'

# Each pkg/* sub-module has its own go.mod, which the root `go test ./...`
# never descends into, so their tests need a run of their own.
test-submodules:
	@bash scripts/test-submodules.sh

# The e2e suite drives the built binary. See scripts/test-e2e.sh for why it is
# its own module behind a build tag, and e2e/doc.go for what each tier costs.
#
# The ceiling is a variable rather than a target per tier, so that tier 3
# arriving needs no new plumbing here:
#
#   make test-e2e                      # hermetic only, the default
#   make test-e2e ASTRO_E2E_MAX_TIER=2 # through to a real Airflow
ASTRO_E2E_MAX_TIER ?= 0

.PHONY: test-e2e test-e2e-tier0

test-e2e:
	@ASTRO_E2E_MAX_TIER=${ASTRO_E2E_MAX_TIER} bash scripts/test-e2e.sh

# Pinned to 0 rather than left to the default above, so an exported
# ASTRO_E2E_MAX_TIER cannot quietly turn this into a longer run. This is the
# tier that needs no tools, and the one a change to this repo must never break.
test-e2e-tier0:
	@ASTRO_E2E_MAX_TIER=0 bash scripts/test-e2e.sh

temp-astro:
	cd $(shell mktemp -d) && ${PWD}/astro dev init

mock:
	GOWORK=off go tool mockery --version
	GOWORK=off go tool mockery

fmt:
	prek run gofumpt --all-files

# Release tag helpers — used by CI and locally.
# Nightly tags are semver-compliant: vX.Y.Z-nightly.YYYYMMDD
# where X.Y.Z is the next minor version (latest minor + 1, patch 0).
# Usage:
#   make nightly-tag                    # semver nightly from main (v1.43.0-nightly.20260522)
#   make rc-tag VERSION=1.43.0          # auto-increment RC (v1.43.0-rc.1, rc.2, ...)
#   make release-tag VERSION=1.43.0     # GA tag (requires at least one RC, validates commit)

.PHONY: nightly-tag rc-tag release-tag validate-version

validate-version:
ifndef VERSION
	$(error VERSION is required. Usage: make rc-tag VERSION=1.43.0)
endif

nightly-tag:
	@set -e; \
	LATEST_TAG=$$(git tag -l "v[0-9]*.[0-9]*.[0-9]*" | grep -v -- '-' | sed 's/^v//' | sort -t. -k1,1n -k2,2n -k3,3n | tail -1 | sed 's/^/v/'); \
	if [ -z "$$LATEST_TAG" ]; then \
		echo "Error: no existing release tags found. Cannot compute next version for nightly." >&2; \
		exit 1; \
	fi; \
	MAJOR=$$(echo "$$LATEST_TAG" | sed 's/^v//; s/\..*//' ); \
	MINOR=$$(echo "$$LATEST_TAG" | sed 's/^v//; s/^[0-9]*\.//; s/\..*//' ); \
	NEXT_MINOR=$$((MINOR + 1)); \
	NEXT_VERSION="$$MAJOR.$$NEXT_MINOR.0"; \
	DATE=$$(date -u +%Y%m%d); \
	EXISTING=$$(git tag -l "v$$NEXT_VERSION-nightly.$$DATE" "v$$NEXT_VERSION-nightly.$$DATE.*" 2>/dev/null | wc -l | tr -d ' '); \
	if [ "$$EXISTING" -eq 0 ]; then \
		echo "v$$NEXT_VERSION-nightly.$$DATE"; \
	else \
		echo "v$$NEXT_VERSION-nightly.$$DATE.$$((EXISTING + 1))"; \
	fi

rc-tag: validate-version
	@set -e; \
	HIGHEST_RC=$$(git tag -l "v$(VERSION)-rc.*" | sed -n 's/.*-rc\.\([0-9]*\)$$/\1/p' | sort -n | tail -1); \
	if [ -z "$$HIGHEST_RC" ]; then \
		NEXT_RC=1; \
	else \
		NEXT_RC=$$((HIGHEST_RC + 1)); \
	fi; \
	echo "v$(VERSION)-rc.$$NEXT_RC"

release-tag: validate-version
	@set -e; \
	RC_COUNT=$$(git tag -l "v$(VERSION)-rc.*" 2>/dev/null | wc -l | tr -d ' '); \
	if [ "$$RC_COUNT" -eq 0 ]; then \
		echo "Error: no RC tags found for v$(VERSION). Create at least one RC before cutting a release." >&2; \
		exit 1; \
	fi; \
	LATEST_RC=$$(git tag -l "v$(VERSION)-rc.*" | sed 's/.*-rc\.//' | sort -n | tail -1); \
	LATEST_RC="v$(VERSION)-rc.$$LATEST_RC"; \
	RC_COMMIT=$$(git rev-list -n 1 "$$LATEST_RC"); \
	HEAD_COMMIT=$$(git rev-parse HEAD); \
	if [ "$$RC_COMMIT" != "$$HEAD_COMMIT" ]; then \
		echo "Error: latest RC ($$LATEST_RC) points to $$RC_COMMIT but HEAD is $$HEAD_COMMIT." >&2; \
		echo "The GA release must be cut from the same commit as the latest RC." >&2; \
		echo "To bypass this check for emergencies, push the tag manually: git tag v$(VERSION) && git push origin v$(VERSION)" >&2; \
		exit 1; \
	fi; \
	if git rev-parse "v$(VERSION)" >/dev/null 2>&1; then \
		echo "Error: tag v$(VERSION) already exists." >&2; \
		exit 1; \
	fi; \
	echo "v$(VERSION)"
