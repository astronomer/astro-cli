# Astro CLI

The Astro CLI runs Apache Airflow anywhere you need it: on your laptop in seconds, or in production on [Astro](https://www.astronomer.io/).

Point it at a Python project and `astro local start` gives you a real, browsable Airflow — no Dockerfile, no containers, no YAML. `astro deploy` ships that same project to production. One project file drives both.

<img referrerpolicy="no-referrer-when-downgrade" src="https://static.scarf.sh/a.png?x-pxid=29deaa05-2c91-4f0b-bb2c-d2e9408867e0" />

## Start with an agent

Already have an Airflow repo? Open it in Claude Code, Cursor, or any coding agent and paste this:

```
Set up the Astro CLI in this repo. Read the guide first:
gh api -H "Accept: application/vnd.github.raw" \
  repos/astronomer/astro-cli/contents/docs/install.md
```

That's the install guide — the same page you'd read yourself, written so an agent can work straight from it. It installs the CLI, signs you in, makes your repo an Astro project, and moves your pins and your Dockerfile's contents into `pyproject.toml`. The agent stops when `astro local check` passes, not when it runs out of things to try. It works in the repo as it stands, so what it did is a `git diff` you review like any other change.

The fetch goes through `gh`, so it reads with your own GitHub account and needs no token in the prompt. Run `gh auth status` if it fails.

You don't have to read the rest of this page. It's the same setup, by hand.

*Coming soon: the install script that guide calls, and a public copy of the guide fetched over plain HTTPS, once the v2 release pipeline ships.*

## Quickstart

You need two things: the `astro` binary (see [Install](#install)) and [uv](https://docs.astral.sh/uv/) 0.9.25 or later on your PATH.

```sh
$ mkdir hello-astro && cd hello-astro
$ astro init
$ astro local start
```

That's it. `init` scaffolds an Astro project, `start` installs the right Airflow into a managed virtualenv (via uv) and brings it up. Your Airflow gets a stable, named URL — `http://hello-astro.localhost:6563` — so two projects never claim the same port. Start as many as you like; `astro local list` shows them all. On Windows, add `--docker` (see [Two ways to run](#two-ways-to-run)).

Airflow keeps running after you close the terminal. Stop it with `astro local stop`, or pass `--stop-with-session` at start to tie it to your terminal session instead.

## Why it feels different

If you run Airflow today — open source on a hand-rolled compose file, MWAA, or Cloud Composer — this is the loop you've been missing:

- **Airflow in seconds, not minutes.** `astro local start` boots a real Airflow before a cloud environment would finish picking up your last upload.
- **Save a file, see it in seconds.** Airflow looks for new DAG files every two seconds and re-parses each file every three. No bucket sync, no image rebuild, no redeploy — save, refresh, run.
- **No Docker required.** Standalone mode runs Airflow in a managed virtualenv on your machine. Docker mode is one flag away when you want the closest match to production — same project, same commands, same URL.
- **Any Airflow version, per project.** The version is one line in `pyproject.toml`. Run two projects on two Airflow versions side by side, each at its own `<name>.localhost` address.
- **Failures surface now.** Local defaults are tuned for development: zero task retries, fast rescans, and DAGs run when you trigger them rather than on their schedules. A broken task shows you its error immediately instead of retrying quietly for ten minutes.
- **Declare config, don't discover it.** List the env vars, Airflow variables, and connections your DAGs need in the project file. `astro local start` reports everything missing at once, before Airflow boots — not one runtime error at a time.
- **Check DAGs without starting Airflow.** `astro local check` parses your DAGs and reports import errors in seconds — made for CI and pre-commit.
- **Check DAGs against MWAA's or Composer's Airflow before you upload.** `astro local check --target mwaa` (or `composer`) builds the exact Airflow that platform runs and parses your DAGs against it, so a version mismatch or a dependency conflict shows up in seconds instead of after a slow environment update.
- **A real shell into your environment.** `astro local shell` and `astro local run` put you inside the exact environment your DAGs run in — the thing managed Airflow never gives you.
- **Private by default.** Everything binds to loopback only; nothing else on your network can reach it.
- **Everything scripts.** Every command takes `--output json`; logs stream as one JSON object per line.

## Commands

```
astro init            Make a directory an Astro project (run it in the Airflow repo you have)
astro local start     Start local Airflow for this project (--docker, --port, --stop-with-session)
astro local stop      Stop it (--force, --clean)
astro local restart   Stop and start
astro local status    Show state, mode, and URL
astro local logs      Show logs (-f follows, --component filters, --tail limits)
astro local list      List every local Airflow on this machine (--all, --clean)
astro local run       Run a command inside the project's Airflow environment
astro local shell     Open a shell inside it
astro local open      Open the Airflow UI in your browser
astro local reset     Stop and wipe what start created (database, logs, environment)
astro local check     Validate this project's DAGs without starting Airflow (--strict, --target mwaa|composer)
astro local upgrade airflow [version]  Move the project to a new Airflow, the newest of its generation if none is given (--with-otto hands the rest to Otto)
```

`astro start`, `astro stop`, and `astro logs` work as shorthand for the `local` versions. The cloud commands — `astro login`, `astro deploy`, `astro deployment`, `astro workspace` — are unchanged from v1.

Every command takes `--output json` for scripting; streaming commands like `logs` emit one JSON object per line.

## How it works: the Astro project

Every command above works on the same unit — the **Astro project**. `astro init` creates one: a `pyproject.toml` with a `[tool.astro]` section:

```toml
[project]
name = 'hello-astro'
version = '0.1.0'
requires-python = '>=3.12'
dependencies = [
    "apache-airflow==3.3.*",
]

[tool.astro]
```

DAGs go in `dags/`, Python dependencies in `[project.dependencies]` — the `apache-airflow` line among them is the Airflow version the project runs, and the only place it is stated — and `[tool.astro]` is the project's identity as an Astro project: the configuration it expects — environment variables, Airflow variables, connections — and the Astro Deployments it ships to. One file is the whole contract. Check it into git and anyone on your team, or any CI job, runs the same Airflow you do; change the `apache-airflow` requirement and the next `astro local start` is on the new version.

Because that file is standard Python packaging, the tools you already use — uv, ruff, your editor — understand an Astro project out of the box.

For every key you can set in `pyproject.toml`, see the [manifest reference](docs/manifest-reference.md); for the smallest real project that puts them together, see the [`examples/etl-demo`](examples/etl-demo) example.

`astro local start` checks the declared configuration before Airflow boots and, if anything is missing, lists it all at once instead of one error at a time. Supply a value with `astro local env variable set NAME` (or the `connection` or `airflow-variable` form), which stores it encrypted in the vault shared with Astro Desktop. Pass `--plain` to store a value that is not secret unencrypted instead, in the project's `.env`, or with `--global`, in the vault without encryption. A new `--global` value reaches no project until you `link` it, or pass `--auto-link` to reach every project, the term Astro Desktop uses. Your shell environment works too. See [docs/secrets.md](docs/secrets.md).

## Two ways to run

**Standalone (the default).** Airflow runs directly on your machine in a uv-managed virtualenv. No Docker anywhere. Startup is fast, logs are local files, and `astro local shell` drops you into the real environment.

**Docker (`--docker`).** Airflow runs in containers built from Astronomer's runtime images, with your project's dependencies installed — the closest local match to what runs on Astro. Requires Docker Desktop or a compatible engine; the CLI starts the engine for you when it can.

Both modes serve the same named URL and answer to the same commands. On Windows, use `--docker`. *Coming soon: standalone mode on Windows.*

## Coming from v1

Run `astro init` in the repo you already have. A directory with no `pyproject.toml` gets one; a directory that has one gains a `[tool.astro]` section, plus any `[project]` key Python packaging requires and doesn't find. Your comments, your key order and your other tools' settings all survive, and your `dags/` folder stays where it is. Nothing is overwritten, so the port is a `git diff` on your own repo — with its history, its CI, and its review — not a new tree beside it.

What `init` can't carry over on its own, it names. A `requirements.txt`, a `packages.txt`, an `airflow_settings.yaml`, a Dockerfile: each one is listed at the end, with where its contents belong. If nothing in your project names an Airflow version, the pin is the current default and the list says so, because most repos state the version they run in a Dockerfile image tag. Reading a Dockerfile means guessing what its `RUN` lines were for, so `init` doesn't guess — it hands you, or the agent working with you, the list. If your old local runs used Docker, `astro local start --docker` is the matching mode.

After converting, check three things yourself:

- Every package the project needs is in `[project] dependencies`. A package installed some other way does not come across: a `RUN pip install` line in a kept Dockerfile, a private git package, a setup script. `astro local check` and `astro local start` without `--docker` read only `pyproject.toml`. See [`dependencies`](docs/manifest-reference.md#dependencies).
- Every environment value the project needs is declared in `[tool.astro.env]`. Declare each one with `astro local env variable declare NAME`, or the `connection` or `airflow-variable` form. Then a teammate's first `astro local start` names what is missing, instead of a task failing later. To find the values, look where the project documents or reads them: an example env file, the README, code that reads the environment, deployment settings. See [`[tool.astro.env]`](docs/manifest-reference.md#toolastroenv).
- Every line `init` printed under **Left to do** is handled.

The `astro dev` commands are gone in v2; running one prints its replacement.

## Sign up

`astro login` opens the sign-up screen in your browser when the CLI has no saved account for the domain. Create your account there.

Astro sends a verification email, so the command stops and asks you to check your inbox. Verify your address. Then run `astro login` again. This time the CLI asks whether to create your first organization and workspace.

Answer no if you're waiting on an invite to someone else's organization — a free account gets one organization, and you can't hand it back.

Already have an account? Log in from the same page, or run `astro login --signin`.

## Install

*Coming soon: Homebrew, winget, and the install script, once the v2 release pipeline ships.* Until then, build from source:

```sh
git clone https://github.com/astronomer/astro-cli && cd astro-cli
make build
```

You'll need Go (version in `go.mod`) and, for standalone mode, [uv](https://docs.astral.sh/uv/) 0.9.25 or later on your PATH.

That leaves the binary in the repo root. `make install` puts it in a directory your shell already searches, and `make uninstall` takes it back out. It checks afterwards and tells you whether typing `astro` now runs the build it just made. Choose the directory yourself with `make install INSTALL_DIR=/usr/local/bin`.

## Contributing

Setup, tests and lint are in [CONTRIBUTING.md](CONTRIBUTING.md). Read [docs/architecture.md](docs/architecture.md) before you write code; the rest of the documentation is indexed in [docs/](docs/README.md).

## Support

Start with the [Astronomer documentation](https://www.astronomer.io/docs/astro/cli/overview). If that doesn't resolve it, post on the [Astronomer forum](https://forum.astronomer.io) or contact [Astronomer support](https://support.astronomer.io).

## License

Apache 2.0 with Commons Clause
