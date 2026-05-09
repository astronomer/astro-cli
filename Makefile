GIT_COMMIT_SHORT=$(shell git rev-parse --short HEAD)
VERSION ?= SNAPSHOT-${GIT_COMMIT_SHORT}
LDFLAGS_VERSION=-X github.com/astronomer/astro-cli/version.CurrVersion=${VERSION}
OUTPUT ?= astro
PWD=$(shell pwd)

generate:
	go generate -x

lint: lint-go lint-deadcode

lint-go:
	prek run golangci-lint --all-files

lint-deadcode:
	prek run deadcode --all-files

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
