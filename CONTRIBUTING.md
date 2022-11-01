
## Local development

1. Install `Go` 1.18 or later. See [Download and install Go](https://go.dev/doc/install).

2. Run the following command to install `golangci-lint` and run linter locally:

    ```brew install golangci-lint```

    ```golangci-lint run .```

3. Run the following command to install `pre-commit` and run lint on every commit:

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

### Run tests

Before you run tests, make sure you have running locally houston or Astro on http://localhost:8871/v1. This is a requirement for running some tests.

To run unit-tests run:

```bash
make test
```

### Generate mocks

Astronomer uses [mockery](https://github.com/vektra/mockery) to generate mocks for Golang interfaces. See the [mockery installation guide](https://github.com/vektra/mockery#installation).

To regenerate an existing interface mocks, run `make mock`.

To generate mocks for a new interface, add the following command below `mock` rule in `Makefile`:

`mockery --filename=<file_name_where_interface_is_present> --output=<output_dir_to store_mocks> --dir=<directory_where_to_search_for_interface_file> --outpkg=<mock_package_name> --name <name_of_the_interface>`
