# Contributing

The Astro CLI is a command-line interface for data orchestration. It allows you to get started with Apache Airflow quickly and it can be used with all Astronomer products.

## Local development

1. Install `Go` 1.19 or later. See [Download and install Go](https://go.dev/doc/install).

2. Clone and build:

    ```bash
    cd $GOPATH/src/github.com/astronomer/astro-cli
    git clone git@github.com:astronomer/astro-cli.git
    cd astro-cli
    make build
    ```

3. Run the following command to install `pre-commit` and run lint on every commit:

    `brew install pre-commit`

    `pre-commit install`

    Run lint locally:

    `pre-commit run --all-files`

4. Lint the `Go` code with the following command:

    ```bash
    make lint
    ```

## Test locally

To test Astro locally you'll need to update your global or local config to point to right platform type and local Astro endpoint. For example:

```yaml
local:
  platform: cloud
  astro: http://localhost:8871/v1
```

Similarly, to test software locally you'll need to update the platform type and local houston endpoint. For example:

```yaml
local:
  platform: software
  houston: http://localhost:8871/v1
```

## Run tests

Before you run tests, make sure you have running locally houston or Astro on http://localhost:8871/v1. This is a requirement for running some tests.

To run unit-tests run:

```bash
make test
```

## Generate mocks

Astronomer uses [mockery](https://github.com/vektra/mockery) to generate mocks for Golang interfaces.

To regenerate an existing interface mocks, run `make mock`.

To generate mocks for a new interface, add it to [.mockery.yml](.mockery.yml) and rerun `make mock`.
