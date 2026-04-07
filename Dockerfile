FROM docker:19.03.2-dind@sha256:e2adb585547bb4aa3816cb3ff5447276f169bdd0e012cb01dfc734c2070d2346

RUN apk add --no-cache bash git make musl-dev go curl
SHELL ["/bin/bash", "-c"]

# Configure Go
ENV GOROOT /usr/lib/go
ENV GOPATH /go
ENV PATH /go/bin:$PATH

RUN mkdir -p ${GOPATH}/src ${GOPATH}/bin

WORKDIR $GOPATH
