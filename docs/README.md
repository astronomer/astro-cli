# Astro CLI v2 docs

Each page describes how v2 works today. When behavior changes, the page changes with it, in the same PR.

For users:

- [install.md](install.md): install the CLI, sign in, and convert an Airflow repo into an Astro project. Written so a coding agent can follow it.
- [manifest-reference.md](manifest-reference.md): every key the CLI reads from `pyproject.toml`.
- [upgrading-from-v1.md](upgrading-from-v1.md): what v2 changes for someone coming from Astro CLI 1.x, and what replaced each removed command and flag.

How v2 works:

- [architecture.md](architecture.md): layers, sub-module rules, the `--output json` contract, local state, dev-mode defaults. Read this before writing code.
- [secrets.md](secrets.md): local environment values, the vault, precedence, and `astro local env`.
- [workspace-link.md](workspace-link.md): values read from a linked Astro workspace, and which login reads them.
- [instances.md](instances.md): which Airflow a command talks to, `astro use`, and link authentication.
- [deploy.md](deploy.md): `astro deploy`, `astro package`, and `astro local check --target`.