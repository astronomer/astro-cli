# Agent notes

This is an Astro project: Apache Airflow Dags, run with the Astro CLI (v2).
`pyproject.toml` is the manifest: the project name, the pinned Airflow
version (the `apache-airflow` requirement in `[project] dependencies`),
and the Python dependencies live there. Read it rather than assuming versions.
To change the Airflow version, change that requirement: an optional
`[tool.astro] runtime` only picks one Astro Runtime build of the same series
for the image, and a `FROM` in a declared Dockerfile has to name the same series.

Layout:

- `dags/`: Airflow Dag definitions
- `include/`: files Dags load or import
- `plugins/`: Airflow plugins
- `tests/`: tests; run them with `uv run pytest`
- `.venv/`: derived environment; never commit or edit it by hand

Add a Python dependency with `uv add <package>` rather than editing
`pyproject.toml` by hand or using pip, so the `[tool.uv]` pins that hold
Airflow to the build a deployment runs stay in place.

Use the installed CLI as the reference: `astro --help`, `astro local --help`
(local Airflow, including `astro local af` for this project's Airflow API).
`astro dev` was removed in v2, so never suggest an `astro dev` command;
running one prints its replacement.
