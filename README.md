# Astro CLI [![Release](https://img.shields.io/github/v/release/astronomer/astro-cli.svg?logo=github)](https://github.com/astronomer/astro-cli/releases) [![GoDoc](https://godoc.org/github.com/astronomer/astro-cli?status.svg)](https://godoc.org/github.com/astronomer/astro-cli) [![Go Report Card](https://goreportcard.com/badge/github.com/astronomer/astro-cli)](https://goreportcard.com/report/github.com/astronomer/astro-cli) [![codecov](https://codecov.io/gh/astronomer/astro-cli/branch/main/graph/badge.svg)](https://codecov.io/gh/astronomer/astro-cli)

The Astro CLI is a command-line interface for data orchestration. It allows you to get started with Apache Airflow quickly and it can be used with all Astronomer products.
<img referrerpolicy="no-referrer-when-downgrade" src="https://static.scarf.sh/a.png?x-pxid=29deaa05-2c91-4f0b-bb2c-d2e9408867e0" />
## Usage

  astro [command]

### Core commands

  - `login`         Log in to the Astro CLI
  - `logout`        Log out of the Astro CLI
  - `dev init`      Initialize an Astro project in an empty local directory
  - `dev start`     Build your Astro project into a Docker image and spin up a local Docker container for each Airflow component
  - `dev stop`      Pause all Docker containers running your local Airflow environment
  - `dev restart`   Stop your Airflow environment, rebuild your Astro project into a Docker image, and restart your Airflow environment with the new Docker image
  - `deploy`        Deploy code to a Deployment on Astro
  - `deployment`    Manage your Deployments running on Astronomer
  - `dev`           Run your Astro project locally
  - `help`          Help about any Astro CLI command
  - `version`       Show the running version of the Astro CLI
  - `workspace`     Manage Astronomer Workspaces

For a list of available Astro CLI commands, see the [Astro CLI command reference](https://www.astronomer.io/docs/astro/cli/reference).

## Install the Astro CLI

Use these instructions to install, upgrade, or uninstall the Astro CLI.

### Prerequisites

To install the Astro CLI on Mac, you'll need:

- [Homebrew](https://brew.sh/)
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

> If you receive a `mkdir` error during installation, download and run the [godownloader](https://raw.githubusercontent.com/astronomer/astro-cli/main/godownloader.sh) script locally using:
>
>```sh
>$ cat godownloader.sh | bash -s -- -b /usr/local/bin
>```

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
    │   ├── exampledag.py
    ├── tests
    │   ├── dags
    │       ├── test_dag_example.py
    ├── Dockerfile
    ├── include
    ├── packages.txt
    ├── plugins
    └── requirements.txt
    ```

    DAGs can go in the `dags` folder, custom Airflow plugins in `plugins`, python packages needed can go in `requirements.txt`, and OS level packages can go in `packages.txt`.

3. Run `astro dev start` to start a local version of airflow on your machine. This will spin up a few locally running docker containers - one for the airflow scheduler, one for the webserver, and one for postgres.
(Run `astro dev ps` to verify).

## Versions

Astro CLI versions are released regularly and use semantic versioning. Backwards compatibility between versions cannot be guaranteed. Compatibility is only guaranteed between matching **minor** versions of the platform and the Astro CLI. For example, Astro CLI 0.9.0` is guaranteed to be compatible with houston-api v0.9.x, but not houston-api v0.10.x.

Astronomer ships major, minor, and patch releases of the Astro CLI in the following format:

`{MAJOR_RELEASE}.{MINOR_RELEASE}.{PATCH_RELEASE}`

All Astro CLI releases prior to 1.0.0 are considered beta.

## Change Log

Change log between each version can be found on the [releases](https://github.com/astronomer/astro-cli/releases) page

## Debug

The Astro CLI includes a debug flag that allows you to view queries and internal logs. To enable it, you can pass `--verbosity=debug` in your commands, or you can add the following entry to your `~/.astro/config.yaml` file:

```yaml
verbosity: debug
```
Adding this entry to your `~/.astro/config.yaml` file turns on debugging for all requests until you change it to `info`, or you remove it from the file.

## Support

To resolve an issue, Astronomer recommends reviewing the [Astronomer documentation](https://www.astronomer.io/docs/astro/cli/overview) first.

If you're unable to resolve your issue after reviewing the documentation, you can post a question on the [Astronomer web forum](https://forum.astronomer.io) or you can contact [Astronomer support](https://support.astronomer.io).

## License

Apache 2.0 with Commons Clause
