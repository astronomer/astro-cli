# The Astro CLI v2 demo

One Astro project, seven walkthroughs, and terraform for the three platforms it
ships to. The commands were run against a v2 binary built from this repository
and the output blocks are what came back; the few things that need an account
or a cloud bill were not run, and [say so below](#what-is-honest-about-this-demo).

**What v2 is.** The Astro CLI's next major version. `astro dev` is gone,
replaced by `astro local` — a local-Airflow command family built on a
`pyproject.toml` manifest. No Dockerfile, no `packages.txt`, no
`requirements.txt`, no `airflow_settings.yaml`: one file that is both a normal
Python project and the whole Astro project definition. Local Airflow runs
without Docker and without an account, the project declares the environment it
needs so a clone tells you what is missing, and the same project builds the
artifact Astro, MWAA, or Cloud Composer each consumes.

Design of record: [`docs/architecture.md`](../docs/architecture.md),
[`docs/manifest-reference.md`](../docs/manifest-reference.md),
[`docs/deploy.md`](../docs/deploy.md),
[`docs/instances.md`](../docs/instances.md),
[`docs/secrets.md`](../docs/secrets.md),
[`docs/workspace-link.md`](../docs/workspace-link.md).

## The three acts

**Act 1 — it just runs.** An empty directory becomes a running Airflow in
about twenty seconds, with no Docker, no account, and no network after the
first dependency download. Then a clone of a real project tells the person who
cloned it exactly which three values it needs and how to set each one.
Workflows [1](workflows/01-zero-to-airflow.md) and
[2](workflows/02-clone-and-run-gate.md).

**Act 2 — it scales sideways.** Three Airflows at once on one laptop — two
projects and a git worktree — each on its own stable hostname, none of them
fighting over port 8080, all of them listed by one command. Workflow
[3](workflows/03-parallel-everything.md).

**Act 3 — it points anywhere, and ships anywhere.** One project names every
Airflow it talks to — your laptop, two Astro Deployments, an MWAA environment,
a Composer environment, a bare URL — holds not one credential, and has one
rule for which of them a command acts on. Workflow
[4](workflows/04-point-at-any-airflow.md). Then the CLI checks your DAGs
against the Airflow each platform *actually runs* before you upload, and builds
each one the artifact it eats: an image for Astro, an S3 tree for MWAA, a GCS
tree for Composer. Workflow [5](workflows/05-three-clouds.md). And under all of
it, every command speaks JSON and the errors are written to teach — workflow
[6](workflows/06-agent-ready.md).

## Layout

```
demo/
  project/          the demo Airflow project — the star of the show
  terraform/        astro/ · aws-mwaa/ · gcp-composer/, one stack each
  workflows/        seven walkthroughs, exact commands and real output
```

## Setup

You need the v2 `astro` binary. Build it from this repository:

```sh
go build -o /tmp/astro .    # from the repo root
export PATH="/tmp:$PATH"
astro version
```

For local Airflow you also need Python 3.10+ and
[uv](https://docs.astral.sh/uv/) on your PATH. Docker is needed only for the
image paths (`astro package`, `astro package astro`, `astro deploy`, and
`astro local start --docker`); everything else runs without it.

Then:

```sh
cd demo/project
astro local env variable set WAREHOUSE_URI       # prompts, no echo
astro local env connection set warehouse
astro local env variable set ORDERS_API_URL
astro local start
```

Suggested values for a self-contained demo — the warehouse is a real SQLite
file the DAGs write to:

```
WAREHOUSE_URI     sqlite:///include/warehouse.db
warehouse         sqlite:///include/warehouse.db
ORDERS_API_URL    https://catalog.example.com/products
```

Skip the three `env set` commands the first time through if you want to see
the clone-and-run gate fire — that is workflow 2, and it is the better opening.

For the cloud stacks, read [`terraform/README.md`](terraform/README.md) first.
MWAA takes 30-60 minutes to build and Composer 20-40, both cost money for as
long as they exist, and neither scales to zero. Start them well before a demo
and destroy them after.

## The project

[`project/`](project/) is a small orders pipeline, and every part of it is
there to show something.

`pyproject.toml` exercises the whole manifest surface: the Airflow pin,
dependencies, OS packages, an environment schema with a committed default, a
workspace-sourced value, and two values required with no default; and five
deployment links covering all four kinds — two Astro, one MWAA, one Composer,
and one endpoint link with env-referenced token auth. Below `[tool.astro]` sit
ruff, ty, and pytest config, because an Astro project is a normal Python
project and its tools live in the same file.

Three DAGs, each reading declared values and each producing something you can
point at:

- **`orders_ingest`** generates a deterministic day of orders sized by the
  `batch_size` Airflow Variable and loads them into the warehouse named by
  `WAREHOUSE_URI`. Emits an asset.
- **`orders_report`** is scheduled *on* that asset. On a deployment,
  triggering the ingest runs the report too. Local Airflow runs no schedules,
  so trigger the report yourself. It resolves the `warehouse` connection, aggregates
  revenue by region into the task log, and writes
  `include/out/daily_report.md`.
- **`orders_api_sync`** branches on `ORDERS_API_URL` — the workspace-sourced
  value — and falls back to a bundled sample so the demo works offline.

Everything runs on SQLite, so there is no database to stand up and nothing to
install. `AGENTS.md` is exactly what `astro init` scaffolds, `CLAUDE.md` is
symlinked to it, and `tests/test_dags.py` is the classic parse test.

## Feature highlights

| Feature | Where it shows |
| --- | --- |
| `astro init` — greenfield scaffold, AGENTS.md, CLAUDE.md symlink | [1](workflows/01-zero-to-airflow.md) |
| `astro init` — adopt an existing repo, and list what it could not carry | *not walked through; run `astro init` in a repo with a `pyproject.toml`, a `requirements.txt`, or a Dockerfile* |
| The pyproject manifest: load, validate, comment-preserving edit | [2](workflows/02-clone-and-run-gate.md), [5](workflows/05-three-clouds.md) |
| Standalone mode — real Airflow, no Docker, uv-built venv | [1](workflows/01-zero-to-airflow.md) |
| Docker mode (`--docker`) | *`astro local start --docker`; needed for OS packages* |
| `<name>.localhost` proxy and parallel environments | [3](workflows/03-parallel-everything.md) |
| `astro local list` — machine-wide, with `--clean` sweep | [3](workflows/03-parallel-everything.md) |
| Env schema: default / required / `source = 'workspace'` | [2](workflows/02-clone-and-run-gate.md) |
| The clone-and-run gate on `astro local start` | [2](workflows/02-clone-and-run-gate.md) |
| `astro local env <noun> set/get/list/delete`, dotenv-first | [2](workflows/02-clone-and-run-gate.md) |
| Environment Manager read-through for workspace values | [2](workflows/02-clone-and-run-gate.md) |
| `astro local check` — parse, import errors, duplicate ids | [1](workflows/01-zero-to-airflow.md), [2](workflows/02-clone-and-run-gate.md) |
| `astro local check --target mwaa/composer` — pre-flight | [5](workflows/05-three-clouds.md) |
| `[tool.astro] packages` — OS packages from the manifest | [5](workflows/05-three-clouds.md) |
| `astro package` — astro, mwaa, composer targets | [5](workflows/05-three-clouds.md) |
| `astro deploy` from a project, image and dags-only | [5](workflows/05-three-clouds.md) |
| Deployment links, `default = true`, inherited workspace/target | [5](workflows/05-three-clouds.md) |
| MWAA, Composer, and endpoint links with the auth axis | [4](workflows/04-point-at-any-airflow.md), [5](workflows/05-three-clouds.md) |
| `--output json` on every v2 command: one object per list, NDJSON for streams | [6](workflows/06-agent-ready.md) |
| The `astro dev` stub, in text and in JSON | [1](workflows/01-zero-to-airflow.md), [6](workflows/06-agent-ready.md) |
| `astro deploy` refuses a non-Astro link by name and kind | [5](workflows/05-three-clouds.md) |
| Deployment resolution, `astro use` and its inventory, the cloud auth resolvers | [4](workflows/04-point-at-any-airflow.md) |
| `astro af dags/runs/tasks/…` (including `runs trigger-wait`, `runs diagnose`, `dags errors`) against a deployment, and `astro local …` against this machine | [4](workflows/04-point-at-any-airflow.md) |
| Debugging a failed run: which task broke, and the traceback that broke it | [4](workflows/04-point-at-any-airflow.md) |
| **Not yet supported:** `astro deploy` shipping to an MWAA or Composer link | [4](workflows/04-point-at-any-airflow.md), [5](workflows/05-three-clouds.md) |

## What is honest about this demo

**Not run when this was written.** Every command block in workflows 1, 2, 3,
and 6 is real output from a real run, and so is every block in workflow 4 but
two. Four things were not run, because they need an account or a cloud bill:

- `astro af dags list -d prod-mwaa` and `-d prod-composer`, the two blocks in
  workflow 4 marked as spec-accurate where they sit. Everything else on that
  page was captured against two live Airflows — the demo project's own, and a
  second local one reached as an endpoint link over HTTP, which is the same
  path a remote Airflow takes.
- `astro deploy` against a real Astro Deployment. The prompt, the refusals, and
  the run up to the placeholder deployment id were all captured; the successful
  ship is not shown.
- `terraform apply` on any of the three stacks. All three pass
  `terraform fmt -check`, `init`, and `validate`, and the MWAA and Composer
  ones are adapted from stacks that did build live environments.
- Docker-mode local Airflow (`astro local start --docker`).

**One gap**, called out where you would hit it: **`astro deploy` only ships to
Astro links.** `astro package mwaa` and `astro package composer` build the
right artifacts and print the exact upload command, but you run that command.
`astro deploy prod-mwaa` refuses by name and kind — `link "prod-mwaa" is mwaa,
and astro deploy ships to Astro Deployments, astro links: dev, prod`.
Deploying to MWAA and Composer is not yet supported.
