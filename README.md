# Astro CLI [![Release](https://img.shields.io/github/v/release/astronomer/astro-cli.svg?logo=github)](https://github.com/astronomer/astro-cli/releases) [![GoDoc](https://godoc.org/github.com/astronomer/astro-cli?status.svg)](https://godoc.org/github.com/astronomer/astro-cli) [![Go Report Card](https://goreportcard.com/badge/github.com/astronomer/astro-cli)](https://goreportcard.com/report/github.com/astronomer/astro-cli) [![codecov](https://codecov.io/gh/astronomer/astro-cli/branch/main/graph/badge.svg)](https://codecov.io/gh/astronomer/astro-cli)

The Astro CLI is the modern command-line interface for data orchestration. It is the easiest way to run Apache Airflow on your local machine. With the Astro CLI, you can both develop and test DAGs locally as well as interact with our Astro and Astronomer Software offerings.

```
astro is a command line interface for Airflow working within the Astronomer Cloud or Software

Usage:
  astro [command]

Available Commands:
  completion  generate the autocompletion script for the specified shell
  config      Manage project configuration
  context     Manage Astro & Software contexts
  deploy      Deploy your project to a Deployment on Astro
  deployment  Manage your Deployments running on Astronomer
  dev         Run your Astro project locally
  help        Help about any command
  login       Log in to Astronomer
  logout      Log out of Astronomer
  version     astro version
  workspace   Manage Astronomer Workspaces

Flags:
  -h, --help   help for astro

Use "astro [command] --help" for more information about a command.
```

## Install `astro`

To install and use the Astro CLI on Mac, you must have:
- [Docker Desktop](https://docs.docker.com/get-docker/) (v18.09 or higher)

To install and use the Astro CLI on Linux, you must have:
- [Docker Engine](https://docs.docker.com/engine/install/) (v0.13.1 or higher).

To install and use the Astro CLI on Windows, you must have:
- Windows 10
- [Docker Desktop WSL 2 backend](https://docs.docker.com/desktop/windows/wsl/) (v0.13.1 or higher).

### Latest Version

#### via Homebrew

```sh
brew install astro
```

#### via cURL

```sh
curl -sSL install.astronomer.io | sudo bash -s
```

### Previous Versions

#### Via `Homebrew`

To install a specific version Astro CLI use @major.minor.patch. For example, to install v1.0.0 run:

```sh
brew install astronomer/tap/astro@1.0.0
```

#### Via `curl`

To install a previous version of Astronomer, you can add the tag at the end of your `curl` command via the following syntax:

```sh
curl -sSL https://install.astronomer.io | sudo bash -s -- [TAGNAME]
```

ie:
```sh
curl -sSL https://install.astronomer.io | sudo bash -s -- v1.0.0
```

> Note: If you get mkdir error during installation please download and run [godownloader](https://raw.githubusercontent.com/astronomer/astro-cli/main/godownloader.sh) script locally.

    $ cat godownloader.sh | bash -s -- -b /usr/local/bin


### Install on Windows


1. Download the latest release of the Astro CLI from [this page](https://github.com/astronomer/astro-cli/releases/) into your project
2. Copy `astro.exe` somewhere in your `%PATH%`
3. Open cmd or PowerShell console and run the `astro version` command. Your output should look something like this:

   ```
   C:\Windows\system32>astro version
   Astro CLI Version: x.y.z
   ```


#### Troubleshooting

1. Make sure you go through instruction to install Docker on windows properly https://docs.docker.com/docker-for-windows/install/
2. Install Windows WSL2 and [Docker Desktop for WSL 2 Backend](https://docs.docker.com/desktop/windows/wsl/) which is the recommend way to run the Astro CLI on Windows.
3. Alternativly, you can run the Astro CLI natively by enabling Hyper-V, which is required for Docker and Linux Containers. Review this document
   https://docs.docker.com/docker-for-windows/troubleshoot/ if you have issues.

## Getting Started

1. Create a project:

```
$ mkdir hello-astro && cd hello-astro
$ astro dev init
```

1. Install the binary and Confirm the install worked:

```
$ ./astro
```

This will generate a skeleton project directory:

```
.
├── dags
│   ├── example-dag-advanced.py
│   ├── example-dag-basic.py
├── tests
│   ├── dags
│       ├── test_dag_integrity.py
├── Dockerfile
├── include
├── packages.txt
├── plugins
└── requirements.txt
```

DAGs can go in the `dags` folder, custom Airflow plugins in `plugins`, python packages needed can go in `requirements.txt`, and OS level packages can go in `packages.txt`.

1. Start airflow

Run `astro dev start` to start a local version of airflow on your machine. This will spin up a few locally running docker containers - one for the airflow scheduler, one for the webserver, and one for postgres.
(Run `astro dev ps` to verify)

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

## Local Development

How to get started as a developer:

1. Install `go` 1.11+ - https://go.dev/doc/install
2. Install `golangci-lint` to run linter locally

    ```brew install golangci-lint```

    ```golangci-lint run .```
3. Install `pre-commit` to run lint on every commit

    ```brew install pre-commit```

    ```pre-commit install```

    Run lint locally:

    ```pre-commit run --all-files```
4. Clone and Build:

```
$ cd $GOPATH/src/github.com/astronomer/astro-cli
$ git clone git@github.com:astronomer/astro-cli.git
$ cd astro-cli
$ make build
```

## Testing Locally
The Astro CLI is a single command-line interface that allows users to interact with both Astro and Astronomer Software.

In order to test cloud locally you will need to update your global or local config to point to right platform type and local astro endpoint.
Ex.
```yaml
local:
  platform: cloud
  astro: http://localhost:8871/v1
```

Similarly in order to test software locally you will need to update to right platform type and local houston endpoint:
```yaml
local:
  platform: software
  houston: http://localhost:8871/v1
```

### Run tests

To run unit-tests you can run:

> Note: Make sure you have running locally houston or Astro on http://localhost:8871/v1 it is required for running some tests

```bash
make test
```

### Generating Mocks

We currently use [mockery](https://github.com/vektra/mockery) to generate mocks for interfaces.
Installation guide for mockery: https://github.com/vektra/mockery#installation

Steps to regenerate already existing interface mocks:
1. Run `make mock`.

Steps to generate mocks for new interface:
1. Run `mockery --filename=<file_name_where_interface_is_present> --output=<output_dir_to store_mocks> --dir=<directory_where_to_search_for_interface_file> --outpkg=<mock_package_name> --name <name_of_the_interface>` to generate mock for an interface.
2. Add the above command in appropriate target under `mock` rule in `Makefile`.

## Versions

The Astronomer platform is under very active development. Because of this we cannot make backwards compatibility guarantees between versions.

The astro-cli is following a semantic versioning scheme

`{MAJOR_RELEASE}.{MINOR_RELEASE}.{PATCH_RELEASE}`

with all releases up until 1.0.0 considered beta.

`astro-cli` tightly tracks the platform release versioning, this means that compatibility is only guaranteed between matching **minor** versions of the platform and the astro-cli. ie. astro-cli `v0.9.0` is guaranteed to be compatible with houston-api `v0.9.x` but not houston-api `v0.10.x`

## Debug

`astro-cli` has a debug flag that allows to see queries and different helpful logs that are done internally. You can enable this by passing `--verbosity=debug` to your commands or you can edit your `~/.astro/config.yaml` and add the following to it. This will turn on debug for all requests that are done until it is changed to `info` or removed from config.

```yaml
verbosity: debug
```

## Support

If you need support, start with the [Astronomer documentation](https://docs.astronomer.io/astro/).

If you still have questions, you can post on the [Astronomer web forum](https://forum.astronomer.io) or if you are a customer, you can [open a support ticket](https://support.astronomer.io).

## License

Apache 2.0 with Commons Clause
