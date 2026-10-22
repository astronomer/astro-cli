# Upgrading from Astro CLI 1.x

This page is for people who use Astro CLI 1.x today. It covers what v2 does differently, then what breaks. Old commands and flags fail with a message that names the v2 replacement, so an old script tells you what to change instead of failing silently.

## v2 requires converting your project

v2 works on projects with a `pyproject.toml`, and only on those. It does not run, start or deploy a project in the 1.x layout (a `Dockerfile` and `.astro/config.yaml`). You don't have to move: Astro CLI 1.x keeps working, and keeps deploying the projects it made. When you do move, convert each project once:

```sh
astro init
```

`astro init` converts a 1.x project in place (see [A project is a `pyproject.toml`](#a-project-is-a-pyprojecttoml) and [install.md](install.md#step-4-check-the-conversion-and-finish-it)). Until a project is converted, `astro deploy` in it fails with `this project uses the Astro CLI 1.x layout …, and Astro CLI v2 deploys only pyproject.toml projects. Convert it with astro init, or deploy it with Astro CLI 1.x`, and `astro dev` commands point at `astro init` too. `astro deploy --image-name` too: a prebuilt image deployed from a 1.x checkout would leave its DAGs stale. Outside any project, `--image-name` deploys an image you already built, alone, with no DAGs.

A conversion removes the 1.x files whose contents it carried: `requirements.txt` and `packages.txt` (unless a Dockerfile `init` keeps still installs them), `airflow_settings.yaml` once everything in it is carried, and a Dockerfile that only picks a base image. After that, Astro CLI 1.x no longer builds the project, so convert on a branch and move everyone who works on it to v2 together.

`init` stores the connection and variable values in `airflow_settings.yaml` in the encrypted vault, whose key is in your OS keyring. On a machine with no reachable keyring (headless Linux, most CI), converting a project whose `airflow_settings.yaml` holds values fails, saying the keyring is unreachable, before it writes anything. Run `astro init` where a keyring is available, such as your own machine.

On Astro Private Cloud, v2 builds and deploys `pyproject.toml` projects, Airflow 2 and 3, to the `apc` links a project names, as it does on Astro (see [deploy.md](deploy.md#astro-private-cloud)). A deploy names its Deployment, as the argument or with `--deployment`, or asks: a 1.x CI job that ran a bare `astro deploy` against the Deployment `--save` remembered fails until it names one. A 1.x project on Astro Private Cloud converts with `astro init` exactly as one on Astro does, and then deploys from its `pyproject.toml`; a Dockerfile on a mirror of Astro Runtime is kept as the project's build, so an air-gapped project keeps building from its mirror. v2 also deploys an image you built (`astro deploy --image-name`, from a converted project or outside any project), and points a Deployment at an image already in a registry (`--image-name <ref> --remote --runtime-version <version>`), as 1.x did. Both resolve the Deployment as a project's deploy does: a link name works wherever an id did, and a run that names none asks, or under `-o json` or with no terminal fails `input_required`, where 1.x showed a list.

## What's new

### A project is a `pyproject.toml`

A v2 project is a directory whose `pyproject.toml` has a `[tool.astro]` table. The Airflow version is the `apache-airflow` requirement in `[project] dependencies`. Python packages are ordinary dependencies. OS packages, pools and linked Deployments are keys under `[tool.astro]`. `[tool.astro.env]` declares, by name, the environment values the project needs, with an optional non-secret default. The values themselves come from the encrypted vault, `.env` or a linked workspace, and a secret never goes in `pyproject.toml` (see [Environment values and secrets](#environment-values-and-secrets)). Every key is in the [manifest reference](manifest-reference.md).

`astro init` creates a new project, or converts a 1.x project in place:
- `requirements.txt` becomes `[project] dependencies`, and `packages.txt` becomes `packages`;
- `airflow_settings.yaml` connections and variables go into the encrypted vault, and its pools go into `[tool.astro.pools]`;
- a deploy target saved in `.astro/config.yaml` becomes a linked Deployment when the file holds both a Deployment id (`project.deployment`) and its workspace (`project.workspace`): an Astro link, or under an Astro Private Cloud context an `apc` link on that context's domain. `astro deploy --save` writes only the first. Under an Astro Private Cloud context `init` takes the workspace from the context's current one, where 1.x looked the saved id up, and links it (if that is the wrong workspace, `astro deploy` says the Deployment is not in it, and `astro link add default --deployment <id> --workspace <workspace-id> --replace` fixes the link); on Astro, and with no current workspace, `init` leaves a note saying the Deployment was not linked, with the `astro link add` and `astro link default` commands that link it.

A Dockerfile that only picks a base image is dropped (an Airflow 2 build such as `astro-runtime:12.6.0` is kept as `[tool.astro] runtime`), and any other Dockerfile is kept as the project's build. `init` ends by listing anything it could not carry. Step by step: [install.md](install.md).

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

`astro deploy` on Astro Private Cloud spells its result's `type` as Astro's deploy does: `image-and-dag`, `image-only` and `dag-only`, where earlier v2 builds wrote `image_and_dags`, `image` and `dags`. A script reading it matches the new values. It also publishes `link` (the link the deploy resolved, when a link named it, `--image-name` and `--remote` included) and `dags_in_image` (an image Deployment, which runs the DAGs inside the image), and `runtime_version` for every image deploy. An `--image-name` deploy from a project to an image Deployment is now `image-and-dag` with `dags_in_image`, as a project's build is, where earlier v2 builds wrote `image-only`.

Commands that ask before deleting or replacing something take `--yes` (`-y`) to answer for you. `astro deploy` has no `--yes`, on Astro or on Astro Private Cloud: it asks only which Deployment to deploy to, which naming one (`--deployment`, or a link name as the argument) answers. Earlier v2 builds took `--yes` on Astro Private Cloud's `astro deploy --image-name`, for a runtime the platform did not recommend and a dags folder with no DAGs; the runtime is now held to the platform's rules (a downgrade refused, any other runtime the platform does not list deployed with a warning), and an empty dags folder is a warning.

The shapes 1.x printed changed once, for v2. The cloud `list` commands and `astro env` publish snake_case keys (`deployment_id`, not `deploymentId`; where `astro deployment inspect` publishes the same field, a list uses its name, so `isDagDeployEnabled` is `dag_deploy_enabled`). Every list is an object holding the array under a named key, such as `{"deployments": [...]}` or `{"variables": [...]}`, never a bare array. A result is pretty-printed on a terminal and compact on one line when piped. Under `-o json` a failure is one object on stdout, `{"error": ..., "code": ..., "kind": ...}`, and a script branches on `kind` rather than the message. A usage error (an unknown command, flag or setting, or an output format the command does not take) exits 2, where 1.x exited 1. See [architecture.md](architecture.md#failures) for the kinds and exit codes.

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
| `astro dbt deploy [DEPLOYMENT_ID]` | `astro deploy bundle [PATH] --deployment <id, name or link name>`: PATH replaces `--project-path`, `--deployment` the argument and `--deployment-name`, `--workspace` `--workspace-id`; `-m`, `-w` and `-t` are unchanged. Under `-o json`, `project` and `project_path` are now `bundle_name` and `bundle_path`, beside `bundle_type` |
| `astro dbt delete [DEPLOYMENT_ID]` | `astro deploy bundle --delete [PATH] --deployment <id, name or link name>`, with the same flags |

Run inside a 1.x project, an `astro dev` command also tells you to convert it with `astro init`. One with a replacement says so in its first line (convert, then use the replacement); bare `astro dev`, `upgrade-test` and `proxy` say it after the list of `astro local` commands.

### Flags

| 1.x | v2 | where |
| --- | --- | --- |
| `--force`, `-f` | `--yes`, `-y` | the 1.x commands that took it and ask for confirmation now (deployment, bundle, token, team, worker-queue and context commands). `astro deploy`, on Astro and on Astro Private Cloud, still accepts `--force` and `--prompt`, hidden, and reads nothing from them: it asks which Deployment whenever none is named, so to choose from the list, name none |
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
| `--save`, `-s` | name the Deployment on each deploy; link it with `astro link add` (an `apc` link under an Astro Private Cloud context) and mark it `default = true` to preselect it | `astro deploy` |
| `--dags-path` | `astro deploy --dags` from the project, which ships its `dags/` | `astro deploy` |
| `--dag-bundle-name` | no replacement yet: use Astro CLI 1.x | `astro deploy` |
| `--no-cache` | no replacement: the build uses the container engine's cache; clear it with `docker builder prune` to build from nothing | `astro deploy` on Astro Private Cloud |
| `--non-dags` | `astro deploy bundle [PATH]`, which detects a dbt project and gives it a mount path | `astro deploy` |
| `--non-dags-local-path` | the PATH argument of `astro deploy bundle` | `astro deploy` |
| `--non-dags-mount-path` | `astro deploy bundle --mount-path` | `astro deploy` |
| `--non-dags-bundle-type` | `astro deploy bundle --type` | `astro deploy` |
| `--build-secrets` | `--build-secret`, repeated once for each secret | `astro deploy`, `astro remote deploy` |
| `--enforce-cicd` | `--cicd-enforcement enable`, or `disable` | `astro deployment create` / `update` |
| `--cluster-type` | `--type` | `astro deployment create` |
| `--force-upgrade-to-af3` | drop it: 1.x ignored it too | `astro deploy` |
| `--hard` | drop it: a delete always removes everything, as it did in 1.x | `astro deployment delete` on Astro Private Cloud |
| `--runtime-version`, `-v` | `--airflow-version`, or the `apache-airflow` requirement in `pyproject.toml`; to pin one Astro Runtime build, `runtime` under `[tool.astro]` | `astro init` |
| `--from-template`, `-t` | no replacement: `astro init` writes one starter project | `astro init` |
| `--force`, `-f` | drop it: `astro init` asks nothing | `astro init` |
| `--remote-execution-enabled` | add a `Dockerfile.client` and use `astro remote deploy`; `astro init` writes no client image files | `astro init` |
| `--remote-image-repository` | `astro config set deploy.remote_execution.client_registry <registry>/<repository>` | `astro init` |
| `--use-astronomer-certified` | no replacement: there is no Astronomer Certified image; pin Airflow with `--airflow-version` | `astro init` on Astro Private Cloud |
| `--docker`, `--standalone` | choose the mode when you start: `astro local start` (standalone, the default) or `astro local start --docker` | `astro init` |
| `-n`, `-a` | `--name`, `--airflow-version` | `astro init` |
| `--no-browser`, `-n` | drop it: a start opens no browser; `astro local open` does | `astro local start`, `astro start` |
| `-p` | `--port` | `astro local start`, `astro start` |
| `--standalone` | drop it: standalone is the default mode | `astro local start`, `astro start` |
| `--wait` | the `ASTRO_LOCAL_HEALTH_TIMEOUT` environment variable, a Go duration: `ASTRO_LOCAL_HEALTH_TIMEOUT=10m astro local start` | `astro local start` / `restart`, `astro start` |
| `--foreground`, `-f` | drop it: a start returns once Airflow is up and leaves it running; `astro local logs -f` follows it | `astro local start` / `restart`, `astro start` |
| `--compose-file` | a `docker-compose.override.yml` at the project root, which Docker mode merges over the compose file it writes | `astro local start` / `restart`, `astro start` |
| `--settings-file`, `-s` | the connections, variables and pools `pyproject.toml` declares (`[tool.astro.env]`, `[tool.astro.pools]`), which `astro init` carries from `airflow_settings.yaml` | `astro local start` / `restart`, `astro start` |
| `--env`, `-e` | the `.env` at the project root, which `astro local env variable set <NAME> --plain` writes | `astro local start` / `restart` / `check`, `astro start` |
| `--workspace-id`, `-w`; `--deployment-id`, `-d` | the workspace `pyproject.toml` names (`workspace` under `[tool.astro]`), whose Environment Manager a start reads; no Deployment's values are read | `astro local start` / `restart`, `astro start` |
| `--image-name`, `-i` | the project's own build, from `pyproject.toml` or the Dockerfile `dockerfile` under `[tool.astro]` names; `astro package --tag` names the image it builds | `astro local start` / `restart` / `check`, `astro start`, `astro package` |
| `--no-cache` | no replacement: standalone mode builds no image, and Docker mode's build uses the container engine's cache; clear it with `docker builder prune` to build from nothing | `astro local start` / `restart`, `astro start` |
| `--no-cache` | no replacement: the build uses the container engine's cache; clear it with `docker builder prune` to build from nothing | `astro package` |
| `--no-proxy` | the direct `localhost` URL `astro local status` shows, on the port `astro local start --port` picks | `astro local start` / `restart`, `astro start` |
| `--port` (`-p`), `--docker`, `--standalone` | `astro local stop`, then `astro local start` with them: a restart keeps the port and mode Airflow is running with | `astro local restart` |
| `--kill`, `-k` | `astro local reset --yes`, then `astro local start` | `astro local restart` |
| `--docker`, `--standalone` | drop them: these work in either mode | `astro local stop` / `status` / `logs` / `shell` / `reset`, `astro stop` / `logs` |
| `--docker`, `--standalone`, `--image-name`, `--build-secret`, `--build-secrets` | drop them: `astro local check` builds no image | `astro local check` |
| `--docker`, `--standalone` | drop them: the image is the same in either mode | `astro package` |
| `--scheduler` (`-s`), `--webserver` (`-w`), `--api-server` (`-a`), `--triggerer` (`-t`), `--dag-processor` (`-d`) | `--component <name>` | `astro local logs`, `astro logs` |
| `--scheduler` (`-s`), `--webserver` (`-w`), `--api-server` (`-a`), `--triggerer` (`-t`), `--dag-processor` (`-d`), `--postgres` (`-p`) | no replacement: the shell opens in the scheduler's container in Docker mode, and in the project's environment in standalone mode | `astro local shell` |

The `astro dev` flags are refused this way on the command that replaced each `astro dev` command (`astro init`, `astro local start` and the rest, `astro package`, and the `astro start`, `stop` and `logs` shorthands), which is where a 1.x script that only renames the command passes them. `astro dev run`, `pytest` and `object` have no such row: their replacements take none of their flags, and the `astro dev` stub names what replaces each.

Some 1.x flags still work but are hidden from help in favor of a new spelling. On the `astro deployment` subcommands and `astro env`, `--deployment-name` (`-n`) and `--deployment-id`, whichever the command has, give way to `--deployment`. `--workspace-id` gives way to `--workspace` on those commands and the other commands that have both, `astro deploy` among them. The `astro deployment team`, `user` and `token` commands other than `token workspace-token add` and `remove`, and on Astro Private Cloud every `astro deployment` command 1.x had other than `create`, `list` and `adopt`, act on a Deployment by its id alone: 1.x read nothing from the `--workspace-id` it gave them, and they still accept it, hidden, and read nothing from it. A hidden spelling given as an empty value still means the default, as in 1.x, so `--workspace-id "$WS"` with `WS` unset keeps working. The new spellings refuse an empty value as a usage error, and so does `-d` on `astro api airflow`, which is `--deployment` there now. `astro api airflow` still accepts 1.x's `--workspace-id` (`-W`), hidden, and reads nothing from it, as 1.x read nothing from it: a name given to `-d` is looked up in the project's workspace, else the context's.

On Astro Private Cloud the same spellings give way. `--deployment` and `--workspace` have no shorthand there: a shorthand 1.x gave the old spelling stays with it, hidden, and means exactly what it did, so `-d <id>` is still an id and `-w ""` still means the context's workspace. The one 1.x shorthand that changes meaning is `-n` on `astro deployment create` and `update`, which is `--name` now (see [Behavior](#behavior)).

| 1.x, hidden and still working | v2 | where, on Astro Private Cloud |
| --- | --- | --- |
| `--workspace-id` | `--workspace` | `astro deploy`, `astro deployment create` / `list` / `adopt`, the `astro workspace team` commands |
| `--workspace-id`, `-w` | `--workspace` | the `astro workspace user` and `astro workspace service-account` commands |
| `--deployment-id`, `-d` | `--deployment` | `astro deployment unadopt`, `airflow upgrade`, `runtime upgrade` / `migrate`, the `astro deployment service-account` commands |
| `--deployment-id` | `--deployment` | the `astro deployment user` and `astro deployment team` commands |
| `--label`, `-l` | `--name`, `-n` | `astro workspace create` / `update` |
| `--label`, `-l` | `--name`, `-n` | `astro deployment create` / `update` / `adopt` (where 1.x's `-n` on create and update was `--nfs-location`, and 1.x's `--name` on adopt was the custom resource's: see below) |
| `--search`, `-s` | `--keyword`, `-k` | 1.x's `astro deployment logs scheduler` / `webserver` / `workers`, which still run, hidden (`astro deployment logs` takes `--keyword` alone) |

`--deployment` there takes an `apc` link name of the project around the working directory, as `astro deploy` does (on the context's domain), or a Deployment id; `--deployment-id` and `-d` are an id, whatever it looks like, as in 1.x. `astro deployment update`, `delete`, `hibernate`, `wake-up` and `logs` take `--deployment` too, beside the Deployment-id argument, which is unchanged; given both, they must name the same Deployment. A `delete` named by a link says which Deployment the link names before it asks. `--role` has `-r` on `astro deployment user add` / `update`, `astro deployment team add` / `update`, `astro workspace team add` / `update` and `astro workspace user update`; `astro deployment adopt`'s `--description` has `-d`. On Astro, `astro deploy`'s `--output` has `-o`, as Astro Private Cloud's had.

Some Astro Private Cloud commands also take what Astro's take. `astro workspace user add`, `astro deployment user add`, `astro workspace team add` and `astro deployment team add` take the email or the team id as the argument, as Astro's do, as well as `--email` / `--team-id`; given both, they must agree. `astro deployment create` and `update` take Astro's executor spellings, `CeleryExecutor`, `KubernetesExecutor` and `LocalExecutor`, as well as `celery`, `kubernetes` and `local`, in any case; `AstroExecutor` is refused, as Astro Private Cloud has no equivalent.

`astro deployment logs` on Astro Private Cloud is shaped as Astro's: `astro deployment logs <DEPLOYMENT_ID>` shows every component's logs, merged oldest first, unless `--webserver`, `--scheduler`, `--workers`, `--triggerer` (where the platform runs one) or `--component <name>` pick some. In text, the logs of more than one component each end with their component, as Astro's end with their source; `-o json` names it on every record, as before. `--follow` streams one component's logs, as Houston does, so it needs one picked. 1.x's `astro deployment logs <component> <DEPLOYMENT_ID>` still runs as it did, with `--search`, hidden from help. `--since` and `--follow` are Astro Private Cloud's own; Astro's `--log-count` and level flags stay Astro's.

### Behavior

- **Astro Private Cloud's `astro deployment team` and `user add` / `remove` / `update` need their Deployment, and `team add` its team.** 1.x meant `--deployment-id` to be required there, and `--team-id` on `astro deployment team add` and `astro workspace team add`, but the marks never took (one was set before the flag existed, the others on a flag the command inherits rather than owns), so a run without one asked Houston about the Deployment or team `""`. Now a run naming none is a usage error, `required flag(s) "deployment" not set` or `required flag(s) "team-id" not set`. The other Deployment commands, where the mark always worked, now name `deployment` in that error where 1.x named `deployment-id`.
- **`project.deployment` is gone.** `astro deploy --save` wrote it into a 1.x project's `.astro/config.yaml`, and nothing in v2 reads it: `astro config set` and `get` of it say it was removed. Name the Deployment on each deploy, as the argument or with `--deployment`, or link it with `astro link add`.
- **A deploy with no terminal never picks a Deployment.** 1.x deployed to a workspace's only Deployment without asking when `ASTRO_API_TOKEN`, `ASTRONOMER_KEY_ID` or `ASTRONOMER_KEY_SECRET` was set, or `auto_select` was on. In v2 a bare `astro deploy` with no terminal, or under `--output json`, fails `input_required`, even with one Deployment, and even with a `default = true` link, which only preselects the prompt; `astro use` and `ASTRO_DEPLOYMENT` do not apply to deploy either. A CI job names the Deployment, as the argument or with `--deployment`.
- **`astro deploy` runs no checks of its own first.** 1.x parsed the DAGs in the built image before an image deploy, unless `--force`, `skip_parse` or `ASTRONOMER_SKIP_PARSE` said not to, and refused a project with uncommitted changes without `--force`. v2 does neither: run `astro local check` and `uv run pytest` before `astro deploy`. A project with uncommitted changes deploys, and the deploy records no git commit.
- **Airflow 2 projects deploy to Astro only from a declared Dockerfile.** A generated build starts from an Airflow 3 Astro Runtime, so on Astro, `astro deploy` and `astro package astro` refuse an Airflow 2 pin (`only Airflow 3 is supported in this release`). Airflow 2 still runs locally and deploys to Astro Private Cloud. A 1.x project whose Dockerfile did more than pick a base keeps it as its declared build, which deploys to Astro whatever Airflow it runs; otherwise deploy an Airflow 2 project with Astro CLI 1.x for now. This is tracked in [#2379](https://github.com/astronomer/astro-cli/issues/2379).
- **Other 1.x-only settings are refused too.** `astro config set` and `get` refuse the settings only 1.x reads (`dev.mode`, `webserver.port`, `postgres.*`, `skip_parse` and the rest), naming what replaces each, and `astro config list` leaves them out. 1.x and v2 share `~/.astro/config.yaml`, so v2 leaves those settings where they are for 1.x and only ever writes the ones you set. Once, the first time it runs, v2 removes from that file the defaults older versions of either CLI wrote into it without being asked (every setting still at the value it shipped with, such as a `postgres.tag` of `12.6`), as Astro CLI 1.46 does, so each setting follows its default again. To run 1.x next to v2, use 1.x v1.46.0 or later: earlier versions write their defaults back into the shared config every time they save, and this cleanup runs only once.
- **Some settings have new names.** Each is named for what it affects. `astro config get` and `set` take only the new name: the 1.x name is refused, naming the new one, as a removed flag is. A value already saved under the 1.x name still applies, and while 1.x and v2 share `~/.astro/config.yaml` they keep each other's changes: v2 reads the 1.x name first, and when the file holds it, a `set` writes both names.

  | 1.x | v2 |
  | --- | --- |
  | `auto_select` | `deployment.auto_select` |
  | `cosmos_boost.pre_deploy` | `deploy.cosmos_boost` |
  | `remote.client_registry` | `deploy.remote_execution.client_registry` |
  | `remote.base_image_registry` | `deploy.remote_execution.base_image_registry` |
  | `sha_as_tag` | `apc.sha_as_tag` |
  | `page_size` | `apc.page_size` |
  | `interactive` | `apc.interactive` |
  | `houston.dial_timeout` | `apc.dial_timeout` |
  | `houston.skip_verify_tls` | `apc.skip_verify_tls` |

  The overrides for Astronomer's own development (`cloud.api.protocol`, `cloud.api.port`, `cloud.api.ws_protocol`, `local.core`, `local.houston`, `local.platform`) moved under `internal.`, under the product they apply to when only one reads them (`internal.astro.*`, `internal.apc.*`), and are left out of `astro config list`. `cloud.api.token` was removed: log in with `astro login`, or set `ASTRO_API_TOKEN`.
- **An `astro api airflow` request needs a target.** 1.x defaulted to `localhost:8080`. v2 needs `-d <link or Deployment>` or `--url`. For the Airflow on your machine, use `astro local api`. `ls` and `describe` only read the API spec, so they run without a target.
- **Local Airflow runs without Docker by default.** Add `--docker` to `astro local start` for containers. uv 0.9.25 or later must be on your PATH. Standalone mode runs on Linux and Apple-silicon macOS. Windows needs `--docker`, and so does an Intel Mac, unless the project adds Intel macOS to `[tool.uv] environments` (see the [manifest reference](manifest-reference.md#tooluv)).
- **Local schedules are off.** `astro local start` creates DAGs unpaused and sets `AIRFLOW__SCHEDULER__USE_JOB_SCHEDULE=False`, so a DAG runs only when you trigger it. 1.x kept Airflow's defaults: DAGs paused at creation, and an unpaused DAG running on its schedule. Set `AIRFLOW__SCHEDULER__USE_JOB_SCHEDULE=True` in `.env` to run DAGs on their schedules (see [architecture.md](architecture.md#dev-mode-airflow-defaults)).
- **`astro local start` does not open a browser.** 1.x opened the Airflow UI once it was healthy, unless `--no-browser`. v2 prints the address, and `astro local open` opens it. The start returns with Airflow still running, as 1.x's did; `--stop-with-session` stops it when the command exits instead.
- **Docker mode starts on a new metadata database.** A converted project's first `astro local start --docker` creates its own database rather than reusing the one `astro dev` made, so past DAG runs, and the connections, variables and users made in the UI, stay in the 1.x volume. The first start names that volume, and Astro CLI 1.x can still start it.
- **Docker mode waits on container healthchecks.** A start is healthy when compose's healthchecks pass. Rootless Podman on Linux without a systemd user session never runs them, so a start there waits out its timeout and gives up (see [install.md](install.md#before-you-start)).
- **`astro workspace delete` asks to confirm.** 1.x deleted without asking, including an organization's only workspace. A script passes `--yes`: under `-o json` the delete is refused without it, and with no terminal it reads its answer from stdin, so a script that gives none deletes nothing.
- **`astro organization team delete` asks to confirm every team.** 1.x asked only about an IdP-managed team. A script passes `--yes`, as above.
- **Astro Private Cloud's `astro workspace delete`, `astro workspace service-account delete` and `astro deployment service-account delete` ask to confirm.** 1.x deleted straight away. They take `--yes` (`-y`), as Astro's deletes do, and a script that runs them non-interactively needs it: under `-o json` the delete is refused without it, and with no terminal a script that answers nothing on stdin deletes nothing.
- **On Astro Private Cloud, `-n` on `astro deployment create` and `update` is `--name`, as on Astro.** In 1.x it was `--nfs-location`, the NFS volume a Deployment mounts its DAGs from, which keeps its long spelling and has no shorthand now. A 1.x line that passes the mount as `-n` is refused rather than taken as the Deployment's name, with `-l` beside it or without: a name shaped as an NFS location, `<host>:/<path>` (an IPv6 host in brackets), is a usage error naming `--nfs-location`. Pass the mount as `--nfs-location <host>:/<path>`.
- **On Astro Private Cloud, `astro deployment adopt` names the custom resource with `--cr-name`, and `--name` is the adopted Deployment's name, as on `create`.** In 1.x, `--name` was the custom resource's `metadata.name` and `--label` the Deployment's. `--label` (`-l`) still names the Deployment, as `--name`'s hidden alias. A 1.x adopt line names no custom resource now, so it is refused, saying to name it with `--cr-name`, rather than adopting under the wrong name: change its `--name` to `--cr-name`.
- **`astro deployment bundle delete` says the deletion was requested.** 1.x printed `Deleted bundle <id> from deployment <id>`. v2 prints `Requested deletion of bundle <id> from deployment <id>`, because the API removes a bundle in the background, and `bundle list` shows it with `deletion_is_pending` until it is gone. A script matching the old line should match the new one, or use `-o json`, whose `action` is `deletion_requested`.
- **`astro local` needs a converted project.** It reads `pyproject.toml`, not `requirements.txt`, `packages.txt` or `airflow_settings.yaml`, so run `astro init` once in a 1.x project. `astro deploy` needs it converted too.
- **`astro deploy --image-name` on Astro Private Cloud follows a project's deploy.** In 1.x it asked to confirm an image whose runtime the platform did not recommend, and an empty dags folder; it now refuses a downgrade, which Houston refuses too, and deploys with a warning a runtime the platform does not list, an Airflow 3 one where it lists no Airflow 3 build and one newer than the newest it lists among them (a build from a project is refused those two), and an empty dags folder is a warning. With `--remote`, the same rules hold `--runtime-version`, and the deploy's `--description` goes with the image. `--prompt` is accepted and does nothing: the deploy asks whenever no Deployment is named, so to choose from the list, name none. A link name works wherever an id did. The image is pushed with the context's token carried by the push, so it no longer logs in to the registry in your Docker config first. A platform that has not turned on Houston's `updateDeploymentImage` (which `--remote` and a BYO registry use) is told the setting that does.
- **Login tokens move into the OS keyring** (see above). A 1.x build cannot read them there, so it sees you as signed out and asks you to log in again. Once you do, v2 sees that 1.x uses that login and keeps it in `config.yaml` from then on, so both tools share it and neither signs the other out. `astro login --vault` logs in and moves the login back out of `config.yaml`, after which 1.x asks you to log in again.

## Installing v2

Until v2 has a stable release, it's published as GitHub pre-releases, and the installer and Homebrew install 1.x by default. Pass `v2` to the installer for the newest v2 release, the newest stable one once there is one. To replace 1.x, install with `sudo`:

```sh
curl -sSL install.astronomer.io | sudo bash -s -- v2
```

That installs into `/usr/local/bin`, where the installer put 1.x, and replaces it there. Running it again without `-- v2` puts 1.x back. Without `sudo` the installer writes to `~/.local/bin` instead and leaves the 1.x `astro` in `/usr/local/bin`, which usually comes first on your PATH, so `astro` would still run 1.x. The installer warns when that happens.

To keep 1.x and run v2 beside it, install v2 without `sudo` into a directory of its own with `ASTRO_INSTALL_DIR`:

```sh
curl -sSL install.astronomer.io | ASTRO_INSTALL_DIR="$HOME/astro-v2" bash -s -- v2
~/astro-v2/astro version
```

To build v2 from source instead, see [install.md](install.md#step-1-install-the-cli).
