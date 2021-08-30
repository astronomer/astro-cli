OUTPUT ?= astro

GIT_COMMIT=$(shell git rev-parse HEAD)
GIT_COMMIT_SHORT=$(shell git rev-parse --short HEAD)
VERSION ?= SNAPSHOT-${GIT_COMMIT_SHORT}

LDFLAGS_VERSION=-X github.com/astronomer/astro-cli/version.CurrVersion=${VERSION}
LDFLAGS_GIT_COMMIT=-X github.com/astronomer/astro-cli/version.CurrCommit=${GIT_COMMIT}

.DEFAULT_GOAL := build
# golangci-lint version
GOLANGCI_LINT_VERSION ?=v1.37.1

SHELL := /bin/bash

GOFMT ?= gofumpt -l -s -extra
GOFILES := $(shell find . -name "*.go" -type f | grep -v /vendor/)
ENVTEST_ASSETS_DIR=$(shell pwd)/testbin

dep:
	dep ensure

build:
	go build -o ${OUTPUT} -ldflags "${LDFLAGS_VERSION} ${LDFLAGS_GIT_COMMIT}" main.go

test: lint
	go test -count=1 -cover ./...
	go test -coverprofile=coverage.txt -covermode=atomic ./...

codecov:
	@eval $$(curl -s https://codecov.io/bash)

cover:
	rm -f cover.out
	go test -coverprofile=cover.out ./...
	go tool cover -func=cover.out

format:
	@echo "--> Running go fmt"
	@go fmt ./...

lint:
	@test -f ${ENVTEST_ASSETS_DIR}/golangci-lint -o -f /go/bin/golangci-lint || curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ${ENVTEST_ASSETS_DIR} ${GOLANGCI_LINT_VERSION}
	@if (test -f ${ENVTEST_ASSETS_DIR}/golangci-lint) then (${ENVTEST_ASSETS_DIR}/golangci-lint run) else (/go/bin/golangci-lint run) fi

tools:
	@echo ">> installing some extra tools"
	@go get -u -v honnef.co/go/tools/...

install: build
	$(eval DESTDIR ?= $(GOBIN))
	mkdir -p $(DESTDIR)
	cp ${OUTPUT} $(DESTDIR)

uninstall:
	$(eval DESTDIR ?= $(GOBIN))
	rm $(GOBIN)/$(OUTPUT)

ifeq (debug,$(firstword $(MAKECMDGOALS)))
  # use the rest as arguments for "debug"
  DEBUG_ARGS := $(wordlist 2,$(words $(MAKECMDGOALS)),$(MAKECMDGOALS))
  $(eval $(DEBUG_ARGS):;@:)
endif

debug:
	echo $(RUN_ARGS)
	dlv debug github.com/astronomer/astro-cli -- $(DEBUG_ARGS)
