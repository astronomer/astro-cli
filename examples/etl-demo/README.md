# etl-demo

> This is the **minimal** example: the smallest real project that shows what a
> manifest looks like, and the one [`docs/manifest-reference.md`](../../docs/manifest-reference.md)
> quotes from. For the full demo — an environment schema, all four kinds of
> deployment link, terraform for Astro, MWAA, and Composer, and seven scripted
> walkthroughs — see [`demo/`](../../demo).

A small, runnable Astro project (Airflow 3, CLI v2). It shows what an Astro project looks like: one `pyproject.toml` that is both the Astro manifest and a normal Python project, three DAGs in different shapes, and a parse test.

## What it shows

This is real `astro init` output with a few things added on top, so the layout is exactly what you get: `AGENTS.md` (with `CLAUDE.md` symlinked to it) for coding agents, `dags/` for your DAGs, `include/` for files DAGs load, `plugins/` for Airflow plugins, `tests/` for tests, `.gitignore`, and the `pyproject.toml` manifest. The additions are the three DAGs, the parse test, and the extra manifest sections described below.

The whole project is one file plus a `dags/` folder. `pyproject.toml` holds the standard `[project]` table (name, Python version, dependencies), the `apache-airflow` requirement that is the Airflow version it runs, the `[tool.astro]` section that makes it an Astro project (OS packages, the default workspace, and the Deployments it ships to), and the config for the tools you already use — `[tool.ruff]` for the linter and formatter, `[tool.ty]` for Astral's type checker, and a `dev` dependency group with ruff, ty, and pytest. The point: your Airflow project is a normal Python project, and your linter, type checker, and test runner live in the same file as the Airflow pin. For every key you can set, see the [manifest reference](../../docs/manifest-reference.md).

The `dags/` folder has three DAGs, each a different common shape: `etl_taskflow.py` is a TaskFlow extract/transform/load where values pass between tasks as return values, `branch_example.py` uses `@task.branch` to pick which downstream task runs, and `dynamic_mapping.py` fans one task out over a list with `.expand()`. `tests/test_dags.py` is the Airflow 3 form of the classic dagbag parse test: it loads every DAG and fails if any of them has an import error.

The deployment ids and workspace id in `pyproject.toml` are placeholders (`your-workspace-id` and the like) — replace them with your real ids before you deploy. The manifest still parses with the placeholders in place, so `astro local check` works with no edits.

## Run it

You need the Astro CLI installed, and an Astro login for the deploy step (local commands work offline with no account).

Start Airflow for this project — the first run builds a uv-managed virtualenv from the dependencies and opens the Airflow UI at a `.localhost` address: `astro local start`

Validate the DAGs without starting Airflow — parses every file in `dags/` and reports import errors, duplicate dag ids, and slow parses: `astro local check`

Deploy the whole project — image and DAGs — to the deployment marked `default = true` (here, `dev`): `astro deploy` (add `--dags` to push only the DAGs)

Build the deployable image without shipping it — for CI, so one job builds and another deploys: `astro package`. It builds the same image `astro deploy` would, tags it `astro-package/etl-demo:<runtime>-<hash>`, and leaves it in your local Docker store; the output prints the exact `astro deploy dev --image-name <tag>` line to hand it off. Add `--save image.tar` to write a tarball a CI job can upload as an artifact and load elsewhere. `astro package` needs Docker; `astro package [target]` takes `astro` (the default), `mwaa`, `composer`, or `oss`. The first three build; `oss` is a registered name that errors until its stage lands. The `mwaa` and `composer` targets need no Docker — they write a bucket-shaped directory, not an image.

You can also run the test suite with `uv run pytest`, and the linter and type checker with `uv run ruff check .` and `uv run ty check`.

## Two things worth knowing

`astro local start` runs Airflow one of two ways: standalone (the default — Airflow runs directly on your machine in a uv-managed virtualenv, no Docker) or Docker (`--docker` — Airflow runs in containers built from Astronomer's runtime images). In standalone mode the CLI prints a warning that it cannot install the apt packages in `[tool.astro] packages`, because standalone mode has no image to bake them into. That is expected; run `astro local start --docker` to get them, or install them on your machine yourself.

`[tool.ty.rules]` turns off ty's `invalid-argument-type` rule. Airflow's `@task` decorator makes a decorated call return an `XComArg` (a placeholder resolved at run time), not the function's own return type, so a static checker flags every TaskFlow chain like `load(transform(extract()))`. Turning off that one rule keeps ty quiet on idiomatic TaskFlow code, at the cost of also silencing genuine wrong-argument mistakes in your own code; ty still checks everything else.
