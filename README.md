# Astronomer CLI

The Astronomer CLI is the recommended way to get started developing and deploying on Astronomer Enterprise Edition.

## Install

- via `curl`
    ```
    curl -sL https://install.astronomer.io | sudo bash
    ```

- via `go get`
    ```
    $ brew install go
    ```

    More info: <https://golang.org/doc/install>

    Set `GOPATH` (recommended: `~/go`) in .bash_profile or .bashrc:

    ```
    export GOPATH=$HOME/go
    export GOBIN=$HOME/go/bin
    export PATH=$PATH:$GOBIN
    ```

    More info: <https://github.com/golang/go/wiki/SettingGOPATH>

    Install astro-cli binary:
    ```
    $ go get github.com/astronomerio/astro-cli
    ```

## Getting Started

1. Run it to see commands:

    ```
    $ astro
    ```

1. Create a project:

    ```
    $ mkdir hello-astro && cd hello-astro
    $ astro airflow init
    ```

## Help

The CLI includes a help command, descriptions, as well as usage info for subcommands.

To see the help overview:

```
$ astro help
```

Or for subcommands:

```
$ astro airflow --help
```

```
$ astro airflow create --help
```

## Development

How to get started as a developer.

1. Build:

    ```
    $ git clone git@github.com:astronomerio/astro-cli.git
    $ cd astro-cli
    $ make build
    ```

1. (Optional) Install to `$GOBIN`:

    ```
    $ make install
    ```

1. Run:

    ```
    $ astro
    ```

### Testing w/ houston-api
astro-cli communicates with [houston-api](https://github.com/astronomerio/houston-api) in order to manage Astronomer EE resources (users, workspaces, deployments). Follow the Development section on the houston-api README in order develop and test the integration between these two systems.
