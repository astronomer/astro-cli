FROM docker:19.03.2-dind

RUN apk add --no-cache bash git make musl-dev go curl
SHELL ["/bin/bash", "-c"]

# Configure Go
ENV GOROOT /usr/lib/go
ENV GOPATH /go
ENV PATH /go/bin:$PATH

RUN mkdir -p ${GOPATH}/src ${GOPATH}/bin

WORKDIR $GOPATH
