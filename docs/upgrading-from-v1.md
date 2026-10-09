# Upgrading from Astro CLI 1.x

This page is for people who use Astro CLI 1.x today. It covers what v2 does differently, then what breaks. Old commands and flags fail with a message that names the v2 replacement, so an old script tells you what to change instead of failing silently.

## v2 requires converting your project

v2 works on projects with a `pyproject.toml`, and only on those. It does not run, start or deploy a project in the 1.x layout (a `Dockerfile` and `.astro/config.yaml`). You don't have to move: Astro CLI 1.x keeps working, and keeps deploying the projects it made. When you do move, convert each project once:

```sh
astro init
```

`astro init` converts a 1.x project in place (see [A project is a `pyproject.toml`](#a-project-is-a-pyprojecttoml) and [install.md](install.md#step-4-check-the-conversion-and-finish-it)). Until a project is converted, `astro deploy` in it fails with `this project uses the Astro CLI 1.x layout …, and Astro CLI v2 deploys only pyproject.toml projects. Convert it with astro init, or deploy it with Astro CLI 1.x`, and `astro dev` commands point at `astro init` too. `astro deploy --image-name` too: a prebuilt image deployed from a 1.x checkout would leave its DAGs stale. Outside any project, `--image-name` deploys an image you already built, alone, with no DAGs.

On Astro Private Cloud, v2 does not build and deploy projects yet, converted or not: use Astro CLI 1.x to deploy there for now. v2 deploys an image you built there (`astro deploy --image-name`, from a converted project or outside any project), and uploads a converted project's DAGs (`astro deploy --dags`). Support for `pyproject.toml` projects on Astro Private Cloud is coming.

## What's new

### A project is a `pyproject.toml`

A v2 project is a directory whose `pyproject.toml` has a `[tool.astro]` table. The Airflow version is the `apache-airflow` requirement in `[project] dependencies`. Python packages are ordinary dependencies. OS packages, pools and linked Deployments are keys under `[tool.astro]`. `[tool.astro.env]` declares, by name, the environment values the project needs, with an optional non-secret default. The values themselves come from the encrypted vault, `.env` or a linked workspace, and a secret never goes in `pyproject.toml` (see [Environment values and secrets](#environment-values-and-secrets)). Every key is in the [manifest reference](manifest-reference.md).

`astro init` creates a new project, or converts a 1.x project in place:
- `requirements.txt` becomes `[project] dependencies`, and `packages.txt` becomes `packages`;
- `airflow_settings.yaml` connections and variables go into the encrypted vault, and its pools go into `[tool.astro.pools]`;
- a deploy target saved in `.astro/config.yaml` becomes a linked Deployment when the file holds both an Astro Deployment id (`project.deployment`) and its workspace (`project.workspace`). `astro deploy --save` writes only the first, so usually `init` leaves a note naming the Deployment instead. Link it yourself with `astro link add <name> --deployment <id>`, or add it under `[tool.astro.deployments]`.

A Dockerfile that only picks a base image is dropped, and any other Dockerfile is kept as the project's build. `init` ends by listing anything it could not carry. Step by step: [install.md](install.md).

### Local Airflow without Docker: `astro local`

`astro local start` runs Airflow straight from your project, with [uv](https://docs.astral.sh/uv/) building the environment, and no Docker needed. `--docker` gives you the containerized setup 1.x used. `astro local` covers the whole loop:
- `start`, `stop`, `restart` and `status`;
- `logs`, `shell` and `run`;
- `open` (the UI);
- `check`, which parses every DAG without starting Airflow;
- `reset`;
- `upgrade`, which moves the project to a new Airflow.

On macOS and Linux each project gets its own address, such as `http://my-project.localhost:6563`, so several can run at once. The local proxy behind those addresses listens on 6563, or on another free port when 6563 is taken; `astro local start` prints the address it got. Windows has no proxy, so there Airflow is reached on its own `localhost` port.

### Environment values and secrets

`astro local env` manages the values a local Airflow needs: environment variables, connections and Airflow variables. A project declares what it needs in `[tool.astro.env]`. Values live in an encrypted vault whose key is in your OS keyring, or in `.env` with `--plain`. A teammate's first `astro local start` names any value they're missing. Values can also come from a linked Astro workspace. See [secrets.md](secrets.md) and [workspace-link.md](workspace-link.md).

`astro env` manages Astro's Environment Manager: workspace- and Deployment-level variables, connections, Airflow variables and metrics exports. A single `set` command creates or updates.

### Talking to Airflow: `astro af`

`astro af` (alias `astro airflow`) works with a running Airflow: DAGs, runs, tasks, assets, connections, variables, pools, health, providers, plugins and config. `astro local af` targets the Airflow on your machine. `astro af` targets a linked Deployment, or any Airflow by `--url`. See [instances.md](instances.md).

### Linked Deployments: `astro link` and `astro use`

A project can link the Deployments it ships to, by name, in `pyproject.toml`. `astro link add` adds one, `astro link default` picks the default, and `astro use <name>` selects one for your own commands. Inside the project, `astro deployment inspect`, `logs`, `update`, `delete`, `hibernate` and `wake-up`, and `astro env`, take a link name wherever they take a Deployment id. See [workspace-link.md](workspace-link.md).

### Deploying and packaging

`astro deploy` builds from the manifest, and fails fast on a runtime the Deployment can't take, before the build rather than after it. `astro package` builds the same artifact without shipping it, for Astro, MWAA or Cloud Composer. A 1.x project is not deployed until it is converted (see [v2 requires converting your project](#v2-requires-converting-your-project)). See [deploy.md](deploy.md).

### Scripting: `-o json`

Commands that print a result take `-o json` (`--output json`), with a stable shape for scripts, agents and Astro Desktop. Under `-o json`, results go to stdout and progress and prompts go to stderr. The exceptions:
- `astro deploy` on Astro takes only the long `--output json`;
- `astro login`, `astro logout` and `astro otto` are interactive or print nothing worth parsing, and have no `-o`;
- requests through `astro api airflow`, `cloud` and `registry` print the API's own response, shaped with `--jq` and `--template` (their `ls` and `describe` take `-o`);
- `astro completion` prints a shell script.

Commands that ask before deleting or replacing something take `--yes` (`-y`) to answer for you. `astro deploy` on Astro has no `--yes`: it asks only which Deployment to deploy to, which naming one (`--deployment`, or a link name as the argument) answers.

### Smaller changes

- **Astro login tokens** are kept in the OS keyring when one is available. `config.yaml` keeps only the non-secret half of a login. Without a keyring (CI, SSH, containers), logins stay in the config as before.
- **The upgrade notice** only suggests releases within the major version you're running.
- **`astro organization cluster`** manages clusters.
- **`astro config list`** shows the CLI's settings.

## Breaking changes

Everything below fails with an error naming the replacement, instead of an "unknown command" or "unknown flag". A removed flag gets that error on the commands that had it in 1.x; on a command 1.x did not have, such as `astro local reset --force`, it is an unknown flag.

### Commands

| 1.x | v2 |
| --- | --- |
| `astro dev init` | `astro init` |
| `astro dev start` / `stop` / `restart` | `astro local start` / `stop` / `restart` |
| `astro dev ps` | `astro local status` |
| `astro dev logs` | `astro local logs` |
| `astro dev bash` | `astro local shell` |
| `astro dev run` | `astro local run` |
| `astro dev parse` | `astro local check` |
| `astro dev pytest` | `uv run pytest` |
| `astro dev build` | `astro package` |
| `astro dev kill` | `astro local reset --yes` |
| `astro dev object export` | `astro local env list` |
| `astro dev object import` | `astro local env` |
| `astro dev upgrade-test`, `astro dev proxy` | no direct replacement |
| `astro run <dag>` | `astro local run airflow dags test <dag>` |
| `astro deployment airflow-variable list` / `delete` | `astro env airflow-variable list` / `delete … --deployment <id>` |
| `astro deployment airflow-variable create` / `update` | `astro env airflow-variable set <key> --deployment <id>`, which creates or updates |
| `astro deployment airflow-variable copy` | no per-Deployment copy: set it once in the workspace with `astro env airflow-variable set <key> --workspace <id> --auto-link`, or on each Deployment |
| `astro deployment connection list` / `delete` | `astro env connection list` / `delete … --deployment <id>` |
| `astro deployment connection create` / `update` | `astro env connection set <key> --deployment <id>`, which creates or updates |
| `astro deployment connection copy` | no per-Deployment copy: set it once in the workspace with `astro env connection set <key> --workspace <id> --auto-link`, or on each Deployment |
| `astro deployment pool …` | the Airflow UI or API, until pools reach the Environment Manager |
| `astro env … create`, `astro env … update` | `astro env … set`, which creates or updates |

Run inside a 1.x project, an `astro dev` command also tells you to convert it with `astro init`. One with a replacement says so in its first line (convert, then use the replacement); bare `astro dev`, `upgrade-test` and `proxy` say it after the list of `astro local` commands.

### Flags

| 1.x | v2 | where |
| --- | --- | --- |
| `--force`, `-f` | `--yes`, `-y` | the 1.x commands that took it and ask for confirmation now (deployment, bundle, token, team, worker-queue and context commands). `astro deploy` still accepts `--force`, hidden, and reads nothing from it |
| `--json` | `-o json` | `list` commands, `astro api … ls` / `describe` |
| `--template` | `-o json`, and jq | `list` commands |
| `-o table`, `-o template`, `-o yaml` | `-o text` (the default) or `-o json` | everywhere `-o` existed, except `astro deployment inspect`, which keeps `-o yaml`. These get the general "unknown output format" error, which lists the formats the command takes |
| `--format` | `-o json`, or `-o dotenv` on `astro env variable get` / `list`; there is no yaml | `astro env … get` / `list` |
| `--deployment-file` | the [Astro Terraform provider](https://registry.terraform.io/providers/astronomer/astro/latest) | `astro deployment create` / `update` |
| `--template` | `astro deployment create --clone`, or Terraform | `astro deployment inspect` |
| `--login-link`, `-l` | `astro login --login-link` | `astro organization switch` |
| `-o <file>` | `--output-file <file>` (`-o` is the output format now) | `astro organization audit-logs export` |
| `--api-url` | `--url` | `astro api airflow`, and its `ls` / `describe` |
| `--deployment-id` | `--deployment`, `-d` | `astro api airflow`, and its `ls` / `describe` |
| `--pytest`, `--test` (`-t`), `--env` (`-e`) | run your tests first: `uv run pytest && astro deploy` | `astro deploy` |
| `--parse` | check your DAGs first: `astro local check && astro deploy` | `astro deploy` |
| `--deployment-name`, `-n` | the argument, or `--deployment`, either taking a link name or a Deployment id | `astro deploy` |
| `--save`, `-s` | name the Deployment on each deploy; on Astro, link it with `astro link add` and mark it `default = true` to preselect it | `astro deploy` |
| `--dags-path` | `astro deploy --dags` from the project, which ships its `dags/` | `astro deploy` |
| `--dag-bundle-name` | no replacement yet: use Astro CLI 1.x | `astro deploy` |
| `--no-cache` | no replacement: v2 builds no image for Astro Private Cloud | `astro deploy` on Astro Private Cloud |
| `--prompt`, `-p` | nothing: `astro deploy` asks which Deployment unless you name one | `astro deploy` on Astro |

Some 1.x flags still work but are hidden from help in favor of a new spelling. On the `astro deployment` subcommands, `astro env` and `astro dbt deploy` / `delete`, `--deployment-name` (`-n`) and `--deployment-id`, whichever the command has, give way to `--deployment`. `--workspace-id` gives way to `--workspace` on those commands and the other commands that have both, `astro deploy` among them.

### Behavior

- **`project.deployment` is gone.** `astro deploy --save` wrote it into a 1.x project's `.astro/config.yaml`, and nothing in v2 reads it: `astro config set` and `get` of it say it was removed. Name the Deployment on each deploy, as the argument or with `--deployment`, or link it with `astro link add`.
- **An `astro api airflow` request needs a target.** 1.x defaulted to `localhost:8080`. v2 needs `-d <link or Deployment>` or `--url`. For the Airflow on your machine, use `astro local api`. `ls` and `describe` only read the API spec, so they run without a target.
- **Local Airflow runs without Docker by default.** Add `--docker` to `astro local start` for containers. uv 0.9.25 or later must be on your PATH. Windows needs `--docker`.
- **`astro local` needs a converted project.** It reads `pyproject.toml`, not `requirements.txt`, `packages.txt` or `airflow_settings.yaml`, so run `astro init` once in a 1.x project. `astro deploy` needs it converted too.
- **Login tokens move into the OS keyring** (see above). A 1.x build cannot read them there, so it sees you as signed out and asks you to log in again. Once you do, v2 sees that 1.x uses that login and keeps it in `config.yaml` from then on, so both tools share it and neither signs the other out. `astro login --vault` logs in and moves the login back out of `config.yaml`, after which 1.x asks you to log in again.

## Installing v2

Until v2 has a stable release, it's published as GitHub pre-releases, and the installer and Homebrew install 1.x by default. Pass `v2` to the installer for the newest v2 release, the newest stable one once there is one:

```sh
curl -sSL install.astronomer.io | sudo bash -s -- v2
```

That installs into `/usr/local/bin`, replacing a 1.x `astro` there. Running it again without `-- v2` puts 1.x back.

To keep 1.x where it is, install v2 into its own directory by running the installer script directly with `-b`:

```sh
curl -sSL https://raw.githubusercontent.com/astronomer/astro-cli/main/godownloader.sh | bash -s -- -b ~/astro-v2 v2
~/astro-v2/astro version
```

To build v2 from source instead, see [install.md](install.md#step-1-install-the-cli).
