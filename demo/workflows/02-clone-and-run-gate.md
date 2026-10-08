# 2. The clone-and-run gate

**The story.** A new teammate clones the repo and runs one command. Instead of
a stack trace deep inside a task an hour later, they get a list of the three
values this project needs and the exact command to set each one. Nothing was
passed around in Slack.

**Shows:** the `[tool.astro.env]` schema, the missing-value gate on
`astro local start`, `astro local env <noun> set/list`, the workspace
Environment Manager read-through.

**Needs:** `demo/project/`. No account (the workspace-sourced value degrades
honestly without one).

---

## What the project declares

From `demo/project/pyproject.toml`:

```toml
[tool.astro.env]
LOG_LEVEL = 'info'                       # a string: a committed default
ORDERS_API_URL = { source = 'workspace' } # from Environment Manager when logged in
WAREHOUSE_URI = {}                       # required; you supply it

[tool.astro.env.connections]
warehouse = {}                           # required; resolves as AIRFLOW_CONN_WAREHOUSE

[tool.astro.env.airflow_variables]
batch_size = '500'                       # default; resolves as AIRFLOW_VAR_BATCH_SIZE
```

One rule covers all of it: **a string is a committed default, a table means
the value lives outside the manifest.** Every declared name is required by default — it
has to resolve from somewhere or the run is refused.

No secret is legal in this file, and no field that would hold one exists.

## Clone and run

```sh
git clone <this repo> && cd <repo>/demo/project
astro local start
```

```
Error: this project needs 3 environment value(s) that are not set on this machine:
  - connection warehouse
      provide it:  astro local env connection set warehouse --project
  - env var ORDERS_API_URL
      source "workspace": Environment Manager returned an error
      provide it:  astro local env variable set ORDERS_API_URL --project
  - env var WAREHOUSE_URI
      provide it:  astro local env variable set WAREHOUSE_URI --project
provide them, then run `astro local start` again.
```

Exit code 1. Three things worth pointing at:

- **Every problem at once**, not one per run.
- **The exact command to fix each one**, with `--project` already on it so
  copy-paste lands in the same file the resolver looked in.
- **`LOG_LEVEL` and `batch_size` are not on the list.** They have committed
  defaults, so nobody has to do anything about them.

`ORDERS_API_URL` says more than the others: it is declared
`{ source = 'workspace' }`, so the CLI tried the workspace's Environment
Manager, could not, and said so — then fell back to telling you how to set it
locally. Logged in, with a real workspace id in the manifest and the value
present in Environment Manager, it resolves on its own and never appears in
this list at all.

The wording of that middle line depends on why the lookup failed. Logged in
with the shipped placeholder `your-workspace-id` you get the API error
above — the id is not a valid workspace id. Logged out you get `your session
expired — log in again with 'astro login'`. Offline you get
`workspace (unavailable: unreachable)`. All of them end the same way: here is
how to set it yourself.

## Fill the gaps

```sh
astro local env variable set WAREHOUSE_URI --value 'sqlite:///include/warehouse.db'
astro local env connection set warehouse --value 'sqlite:///include/warehouse.db'
astro local env variable set ORDERS_API_URL --value 'https://catalog.example.com/products'
```

```
set variable WAREHOUSE_URI in project (/…/demo/project/.env)
set connection warehouse in project (/…/demo/project/.env)
set variable ORDERS_API_URL in project (/…/demo/project/.env)
```

> **In a real demo, drop `--value`.** A bare `astro local env variable set WAREHOUSE_URI`
> prompts with echo off, so the value never reaches shell history or `ps`.
> `--value` exists for scripts and is documented as history-leaking; the
> scripted form is used here so the walkthrough is copy-pasteable.

Three values, one file: `.env`, mode 0600, already gitignored by `astro init`.
The connection went in as `AIRFLOW_CONN_WAREHOUSE` and an Airflow Variable
would go in as `AIRFLOW_VAR_<KEY>` — one file, one format, one door.

## See where everything comes from

```sh
astro local env list
```

```
KIND  NAME            SOURCE   NOTE
conn  warehouse       project
env   LOG_LEVEL       default
env   ORDERS_API_URL  project
env   WAREHOUSE_URI   project
var   batch_size      default
```

The `SOURCE` column is the whole point: for each declared name, which layer
won. The chain, stated the same way everywhere:

```
project .env  >  shell env  >  project vault  >  global vault  >  workspace EM  >  manifest default
```

Values are never printed here. `astro local env variable get <NAME>` is the one
deliberate reveal.

## Start, for real

```sh
astro local start
astro local check
```

```
checks passed: 3 DAGs, 0 errors, 0 warnings
```

Then run the pipeline and watch the values land:

```sh
astro local run airflow dags test orders_ingest
```

```
generated 500 orders for 2026-07-30           # ← batch_size, the Variable default
loaded 500 orders into /…/include/warehouse.db (500 rows total)   # ← WAREHOUSE_URI
```

```sh
astro local run airflow dags test orders_report
```

```
apac      173 orders  $22,028.00
amer      162 orders  $20,758.38
emea      165 orders  $20,608.27
connection 'warehouse' resolved: type=sqlite host=None schema=include/warehouse.db
wrote /…/include/out/daily_report.md
```

Open `include/out/daily_report.md` for the finished thing.

## Why it is portable

The `.env` never leaves the machine and is never committed. What *is*
committed is the schema — the list of names the project needs. So the same
clone on a different laptop, or in CI, or in a container, is told exactly the
same three things, and CI satisfies them by exporting plain environment
variables, which top the chain.

A team that keeps secrets in a real manager wraps the command and changes
nothing else:

```sh
sops exec-env dev.enc.env -- astro local start
op run --env-file=.env.tpl -- astro local start
```

The shell environment beats both files, so whatever the tool exports wins and
never touches disk.
