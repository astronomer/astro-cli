# 6. Agent-ready

**The story.** Every v2 command speaks JSON from its first release, the project
scaffolds notes for coding agents, and the errors are written to teach. A
coding agent working in an Astro project does not need a wrapper, a plugin, or
a doc site — the CLI is the interface.

**Shows:** `--output json` across the v2 tree (one object per list, NDJSON for streams), the scaffolded
`AGENTS.md`, the structured `astro dev` stub, value-free `astro local env list`.

**Needs:** the `astro` v2 binary. `jq` for the examples below.

---

## JSON everywhere, not as a retrofit

```sh
astro init myproject --output json
```

```json
{"dir":"/Users/you/myproject","name":"myproject","airflow":"3.3","created":["dags/","include/","plugins/","tests/","pyproject.toml",".gitignore","AGENTS.md","CLAUDE.md -> AGENTS.md"]}
```

```sh
astro local status --output json
```

```json
{"project_path":"/tmp/orders-demo","mode":"standalone","state":"running","pid":55897,"port":10129,"hostname":"orders-demo.localhost","started_at":"2026-07-30T20:53:14.709977Z"}
```

```sh
astro local check --output json
```

```json
{"event":"summary","dags":3,"errors":0,"warnings":0,"strict":false,"passed":true}
```

```sh
astro package composer --output json | tail -1 | jq .
```

```json
{
  "target": "composer",
  "kind": "tree",
  "tree_path": "/…/dist/composer",
  "deps_file": "/…/dist/composer/composer-requirements.txt",
  "warnings": ["Composer installs no OS packages from this artifact; …"],
  "next_steps": [
    "gcloud composer environments storage dags import --environment=<env> …",
    "gcloud composer environments update <env> --location=<region> …"
  ]
}
```

Human output is the default rendering of the same data, not a second code
path — so anything a person can read, a program can parse.

## Lists are one object; streams are NDJSON

A list is one JSON object with its rows under a named key — `[]` when there
are none, never `null`:

```sh
astro local list --output json
```

```json
{"projects":[{"project":"/tmp/orders-demo","hostname":"orders-demo.localhost","mode":"standalone","state":"running","port":14751,"url":"http://localhost:14751","started_at":"2026-07-30T20:34:22Z","uptime":"32s"},{"project":"/tmp/orders-wt","hostname":"orders-wt.orders-demo.localhost","mode":"standalone","state":"running","port":19371,"url":"http://localhost:19371","started_at":"2026-07-30T20:53:03Z","uptime":"11s"}]}
```

`astro af` lists use the standalone `af` CLI's keys and envelope, so what was
written against `af` reads them unchanged:

```sh
astro af runs list --output json | jq '.dag_runs[] | select(.state == "failed")'
astro af dags errors --output json | jq '.total_import_errors'
```

Streaming surfaces — logs, events, build and deploy progress — are NDJSON: one
JSON object per line, so a reader consumes them as they arrive rather than
waiting for the end:

```sh
astro local logs --follow --output json | jq -r 'select(.component=="scheduler") | .text'
```

```json
{"event":"log","component":"system","time":"2026-07-30T16:55:05-04:00","text":"supervise: watching parent=0, child=55901"}
```

Build and deploy progress streams the same way — `{"event":"log",…}` and
`{"event":"state",…}` lines, then one final object with the result.

## Errors are structured too

This is the part people do not expect. The `astro dev` stub is not a string —
it is data:

```sh
astro dev start --output json
```

```json
{
  "error": "`astro dev start` was removed in Astro CLI v2",
  "typed_command": "astro dev start",
  "replacement": "astro local start",
  "mapping": [
    {"command": "start", "replacement": "astro local start"},
    {"command": "ps", "replacement": "astro local status"},
    {"command": "parse", "replacement": "astro local check"},
    {"command": "pytest", "replacement": "uv run pytest"},
    {"command": "object", "replacement": "astro local env"}
  ]
}
```

An agent that runs `astro dev start` out of training-data habit reads
`replacement` and gets it right on the next call — and gets the whole mapping
for free, so it also stops reaching for `astro dev ps` and `astro dev bash`.
The error is the documentation.

The missing-value gate works the same way:

```sh
astro local start --output json
```

```json
{
  "error": "required environment values are not set on this machine",
  "project": "/…/demo/project",
  "missing": [
    {"Section": "connection", "Name": "warehouse", "EnvKey": "AIRFLOW_CONN_WAREHOUSE",
     "set_command": "astro local env connection set warehouse --project"},
    {"Section": "env_var", "Name": "WAREHOUSE_URI", "EnvKey": "WAREHOUSE_URI",
     "set_command": "astro local env variable set WAREHOUSE_URI --project"}
  ]
}
```

Every entry carries the command that fixes it. An agent can resolve its own
blocker without being told how.

## AGENTS.md, scaffolded

`astro init` writes an `AGENTS.md` and symlinks `CLAUDE.md` to it, so the
project explains itself to whichever agent shows up. The file is yours once it
is written, and `init` never rewrites it, so it carries only what stays true as
the CLI changes: the manifest, the layout, and where to look for commands:

```markdown
Use the installed CLI as the reference: `astro --help`, `astro local --help`
(local Airflow, including `astro local af` for this project's Airflow API).
`astro dev` was removed in v2; running an `astro dev` command prints its
replacement.
```

The commands themselves come from the CLI that is installed, which is always
current: its `--help`, and the `astro dev` stub above, which names the exact
replacement for whatever was typed.

## Safe by construction

`astro local env list` is built from the schema and the resolver's source
metadata, never from decoded values. It is *structurally* incapable of
printing a secret:

```sh
astro local env list --output json
```

```json
{"entries":[{"kind":"conn","name":"warehouse","required":true,"secret":true,"source":"project"},{"kind":"env","name":"WAREHOUSE_URI","required":true,"secret":false,"source":"project"}]}
```

Names, kinds, and where each resolved from. `astro local env variable get <NAME>` is
the one deliberate reveal, so an agent can enumerate what a project needs and
where it comes from without ever touching a value.

## What this is building toward

`--output json` on every command is what makes a VSCode extension a thin
wrapper rather than a reimplementation. A push-based `astro rpc --stdio` mode
over the same types would be a third renderer, not a third code path.
