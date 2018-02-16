# Astronomer CLI

## Setup

1. Install Go:

    ```
    brew install go
    ```

    More info: https://golang.org/doc/install

2. Set `GOPATH` (recommended: `~/go`) etc in .bash_profile or .bashrc:

    ```
    export GOPATH=$HOME/go
    export GOBIN=$HOME/go/bin
    export PATH=$PATH:$GOBIN
    ```

    More info: https://github.com/golang/go/wiki/SettingGOPATH

### Vendor
- All dependnecies are managed with dep, with the exception of libcompose. It needs to be manually added with `go get github.com/docker/libcompose`. Issue here: https://github.com/docker/libcompose/issues/503.
- Alternatively, it is also a git submodule, which should get pulled in when cloned.
