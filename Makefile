OUTPUT ?= astro

GIT_COMMIT=$(shell git rev-parse HEAD)
GIT_COMMIT_SHORT=$(shell git rev-parse --short HEAD)
VERSION ?= SNAPSHOT-${GIT_COMMIT_SHORT}

LDFLAGS_VERSION=-X github.com/astronomerio/astro-cli/cmd.currVersion=${VERSION} 
LDFLAGS_GIT_COMMIT=-X github.com/astronomerio/astro-cli/cmd.currCommit=${GIT_COMMIT}

.DEFAULT_GOAL := build

dep:
	dep ensure

build:
	go build -o ${OUTPUT} -ldflags "${LDFLAGS_VERSION} ${LDFLAGS_GIT_COMMIT}" main.go

test:
    go test -v

format:
	@echo "--> Running go fmt"
	@go fmt $(GOFILES)

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
