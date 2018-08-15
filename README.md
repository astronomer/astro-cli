# Astronomer CLI

The Astronomer CLI is the recommended way to get started developing and deploying on Astronomer Enterprise Edition.

## Install

- via `curl`
    ```
    curl -sL https://install.astronomer.io | sudo bash
    ```

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
│   ├── example-dag.py
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

### Testing w/ houston-api
astro-cli communicates with [houston-api](https://github.com/astronomerio/houston-api) in order to manage Astronomer EE resources (users, workspaces, deployments). Follow the Development section on the houston-api README in order develop and test the integration between these two systems.

## Docs
Docs (/docs) are generated using the `github.com/spf13/cobra/doc` pkg. Currently this pkg is broken with go vendoring, the following instructions include a workaround

1. Remove the `/vendor/github.com/spf13/cobra` pkg, forcing Go to search your go path for the package instead
2. `go run gendocs/gendocs.go`
3. restore `/vendor/github.com/spf13/cobra`
