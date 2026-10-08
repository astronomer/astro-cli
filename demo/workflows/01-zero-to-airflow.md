# 1. Zero to Airflow in a minute

**The story.** Someone with no Astro account, no Docker, and no Airflow gets a
real Airflow running on their machine from an empty directory. Then they type
the command they have typed for five years and the CLI teaches them the new
one.

**Shows:** `astro init`, the uv-built project environment, standalone mode,
the `<name>.localhost` proxy, `astro local check`, the `astro dev` stub.

**Needs:** the `astro` v2 binary and Python. No Docker. No account. No network
after the first dependency download.

---

## Make a project

```sh
mkdir orders-demo && cd orders-demo
astro init
```

```
Created Astro project orders-demo (Airflow 3.3) in /Users/you/orders-demo
  dags/
  include/
  plugins/
  tests/
  .gitignore
  AGENTS.md
  dags/exampledag.py
  CLAUDE.md -> AGENTS.md
  pyproject.toml

Next: astro local start
```

`dags/exampledag.py` is a DAG to start from, written only when the project has
no DAGs of its own and only when the pin is Airflow 3 — the example imports
`airflow.sdk`, which Airflow 2 does not have.

Open `pyproject.toml`. Seven lines:

```toml
[project]
name = 'orders-demo'
version = '0.1.0'
requires-python = '>=3.12'
dependencies = ['apache-airflow==3.3.*']

[tool.astro]
```

That is the whole project definition. No Dockerfile, no `packages.txt`, no
`requirements.txt`, no `airflow_settings.yaml`. A normal Python project with
one extra table.

> **Talking point.** The Airflow version lives in one place: the
> `apache-airflow` requirement. It is what standalone installs, what Docker mode
> and deploy pick the runtime image from, and what uv, ty and your editor read.
> Change that one line to upgrade.

## Start Airflow

```sh
astro local start
```

First run builds the environment with uv, then starts Airflow:

```
airflow: starting
[uv] Using CPython 3.12.13
[uv] Creating virtual environment at: .venv
[uv] Resolved 132 packages in 692ms
[uv] Installed 124 packages in 207ms
 + apache-airflow==3.3.2
 …
airflow: running
project: /Users/you/orders-demo
state: running
mode: standalone
pid: 47788
url: http://orders-demo.localhost:6563
direct: http://localhost:10844
```

About twenty seconds cold, a couple of seconds warm. No Docker daemon was
involved: `mode: standalone` means Airflow is a process on the machine, in a
virtualenv the CLI built from the manifest.

Two URLs, both real. The `.localhost` one goes through the CLI's local proxy —
a stable name that survives restarts. The `direct:` one is the port Airflow
actually bound.

## Open it

```sh
astro local open
```

The Airflow 3 UI, on your machine, with no containers running.

## Run something

Add a DAG at `dags/hello.py`:

```python
from airflow.sdk import dag, task


@dag(schedule=None, catchup=False, tags=["demo"])
def hello():
    @task
    def greet() -> str:
        return "running locally, no Docker"

    greet()


hello()
```

Check it parses — in the project's real environment, with no Airflow running:

```sh
astro local check
```

```
checks passed: 2 DAGs, 0 errors, 0 warnings
```

Two, because `dags/exampledag.py` came with the project and `dags/hello.py` is
the one you just wrote.

It parses every file in `dags/` and reports import errors, duplicate DAG ids,
and slow parses. It needs the project environment to exist, so on a
never-started project it says so and points at `astro local start`; after that
it runs offline and starts nothing.

Run it in the project's own environment:

```sh
astro local run airflow dags test hello
```

The task log scrolls past and the run ends `Marking run … successful`.
`astro local run` puts any command inside the project's virtualenv. For tests
and lint, use `uv run pytest` and `uv run ruff check .`.

> **Careful.** Root-level `astro run DAG-ID` is still the **v1** command — it
> builds a container. The v2 way to run something in the project environment
> is `astro local run`.

## The teaching moment

Now type what you have always typed:

```sh
astro dev start
```

```
Error: `astro dev start` was removed in Astro CLI v2. Use `astro local start` instead.
Local Airflow now lives under `astro local`:

  astro local start        # was: astro dev start
  astro local logs         # was: astro dev logs
  astro init               # was: astro dev init
```

Exit code 1. Every `astro dev` subcommand does this, each naming the
replacement for the exact command typed. It is written for coding agents as
much as for people: an agent that runs `astro dev start` reads the fix in the
error and gets it right on the second try, with nothing else to look up.

## Stop

```sh
astro local stop
```

```
airflow: stopped
```

By default Airflow keeps running when the starting shell exits — close the
terminal and it is still there, and any other shell can `astro local status`,
`astro local logs`, or `astro local stop` it. Pass `--stop-with-session` to
`start` if you want it to die with the process that started it.
