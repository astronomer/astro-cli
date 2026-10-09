# Upgrading from Astro CLI 1.x

This page is for people who use Astro CLI 1.x today. It covers what v2 does differently, then what breaks. Old commands and flags fail with a message that names the v2 replacement, so an old script tells you what to change instead of failing silently.

## What's new

### A project is a `pyproject.toml`

A v2 project is a directory whose `pyproject.toml` has a `[tool.astro]` table. The Airflow version is the `apache-airflow` requirement in `[project] dependencies`. Python packages are ordinary dependencies, and OS packages, environment values, pools and linked Deployments are keys under `[tool.astro]`. Every key is in the [manifest reference](manifest-reference.md).

`astro init` creates a new project, or converts a 1.x project in place:
- `requirements.txt` becomes `[project] dependencies`, and `packages.txt` becomes `packages`;
- `airflow_settings.yaml` connections and variables go into the encrypted vault, and its pools go into `[tool.astro.pools]`;
- a saved deploy target becomes a linked Deployment.

A Dockerfile that only picks a base image is dropped, and any other Dockerfile is kept as the project's build. `init` ends by listing anything it could not carry. Step by step: [install.md](install.md).

### Local Airflow without Docker: `astro local`

`astro local start` runs Airflow straight from your project, with [uv](https://docs.astral.sh/uv/) building the environment, and no Docker needed. `--docker` gives you the containerized setup 1.x used. `astro local` covers the whole loop:
- `start`, `stop`, `restart` and `status`;
- `logs`, `shell` and `run`;
- `open` (the UI);
- `check`, which parses every DAG without starting Airflow;
- `reset`;
- `upgrade`, which moves the project to a new Airflow.

Each project gets its own address, such as `http://my-project.localhost:6563`, so several can run at once.

### Environment values and secrets

`astro local env` manages the values a local Airflow needs: environment variables, connections and Airflow variables. A project declares what it needs in `[tool.astro.env]`. Values live in an encrypted vault whose key is in your OS keyring, or in `.env` with `--plain`. A teammate's first `astro local start` names any value they're missing. Values can also come from a linked Astro workspace. See [secrets.md](secrets.md) and [workspace-link.md](workspace-link.md).

`astro env` manages Astro's Environment Manager: workspace- and Deployment-level variables, connections, Airflow variables and metrics exports. A single `set` command creates or updates.

### Talking to Airflow: `astro af`

`astro af` (alias `astro airflow`) works with a running Airflow: DAGs, runs, tasks, assets, connections, variables, pools, health, providers, plugins and config. `astro local af` targets the Airflow on your machine. `astro af` targets a linked Deployment, or any Airflow by `--url`. See [instances.md](instances.md).

### Linked Deployments: `astro link` and `astro use`

A project can link the Deployments it ships to, by name, in `pyproject.toml`. `astro link add` adds one, `astro link default` picks the default, and `astro use <name>` selects one for your own commands. Inside the project, `astro deployment inspect`, `logs`, `update`, `delete`, `hibernate` and `wake-up`, and `astro env`, take a link name wherever they take a Deployment id. See [workspace-link.md](workspace-link.md).

### Deploying and packaging

`astro deploy` builds from the manifest, and fails fast on a runtime the Deployment can't take, before the build rather than after it. `astro package` builds the same artifact without shipping it, for Astro, MWAA or Cloud Composer. A 1.x project with no `[tool.astro]` still deploys the 1.x way, unchanged. See [deploy.md](deploy.md).

### Scripting: `-o json` everywhere

Every command takes `-o json` (`--output json`), with a stable shape for scripts, agents and Astro Desktop. Under `-o json`, results go to stdout and progress and prompts go to stderr. Commands that ask for confirmation take `--yes` (`-y`).

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
| `astro deployment airflow-variable …` | `astro env airflow-variable … --deployment <id>` |
| `astro deployment connection …` | `astro env connection … --deployment <id>` |
| `astro deployment pool …` | the Airflow UI or API, until pools reach the Environment Manager |
| `astro env … create`, `astro env … update` | `astro env … set`, which creates or updates |

Run inside a 1.x project, an `astro dev` command first tells you to run `astro init`.

### Flags

| 1.x | v2 | where |
| --- | --- | --- |
| `--force`, `-f` | `--yes`, `-y` | the 1.x commands that took it and ask for confirmation now (deployment, bundle, token, team, worker-queue and context commands). `astro deploy --force` is unchanged |
| `--json` | `-o json` | `list` commands, `astro api … ls` / `describe` |
| `--template` | `-o json`, and jq | `list` commands |
| `-o table`, `-o template`, `-o yaml` | `-o text` (the default) or `-o json` | everywhere `-o` existed, except `astro deployment inspect`, which keeps `-o yaml` |
| `--format` | `-o json`, or `-o dotenv` on `astro env variable get` / `list`; there is no yaml | `astro env … get` / `list` |
| `--deployment-file` | the [Astro Terraform provider](https://registry.terraform.io/providers/astronomer/astro/latest) | `astro deployment create` / `update` |
| `--template` | `astro deployment create --clone`, or Terraform | `astro deployment inspect` |
| `--login-link`, `-l` | `astro login --login-link` | `astro organization switch` |
| `-o <file>` | `--output-file <file>` (`-o` is the output format now) | `astro organization audit-logs export` |
| `--api-url` | `--url` | `astro api airflow`, and its `ls` / `describe` |
| `--deployment-id` | `--deployment`, `-d` | `astro api airflow`, and its `ls` / `describe` |

Some flags were renamed but still work, hidden from help: `--deployment-name` (`-n`) and `--deployment-id` on most Deployment commands now `--deployment`, `--workspace-id` now `--workspace`, and `astro deploy --dags-path`.

### Behavior

- **`astro api airflow` needs a target.** 1.x defaulted to `localhost:8080`. v2 needs `-d <link or Deployment>` or `--url`. For the Airflow on your machine, use `astro local api`.
- **Local Airflow runs without Docker by default.** Add `--docker` to `astro local start` for containers. uv 0.9.25 or later must be on your PATH. Windows needs `--docker`.
- **`astro local` needs a converted project.** It reads `pyproject.toml`, not `requirements.txt`, `packages.txt` or `airflow_settings.yaml`, so run `astro init` once in a 1.x project. `astro deploy` still deploys an unconverted 1.x project.
- **Login tokens move into the OS keyring** (see above). If you go back to a 1.x build, it sees you as signed out and asks you to log in again.

## Running 1.x and v2 side by side

Until v2 is the stable release, it's published as GitHub pre-releases, and the default installer and Homebrew stay on 1.x. To try v2 without replacing 1.x, install it into its own directory:

```sh
curl -sSL https://raw.githubusercontent.com/astronomer/astro-cli/main/godownloader.sh | bash -s -- -b ~/astro-v2 v2
~/astro-v2/astro version
```

`curl -sSL install.astronomer.io | sudo bash -s -- v2` installs the newest v2 into `/usr/local/bin`, replacing 1.x. Running `curl -sSL install.astronomer.io | sudo bash -s` without `-- v2` puts 1.x back.
