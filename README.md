# Astronomer CLI [![GoDoc](https://godoc.org/github.com/astronomer/astro-cli?status.svg)](https://godoc.org/github.com/astronomer/astro-cli) [![Go Report Card](https://goreportcard.com/badge/github.com/astronomer/astro-cli)](https://goreportcard.com/report/github.com/astronomer/astro-cli)

The Astronomer CLI can be used to build Airflow DAGs locally and run them via Docker-Compose, as well as to deploy those DAGs to Astronomer-managed Airflow clusters and interact with the Astronomer API in general.

```
astro is a command line interface for working with the Astronomer Platform.

Usage:
  astro [command]

Available Commands:
  airflow         Manage airflow projects and deployments
  auth            Manage astronomer identity
  cluster         Manage Astronomer EE clusters
  config          Manage astro project configurations
  deployment      Manage airflow deployments
  help            Help about any command
  service-account Manage astronomer service accounts
  upgrade         Check for newer version of Astronomer CLI
  user            Manage astronomer user
  version         Astronomer CLI version
  workspace       Manage Astronomer workspaces

Flags:
  -h, --help   help for astro

Use "astro [command] --help" for more information about a command.
```

## Installing `astro`

### If you're using Astronomer Cloud or Astronomer Enterprise v0.7.x

via `curl`:

```
curl -sSL https://install.astronomer.io | sudo bash -s -- v0.7.5
```

### If you're using Astronomer Enterprise v0.8

via `curl`
```
curl -sSL https://install.astronomer.io | sudo bash
```

### Previous Versions

via `curl`
```
curl -sSL https://install.astronomer.io | sudo bash -s -- [TAGNAME]
```
   
ie:
```
curl -sSL https://install.astronomer.io | sudo bash -s -- v0.3.1
```

> Note: If you get mkdir error during installation please download and run [godownloader](https://raw.githubusercontent.com/astronomerio/astro-cli/master/godownloader.sh) script locally. 

    $ cat godownloader.sh | bash -s -- -b /usr/local/bin

### Installing on Windows

> Note: Make sure you have Windows 10 and Docker installed

1. Download latest release of astro-cli using this [astro_0.8.2_windows_386.zip](https://github.com/astronomer/astro-cli/releases/download/v0.8.2/astro_0.8.2_windows_386.zip)
2. Extract `astro_0.8.2_windows_386.zip` and copy `astro.exe` somewhere in your `%PATH%`
3. Open cmd or PowerShell console and run:

```
C:\Windows\system32>astro version
Astro CLI Version: 0.8.2
Git Commit: f5cdab8f832da3c6184a7ac167b491e3bac3c022
```

#### Troubleshooting:

1. Make sure you go through instruction to install Docker on windows properly https://docs.docker.com/docker-for-windows/install/
2. Make sure you enabled Hyper-V, it's requre to Docker and Linux Containers, also plesae review this document
https://docs.docker.com/docker-for-windows/troubleshoot/


## Getting Started

1. Confirm the install worked:

```
$ astro
```

2. Create a project:

```
$ mkdir hello-astro && cd hello-astro
$ astro airflow init
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

Dags can go in the `dags` folder, custom airflow plugins in `plugins`, python packages needed can go in `requirements.txt`, and OS level packages can go in `packages.txt`.

1. Start airflow

Run `astro airflow start` to start a local version of airflow on your machine. This will spin up a few locally running docker containers - one for the airflow scheduler, one for the webserver, and one for postgres.
(Run `docker ps` to verify)

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
$ astro airflow deploy --help
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

### Testing Locally
astro-cli is a single component of the much larger Astronomer Enterprise platform. In order to test locally you will need to

1. setup both [houston-api](https://github.com/astronomerio/houston-api) and [orbit-ui](https://github.com/astronomerio/orbit-ui).
2. edit your global or project config to enable local development

ex.

```yaml
local:
  enabled: true
  houston: http://localhost:8871/v1
  orbit: http://localhost:5000
```

## Docs
Docs (/docs) are generated using the `github.com/spf13/cobra/doc` pkg. Currently this pkg is broken with go vendoring, the following instructions include a workaround

1. Remove the `/vendor/github.com/spf13/cobra` pkg, forcing Go to search your go path for the package instead
2. `go run gendocs/gendocs.go`
3. restore `/vendor/github.com/spf13/cobra`

## Versioning

Astronomer Enterprise is under very active development. Because of this we cannot make backwards compatibility guarantees between versions.

THe astro-cli is following a semantic versioning scheme

`{MAJOR_RELEASE}.{MINOR_RELEASE}.{PATCH_RELEASE}`

with all releases up until 1.0.0 considered beta.


### Compatibility
Starting with `v0.3.3` the astro-cli began tightly tracking the platform release versioning, this means that compatibility is only guaranteed between matching __minor__ versions of the platform and the astro-cli.

ie. astro-cli `v0.4.0` is guaranteed to be compatible with houston-api `v0.4.x` but with houston-api `v0.5.x`

### Note
These changes were introduced platform wide with v0.4.0
