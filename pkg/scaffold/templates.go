package scaffold

import _ "embed"

// exampleDag is the DAG a new project starts with.
//
// Embedded rather than a string constant, unlike the other templates here,
// because it is Python: a file is syntax-highlighted, diffable and lintable
// where a Go string literal is none of those.
//
// It imports only airflow.sdk and the standard library, and that is a
// requirement rather than a coincidence. Under 1.x a project's dependencies came
// from a fat runtime image, so the example could `import requests` and call an
// API. A project installs exactly what [project.dependencies] names, which
// this scaffold writes as apache-airflow alone — so an example carrying a
// third-party import would fail to load on the first `astro local start`, which
// is a worse first run than no example at all.
//
// It is Airflow 3 only, because airflow.sdk is. The 1.x templates were a
// per-major pair, pkg/airflowrt/include/airflow2/exampledag.py beside
// .../airflow3; nothing wrote them once 1.x's init was gone and they were deleted
// with it, so an Airflow 2 variant, if one is ever wanted, is modeled from that
// file in git history. starterDagSuits is what keeps this file away from a
// project that pins 2 in the meantime.
//
//go:embed include/exampledag.py
var exampleDag string

const gitignoreTemplate = `# Derived environment (rebuilt by the astro CLI; never commit it)
.venv/
__pycache__/

# Local-only files
.env
.DS_Store

# Per-machine files Astro tools write into the project: local Airflow
# state, credentials and tokens. .astro/config.yaml is shared; keep committing it.
.astro/standalone/
.astro/worktrees/
.astro/*.local.yaml
.astro/*.local.yml
.astro/otto/*.local.json
.astro/otto/mcp.json
# Airflow 2 on macOS: the standalone engine regenerates this plugin.
plugins/fix_local_executor_pickle.py
`

// agentsContent is AGENTS.md. The project owns the file once init writes it:
// init never rewrites it, so it holds only facts about the project that stay
// true as the CLI changes, and sends the reader to the installed CLI's help for
// commands. It references the manifest instead of restating it, so it cannot
// drift from pyproject.toml either.
const agentsContent = `# Agent notes

This is an Astro project: Apache Airflow Dags, run with the Astro CLI (v2).
` + "`pyproject.toml`" + ` is the manifest: the project name, the pinned Airflow
version (the ` + "`apache-airflow`" + ` requirement in ` + "`[project] dependencies`" + `),
and the Python dependencies live there. Read it rather than assuming versions.
To change the Airflow version, change that requirement: an optional
` + "`[tool.astro] runtime`" + ` only picks one Astro Runtime build of the same series
for the image, and a ` + "`FROM`" + ` in a declared Dockerfile has to name the same series.

Layout:

- ` + "`dags/`" + `: Airflow Dag definitions
- ` + "`include/`" + `: files Dags load or import
- ` + "`plugins/`" + `: Airflow plugins
- ` + "`tests/`" + `: tests; run them with ` + "`uv run pytest`" + `
- ` + "`.venv/`" + `: derived environment; never commit or edit it by hand

Add a Python dependency with ` + "`uv add <package>`" + ` rather than editing
` + "`pyproject.toml`" + ` by hand or using pip, so the ` + "`[tool.uv]`" + ` pins that hold
Airflow to the build a deployment runs stay in place.

Use the installed CLI as the reference: ` + "`astro --help`" + `, ` + "`astro local --help`" + `
(local Airflow, including ` + "`astro local af`" + ` for this project's Airflow API).
` + "`astro dev`" + ` was removed in v2, so never suggest an ` + "`astro dev`" + ` command;
running one prints its replacement.
`
