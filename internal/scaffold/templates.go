package scaffold

import (
	"fmt"
	"strings"
)

const gitignoreTemplate = `# Derived environment (rebuilt by the astro CLI; never commit it)
.venv/
__pycache__/

# Local-only files
.env
.DS_Store
`

// agentsIntro is the static half of AGENTS.md. It references the manifest
// instead of restating it, so the file cannot drift from pyproject.toml.
const agentsIntro = `# Agent notes

This is an Astro project: Apache Airflow DAGs, run locally with the Astro
CLI (v2). ` + "`pyproject.toml`" + ` is the manifest — the project name, the pinned
Airflow version (` + "`[tool.astro] airflow`" + `), and the Python dependencies live
there. Read it rather than assuming versions.

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
	fmt.Fprintf(&b, "\nFull mapping: %s\n", DevMappingDoc)
	return b.String()
}
