package scaffold

import (
	_ "embed"
	"fmt"
	"strings"
)

// exampleDag is the DAG a new project starts with.
//
// Embedded rather than a string constant, unlike the other templates here,
// because it is Python: a file is syntax-highlighted, diffable and lintable
// where a Go string literal is none of those.
//
// It imports only airflow.sdk and the standard library, and that is a
// requirement rather than a coincidence. Under v1 a project's dependencies came
// from a fat runtime image, so the example could `import requests` and call an
// API. A v2 project installs exactly what [project.dependencies] names, which
// this scaffold writes as apache-airflow alone — so an example carrying a
// third-party import would fail to load on the first `astro local start`, which
// is a worse first run than no example at all.
//
// It is Airflow 3 only, because airflow.sdk is. The v1 templates were a
// per-major pair, pkg/airflowrt/include/airflow2/exampledag.py beside
// .../airflow3; nothing wrote them once v1 init was gone and they were deleted
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

// agentsIntro is the static half of AGENTS.md. It references the manifest
// instead of restating it, so the file cannot drift from pyproject.toml.
const agentsIntro = `# Agent notes

This is an Astro project: Apache Airflow DAGs, run locally with the Astro
CLI (v2). ` + "`pyproject.toml`" + ` is the manifest — the project name, the pinned
Airflow version (the ` + "`apache-airflow`" + ` requirement in ` + "`[project] dependencies`" + `),
and the Python dependencies live there. Read it rather than assuming versions.
To change the Airflow version, change that requirement: an optional
` + "`[tool.astro] runtime`" + ` only picks one Astro Runtime build of the same series
for the image, and a ` + "`FROM`" + ` in a declared Dockerfile has to name the same series.

Layout:

- ` + "`dags/`" + ` — Airflow DAG definitions
- ` + "`include/`" + ` — files DAGs load or import
- ` + "`plugins/`" + ` — Airflow plugins
- ` + "`tests/`" + ` — tests; run them with ` + "`uv run pytest`" + `
- ` + "`.venv/`" + ` — derived environment; never commit or edit it by hand

## Local Airflow

Local Airflow lives under ` + "`astro local`" + `. It works offline, needs no
account, and most commands take ` + "`--output json`" + ` for machine-readable
output. The passthrough commands (` + "`run`" + `, ` + "`shell`" + `) stream the
child process's own output instead.

    astro local start          # start Airflow for this project
    astro local stop           # stop it (--clean also wipes runtime state)
    astro local restart        # stop, then start
    astro local status         # state of this project's Airflow
    astro local logs           # show logs (--follow to stream)
    astro local run <cmd>      # run a command in the project environment
    astro local shell          # open a shell in the project environment
    astro local open           # open the Airflow UI in the browser
    astro local check          # validate DAGs without starting Airflow
    astro local reset          # stop Airflow and wipe derived state
    astro local list           # every local Airflow on this machine

## astro dev was removed

Astro CLI v2 removed the whole ` + "`astro dev`" + ` tree. Never suggest an
` + "`astro dev`" + ` command; use the v2 form:

| v1 command | use instead |
| --- | --- |
`

// agentsContent renders AGENTS.md: the static intro plus the dev-to-local
// table, generated from the same data the `astro dev` stub prints.
func agentsContent() string {
	var b strings.Builder
	b.WriteString(agentsIntro)
	for _, m := range DevReplacements() {
		fmt.Fprintf(&b, "| `astro dev %s` | `%s` |\n", m.Command, m.Replacement)
	}
	return b.String()
}
