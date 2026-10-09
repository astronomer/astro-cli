# Install the Astro CLI and convert your Airflow project

This page takes you from an Airflow repo to a running local Airflow. Follow it
top to bottom. Each step ends with a check, so you always know whether it
worked before you start the next one.

An agent can follow this page for you. Paste this into Claude Code, Cursor, or
any coding agent, in your Airflow repo:

```
Set up the Astro CLI in this repo. Read the guide first:
gh api -H "Accept: application/vnd.github.raw" \
  repos/astronomer/astro-cli/contents/docs/install.md
```

The fetch goes through `gh`, so it reads with your own GitHub account and needs
no token in the prompt. If it fails, check `gh auth status`.

The work happens in the repo as it stands. Nothing here moves your dags.
`astro init` edits an existing `pyproject.toml` and `.gitignore` in place, and
removes only the files of a project made by Astro CLI 1.x (a 1.x project)
whose contents it carried across, saying so. So
everything below shows up as a `git diff` you can read before you commit it.

## Two things to ask about first

If you are an agent following this page, stop and ask the person before you do
either of these. Everything else on this page is safe to do without asking.

1. **Creating an Astro organization.** A free account gets one, and it can't be
   handed back. Someone waiting on an invite to a colleague's organization must
   not create their own.
2. **Deleting files `init` kept.** `init` removes the 1.x files it carried
   across completely. A file it keeps is still read or still needs a decision:
   a `Dockerfile` it declared as the project's build, with the
   `requirements.txt` and `packages.txt` that build installs, or the same
   three kept because the project deploys to Astro Private Cloud. Ask before
   you remove one.

Report what you changed when you finish. Say which files you wrote, what you
put in the Airflow pin, and what you left alone.

## Before you start

You need:

- **[uv](https://docs.astral.sh/uv/) 0.9.25 or later** on your PATH. Standalone
  mode uses it to build the Airflow environment.
- **Python 3.10 or later**, for your dags.
- **Docker**, only if you want Docker mode or you're on Windows.

Check uv with `uv --version`. To install it, see the
[uv install guide](https://docs.astral.sh/uv/getting-started/installation/).

## Step 1: Install the CLI

Run:

```sh
curl -sSL install.astronomer.io | sudo bash -s
```

*Coming soon: this installer serves the current CLI. Until the new release
pipeline ships, build from source instead. You need Go (the version in
`go.mod`):*

```sh
git clone https://github.com/astronomer/astro-cli && cd astro-cli
make install
```

`make install` builds the binary and puts it in a directory your shell already
searches (`make install INSTALL_DIR=/usr/local/bin` to choose one), then says
whether typing `astro` runs it. `make build` alone leaves `./astro` in the repo
root.

**You're done when** `astro version` prints a version.

## Step 2: Sign in or sign up

Run `astro login`. It opens your browser.

If the CLI already holds a login that works, `astro login` uses it and does not
open the browser. To log in again anyway, run `astro login --force`.

Some shells do not give the CLI a terminal, for example a coding agent's
shell. There, `astro login` cannot open the browser. It prints a link instead.
Open the link and sign in there.

If the CLI has no saved account for the domain, it opens the sign-up screen.
Create your account there. If you already have an account, log in from the same
page, or run `astro login --signin` to open the sign-in screen directly.

After you sign up, Astro sends you a verification email, so the command stops
here:

```
Thanks for signing up. Check your inbox for a verification email.
After you verify your email address, run 'astro login' to finish signing in.
```

That is not a failure, and the command exits zero. Open the email and verify
your address. Then run `astro login` again. This time the CLI asks whether to
create your organization and workspace.

Read [Two things to ask about first](#two-things-to-ask-about-first) before you
answer. Answer no if you're waiting on an invite to someone else's organization.

**You're done when** `astro workspace list` prints a workspace.

## Step 3: Make the repo an Astro project

From the root of your Airflow repo, run:

```sh
astro init
```

What it does depends on what is already there:

- **No `pyproject.toml`.** It writes one, with a `[tool.astro]` section and an
  `apache-airflow` pin.
- **A `pyproject.toml` already.** It adds the `[tool.astro]` section to that
  file, and fills in the `[project]` keys Python packaging requires if they're
  missing — a `name`, a `version`, and an `apache-airflow` entry under
  `dependencies`. It changes nothing else: your comments, your key order and
  your other tools' settings all survive. If the file already pins
  `apache-airflow`, that pin becomes the project's Airflow version, so a
  project running 2.9 stays on 2.9. The requirement is the only place the
  version lives, so it has to pin one series or release: a range like
  `apache-airflow>=2.9`, or dependencies declared `dynamic`, is refused with
  the line to change.

When nothing in the project names an Airflow version, `init` pins the newest
supported Airflow series, read from Astronomer's public runtime catalog
(`https://updates.astronomer.io/astronomer-runtime`), with the `requires-python`
that series' runtime ships. The first line of its output says where the
version came from. The request names only the client and its version
(`astro-cli/<version>`); it is not telemetry, so `ASTRO_TELEMETRY_DISABLED`
does not affect it. The answer is cached for a day. With no network, `init`
uses an older cached copy or the series built into the CLI, and still
succeeds. To choose the version yourself, pass `--airflow-version 3.3`. To read
the catalog from a mirror, or to stop the request, set
`ASTRO_RUNTIME_VERSIONS_URL` to the mirror's address, or to one that does not
answer. The Airflow 2 image lookup in Docker mode reads the same address.

Either way it creates the standard directories it doesn't find (`dags/`,
`include/`, `plugins/`, `tests/`), writes a `.gitignore` and an `AGENTS.md` if
they're missing, and keeps everything that already exists. A `dags/` folder you
already have is left as it is. The `.gitignore` keeps `.env` and the per-machine
files Astro tools write under `.astro/` (local Airflow state, `*.local.yaml`,
Otto's `mcp.json` and `*.local.json`) out of git, plus the
`plugins/fix_local_executor_pickle.py` that standalone Airflow 2 regenerates on
macOS, while `.astro/config.yaml` stays committed. A `.gitignore` you already
have is kept, and only the lines it is missing are appended. When `dags/` is
empty it writes a starter Dag, and outside Windows it links `CLAUDE.md` to
`AGENTS.md`.

Beyond an Airflow requirement it cannot read, `init` refuses a
`pyproject.toml` that already has a `[tool.astro]` section. That directory is
an Astro project, and re-running `init` would overwrite it. Edit the manifest
instead.

**You're done when** `git diff` shows a `pyproject.toml` carrying `[tool.astro]`,
the `[project]` keys above, and the `[tool.uv]` settings that point Airflow at
Astronomer's build of it (see the [manifest reference](manifest-reference.md#tooluv)),
the new files above and, for a 1.x project, the conversion described in Step 4.

## Step 4: Check the conversion and finish it

In a 1.x project, `init` converts what it can read and says what it did. Its
output ends with **Left to do**: what it found and could not carry. That list
is the work. Take it one line at a time.

What `init` converts:

| 1.x file | What `init` does |
| --- | --- |
| `requirements.txt` | Each requirement becomes an entry in `[project] dependencies`; an `apache-airflow` pin becomes the project's Airflow version. Lines a dependency list cannot hold (`-r`, `-c`, `-e`, index options, hashes, bare URLs, local paths) are listed under Left to do. The file is removed, unless a kept Dockerfile still installs it. |
| `packages.txt` | Each line becomes an entry in `packages` under `[tool.astro]`. Removed, unless a kept Dockerfile still installs it. |
| `airflow_settings.yaml` | Connection and variable values go to this machine's encrypted vault, and `[tool.astro.env]` declares them without their values. A variable with an empty value is declared optional, so `astro local start` runs without it, as v1 did. Pools go to `[tool.astro.pools]`, and `astro local start` creates them in Airflow. The file is removed once everything in it is carried. |
| `Dockerfile` | Its `FROM` line is read for the Airflow version (below). A Dockerfile that only names a base image is removed, since the requirement now says the same thing. Any other Dockerfile is kept and declared as the project's build (`[tool.astro] dockerfile`), and `init` writes a `.dockerignore` for it. |
| `Dockerfile`, `requirements.txt` and `packages.txt` in a project that deploys to Astro Private Cloud | Kept, even where the rows above remove them. A Dockerfile that only names a base image stays undeclared, so Astro and `astro local` still build from `pyproject.toml`. APC's `astro deploy` still builds the 1.x layout: it builds this Dockerfile as it stands, and a runtime base image installs `requirements.txt` and `packages.txt` during that build (a `-base` runtime tag runs no ONBUILD steps, so it installs neither unless the Dockerfile does). Their contents are carried into `pyproject.toml` as well, for `astro local`, so Left to do names each file kept for APC and asks you to change both together while the project deploys there. `init` also writes a `.dockerignore` for that build. The Airflow pin is held to the exact series that Dockerfile's `FROM` carries, since that is what APC deploys: `init` reads it from the `FROM` when nothing else pins one, and refuses an `--airflow-version` (or an existing requirement) naming another series or only the generation (`3` beside `runtime:3.1-12`, which `astro local` would resolve to the newest 3.x). An Airflow 2 tag (`12.1.0`) names a runtime version, not an Airflow one, so its series comes from the runtime catalog (a cached copy works offline); when the catalog cannot be read, Left to do says the pin was not checked and asks you to set it. Where the project deploys is `--deploy-target astro` or `--deploy-target apc` when you pass it, and otherwise the current context's platform, the one `astro deploy` uses; no context counts as Astro. Nothing in the project's files decides it. Each note and refusal that turns on it says which decided and how to choose the other. |
| `.astro/config.yaml` | A saved deploy target (`project.deployment` with `project.workspace`) becomes a link named `default` under `[tool.astro.deployments]`, marked `default = true`, when both are Astro ids and the project deploys to Astro. APC saves ids of the same shape, so a project that deploys to APC (see the row above) leaves it where APC's `astro deploy` reads it, and Left to do says so. Under Astro, anything else is listed as a note naming the entry to add by hand. |
| `docker-compose.yml` | Nothing to do: `astro local start` replaces it. |
| `docker-compose.override.yml` | Kept. `astro local start --docker` merges it over the services it generates, as 1.x did, and `astro local stop` and `reset` take its services down too. Standalone mode runs no containers, so there it does nothing, and `astro local start` says so. |

### The Airflow version

The first line of `init`'s output says where the version came from. It takes
the first of:

1. `--airflow-version`;
2. an `apache-airflow` pin already in `pyproject.toml`;
3. the Dockerfile's `FROM`, when a stage builds on an Astro Runtime image. A
   tag like `runtime:3.3-2` names Airflow 3.3. An older tag like
   `astro-runtime:9.1.0` is a *runtime* version that does not name the
   Airflow minor, so `init` pins `apache-airflow==2.*` and says so under Left
   to do;
4. an `apache-airflow==` pin in `requirements.txt`;
5. the newest supported series from the runtime catalog.

Check the result against what you run today, and settle it when Left to do
names it. For an `astro-runtime:9.1.0`-style tag, map the runtime version
with Astronomer's
[Runtime release notes](https://www.astronomer.io/docs/astro/runtime-release-notes)
(runtime 9.1.0 is Airflow 2.7.1). A `FROM apache/airflow:2.7.3` line is not
read; its tag is the version. If you cannot tell, ask the person rather than
guess.

Pin the `apache-airflow` requirement in `[project] dependencies` to that
version, as `apache-airflow==2.7.*` for a series or `apache-airflow==2.7.1` for
one release. Keep the version you run today, and upgrade later as its own step
(`astro local upgrade airflow`): porting and upgrading at once turns one
failure into two. The requirement is the only place the version goes.

`init` also writes `requires-python` for the version it pins (`==3.13.*` when a
kept Dockerfile's tag names Python 3.13). If you change the pin, check it: an
older Airflow has no wheels for a recent Python, and the install then fails
while compiling a dependency, which reads like a broken package rather than a
Python that is too new. Airflow 2.7 wants 3.11 or lower.

After you change the pin, the next standalone `astro local start` updates the
`[tool.uv]` pins to Astronomer's build of the new version. On an Intel Mac,
run Docker mode (`astro local start --docker`): the `environments` that `init`
writes leave Intel macOS out.

### A kept Dockerfile

A kept Dockerfile stays the project's build: Docker mode, `astro deploy` and
`astro package` build from it, and its `RUN`, `ENV` and other instructions
still run there. Left to do names what the manifest therefore does not
describe:

- Python packages a `RUN pip install` or `uv pip install` step installs.
  Standalone mode and `astro local check` read only `pyproject.toml`, so add
  them to `[project] dependencies` too.
- Each `RUN --mount=type=secret` step, with the
  [`build-secrets`](manifest-reference.md#build-secrets) entry to add.
- Instructions it did not read (`ENV` and the rest). Standalone mode never
  sees an `ENV` line, so a value a Dag reads needs a home there too: declare
  it in `[tool.astro.env]`, with the value as its default, or set it with
  `astro local env variable set`.

If a `RUN` line does something you cannot place, ask the person what it was
for. Don't guess.

To add a Python package later, run `uv add <package>`; a running standalone
Airflow picks it up.

### After converting, check

`init` moves what it can read. Check three more things yourself:

- Every package the project needs is in `[project] dependencies`. A package
  installed some other way does not come across: a `RUN pip install` line in a
  kept Dockerfile, a private git package, a setup script. `astro local check`
  and `astro local start` without `--docker` read only `pyproject.toml`. See
  [`dependencies`](manifest-reference.md#dependencies).
- Every environment value the project needs is declared in
  `[tool.astro.env]`. Declare each one with `astro local env variable declare
  NAME`, or the `connection` or `airflow-variable` form. Then a teammate's
  first `astro local start` names what is missing, instead of a task failing
  later. To find the values, look where the project documents or reads them:
  an example env file, the README, code that reads the environment, deployment
  settings. See [`[tool.astro.env]`](manifest-reference.md#toolastroenv).
- Every line under **Left to do** is handled.

**You're done when** all three hold.

## Step 5: Check the dags

Run:

```sh
astro local check
```

It builds the environment, parses every dag, and reports the import errors. It
exits non-zero when a dag fails to import, so it's the step that tells you the
port actually worked. The first run installs Airflow, so give it a few minutes
before you decide something is wrong.

Fix what it reports and run it again. A missing import means a dependency
didn't come across — add it to `[project.dependencies]`.

**You're done when** `astro local check` exits zero. If you're an agent, this
is your stop condition. Don't report the setup as finished until this passes.

## Step 6: Start Airflow

Run:

```sh
astro local start
```

It installs the Airflow your project pins, brings it up, and prints a URL of
the form `http://my-project.localhost:6563`. Open it.

On Windows, add `--docker`.

**You're done when** the URL loads the Airflow UI and your dags are listed.

## What to do next

- `astro local logs -f` follows the logs.
- `astro local stop` stops Airflow. It keeps running until you do.
- `astro local shell` opens a shell inside the environment your dags run in.
- `astro deploy` ships the project to an Astro Deployment.

For every key you can set in `pyproject.toml`, see the
[manifest reference](manifest-reference.md).
