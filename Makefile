GIT_COMMIT_SHORT=$(shell git rev-parse --short HEAD)
VERSION ?= SNAPSHOT-${GIT_COMMIT_SHORT}
LDFLAGS_VERSION=-X github.com/astronomer/astro-cli/version.CurrVersion=${VERSION}
OUTPUT ?= astro
PWD=$(shell pwd)

generate:
	go generate -x

lint:
	go run github.com/golangci/golangci-lint/cmd/golangci-lint version
	go run github.com/golangci/golangci-lint/cmd/golangci-lint run --timeout 15m0s

build:
	go build -o ${OUTPUT} -ldflags "${LDFLAGS_VERSION}" main.go

test:
	go test -count=1 -cover -coverprofile=coverage.txt -covermode=atomic ./...

temp-astro:
	cd $(shell mktemp -d) && ${PWD}/astro dev init

mock:
	go run github.com/vektra/mockery/v2 --version
	go run github.com/vektra/mockery/v2

codecov:
	@eval $$(curl -s https://codecov.io/bash)

install: build
	cp astro ~/go/bin

uninstall:
	rm ~/go/bin/astro
