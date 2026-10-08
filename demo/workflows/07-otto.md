# 7. Otto in the loop

**The story.** The AI data engineer rides the CLI: `astro otto` installs the
agent, keeps it current, and hands it the project and the running Airflow —
no wrapper, no config file, no pasting URLs. The agent answers from live
state and speaks the v2 surface back at you.

**Shows:** Otto binary management, project detection inside Otto,
zero-config discovery of the running local Airflow, and the `astro local`
surface in Otto's own answers.

**Needs:** the `astro` v2 binary and an Astro login. This is the one workflow
that needs an account and network: Otto's brain is the Astronomer gateway.

---

## The CLI owns the agent

No install step. The first `astro otto` downloads the Otto binary; every later
launch checks for updates.

```sh
astro otto version
```

```
Otto 0.1.23
```

## It already finds your Airflow

Pick up where workflow 1 left off: `orders-demo` with `astro local start`
running and one DAG. Ask Otto what it sees — from the project directory,
nothing else:

```sh
astro otto --mode text "Which Airflow are you connected to, and what DAGs does it have? Two lines."
```

```
Connected to the `af` instance `local` at `http://localhost:19054`.
DAGs: `daily_orders` (paused; source `dags/daily_orders.py`).
```

That URL came from the local-state record the engine writes at start — the
same record `astro local status` reads. No `AIRFLOW_API_URL`, no login to the
webserver, and a stale record from a crashed Airflow fails the health probe
and is ignored rather than handed over.

## It speaks v2

Otto detects the pyproject manifest and its prompt teaches the `astro local`
surface for this machine and deployments for everything remote — in a v2
project it will not steer you to `astro dev`:

```sh
astro otto --mode text "Is this project healthy, and how would I ship it to a deployment when I have one? Three lines, exact commands."
```

```
Healthy on `af` instance `local`: Airflow reports healthy, `daily_orders` is loaded, and there are no import errors.

af health
af dags errors
astro deploy
```

The interactive TUI is the same brain with a session: `astro otto` with no
flags. Every mode reads the same detection, so a subagent or an RPC consumer
sees the same Airflow the TUI does.

## What is not there yet

Two edges, both named:

* **An Astro account is required.** `astro otto` without a login points at
  `astro login` and the trial page.
* **Hosted skills still speak some v1.** Skill content is served per-org from
  the platform and predates v2; an answer drawn from one can still say
  `astro dev restart` inside a project, where the stub will correct it.
