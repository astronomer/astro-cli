# Astro CLI [![Release](https://img.shields.io/github/v/release/astronomer/astro-cli.svg?logo=github)](https://github.com/astronomer/astro-cli/releases) [![GoDoc](https://godoc.org/github.com/astronomer/astro-cli?status.svg)](https://godoc.org/github.com/astronomer/astro-cli) [![Go Report Card](https://goreportcard.com/badge/github.com/astronomer/astro-cli)](https://goreportcard.com/report/github.com/astronomer/astro-cli) [![codecov](https://codecov.io/gh/astronomer/astro-cli/branch/main/graph/badge.svg)](https://codecov.io/gh/astronomer/astro-cli)

The Astro CLI is a command-line interface for data orchestration. It allows you to get started with Apache Airflow quickly and it can be used with all Astronomer products.

## Usage
  
  astro [command]

### Core commands

  - `login`       Log in to the Astro CLI
  - `logout`      Log out of the Astro CLI
  - `dev init`    Initialize an Astro project in an empty local directory
  - `dev start`   Build your Astro project into a Docker image and spin up a local Docker container for each Airflow component
  - `dev stop`    Pause all Docker containers running your local Airflow environment
  - `dev restart` Stop your Airflow environment, rebuild your Astro project into a Docker image, and restart your Airflow environment with the new Docker image
  - `deploy`      Deploy code to a Deployment on Astro
  - `deployment`  Manage your Deployments running on Astronomer
  - `dev`         Run your Astro project locally
  - `help`        Help about any Astro CLI command
  - `version`     Show the running version of the Astro CLI
  - `workspace`   Manage Astronomer Workspaces

For a list of available Astro CLI commands, see the [Astro CLI command reference](https://docs.astronomer.io/astro/cli/reference).

## Install the Astro CLI

Use these instructions to install, upgrade, or uninstall the Astro CLI.

### Prerequisites

To install the Astro CLI on Mac, you'll need:

- [Homebrew](https://docs.docker.com/get-docker/)
- [Docker Desktop](https://docs.docker.com/get-docker/) (v18.09 or higher)

To install the Astro CLI on Windows, you'll need:

- Windows 10 or later
- [Docker Desktop for Windows](https://docs.docker.com/desktop/install/windows-install/)
- [Docker Engine 1.13.1 or later](https://docs.docker.com/engine/install/)
- [WSL](https://learn.microsoft.com/en-us/windows/wsl/install) enabled on your local machine

To install the Astro CLI on Windows with the Windows Package Manager winget command-line tool, you'll need:

- Windows 10 1709 (build 16299) or later or Windows 11
- Astro CLI version 1.6 or later
- The latest version of the Windows [App Installer](https://apps.microsoft.com/store/detail/app-installer/9NBLGGH4NNS1?hl=en-ca&gl=ca)
- [Docker Desktop for Windows](https://docs.docker.com/desktop/install/windows-install/)
- [Docker Engine 1.13.1 or later](https://docs.docker.com/engine/install/)
- [WSL](https://learn.microsoft.com/en-us/windows/wsl/install) enabled on your local machine 

To install the Astro CLI on Linux, you'll need:

- [Docker Engine 0.13.1 or later](https://docs.docker.com/engine/install/)

### Latest version

#### Mac

```sh
brew install astro
```
#### Windows

1. Go to the [Releases page](https://github.com/astronomer/astro-cli/releases)  of the Astro CLI GitHub repository, scroll to a CLI version, and then download the `.exe` file that matches the CPU architecture of your machine.

    For example, to install v1.0.0 of the Astro CLI on a Windows machine with an AMD 64 architecture, download astro_1.0.0-converged_windows_amd64.exe.

2. Rename the file `astro.exe`.

3. Add the filepath for the directory containing the new `astro.exe` as a PATH environment variable. For example, if `astro.exe` is stored in `C:\Users\username\astro.exe`, you add `C:\Users\username` as your PATH environment variable. To learn more about configuring the PATH environment variable, see [How do I set or change the PATH system variable?](https://www.java.com/en/download/help/path.html).

4. Restart your machine.

#### Windows with winget

Starting with Astro CLI version 1.6, you can use the Windows Package Manager winget command-line tool to install the Astro CLI.

Open Windows PowerShell as an administrator and then run the following command:

```sh
winget install -e --id Astronomer.Astro
```

#### Linux

```sh
curl -sSL install.astronomer.io | sudo bash -s
```

### Specific version

#### Mac

To install a specific version of the Astro CLI, specify the version you want to install at the end of the command:

```sh
brew install astro@<major.minor.patch-version>
```

#### Windows

1. Delete the existing `astro.exe` file on your machine.

2. Go to the [Releases page](https://github.com/astronomer/astro-cli/releases)  of the Astro CLI GitHub repository, scroll to a CLI version, and then download the `.exe` file that matches the CPU architecture of your machine.

    For example, to install v1.0.0 of the Astro CLI on a Windows machine with an AMD 64 architecture, download astro_1.0.0-converged_windows_amd64.exe.

3. Rename the file `astro.exe`.

4. Add the filepath for the directory containing the new `astro.exe` as a PATH environment variable. For example, if `astro.exe` is stored in `C:\Users\username\astro.exe`, you add `C:\Users\username` as your PATH environment variable. To learn more about configuring the PATH environment variable, see [How do I set or change the PATH system variable?](https://www.java.com/en/download/help/path.html).

5. Restart your machine.

#### Windows with winget

Starting with Astro CLI version 1.6, you can use the Windows Package Manager winget command-line tool to install a specific version of the Astro CLI.

To install a specific version of the Astro CLI, specify the version you want to install at the end of the command. For example, running the following command in Windows PowerShell as an administrator installs Astro CLI version 1.6:

```sh
winget install -e --id Astronomer.Astro -v 1.6.0
```

#### Linux

To install a specific version of the CLI, specify the version number as a flag at the end of the command. For example, to install v1.1.0 of the CLI, you would run:

```sh
curl -sSL install.astronomer.io | sudo bash -s -- v1.1.0
```

:::info

If you receive a `mkdir` error during installation, download and run the [godownloader](https://raw.githubusercontent.com/astronomer/astro-cli/main/godownloader.sh) script locally.

    $ cat godownloader.sh | bash -s -- -b /usr/local/bin

:::


#### Troubleshoot installation issues

If you encounter issues when installing the Astro CLI:

- Make sure Docker Desktop is installed correctly. See [Install Docker Desktop on Windows]https://docs.docker.com/desktop/install/windows-install/).

- Make sure the Docker Desktop WSL 2 backend is installed and configured correctly. See [Docker Desktop WSL 2 backend](https://docs.docker.com/desktop/windows/wsl/).

- Enable Hyper-V on the Docker and Linux Containers and run the Astro CLI natively. For additional information, see the [Docker docs troubleshooting guide](https://docs.docker.com/desktop/troubleshoot/overview/).

## Get started

1. Create a project

    ```
    $ mkdir hello-astro && cd hello-astro
    $ astro dev init
    ```

2. Install the binary and Confirm the install worked:

    ```
    $ ./astro
    ```

    This generates a basic project directory:

    ```
    .
    ├── dags
    │   ├── example_dag_advanced.py
    │   ├── example_dag_basic.py
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

3. Run `astro dev start` to start a local version of airflow on your machine. This will spin up a few locally running docker containers - one for the airflow scheduler, one for the webserver, and one for postgres.
(Run `astro dev ps` to verify).

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

## Versions

Astro CLI versions are released regularly and use semantic versioning. Backwards compatibility between versions cannot be guaranteed. Compatibility is only guaranteed between matching **minor** versions of the platform and the Astro CLI. For example, Astro CLI 0.9.0` is guaranteed to be compatible with houston-api v0.9.x, but not houston-api v0.10.x.

Astronomer ships major, minor, and patch releases of the Astro CLI in the following format:

`{MAJOR_RELEASE}.{MINOR_RELEASE}.{PATCH_RELEASE}`

All Astro CLI releases prior to 1.0.0 are considered beta.

## Debug

The Astro CLI includes a debug flag that allows you to view queries and internal logs. To enable it, you can pass `--verbosity=debug` in your commands, or you can add the following entry to your `~/.astro/config.yaml` file:

```yaml
verbosity: debug
```
Adding this entry to your `~/.astro/config.yaml` file turns on debugging for all requests until you change it to `info`, or you remove it from the file.

## Request a documentation change

If you notice something in our documentation that is wrong, misleading, or could use additional context, the easiest way to make an impact is to create a GitHub issue in this repository.

GitHub issues are triaged by the Astronomer documentation team and addressed promptly. After you've created a GitHub issue, our team may follow up with you with additional questions or comments. Once our team has addressed it, you'll get a notification from GitHub that the issue has been closed and that a change is now live.

1. Go to [Issues](https://github.com/astronomer/docs/issues).

2. Select **New Issue**.

3. Depending on the change you want implemented, select a GitHub issue template.

4. Tell us what you think we can do better by answering the questions in the template.

## Contribute

If you'd like to contribute to Astronomer documentation, you're welcome to create a Pull Request (PR) to this repository with your suggested changes.

1. Fork the repository.

2. Create a branch from `main`.

3. Make your changes in your branch.

4. Submit a PR for review.

    After you've submitted a PR for your changes, Netlify will add a comment to your PR that includes a link to a staging website with your changes.

    Small edits and typo fixes don't need to be linked to an issue and should be merged quickly. To get a timely review on a larger contribution, we recommend first creating a detailed GitHub issue describing the problem and linking that within your PR.

    Every update to the `main` branch of this repository triggers a rebuild of our production documentation page at https://www.docs.astronomer.io. It might take a few moments for your merged changes to appear.

## Support

To resolve an issue, Astronomer recommends reviewing the [Astronomer documentation](https://docs.astronomer.io/astro/cli/overview) first.

If you're unable to resolve your issue after reviewing the documentation, you can post a question on the [Astronomer web forum](https://forum.astronomer.io) or you can contact [Astronomer support](https://support.astronomer.io).

## License

Apache 2.0 with Commons Clause
