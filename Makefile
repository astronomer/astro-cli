OUTPUT ?= astro

.DEFAULT_GOAL := build

dep:
	dep ensure

build:
	go build -o ${OUTPUT} main.go

install: build
	$(eval DESTDIR ?= $(GOBIN))
	mkdir -p $(DESTDIR)
	cp ${OUTPUT} $(DESTDIR)
