GIT_COMMIT_SHORT=$(shell git rev-parse --short HEAD)
VERSION ?= SNAPSHOT-${GIT_COMMIT_SHORT}
LDFLAGS_VERSION=-X github.com/astronomer/astro-cli/version.CurrVersion=${VERSION}
OUTPUT ?= astro
PWD=$(shell pwd)

generate:
	go generate -x

lint:
	prek run golangci-lint --all-files

build:
	go build -o ${OUTPUT} -ldflags "${LDFLAGS_VERSION}" main.go

test:
	go test -count=1 -cover -coverprofile=coverage.txt -covermode=atomic ./... -test.v

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
