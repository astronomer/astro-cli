# Astronomer CLI [![Release](https://img.shields.io/github/v/release/astronomer/astro-cli.svg?logo=github)](https://github.com/astronomer/astro-cli/releases) [![GoDoc](https://godoc.org/github.com/astronomer/astro-cli?status.svg)](https://godoc.org/github.com/astronomer/astro-cli) [![Go Report Card](https://goreportcard.com/badge/github.com/astronomer/astro-cli)](https://goreportcard.com/report/github.com/astronomer/astro-cli) [![codecov](https://codecov.io/gh/astronomer/astro-cli/branch/master/graph/badge.svg)](https://codecov.io/gh/astronomer/astro-cli)


The Astronomer CLI can be used to build Airflow DAGs locally and run them via docker-compose, as well as to deploy those DAGs to Astronomer-managed Airflow clusters and interact with the Astronomer API in general.

```
astro is a command line interface for working with the Astronomer Platform.

Usage:
  astro [command]

Available Commands:
  auth            Manage astronomer identity
  cluster         Manage Astronomer EE clusters
  completion      Generate autocompletions script for the specified shell (bash or zsh)
  config          Manage astro project configurations
  deploy          Deploy an airflow project
  deployment      Manage airflow deployments
  dev             Manage airflow projects
  help            Help about any command
  upgrade         Check for newer version of Astronomer CLI
  user            Manage astronomer user
  version         Astronomer CLI version
  workspace       Manage Astronomer workspaces

Flags:
  -h, --help   help for astro

Use "astro [command] --help" for more information about a command.
```

## Installing `astro`

The Astronomer CLI is an open-source project. Installing it to your machine allows you to easily spin up a local instance of Apache Airflow and allows you to easily deploy code to remote Airflow environments if you are an Astronomer customer.

> **Note:** If you are an Astronomer customer, your CLI version _must_ match the version of Astronomer you are running. If you are using Astronomer Cloud, the latest version should always be correct. If you have a custom Astronomer Enterprise installation, you may be on a different version, which means you may need to ensure that your CLI and platform match up; you can check which version of Astronomer you're running by clicking the user icon in the top right corner of our UI.

### Latest Version

#### Via `Homebrew`

```sh
brew install astronomer/tap/astro
```

#### Via `curl`

> Note: If you are a Mac user on Catalina make sure you are using the latest version of `curl` or you may receive certificate errors.

```
curl -sSL https://install.astronomer.io | sudo bash -s
```

### Previous Versions

#### Via `Homebrew`

To install a specific version Astro CLI use @major.minor.patch for example, to install v0.13.1 run:

```sh
brew install astronomer/tap/astro@0.13.1
```

#### Via `curl`

To install a previous version of Astronomer, you can add the tag at the end of your `curl` command via the following syntax:

```
curl -sSL https://install.astronomer.io | sudo bash -s -- [TAGNAME]
```

ie:
```
curl -sSL https://install.astronomer.io | sudo bash -s -- v0.7.5
```

> Note: If you get mkdir error during installation please download and run [godownloader](https://raw.githubusercontent.com/astronomer/astro-cli/main/godownloader.sh) script locally.

    $ cat godownloader.sh | bash -s -- -b /usr/local/bin


### Installing on Windows

> Note: Make sure you have Windows 10 and Docker installed

1. Download latest release of astro-cli using this [astro_0.13.1_windows_386.zip](https://github.com/astronomer/astro-cli/releases/download/v0.13.1/astro_0.13.1_windows_386.zip)
2. Extract `astro_0.13.1_windows_386.zip` and copy `astro.exe` somewhere in your `%PATH%`
3. Open cmd or PowerShell console and run:

```
C:\Windows\system32>astro version
Astro CLI Version: 0.13.1
Git Commit: 829e4702ca36dd725f1a98d82b6fdf889e5f4dc3
```

#### Troubleshooting

1. Make sure you go through instruction to install Docker on windows properly https://docs.docker.com/docker-for-windows/install/
2. Make sure you enabled Hyper-V, it's required for Docker and Linux Containers, also please review this document
https://docs.docker.com/docker-for-windows/troubleshoot/

## Getting Started

1. Confirm the install worked:

```
$ astro
```

2. Create a project:

```
$ mkdir hello-astro && cd hello-astro
$ astro dev init
```

This will generate a skeleton project directory:

```
.
├── dags
│   ├── example-dag.py
├── Dockerfile
├── include
├── packages.txt
├── plugins
└── requirements.txt
```

DAGs can go in the `dags` folder, custom Airflow plugins in `plugins`, python packages needed can go in `requirements.txt`, and OS level packages can go in `packages.txt`.

1. Start airflow

Run `astro dev start` to start a local version of airflow on your machine. This will spin up a few locally running docker containers - one for the airflow scheduler, one for the webserver, and one for postgres.
(Run `docker ps` to verify)

## Help

The CLI includes a help command, descriptions, as well as usage info for subcommands.

To see the help overview:

```
$ astro help
```

Or for subcommands:

```
$ astro dev --help
```

```
$ astro deploy --help
```

## Development

How to get started as a developer:

1. Build:

```
$ git clone git@github.com:astronomer/astro-cli.git
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

### Testing Locally

astro-cli is a single component of the much larger Astronomer Enterprise platform. In order to test locally you will need to

1. setup both [houston-api](https://github.com/astronomer/houston-api) and [astro-ui](https://github.com/astronomer/astro-ui).
2. edit your global or project config to enable local development

ex.

```yaml
local:
  enabled: true
  houston: http://localhost:8871/v1
  orbit: http://localhost:5000
```

### Run tests

To run unit-tests you can run:
> Note: Make sure you have running locally houston on http://localhost:8871/v1 it required for running tests

```bash
make test
```

## Docs

Docs (/docs) are generated using the `github.com/spf13/cobra/doc` pkg. Currently this pkg is broken with go vendoring, the following instructions include a workaround

1. Remove the `/vendor/github.com/spf13/cobra` pkg, forcing Go to search your go path for the package instead
2. `go run gendocs/gendocs.go`
3. restore `/vendor/github.com/spf13/cobra`

## Versions

The Astronomer platform is under very active development. Because of this we cannot make backwards compatibility guarantees between versions.

The astro-cli is following a semantic versioning scheme

`{MAJOR_RELEASE}.{MINOR_RELEASE}.{PATCH_RELEASE}`

with all releases up until 1.0.0 considered beta.

`astro-cli` tightly tracks the platform release versioning, this means that compatibility is only guaranteed between matching __minor__ versions of the platform and the astro-cli. ie. astro-cli `v0.9.0` is guaranteed to be compatible with houston-api `v0.9.x` but not houston-api `v0.10.x`

## Support

If you need support, start with the [Astronomer documentation](https://www.astronomer.io/docs/).

If you still have questions, you can post on the [Astronomer web forum](https://forum.astronomer.io) or if you are a customer, you can [open a support ticket](https://support.astronomer.io).
