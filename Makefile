OUTPUT ?= astro

GIT_COMMIT=$(shell git rev-parse HEAD)
GIT_COMMIT_SHORT=$(shell git rev-parse --short HEAD)
VERSION ?= SNAPSHOT-${GIT_COMMIT_SHORT}

LDFLAGS_VERSION=-X github.com/astronomerio/astro-cli/cmd.version=${VERSION} 
LDFLAGS_GIT_COMMIT=-X github.com/astronomerio/astro-cli/cmd.gitcommit=${GIT_COMMIT}

.DEFAULT_GOAL := build

dep:
	dep ensure

build:
	go build -o ${OUTPUT} -ldflags "${LDFLAGS_VERSION} ${LDFLAGS_GIT_COMMIT}" main.go

install: build
	$(eval DESTDIR ?= $(GOBIN))
	mkdir -p $(DESTDIR)
	cp ${OUTPUT} $(DESTDIR)
