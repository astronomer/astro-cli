OUTPUT ?= astro

GIT_COMMIT=$(shell git rev-parse HEAD)
GIT_COMMIT_SHORT=$(shell git rev-parse --short HEAD)
VERSION ?= SNAPSHOT-${GIT_COMMIT_SHORT}

LDFLAGS_VERSION=-X github.com/astronomer/astro-cli/version.CurrVersion=${VERSION}
LDFLAGS_GIT_COMMIT=-X github.com/astronomer/astro-cli/version.CurrCommit=${GIT_COMMIT}

.DEFAULT_GOAL := build

dep:
	dep ensure

build:
	go build -o ${OUTPUT} -ldflags "${LDFLAGS_VERSION} ${LDFLAGS_GIT_COMMIT}" main.go

test:
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

vet:
	@echo "--> Running go vet"
	@go vet $(GOFILES); if [ $$? -eq 1 ]; then \
		echo ""; \
		echo "Vet found suspicious constructs. Please check the reported constructs"; \
		echo "and fix them if necessary before submitting the code for review."; \
		exit 1; \
	fi

style:
	@echo ">> checking code style"
	@! gofmt -d $(shell find . -path ./vendor -prune -o -name '*.go' -print) | grep '^'

staticcheck:
	@echo ">> running staticcheck"
	@staticcheck $(GOFILES)

gosimple:
	@echo ">> running gosimple"
	@gosimple $(GOFILES)

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
