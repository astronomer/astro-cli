# Astronomer CLI

## Setup

1. Install Go:

    ```
    $ brew install go
    ```

    More info: https://golang.org/doc/install

2. Set `GOPATH` (recommended: `~/go`) etc in .bash_profile or .bashrc:

    ```
    export GOPATH=$HOME/go
    export GOBIN=$HOME/go/bin
    export PATH=$PATH:$GOBIN
    ```

    More info: https://github.com/golang/go/wiki/SettingGOPATH

## Quickstart

How to get started as a user.

1. Install Astro CLI:

    ```
    $ go get github.com/astronomerio/astro-cli
    ```

2. Run it to see commands:

    ```
    $ astro-cli
    ```

3. Create a project:

    ```
    $ mkdir hello-astro && cd hello-astro
    $ astro airflow init
    ```

## Development

How to get started as a developer.

1. Build:

    ```
    git clone git@github.com:astronomerio/astro-cli.git
    cd astro-cli
    make build
    ```

2. (Optional) Install to `$GOBIN`:

    ```
    make install
    ```

3. Run:

    ```
    $ astro
    ```

### Vendor

- All dependencies are managed with dep, with the exception of libcompose. It needs to be manually added with `go get github.com/docker/libcompose`. Issue here: https://github.com/docker/libcompose/issues/503.
- Alternatively, it is also a git submodule, which should get pulled in when cloned.
