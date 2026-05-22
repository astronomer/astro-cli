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
# Usage:
#   make nightly-tag                    # date-based tag from main (nightly-20260522)
#   make rc-tag VERSION=1.43.0          # auto-increment RC (v1.43.0-rc.1, rc.2, ...)
#   make release-tag VERSION=1.43.0     # GA tag (requires at least one RC)

.PHONY: nightly-tag rc-tag release-tag validate-version

validate-version:
ifndef VERSION
	$(error VERSION is required. Usage: make rc-tag VERSION=1.43.0)
endif

nightly-tag:
	@set -e; \
	DATE=$$(date -u +%Y%m%d); \
	EXISTING=$$(git tag -l "nightly-$$DATE" "nightly-$$DATE.*" 2>/dev/null | wc -l | tr -d ' '); \
	if [ "$$EXISTING" -eq 0 ]; then \
		echo "nightly-$$DATE"; \
	else \
		echo "nightly-$$DATE.$$((EXISTING + 1))"; \
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
	if git rev-parse "v$(VERSION)" >/dev/null 2>&1; then \
		echo "Error: tag v$(VERSION) already exists." >&2; \
		exit 1; \
	fi; \
	echo "v$(VERSION)"
