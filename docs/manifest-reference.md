# The Astro manifest: what you can set in pyproject.toml

An Astro project is a normal Python project with one extra section. `pyproject.toml` holds the standard `[project]` table plus a `[tool.astro]` section, and together they are the whole contract: the Airflow version the project runs, the Python and OS packages it needs, the configuration it expects, and the Airflows it ships to and talks to. This page lists every key the CLI reads, one section at a time, each with a plain description and a small example. Anything not listed here — `[tool.ruff]`, `[tool.ty]`, `[dependency-groups]`, and so on — is standard Python tooling that lives in the same file; the CLI does not read it, but your linter, type checker, and test runner do.

The CLI reads this file when it starts a project. If a value is wrong, `astro local start` reports every problem it can see at once, each addressed by its dotted key, rather than one error at a time.

"Every problem it can see" rather than every problem in the file, because the file is read in two stages by two packages. [`pkg/manifest`](../pkg/manifest/manifest.go) validates `[project]` and `[tool.astro]` — the deployment links included — and reports all of those together in one error. [`pkg/envschema`](../pkg/envschema) then types `[tool.astro.env]`, and only once the first stage is sound. The split is deliberate: the two are separate modules and the first does not import the second, so each consumer composes them. A file with mistakes in both therefore takes two passes to clear — you fix the keys the first report names, re-run, and meet the environment ones.

Which commands report what is worth knowing, because they differ:

| command | `[project]` + `[tool.astro]` | `[tool.astro.env]` |
| --- | --- | --- |
| `astro local start` | yes | yes |
| `astro local env list` / `get` | yes | yes |
| `astro local env <noun> declare` / `undeclare` | yes | yes |
| `astro package` | yes | yes |
| `astro local check` | yes | yes, except values from the linked workspace |

`astro local check` reports the environment the way `astro local start` gates on it. A declaration that does not parse stops the check. A required value with no source on this machine is an `env_missing` error, which fails the check as it blocks a start, and names the `astro local env <noun> set` command that provides it. A value that is present but is not what its declaration says (its `type`, `enum` or `conn_type`) is an `env_invalid` warning, which a start prints and runs past, and which `--strict` fails on. The check runs offline and never asks the Environment Manager, so a `source = 'workspace'` value that no local source holds is not reported, and neither is anything else the linked workspace supplies: `astro local start` reads them, and says so if it cannot. When the manifest links a workspace, the check's note says its values were not checked. It imports the DAGs under the values a start would give them (the project's `.env`, the vault, declared defaults), so a value held in the vault can bring up the same keychain prompt a start does. The environment is reported for the plain check and `--target astro`; `--target mwaa` and `--target composer` check a platform that sets its own values, from the `ENV_SETUP.md` that `astro package` writes.

`astro package astro` builds the image from the dependencies and OS packages alone, so the image carries none of the declared values, defaults included. When the manifest declares any, it warns and lists them, each with where its value comes from: set them on the Deployment, as its environment variables or the Environment Manager objects linked to it. `astro deploy` builds the same image and does not check the declarations.

## `[project]`

The standard Python packaging table. The CLI reads three fields from it.

### `name`

The project's name. It follows Python's own naming rules — letters, digits, and `-._`, starting and ending with a letter or digit — and it is required. Note that it is not what local Airflow is served at: the `<name>.localhost` hostname comes from the project's directory (or, in a git worktree, `<worktree>.<repo>.localhost`), and a project's identity is its path, hashed. Hostnames are display labels.

```toml
[project]
name = 'etl-demo'
```

### `requires-python`

The range of Python versions the project runs on, in the standard specifier form. `astro init` always writes one, bounded for the Airflow it pins: `>=3.12` for Airflow 3.2 and later, `>=3.10` for 3.0 and 3.1, `>=3.10,<3.13` for Airflow 2.9 and later, and `>=3.10,<3.12` before 2.9. A version picked from the runtime catalog takes the catalog's range. When `init` keeps your Dockerfile as the build and its base image tag names a Python, as `runtime:3.3-2-python-3.13` does, it writes that minor alone, `==3.13.*`. uv resolves for every Python the range allows, so a wider range can fail to resolve on a Python the project never runs.

One rule picks the Python within the range, and a generated image and a standalone environment both follow it, so a project runs the same Python in Docker mode, in standalone mode, and on a deployment. Each runtime build ships several Pythons and runs one by default. When `requires-python` admits the default, that is the project's Python. Otherwise it is the newest Python the build ships that `requires-python` admits. The build is the [`runtime`](#runtime) build, or the newest build of the series when none is set. A Python other than the default is built from that exact build's Python tag: `==3.13.*` on Airflow 3.3, whose runtime runs Python 3.14 by default, builds `FROM runtime:3.3-8-python-3.13`, and standalone mode asks uv for Python 3.13. This needs Astronomer's runtime catalog, read with a 3-second limit, and Airflow 3, whose builds list their Pythons.

| | generated image (Docker mode, `astro deploy`, `astro package`) | standalone environment (`astro local start`, Astro Desktop) |
| --- | --- | --- |
| range admits the build's default (`>=3.12`) | the series tag, `runtime:3.3`, on its default Python | the default Python |
| range admits another Python the build ships (`==3.13.*`) | `runtime:3.3-8-python-3.13` | that Python, 3.13 |
| range admits none of the build's Pythons | refused before the build, naming the build and its Pythons | a warning saying the image would be refused; uv picks an interpreter within the range |
| catalog unreadable, Airflow 2, or a range the CLI does not read (a pre-release, `===`) | the series tag on its default Python | uv picks an interpreter within the range |
| no `requires-python` | the series tag on its default Python | Python 3.12, or 3.11 for an Airflow 2 before 2.9 |

When the Python changes, the next standalone start rebuilds `.venv` on the new one. The fallback for a manifest without a range (hand-written, or declaring `requires-python` dynamic) does not follow new Python releases, so state the range your project runs on.

```toml
[project]
requires-python = '>=3.12'
```

### `dependencies`

The project's Python dependencies, in the standard PEP 508 form, alongside your providers and libraries; the CLI installs them into the project environment.

**This is where the project's Airflow version lives, and the only place.** The `apache-airflow` requirement (or `apache-airflow-core`, the slimmer distribution without the bundled providers) states it, and every mode reads it from there: standalone installs Astronomer's build of it (see [`[tool.uv]`](#tooluv)), Docker mode, `astro deploy` and `astro package` build from the Astro Runtime image for its series (or the one build [`runtime`](#runtime) names, which has to be of that series), and uv, ty, Dependabot and your IDE read the same line. Change this one line and the next `astro local start` is on the new version.

`astro local upgrade airflow [version]` makes that change for you. It sets the requirement, plus `requires-python` and the [`runtime`](#runtime) pin when they have to move. With no version, it uses the newest Airflow the runtime catalog offers in the project's own Airflow generation: it never moves an Airflow 2 project to Airflow 3, and says when Airflow 3 is available. It does not change Dag code, providers, or a declared [`dockerfile`](#dockerfile). If Airflow is running, restart it afterwards. `--with-otto` starts Otto afterwards to update Dags and providers for the new version.

```toml
[project]
dependencies = [
    "apache-airflow==3.3.*",
    "apache-airflow-providers-standard",
]
```

`astro init` also lists `apache-airflow-core` and `apache-airflow-task-sdk` beside the requirement, with no version. They state nothing: `[tool.uv.sources]` applies only to direct dependencies, and listing them is what lets uv take them from Astronomer's index (see [`[tool.uv]`](#tooluv)). A bare `apache-airflow-core` beside the `apache-airflow` requirement is not a second statement of the version.

Pin one series (`==3.3.*`, which tracks its patch releases) or one release (`==3.3.2`). Mind the wildcard: without it, `==3.3` is the one release 3.3.0, as it is to pip and uv, not the newest 3.3. Extras and environment markers are fine. The manifest is refused, in every mode, when the requirement leaves the version unclear:

- a range, a direct URL, or a bare name pins no single series: `apache-airflow>=3.1 does not pin an Airflow series. Pin one, like apache-airflow==3.3.*`;
- no Airflow requirement at all: `names no Airflow. Add one, like apache-airflow==3.3.*`;
- both `apache-airflow` and `apache-airflow-core`, or one listed twice with different pins (under different markers, say): list one, pinned once;
- `apache-airflow-core` pinned to an Airflow 2, which it was never published for: use `apache-airflow==2.x.*`;
- `dependencies` listed in `[project] dynamic`, with or without a static list beside it: there is then nowhere to state the requirement, so list the dependencies here and drop `dependencies` from `dynamic`.

The runtime image already is Airflow, so a generated image leaves both distributions out of what it installs over it; the image's tag is what decides the version there, and that is why the tag is held to the requirement: by series for a [`runtime`](#runtime) build, and for a declared [`dockerfile`](#dockerfile) by its `FROM` line.


## `[tool.astro]`

The section that makes a Python project an Astro project. Its absence is how the CLI tells an Astro project from a plain one. Every key it holds is listed below; a key that is not — a typo, a setting from another tool — is an error, so nothing you write here is quietly ignored.

There is no `airflow` key: the requirement is the one statement of the version, so nothing can run one Airflow in Docker mode and another in standalone. A manifest with an `airflow` line is refused until it is deleted, and the error quotes the requirement that decides the version:

```
tool.astro.airflow: is no longer read: the Airflow version is the requirement apache-airflow==3.1.* in [project] dependencies. Delete the airflow line.
```

When the line named a different series, the error also says what actually runs, and how to run the other one: `It said 3.3, and this project runs 3.1; to run 3.3, change the requirement to apache-airflow==3.3.*.` A manifest where the line was the only statement of the version, with no Airflow requirement, is also told the requirement to add in its place.

### `packages`

OS-level (apt) packages the project needs at the system level — the manifest's form of a 1.x project's `packages.txt`. Docker mode installs them into the runtime image; standalone mode cannot install system packages and warns instead.

```toml
[tool.astro]
packages = ['libpq-dev', 'build-essential']
```

### `runtime`

One Astro Runtime build for the image, when the newest build of the requirement's series is not the one you want. The requirement can name an Airflow release but not a runtime build: builds `3.3-3` to `3.3-7` all carry Airflow 3.3.1 and differ in everything else. Omit it, which is the common case, and the image is built from the series tag (`runtime:3.3`), so each start and deploy takes that series' newest build.

```toml
[project]
dependencies = ['apache-airflow==3.3.*']

[tool.astro]
runtime = '3.3-8'
```

It picks **the build**. Docker mode, `astro deploy` and `astro package` build `FROM runtime:3.3-8` instead of `runtime:3.3`, and the `[tool.uv]` pins that standalone mode installs from follow the Airflow release `3.3-8` carries, when the requirement covers it. It never stands in for the requirement: a requirement that pins no series is refused whether or not `runtime` is set, and to change the Airflow version you change the requirement. `astro init` writes it in one case: converting a 1.x Dockerfile that is only `FROM astro-runtime:<build>` on one Airflow 2 build (`12.6.0`, not a floating `12` or a flavor), whose tag names no Airflow minor, so the requirement becomes `apache-airflow==2.*` and `runtime` holds the project to the build it ran.

The value is one build: `3.3-8` for Airflow 3, or `13.11.0` for Airflow 2, whose runtimes are numbered on their own. The overlap with the requirement is checked every time the manifest is read, offline, as far as the tag can say:

- an Airflow 3 build must be of the requirement's series, so `runtime = '3.2-10'` beside `apache-airflow==3.3.*` is refused (`runtime_mismatch`);
- an Airflow 2 build names a runtime version, not an Airflow one, so only its generation is checked here: an Airflow 2 build beside an Airflow 3 requirement, or the reverse, is refused (`runtime_mismatch`);
- a value that is not one build — `latest`, a floating `3.3`, a flavor such as `3.3-8-python-3.12` — is refused (`runtime_invalid`). To run another Python, set [`requires-python`](#requires-python);
- beside a [`dockerfile`](#dockerfile), whose `FROM` names the base instead, it would pick nothing, and is refused (`runtime_with_dockerfile`).

The rest needs Astronomer's runtime catalog, so it is checked where an image is about to be built from the build — a Docker-mode start, `astro deploy`, `astro package` — and not when the manifest is only read:

- a build whose exact Airflow the requirement excludes is a warning: `3.3-8` carries 3.3.2, so beside `apache-airflow==3.3.1` the image runs one patch and standalone installs another;
- a build the catalog has yanked is a warning, with the catalog's reason;
- an Airflow 2 build carrying another series than the requirement pins is an error, when the catalog can be read. Offline the build goes ahead, and a warning says its series was not checked.

Changing the requirement through Astro Desktop (or anything else calling `SetAirflowVersionWith`) keeps `runtime` while the build still agrees with it, and otherwise moves it to the newest build carrying the new pin that is not yanked, or deletes the line when there is none to name. Without the catalog, agreeing means what the tag shows, so a series pin's patch move leaves it alone and a move to another series deletes it. With the catalog, the build must also carry an Airflow the new requirement covers: an exact pin the build does not carry (`==3.3.2` beside `3.3-5`, which carries 3.3.1), or an Airflow 2 build of another series, moves it.

### `dockerfile`

A project-relative path to the project's own Dockerfile — the escape hatch for a multi-stage build, or anything else this section cannot express. Omit it, which is the common case, and the image is generated for you from the Airflow requirement, `packages`, and the rest of `[project] dependencies`.

Set it and **that file is the build**: the Airflow requirement, `packages` and your other dependencies stop describing the image, because the Dockerfile decides what goes in — unless the file installs them the way a generated image does, which is how `astro init` writes one (see [Your own Dockerfile](#your-own-dockerfile)). Otherwise, install your Python and OS packages inside the file itself. The Airflow requirement is still required and still read — standalone mode installs it, and it tells the CLI which Airflow generation to run (the service set differs between 2 and 3) — but it no longer picks the image. [`runtime`](#runtime) is refused beside it, since the `FROM` line picks the base.

Docker mode builds only on an Astro Runtime image, and refuses a final stage on any other base (`unsupported_base`) before it starts anything. That is one of Astronomer's published repositories, `astrocrpublic.azurecr.io/runtime` or `quay.io/astronomer/astro-runtime`, with any tag, or a mirror of one in another registry: a repository whose last path segment is exactly `runtime` or `astro-runtime` and whose tag is an exact Runtime version, such as `registry.example.com/astronomer/astro-runtime:12.1.0` or `mirror/runtime:3.1-12` (a `-python-X.Y`, `-base` or `-slim` flavor is fine). A mirror is recognized by both because the name alone is not enough: `mcr.microsoft.com/dotnet/runtime:8.0`, `ghcr.io/x/runtime:latest`, a mirror on a floating tag (`:3.1`, `:stable`) or none, one pinned by digest alone, and a Docker Hub official name (`runtime`, `docker.io/library/runtime`) are refused, as are `myco/our-runtime` and `python`.

A mirror's tag is read the same way as the published image's, so the Airflow generation and the check below work for it too. Its build does not force a pull, so a mirror in a registry the daemon is not logged into still builds from the copy the daemon has. The published names always build with `--pull`, to pick up a new patch of a floating tag, so a Runtime image loaded with `docker load` for an offline build has to be retagged under a mirror name first (`docker tag astrocrpublic.azurecr.io/runtime:3.1-12 mirror/runtime:3.1-12`, then `FROM mirror/runtime:3.1-12`).

The file's `FROM` and the requirement have to agree, or Docker mode runs one Airflow and standalone installs another. When the final stage's `FROM` names an Astro Runtime tag, it is compared with the requirement — by series for an Airflow 3 tag (`runtime:3.2-4` beside `apache-airflow==3.3.*`), by generation for an Airflow 2 one — and a disagreement is refused, naming both lines and the fix:

```
tool.astro.dockerfile: Dockerfile builds FROM astrocrpublic.azurecr.io/runtime:3.2-4, which is Airflow 3.2, and the requirement apache-airflow==3.3.* in [project] dependencies is Airflow 3.3: Docker mode would run one and standalone the other. To fix it, change the requirement to apache-airflow==3.2.*, or change the FROM line to a 3.3 runtime
```

It is refused wherever the project runs, in both modes: `astro local start` and `restart` (standalone included, since that is where the requirement is what runs), `astro local check`, `astro package` and `astro deploy`, and Astro Desktop, which offers the requirement change as a fix (`dockerfile_airflow_mismatch`). A `FROM` that cannot be compared is let through: one built from a build argument (`FROM ${BASE}`), one pinned by digest, one that is not an Astro Runtime image, and one with no tag.

Every path that builds an image honours it: `astro local start --docker`, `astro deploy`, `astro package`, and Astro Desktop. Standalone mode has no image and ignores it, so a project pinned to a Dockerfile gets no benefit from it there.

The build context is the whole project directory, and only `.dockerignore` (or a `<Dockerfile>.dockerignore` beside the file, which Docker reads instead) keeps anything out of it. The build copies the rest into the image (the install's `COPY . .`, or an ONBUILD tag's), including local secrets and the per-machine files Astro tools write: the virtualenv, git's metadata (whose config can hold a token), `.env` files at any depth, a 1.x `airflow_settings.yaml`, an Airflow run's `airflow.db`, `airflow.cfg` and `logs/` at the root, and standalone Airflow's database, config and logs under `.astro/standalone/`. When `astro init` keeps a Dockerfile, it creates that ignore file or adds each of these lines it does not already exclude, and leaves the lines already there alone:

```
.venv/
**/.git
**/.env
**/.env.*
**/.envrc
**/airflow_settings.yaml
airflow.db
airflow.cfg
logs/
.astro/standalone/
.astro/worktrees/
.astro/*.local.yaml
.astro/*.local.yml
.astro/otto/*.local.json
.astro/otto/mcp.json
plugins/fix_local_executor_pickle.py
```

If you added `dockerfile` to the manifest by hand, add the lines yourself. Until you do, every `astro` build from the Dockerfile warns and names the files it would copy in (`astro local start` leaves `.git` out of that warning: a local image is not pushed). `astro deploy` and `astro package astro` also refuse a build whose context carries an uncommitted file that looks like a credential, whatever the Dockerfile copies (see [deploy.md](deploy.md#gitignored-files-and-credentials)).

Use forward slashes, on every platform. The path has to stay inside the project: an absolute path, or one climbing out with `..`, is refused when the manifest is read.

It also has to name a file that exists, but that is checked when something builds rather than when the manifest is validated — `astro local check` does no I/O on it, so a path pointing at a file you have since moved is reported by the build, naming the path to fix.

```toml
[tool.astro]
dockerfile = 'Dockerfile'
```

A build kept in a subdirectory:

```toml
[tool.astro]
dockerfile = 'docker/Dockerfile'
```

`--build-secret` works with a declared Dockerfile. Mount the secret in a `RUN` step, and the build gets it. Without a Dockerfile, only a secret with id `netrc` works: see [Private packages without a Dockerfile](#private-packages-without-a-dockerfile). The flag is on `astro deploy`, `astro local start --docker`, `astro local restart` and `astro package astro`. Name the source with `src=` for a file or `env=` for an environment variable. To pass more than one secret, repeat the flag:

```dockerfile
RUN --mount=type=secret,id=netrc,target=/root/.netrc pip install -r private-requirements.txt
```

```sh
astro local start --docker --build-secret id=netrc,env=NETRC_CONTENT
```

Without the flag, each of these commands reads `BUILD_SECRET_INPUT`, one secret per line, the way `astro deploy` does. Without either, they use [`build-secrets`](#build-secrets), so a project whose build always needs a secret can declare it once. The CLI passes on only the spec and Docker reads the value itself, so the CLI never writes the value to disk. For that reason, a flag or `BUILD_SECRET_INPUT` has to be given again on each later start or restart that builds the image. A generated image's build mounts only a `netrc` secret, while it installs your dependencies, so a flag with any other id is refused there rather than accepted and dropped. A `BUILD_SECRET_INPUT` line with another id is dropped for a generated image, since a CI runner may set the variable for every job. `astro package` refuses it for a bucket target too. Standalone mode builds no image, so it warns and starts without the secret.

Before each of these commands builds, it reads the Dockerfile for `RUN --mount=type=secret` steps. For each secret id that no flag, `BUILD_SECRET_INPUT` line or `build-secrets` entry supplies, it prints a warning that names the id, the line and the flag. It is a warning and not a refusal, because a secret mount is optional unless it sets `required=true`. If the build then fails, the build output has usually pushed the warning out of sight, so the final error names the missing ids and the flag again.

A conversion sets this for you when it finds a Dockerfile it cannot carry — one doing more than naming a base image. A Dockerfile that only pins a runtime is retired instead, since the Airflow requirement now says the same thing. A kept Dockerfile's `FROM` is where the conversion reads the version from, so the two agree; when `--airflow-version`, or the requirement an existing `pyproject.toml` already carries, names another Airflow than that `FROM`, the conversion refuses with the message above rather than write a project that would not start.

#### Your own Dockerfile

Astro Runtime publishes every image twice, under two tags of the same repository: `12.6.0` and `12.6.0-base`, `3.1-12-python-3.12` and `3.1-12-python-3.12-base`. The plain tag carries ONBUILD steps that, as soon as anything builds `FROM` it, install `packages.txt` and `requirements.txt` from the build context. The `-base` tag is the same image without them. A Dockerfile built on the plain tag therefore needs those two files beside `pyproject.toml`, and nothing keeps the three lists in step: `uv add` reaches standalone mode and not the image.

So a Dockerfile of your own builds on the `-base` tag and installs from `pyproject.toml`, with the steps a generated image runs written out ahead of your own:

```dockerfile
# Installs this project's packages from pyproject.toml and uv.lock
# (.astro/install-project.py reads them; astro init keeps both up to date).
FROM quay.io/astronomer/astro-runtime:12.6.0-base AS astro-install
USER root
COPY pyproject.toml uv.lock* .astro/install-project.py /tmp/astro-project/
RUN python /tmp/astro-project/install-project.py --project /tmp/astro-project --out /tmp/astro-install
FROM quay.io/astronomer/astro-runtime:12.6.0-base
COPY --from=astro-install /tmp/astro-install/packages.txt .
USER root
RUN if [ -x /usr/local/bin/install-system-packages ]; then /usr/local/bin/install-system-packages; elif [ -s packages.txt ]; then apt-get update && cat packages.txt | tr '\r\n' '\n' | sed -e 's/#.*//' | xargs apt-get install -y --no-install-recommends && apt-get clean && rm -rf /var/lib/apt/lists/*; fi
COPY --from=astro-install /tmp/astro-install/requirements.txt /tmp/astro-install/uv-lock-constraints.txt ./
RUN --mount=type=secret,id=netrc,target=/root/.netrc,required=false if [ -x /usr/local/bin/install-python-dependencies ]; then /usr/local/bin/install-python-dependencies; else pip install --no-cache-dir --root-user-action=ignore -r requirements.txt; fi
USER astro
COPY --chown=astro:0 . .

# This project's own steps.
ENV TZ=UTC
```

The first stage runs `.astro/install-project.py`, which reads `pyproject.toml` and `uv.lock` and writes what the runtime's own install steps read: your `[project] dependencies` without Airflow, a `[tool.uv.sources]` git or URL source as a direct reference, an index source as a `--find-links` line, `[tool.astro] packages`, and every package at the version `uv.lock` resolved, so the image installs what `uv sync` installs in standalone mode. The second stage is the runtime's ONBUILD steps, in the same order and as the same users, so the runtime's constraints still apply, and the `netrc` secret reaches the install on every runtime. Your own steps follow, and run as they did. `[tool.astro] packages` and your dependencies describe the image again: `uv add` and `astro local deps` change what the next build installs.

`astro init` writes this for you. Converting a 1.x project whose Dockerfile does more than name a base, it moves the file onto its tag's `-base` image, mirrors included (`registry.example.com/astronomer/astro-runtime:12.1.0` becomes `…:12.1.0-base`, so mirror that tag too), keeps every line after the original `FROM` in order, writes `.astro/install-project.py`, and removes `requirements.txt` and `packages.txt`, which are in `pyproject.toml` and which nothing reads any more. It adds an exception to `.dockerignore` for any of `pyproject.toml`, `uv.lock` and `.astro/install-project.py` the file would leave out. It leaves the file on its ONBUILD tag, keeps both lists, and says why in its notes, when the move would have to guess:

- a line of `requirements.txt` or `packages.txt` that `pyproject.toml` cannot carry (`--extra-index-url`, `-r`, `-c`, `-e`, a local path, a URL with no name): the file is its only record, and the ONBUILD tag still installs it;
- an earlier stage built on an ONBUILD tag, whose steps would still read the two files;
- a `FROM` pinned by digest, built from a build argument, or continued onto a second line;
- a Dockerfile that reads `requirements.txt` or `packages.txt` itself, or that has a stage named `astro-install`;
- a runtime older than 9 (Airflow 2.7), or a tag whose runtime version it cannot read.

Commit `.astro/install-project.py` with the Dockerfile: it is what `docker build .` runs, so a clone builds without the CLI. The file needs BuildKit, for the `netrc` secret's `RUN --mount`: the default builder in Docker Engine 23 and later and in Docker Desktop, and `DOCKER_BUILDKIT=1` before that. The install stage's `COPY` names `uv.lock*`, so a project without a `uv.lock` builds too: Docker (BuildKit and the legacy builder) and Podman 5.8 (Buildah 1.43) copy nothing for it. An older Buildah is untested there. With it, `docker build .` builds the image `astro package astro` builds, except for three things the CLI adds: `--platform linux/amd64`, the labels a Deployment reads, and the build secrets you pass with `--build-secret`, `BUILD_SECRET_INPUT` or [`build-secrets`](#build-secrets). The `netrc` mount says `required=false`, so a build without that secret is not warned about it.

The `FROM` names the runtime, and the seed of `uv.lock` follows it: a floating tag (`runtime:3.3-base`) seeds from the newest build it serves when the lock is seeded. If the image is later built on another runtime than the lock was made for (the floating tag moved on, or the `FROM` was edited without a refresh), the install step says so in the build output, naming both Airflows, and installs the lock's versions anyway. Run `astro init`, which seeds the lock again, to move it to the `FROM`'s runtime, or pin the `FROM` to an exact build so it cannot move.

A Dockerfile still on an ONBUILD tag is judged by whether `astro init` can move it. Run in a project that is already an Astro project, `astro init` brings what astro writes there up to date: it moves a declared Dockerfile on an ONBUILD tag onto `-base` under the gates above, plus one more, that every line of a `requirements.txt` or `packages.txt` beside it is already in `pyproject.toml` (the lists have had time to drift), and it writes or refreshes `.astro/install-project.py`. So:

- when `astro init` would move the file, every build of it refuses it (`onbuild_base`): `astro local start --docker` and `restart`, `astro package astro` and `astro deploy`. The message shows the lines that replace the `FROM`, the stage's name and flags kept, and says to run `astro init`, which does it. `astro local check` and `astro package mwaa` or `composer` build no image from the file, and are not held to it.
- when a gate stops the move, the build goes ahead on the ONBUILD tag as it always has, the tag's steps installing `requirements.txt` and `packages.txt`, and warns each time with what `astro init` would say: the line that is not in `pyproject.toml`, the earlier stage, the `.gitignore` rule.

Standalone mode builds no image, and is held to neither.

`.astro/install-project.py` carries a version stamp in its header (`# astro-install-helper: <n>`, raised whenever the file changes). `astro init` refreshes the project's copy only when it is older than the one this astro writes, or carries no stamp, and never replaces a newer astro's; a build warns about a copy that is missing or older, and says nothing about one at this version or newer, edits included.

### `build-secrets`

The build secrets the image build needs, as a list of `--build-secret` specs. Each entry names where a secret comes from and never holds the secret, so the list is safe to commit:

```toml
[tool.astro]
dockerfile = 'Dockerfile'
build-secrets = ['id=netrc,env=NETRC_CONTENT', 'id=pip,src=~/.config/pip/pip.conf']
```

`astro local start --docker`, `astro local restart` in Docker mode, `astro package astro` and `astro deploy` build with these specs. A `--build-secret` flag replaces the whole list. Without a flag, `BUILD_SECRET_INPUT` replaces it. The sources never merge: the first one that gives any secret is the whole set. Standalone mode builds no image and ignores the list without a warning.

Each entry takes an `id` and exactly one source: `env=` names an environment variable, and `src=` names a file. A `src=` has to be an absolute path or start with `~/`, which the CLI expands to your home directory, since Docker does not. A relative path is refused, because Docker would read it from whatever directory the command runs in. Keep the secret file outside the project, too: the build context is the project, so a file inside it would be copied into the image. An entry that is not a spec, such as a bare value or a `value=` key, is refused, and the error does not repeat the entry, in case it is the secret.

Without a declared `dockerfile`, the list can hold only a secret with id `netrc`. Any other id is refused when the manifest is read, by `astro local check` and every other command (`build_secrets_without_dockerfile`), for the reason the flag is refused there. A bad entry is `build_secret_invalid`.

When an image build runs, each `env=` source it uses has to be set, even when Docker could reuse the cached step that mounts the secret. If the variable is empty or not set, the command stops before it builds and names the secret's id. It names the variable too when the name is in upper case, since a `$VAR` the shell replaced with a token could look like a name. Without this check, Docker would mount nothing and the build would fail later, in the step that reads the secret. A spec that does not parse, or whose `env=` is not a variable name, is refused too, and the error does not repeat it.

`astro init` does not write this list, because a Dockerfile does not say where its secrets come from. For each secret a converted Dockerfile mounts, it prints a line under "Left to do" with the entry to add.

### `workspace`

The project's Astro workspace. It does three jobs:

- It is the default workspace every deployment link inherits when the link sets none, which saves repeating the same id on each link below.
- Its Environment Manager objects (environment variables, Airflow variables, connections) reach the project's local Airflow, declared or not, below every local source and above a declaration's default, and a `{ source = 'workspace' }` value in `[tool.astro.env]` resolves from it. See [`[tool.astro.env]`](#toolastroenv).
- It is the workspace `astro env` and `astro deployment` act on when run inside the project without `--workspace`, in place of the one `astro workspace switch` picked. See [Commands that manage Astro](workspace-link.md#commands-that-manage-astro).

**Experimental: the workspace link is set from Astro Desktop.** Link, switch or unlink the workspace there; it writes `workspace`, `domain` and `organization` together, and a switch or unlink first writes the old id onto each Astro link that was inheriting it, so no link moves to a workspace its Deployment is not in. The CLI reads all three, so a project linked in Astro Desktop behaves the same in both tools and for anyone who clones it, but the CLI has no command to set them, and they are not meant to be edited by hand. See [Setting the link](workspace-link.md#setting-the-link).

```toml
[tool.astro]
workspace = 'your-workspace-id'
```

### `domain`

The Astro host that `workspace` and deployment links with `method = 'astro'` live on, such as `astronomer.io`. It is the host you give `astro login`, and it is read the same way, so `https://cloud.astronomer.io/` also means `astronomer.io`. Values that resolve from the workspace are read with your login for this domain, even while the CLI is switched to another one, and so are Astro deployment links with `method = 'astro'`. `astro deploy` uses the same login, for a deployment link and for a Deployment named by bare id. If it is absent, it means `ASTRO_DOMAIN` if that is set, else `astronomer.io` for the workspace and your current login for deployment links. Setting it is an error when the project has no `workspace` and no deployment link with `method = 'astro'`, since nothing would use it. Astro Desktop writes it with the workspace. See [workspace-link.md](workspace-link.md).

```toml
[tool.astro]
workspace = 'your-workspace-id'
domain = 'astronomer.io'
```

### `organization`

The id of the Astro organization `workspace` is in. Optional. Values that resolve from the workspace are read under it, with your login for `domain`, even while that login is switched to another organization: a login reads every organization you belong to, so no `astro organization switch` is needed. If it is absent, the workspace is read under your login's current organization. Setting it is an error when the project has no `workspace` (`organization_without_workspace`), since it names the organization of the linked workspace. Astro Desktop writes it with the workspace. If the read is refused (403) or finds nothing (404) under this organization, the message names it and suggests `astro organization list`. `astro env` and `astro deployment` still act in your current organization. Deployment links do not use it. See [workspace-link.md](workspace-link.md).

```toml
[tool.astro]
workspace = 'your-workspace-id'
domain = 'astronomer.io'
organization = 'your-organization-id'
```

### `target`

The default target every deployment link inherits when the link sets none. A target names how the project builds and where it deploys; when neither a link nor this default names one, the link falls back to the built-in target `astro`. Backend configuration for a target lives in its own table, `[tool.astro.targets.<name>]` — a separate, plural key, because one TOML key cannot be both a string and a table.

```toml
[tool.astro]
target = 'astro'
```

## `[tool.astro.deployments.<name>]`

One table per Airflow the project talks to, keyed by a name you choose (`dev`, `prod`, and so on). This is the committed inventory: a teammate who clones the repo has the same list you do, with nothing to pass around. `astro deploy` ships to one of these links, and the commands that query an Airflow read the same table.

Inside the project, `astro env` and `astro deployment` take an Astro link's name wherever they take a Deployment id: `--deployment` and the Deployment-id argument of `inspect`, `logs`, `update`, `delete`, `hibernate` and `wake-up`. The command acts on the link's Deployment, in the link's workspace; see [Commands that manage Astro](workspace-link.md#commands-that-manage-astro).

A link says where the Airflow is and, when the default is not what you want, how to prove yourself to it. It never holds a credential — the values themselves come from the environment when a command runs.

There are five kinds of link, and the kind follows from what the link sets:

- an **Astro** link sets `deployment`, the Deployment id;
- an **MWAA** link sets `target = 'mwaa'` and `environment`, the MWAA environment name;
- a **Composer** link sets `target = 'composer'` and `environment`, the Composer environment name;
- an **endpoint** link sets `url`, the address of an Airflow with no control plane to ask — an instance on a VM, a platform team's shared Airflow;
- an **Astro Private Cloud** link sets `target = 'apc'`, `domain`, `workspace` and `deployment`: a Deployment on an Astro Private Cloud installation. See [Astro Private Cloud links](#astro-private-cloud-links).

A link names coordinates (`deployment` or `environment`) or a `url`, never both.

Pick any name you like, with two rules: it cannot be empty, and it cannot be `local`, which always means the Airflow running on your own machine. Keys the table does not define are errors rather than silence — misspell `default` on the link you meant to be the default and the CLI says so, instead of quietly resolving to another link.

### `deployment`

The Deployment id this link points at: an Astro Deployment's on an Astro link, an Astro Private Cloud Deployment's on an `apc` link. Required on both, and an error on an MWAA or Composer link, which name an `environment` instead.

### `environment`

The environment name on the platform the link's `target` names — the MWAA environment, the Composer environment. Required on an MWAA or Composer link, and an error on any other kind, which has no such name.

### `url`

The base URL of an Airflow the CLI cannot look up, like `https://airflow.staging.corp.dev`. It must be a full `http` or `https` address, host included. A link with a `url` needs an `auth` table: nothing about a URL says how its Airflow checks callers.

A `url` must not carry a username or password (`https://admin:hunter2@airflow.corp.dev` is an error). That is a credential, and credentials are named, never written — use `auth = { method = 'basic', username-env = 'AIRFLOW_USER', password-env = 'AIRFLOW_PASSWORD' }`.

### `workspace`

The workspace an Astro Deployment lives in. Optional if `[tool.astro]` sets a default `workspace`; the link's own value wins when it sets one. An Astro link with no workspace at either level is an error. On an `apc` link it is the Astro Private Cloud workspace, required on the link itself: the `[tool.astro]` default is an Astro workspace and never reaches an `apc` link. Only Astro and `apc` links have a workspace — MWAA, Composer and endpoint links are not in one, the `[tool.astro]` default does not reach them, and setting `workspace` on one is an error.

### `target`

The target to build and deploy this link with, and — for `mwaa` and `composer` — the platform the link points at. Optional: it falls back to the `[tool.astro]` default `target`, and then to the built-in `astro` when neither level names one.

A link's target must be `astro`, `mwaa`, `composer` or `apc`, spelled in lower case; anything else is an error, including a value inherited from `[tool.astro]`. (The package targets are a wider set — `astro package oss` builds an artifact for an Airflow there is nothing to link to.) Setting it to an empty string is an error too — an empty string means you meant something and got it wrong.

### `auth`

How the CLI proves itself to this link's Airflow, as an inline table: `auth = { method = '…', … }`. Where an Airflow is and how it checks callers are separate questions, so any method attaches to any kind of link — a self-hosted Airflow behind Google IAP is a `url` link with `method = 'google'`, an Airflow 3 under Keycloak a `url` link with `method = 'airflow-token'`.

The table is optional on every kind but the endpoint one, where there is no safe default. Left out, a link takes its kind's default: an Astro link `astro`, an MWAA link `aws`, a Composer link `google`, an `apc` link `apc`.

| `method` | Where the credential comes from | Fields |
| -- | -- | -- |
| `astro` | `ASTRO_API_TOKEN`, else your `astro login` session for [`domain`](#domain) (the current one if the project names no domain) | none |
| `aws` | the AWS credential chain (env, profile, SSO, instance role) | none |
| `google` | Application Default Credentials | none |
| `basic` | a username and password | `username-env`, `password-env` |
| `token` | a bearer token | `token-env` |
| `airflow-token` | credentials exchanged at the Airflow's own `/auth/token` | `client-id-env` with `client-secret-env`, or `username-env` with `password-env` — one pair, whichever your instance takes |
| `exec` | a command you run that prints a token | `command` |
| `none` | nothing is sent: local instances, open dev servers | none |
| `apc` | your `astro login` session for the `apc` link's [`domain`](#astro-private-cloud-links). Only an `apc` link names a domain, so the method is an error on any other kind | none |

Every field that carries a credential is a `*-env` name: it names the environment variable holding the value, never the value. No literal secret is legal in this file, and a field that would hold one does not exist. The variables themselves resolve the way every other value does — the shell, the project `.env`, `astro local env <noun> set` — so a teammate who is missing one is told which variable to set.

`exec`'s `command` is an array — the program and its arguments, as you would type them apart: `command = ['acme-airflow-token', '--profile', 'prod']`. The CLI runs the program directly and never through a shell, so a single string is an error rather than a quoting puzzle.

Anything else is an error: an unknown method, a field the method does not take, a missing credential field, half a credential pair, both credential pairs at once.

### `default`

Marks the project's default link. At most one link in the file may set it; two or more is an error. A project with a single link treats it as the default without the mark, and adding a second link writes the mark onto that first one, so the default does not vanish because a link was added; `astro link add` says so, and reports the link as `kept_default` under `--output json`. `astro link default NAME` moves it. `astro link default --unset` writes `default = false` on the link that was the default, which no later save marks again.

- `astro af` and the other commands that query an Airflow use it when nothing else names a link: no `-d`, no `ASTRO_DEPLOYMENT`, no `astro use` selection (see [instances.md](instances.md#resolution-order-for-astro-af)).
- `astro deploy` never ships to it on its own. An interactive deploy that names no link asks, with the default preselected; a run that cannot be asked must name one (see [deploy.md](deploy.md#3-selecting-the-deployment)).

```toml
[tool.astro]
workspace = 'your-workspace-id'

[tool.astro.deployments.dev]         # an Astro link
deployment = 'your-dev-deployment-id'
default = true

[tool.astro.deployments.prod-mwaa]   # an MWAA link
target = 'mwaa'
environment = 'orders-prod'

[tool.astro.deployments.staging]     # an endpoint link
url = 'https://airflow.staging.corp.dev'
auth = { method = 'token', token-env = 'STAGING_AIRFLOW_TOKEN' }

[tool.astro.deployments.legacy]      # self-hosted, behind Google IAP
url = 'https://airflow.internal.corp'
auth = { method = 'google' }

[tool.astro.deployments.bespoke]     # whatever your platform team ships
url = 'https://airflow.acme.internal'
auth = { method = 'exec', command = ['acme-airflow-token'] }
```

### Astro Private Cloud links

An `apc` link names a Deployment on an Astro Private Cloud (APC) installation, so a project can link APC Deployments beside Astro ones while it moves between them:

```toml
[tool.astro.deployments.prod-apc]
target = 'apc'
domain = 'astro.example.com'    # the APC base domain, as astro login takes it
workspace = 'your-apc-workspace-id'
deployment = 'your-apc-deployment-id'
default = true                  # optional
```

- **`domain` is required, on the link.** It is the APC base domain you give `astro login`, and it picks the login the link is reached with. It is written the way `astro login` stores an APC login: lower case, with any `https://` and trailing `/` removed. Unlike `[tool.astro] domain` it keeps a leading `cloud.` or `app.`, because an APC login is stored under the base domain exactly as given. `[tool.astro] domain` is Astro's host and never reaches an `apc` link, and `domain` on any other kind of link is an error (`domain_on_non_apc_link`), as any key a link does not take is.
- **`workspace` is required, on the link.** `[tool.astro] workspace` is an Astro workspace, so an `apc` link never inherits it.
- **`target = 'apc'` is what makes it one.** APC and Astro ids look alike, so the target, not the id, says which platform a link is on.
- **No `environment` or `url`.** An `apc` link names a Deployment, and its Airflow is that Deployment's. A link has no `organization` key on any kind.
- **What reads it today.** `astro link` writes and lists `apc` links, and `astro use` lists and selects them like any other. Under an APC context `astro deploy` ships to the `apc` links on that context's domain, as it ships to Astro links under an Astro one: named, or offered with the `default = true` one preselected, and a Deployment picked from the workspace is saved as an `apc` link (see [deploy.md](deploy.md#astro-private-cloud)). An `apc` link on another domain is refused, naming the `astro context switch` or `astro login` that reaches it; under an Astro context an `apc` link is never offered. The commands that query an Airflow (`astro af`, `astro api airflow`) refuse an `apc` link by name rather than reach for it, since querying an APC Deployment's Airflow is not supported yet.

Each problem has its own code: `domain_required`, `workspace_required` and `deployment_required` for a missing field, `environment_on_apc_link` and `url_on_apc_link` for a field an `apc` link does not take, `domain_on_non_apc_link` for a domain on another kind, and `auth_apc_on_non_apc_link` for the `apc` method on another kind.

### How links resolve

The two levels fold together at parse time, so by the time the CLI acts on a link every value is already filled in. The rules: an Astro link's `workspace` is its own value if it set one, else the `[tool.astro]` default, and it must end up with one; an `apc` link's `domain` and `workspace` are its own and never inherited; a link's `target` is its own value, else the `[tool.astro]` default, else the built-in `astro`; a link's auth method is its own `auth` table if it has one, else its kind's default; and at most one link may carry `default = true`.

### Linking from the command line

`astro link add <name>` writes a link, `astro link remove <name>` removes one, and `astro link default <name>` moves the `default` mark (`--unset` clears it). Astro Desktop's link editor writes through the same code, so both refuse a link the next read would not load, and both leave the rest of the file, comments included, as it was.

In a terminal, a missing argument is asked for instead: `astro link add` with no `--deployment` lists the Astro Deployments in the project's workspace (else your current one) with the picker `astro deploy` uses, and names the link after the one chosen, lowercased with each run of other characters turned into a dash; `remove` and `default` list the project's links. A run that cannot be asked, with no terminal or with `--output json`, never prompts and names the missing argument instead.

Under an Astro Private Cloud context, `astro link add` writes an `apc` link when no `--target` is given and `[tool.astro] target` is unset or `astro`, on the current context's domain. A project whose target is `mwaa` or `composer` keeps that kind, so `--environment` and its coordinates work as they do under Astro, unless `--deployment` is given: that names an APC Deployment, so the link is `apc`, as under Astro it is an Astro link. With no `--deployment` it lists the APC Deployments of `--workspace`, else the context's current workspace (never `[tool.astro] workspace`, which is Astro's), and names the link the same way. Otherwise the link's workspace is `--workspace`, else the context's. `--target mwaa`, `composer`, `astro` and `--url` still link those kinds there. Under an Astro context, `--target apc` is refused: the domain is the APC context's, so switch to it first.

```sh
astro link add prod --deployment your-prod-deployment-id
astro link add prod-mwaa --target mwaa --environment orders-prod --region eu-west-1
astro link add gcp --target composer --environment orders --project acme-data --location us-central1
astro link add staging --url https://airflow.staging.corp.dev --auth token --token-env STAGING_AIRFLOW_TOKEN
astro link add bespoke --url https://airflow.acme.internal --auth exec -- acme-airflow-token --profile prod
astro link add prod-apc --deployment your-apc-deployment-id   # under an Astro Private Cloud context
```

A link writes `target` and `workspace` only where they differ from what it inherits, so it keeps following `[tool.astro]` when that changes; a link that already sets one keeps it. The Composer `project` and `location` and the MWAA `region` go to the shared `[tool.astro.targets.<name>]` section one key at a time, and a Composer link that names neither takes the ones already there. `add` refuses a name already linked unless `--replace` is passed, and a replaced link keeps its `default` and, when `--workspace` is not given, the workspace it sets itself. A `*-env` flag is refused unless `--auth` names a method that reads it. `--output json` prints `{"name", "kind", "status", "manifest"}`, with `status` one of `added`, `replaced`, `removed`, `default`, `cleared` or `unchanged`.

## `[tool.astro.env]`

The environment the project expects. It declares three kinds of value: environment variables, Airflow variables, and connections, so a missing value is a clear error before Airflow boots rather than a failure deep in a task. The manifest carries non-secret defaults; each environment carries its own overrides; secrets never go inline.

The rule is one line: a string value is a committed default; a table means the value lives outside the manifest. Every declared name is required by default — it must resolve from somewhere or `astro local start` refuses with the list of what to set:

- A string is a committed **default**: `LOG_LEVEL = 'info'`. It sits at the bottom of the resolution chain, so any file value overrides it. An empty string is a real default (empty), not a marker — `X = ''` resolves to the empty value.
- An empty table is **required with no default**: `WAREHOUSE_URI = {}`. You must supply it — the shell, the project `.env`, or `astro local env <noun> set`.
- `{ source = 'workspace' }` also resolves from the workspace's Environment Manager values: environment variables, Airflow variables, and connections, read from the workspace named by `[tool.astro] workspace` with your login for `[tool.astro] domain`. Astro Desktop reads the same declarations from the same workspace. If a required one cannot be read, `astro local start` refuses and names the cause; `--allow-missing` starts without it. `workspace` is the only source today.

Everything that reaches a project goes to its Airflow, declared here or not: the project's own `.env` and vault secrets, every global vault entry linked to the project (`astro local env <noun> link` narrows which projects a global reaches), and the Environment Manager objects of the workspace `[tool.astro] workspace` links, below every local source. A start that cannot read the workspace goes on without its values and says so in one line, unless a required `source = 'workspace'` value needs it. A declaration is a requirement: a declared name nothing supplies stops `astro local start`, the value is validated against its type, `enum` and `conn_type`, and a `default` applies only when nothing supplies one. `astro local env list` shows an undeclared value the project gets as `not declared`, with the `declare` command, with source `workspace (<id>)` for one the workspace supplies, and `astro local check` and `astro package` name the local ones, because they will not follow the project to a Deployment or a teammate's clone, and say that the workspace's were not checked.

### Annotations

A table declaration can also describe the value. Every annotation is optional, and the string shorthand is exactly `{ default = ... }` with none of them.

| key | meaning |
| --- | --- |
| `default` | the committed default, same as the string shorthand. A string, number, or boolean — `default = 8080` is fine beside `type = 'port'` |
| `optional` | `true` exempts this one declaration from the required gate |
| `secret` | `true` means the value belongs in a vault, never a plaintext file. Not allowed with `default`; always on for connections. `sensitive` is refused as an unknown field, with a hint to use `secret` |
| `type` | `string` (the default), `int`, `number`, `bool`, `enum`, `url`, `port`, `json`. Checked against the resolved value — see below |
| `enum` | the allowed values; needs `type = 'enum'`, and vice versa |
| `description` | prose for whoever has to supply the value; `astro local env list` shows it |
| `conn_type` | the expected connection type — only under `[tool.astro.env.connections]`, and checked when the resolved connection's kind is known (see below) |

```toml
[tool.astro.env]
LOG_LEVEL = { default = 'info', type = 'enum', enum = ['debug', 'info', 'warn'] }
DB_PASSWORD = { secret = true, description = 'Warehouse password' }
SLACK_WEBHOOK = { optional = true, type = 'url' }
```

### Declaring from the command line

`astro local env <noun> declare <name>` adds a declaration or changes one, and `undeclare <name>` removes it, for each of the three nouns (`variable`, `connection`, `airflow-variable`). Only the annotations you pass change; the rest of the declaration, and the comments around it, stay as they are. Astro Desktop edits declarations through the same writer, so both refuse a change the next `astro local start` would not load, and leave the file untouched when they do.

| flag | annotation | nouns |
| --- | --- | --- |
| `--description <text>` | `description`; an empty string removes it | all |
| `--optional`, `--optional=false` | `optional` | all |
| `--source workspace`, `--source local` | `source`; `local` removes it | all |
| `--type <type>` | `type` for the variable nouns; `conn_type` for `connection`, as with `connection set --type` | all |
| `--enum a,b` | `enum`, and `type = 'enum'` unless `--type` says otherwise | `variable`, `airflow-variable` |
| `--secret`, `--secret=false` | `secret` | `variable`, `airflow-variable` |
| `--default <value>`, `--no-default` | `default` | `variable`, `airflow-variable` |

```sh
astro local env variable declare API_TOKEN --secret --description 'Token for the API'
astro local env connection declare warehouse --type snowflake --source workspace
astro local env airflow-variable declare batch_size --type int --default 500
astro local env variable undeclare OLD_FLAG
```

A name with a default is not given `--source workspace`, because a workspace-sourced name never falls back to its default; pass `--no-default` in the same command to drop it. An env-form key is declared by its plain name, so `airflow-variable declare AIRFLOW_VAR_REGION` declares `region`. `undeclare` removes only the declaration: a value already set for the name stays where it is, and `delete` removes it. The reverse holds too: `delete` removes only the value, and then says what the declaration leaves (another source supplying the name, an absent optional name, or a required one the next start will refuse without). `delete --undeclare` removes both, and edits only the current project's declaration, with `--global` too. `--output json` prints `{"kind", "name", "status", "manifest"}`, with `status` one of `declared`, `undeclared` or `unchanged`.

### Types are checked, and reported rather than enforced

A value that resolves to something its `type` did not promise is reported as a warning when you
start, and does **not** stop the start:

```
warning: env var PORT: expected a port between 1 and 65535, got "99999"
warning: connection warehouse: declared conn_type "snowflake" but resolved to "postgres"
```

Warning rather than refusing is deliberate. The value may well work, and `type` is documentation you
wrote for your own team — a tool that will not start your project because you annotated a port and
then set it to 99999 is arguing with you about your own note. A **missing** required value is the
opposite case and does stop the start: Airflow cannot run without it.

Two consequences worth knowing:

- **`optional` does not exempt a value from the type check.** It means "you need not set this", not
  "anything goes if you do". An optional value that is absent is fine; one that is present is
  checked.
- **A present but empty value is not checked.** An empty string satisfies the required gate, so
  refusing it here would have the two checks disagree about a value one of them deliberately allowed.

`string` and `json` accept anything. `json` is a deliberate non-check rather than an oversight:
these values are routinely templated (`{{ var.value.x }}`), so a value that is not valid JSON at
rest is normal.

`secret` marks a value as belonging in a vault rather than a plaintext file. Declare it rather than relying on a tool to guess from the name, because guessing is how a credential ends up in `.env`.

`astro local env <noun> set` stores every value in the vault shared with Astro Desktop, encrypted, declared or not, so the flag matters when `--plain` asks for an unencrypted copy: that is refused for a name declared secret. When a `--plain` set cannot read the declarations because the manifest does not parse, it is refused rather than guessing. A secret value **may not carry a `default`**: a default is committed to this file and injected into the environment at start, which is what the flag exists to prevent.

**Connections are always secret**, whether or not you say so, and saying `secret` under `[tool.astro.env.connections]` is an error rather than a no-op. A connection carries a credential by construction, so it may not carry a default either — including through the string shorthand.

`conn_type` is compared case-insensitively, and only when the resolved connection's kind could be
determined. That means a JSON-encoded connection carrying a `conn_type` field. A connection supplied
as a URI — `AIRFLOW_CONN_WAREHOUSE=snowflake://user:pass@acct/db`, Airflow's own documented form —
does not decode to a kind, so its `conn_type` is not compared; you get a separate complaint
that the stored value is not valid connection JSON. A JSON blob with no `conn_type` field is
likewise accepted against any declaration. Neither reports a false mismatch, but do not read a clean
start as proof that a `conn_type` matched.

A connection also **may not declare `type` or `enum`**. It declares its kind with `conn_type`: what
resolves for a connection is a URI or a JSON blob, reduced to a connection type before anything
judges it, so a `type` here could not be checked even in principle. Refused rather than ignored, so
it cannot look enforced when it is not.

**`optional` is a field, not a spelling of "give it an empty default".** `X = ''` resolves, and a resolved default is injected — so the variable would be *set to empty* rather than absent. `os.environ['X']` would succeed where it should raise, `os.environ.get('X', fallback)` would return `''` instead of the fallback, and an empty `AIRFLOW_CONN_*` is a malformed connection URI rather than a missing one. So a value that is genuinely optional and has no sensible default says `optional = true` and keeps its type, description and routing.

A declaration may be both `optional` and defaulted. The default already satisfies the run gate, so `optional` changes nothing there — but it still says the value is not one a person has to supply, which is what a UI listing the environment needs in order to mark it.

Unknown keys are an error, as they are everywhere in this section. That is why a typo'd `sensitiv` cannot quietly ship a secret to the wrong place.

Plain env vars live directly under `[tool.astro.env]`. Connections and Airflow variables use their own sub-sections, which pick the `AIRFLOW_CONN_` / `AIRFLOW_VAR_` encoding — a connection `warehouse` resolves as `AIRFLOW_CONN_WAREHOUSE`, a variable `batch_size` as `AIRFLOW_VAR_BATCH_SIZE`.

Airflow variables take the same spellings as plain env vars. **Connections do not take a committed default**, in either spelling: a connection URI carries a credential, so `warehouse = 'postgres://user:pass@host/db'` and `warehouse = { default = '...' }` are both refused. Declare the connection and supply its value from a file, the vault, or the workspace.

Resolution order, from first winner to last. The two vault tiers are the store shared with Astro Desktop, where `astro local env <noun> set` puts every value, encrypted, unless `--plain` asks otherwise (a plain global is kept there too, unencrypted); a plaintext `.env` still beats a vaulted value of the same name.

```
project .env > shell env > project vault > global vault > workspace EM > manifest default
```

```toml
[tool.astro.env]
LOG_LEVEL = 'info'                    # a committed default
API_TOKEN = { source = 'workspace' }  # from Environment Manager when logged in
WAREHOUSE_URI = {}                    # required; you provide it

[tool.astro.env.connections]
warehouse = {}                        # required; resolves as AIRFLOW_CONN_WAREHOUSE

[tool.astro.env.airflow_variables]
batch_size = '500'                    # default; resolves as AIRFLOW_VAR_BATCH_SIZE
```

```toml
# The same three sections, annotated.
[tool.astro.env]
LOG_LEVEL = { default = 'info', type = 'enum', enum = ['debug', 'info', 'warn'] }
SLACK_WEBHOOK = { optional = true, type = 'url', description = 'Alerts go here when set' }

[tool.astro.env.connections]
warehouse = { conn_type = 'postgres' }
```

## `[tool.astro.pools]`

The Airflow pools the project's Dags use. A task assigned to a pool that Airflow does not have waits in the queue and never runs, so a project that uses pools declares them here, and a local Airflow has them from its first start. Each key is a pool name.

```toml
[tool.astro.pools]
etl = {slots = 4, description = 'ETL loads'}
ml = {slots = 1, include_deferred = true}
default_pool = {slots = 16}
```

| key | required | what it sets |
| --- | --- | --- |
| `slots` | yes | How many tasks can run in the pool at once: a whole number above zero, or `-1` for no limit. |
| `description` | no | The text the Airflow UI shows for the pool. |
| `include_deferred` | no | `true` counts deferred tasks against the pool's slots. Airflow 2.7 and later. |

A pool name is at most 256 characters and cannot contain a slash, since the Airflow API reads the name from the URL path. Any other key in a pool's table is an error.

`astro local start` and `astro local restart` create or update each pool once Airflow answers, in standalone and Docker mode. Details:

- **Only what the manifest sets.** A pool already as declared is not written. When you leave out `description` or `include_deferred`, the pool keeps the value it has in Airflow.
- **Pools the manifest does not list stay.** Nothing is deleted, so a pool you make in the Airflow UI survives a restart. To remove a pool, delete it from the table and from Airflow.
- **`default_pool`** is the pool Airflow makes itself and every task uses when it names none. You can set its `slots` and `include_deferred`. You cannot set its `description`, since Airflow 3 does not let a caller change it.
- **A failure does not stop the start.** A pool that Airflow refuses prints one warning, `warning: pool etl was not created or updated: ...`, and Airflow keeps running.

`astro init` carries the pools in a 1.x project's `airflow_settings.yaml` into this table. See [converting a 1.x project](install.md).

## `[tool.astro.targets.<name>]`

Backend-specific configuration for one target, kept as plain data. A target section means something only to its own backend, so the manifest carries it through untyped and each backend reads its own section.

Note the plural: the default-target key above is `target` (a name), and this is `targets` (a table of config). One TOML key cannot be both a string and a table, so the two are spelled apart.

An MWAA or Composer link takes the rest of its coordinates from its target's section: the link names the environment, the section says where that environment lives. These are the keys those sections take:

```toml
[tool.astro.targets.mwaa]
region = 'us-east-1'                   # where the environment lives; AWS_REGION when unset
bucket = 's3://acme-airflow-orders'    # named in the upload `astro package mwaa` prints

[tool.astro.targets.composer]
project = 'acme-data'                  # the GCP project holding the environment
location = 'us-central1'               # its region
```

Any other key in those two sections, or a section named for no target, is a warning: the manifest still loads, and `astro local start` and `restart` print the key, since a misspelled `region` reads the same as no region at all. It is a warning rather than an error, unlike an unknown key elsewhere in `[tool.astro]`, because these sections are open by design: each backend owns its own section.

`astro` and `oss` are targets too, but nothing reads their sections yet. They are reserved: carried through as they are and never warned about, until a reader gives them a shape.

## `[tool.uv]`

uv's own table, and yours. uv applies it to the project's environment: `astro local start` builds that environment with `uv sync`, and `uv run`, `uv add` and `uv lock` read the same table, so they all agree.

`astro init` writes settings into it that hold the project to what a deployment runs: Astronomer's build of Airflow, such as `3.3.2+astro.2`, which differs from PyPI's `3.3.2`. It holds the versions of everything else the runtime image ships in `uv.lock` instead, which it seeds (see below). For the pin `apache-airflow==3.3.*` it writes:

```toml
[tool.uv]
environments = ["sys_platform == 'linux'", "sys_platform == 'darwin' and platform_machine == 'arm64'"]
constraint-dependencies = [
    'apache-airflow==3.3.2+astro.2',
    'apache-airflow-task-sdk==1.3.2+astro.1',
]

[tool.uv.sources]
apache-airflow = {index = 'astronomer'}
apache-airflow-core = {index = 'astronomer'}
apache-airflow-task-sdk = {index = 'astronomer'}

[[tool.uv.index]]
name = 'astronomer'
url = 'https://pip.astronomer.io/v2/'
explicit = true

[tool.uv.exclude-newer-package]
apache-airflow = false
apache-airflow-core = false
apache-airflow-task-sdk = false
```

- **The pins** are the ones the runtime image pins. The release is the one the runtime catalog says a deployment runs: the one the [`runtime`](#runtime) build carries when it is set, and otherwise the one the newest runtime of the requirement's series carries. The `+astro` build is the one the image's package list names, or, without that list, the newest of the release on the index.
- **The index is explicit**, so uv takes only the three packages the sources name from it. Every other package still comes from PyPI, or from the indexes you add.
- **`environments`** limits the lockfile to the platforms Airflow runs on locally. Without it, uv also locks for Intel macOS, where one package's marker (shap's `numba<0.63`) pulls numba and llvmlite back to releases that do not build. A project that names its own `environments` keeps them. On an Intel Mac, `astro local start` stops before uv and says so: run Docker mode with `--docker`, or add `"sys_platform == 'darwin' and platform_machine == 'x86_64'"` to the list. Windows is left out too: standalone does not run there, so a Docker-mode project on Windows that runs `uv sync`, `uv run` or `uv add` on the host needs `"sys_platform == 'win32'"` added.
- **`exclude-newer-package`** turns any `exclude-newer` cutoff off for the three packages, `false` for each. Astronomer's index shows upload times only in the text of its pages, which uv does not read, so under a cutoff, the project's own or a `UV_EXCLUDE_NEWER`, uv would treat every build as too new. This is why the CLI needs uv 0.9.25 or later.

A standalone `astro local start` keeps these settings current. When the pin or `runtime` moves, or Astronomer ships a newer build of the release, it rewrites them and says so. It leaves your other keys and your comments alone, and it never takes over an index named `astronomer` at another address, or a source you wrote for one of the three packages: it warns and writes nothing instead. When Astronomer's index has no build the requirement covers, it takes out the pins, the sources, the index and the `environments` it wrote, and Airflow comes from PyPI; the bare `apache-airflow-core` and `apache-airflow-task-sdk` stay, except on Airflow 2, which has neither. When the index cannot be read, it leaves the settings as they are, unless the pin has moved off the build's release, in which case it takes out everything it wrote, as for an index with no build.

### The lockfile is seeded with the runtime image's versions

`astro init` seeds `uv.lock` with the version of every package the runtime image ships, from the package list Astronomer publishes for each build and Python (`https://cdn.astronomer.io/runtime-freeze/runtime-<build>-python-<python>.txt`; set `ASTRO_RUNTIME_FREEZE_URL` to read the same file names from a mirror, or to an address that does not answer to stop the request). It locks the project in place with those versions added to `pyproject.toml` as constraints, puts `pyproject.toml` back as it was, and locks once more, where uv keeps every version the lock holds; it reports how many packages the lock then holds at the image's versions. None of them stays in `pyproject.toml`. Locking in place means the project locks as it always does: a dynamic version, a build backend, relative `find-links` or index paths, `${PROJECT_ROOT}` and a `uv.toml` all apply. `pyproject.toml` is written as every other write of it is: through a symlink to the file it points at, and never when it is read-only for you. The constraints sit between a comment that begins `Astro Runtime <build> (Python <version>) ships these versions` and one that begins `End of what astro writes`, so if the process is killed before it puts the file back, the next `astro init` or start takes them out, and they are never read as your own. A member of a uv workspace is not seeded: uv locks a workspace once, at its root, so the member has no `uv.lock` of its own; the CLI says so. Without the seed uv takes the newest of each, such as SQLAlchemy 2.1 beside an image that holds it to 2.0. A seed adds no package: only the ones your project pulls in are held.

From then on uv keeps the versions itself. `uv add`, `uv lock` and `uv sync` prefer the versions a lock already holds, so they change one only where a requirement needs another. `uv lock --upgrade`, or deleting `uv.lock`, lets them go on purpose; run `astro init` to seed the lock again.

- **The build** is named in one place: a declared [`dockerfile`](#dockerfile)'s `FROM` (the newest build a floating tag such as `runtime:3.3` serves), else [`runtime`](#runtime), else the newest build carrying a release the requirement covers (for `apache-airflow==3.3.1`, a build of 3.3.1). The Airflow pin above is of the same release; when a Dockerfile's `FROM` carries a release the requirement excludes, nothing is seeded and the CLI says so. The Python is the one the project runs: the one its standalone venv and its generated image run, or a declared Dockerfile's image's. For a patch pin, whose build is older than the series' newest, that can be a Python the build publishes no list for; nothing is seeded then, and the CLI says so. When Astronomer's index has no build of the pin, Airflow comes from PyPI, and the runtime build's versions still seed the lock while it carries a release the pin covers.
- **To choose a version yourself, give the package a bound the image's version doesn't meet.** A package you name in `dependencies`, `[project.optional-dependencies]`, `[dependency-groups]` (such as the `dev` group `uv add --dev` writes), `[tool.uv] dev-dependencies` or `constraint-dependencies` is seeded at the image's version while what you wrote admits it: `pandas`, and `pandas>=2.3.3` as `uv add pandas` writes it, lock pandas at the image's 2.3.3. A bound it doesn't meet (`pandas>=3`) leaves it to uv. An environment marker is left out of the judgement: `tomli>=1; python_version < '3.11'` is seeded when the image's tomli is 1 or later. A direct reference (`name @ url`) or `===` leaves the package to uv, since the CLI doesn't weigh it. A package named in `[tool.uv] override-dependencies` is never seeded.
- **When:** `astro init` seeds the lock every time it runs, in a new project and an existing one. A standalone `astro local start` seeds one when the project has no `uv.lock`, when the `astro-runtime` group gains packages the lock may not hold, or when the Airflow pin moved off the release of the build the lock was seeded from. A newer build of the same release does not re-seed on a start: the lock, its record and the group stay that build's until `astro init` moves them together. With a lock there, it compares the lock's versions of the image's packages with the image's, and when some differ for no reason your own requirements give, it says `uv.lock: N packages differ from Astro Runtime <build>; run astro init to re-seed`. That is how a newer build of the runtime shows up too.
- **The record** of which build and Python the lock was seeded from is the first comment of the `astro-runtime` group (below): `# astro seeded uv.lock from Astro Runtime 3.3-9 (Python 3.14). Run astro init to seed it again.` The CLI reads it, and so does a build of the project: it is what Docker mode and a deploy build on. Leave it in place. The record and the group of a new build are written only once its seed has worked.
- **When not, and the record comes out:** under the project's own cutoff (`exclude-newer` in `[tool.uv]` or in its `uv.toml`), since the list is of the present; for a runtime that publishes no list (**Airflow 2 builds before runtime 11.0.0**, an answer remembered for a day); and when the list's Airflow is another release than the Airflow pin's. The lock stays as uv has it.
- **When nothing changes:** a start that cannot reach Astronomer's index or the package list (offline, a timeout, a mirror that does not answer), or that runs under a cutoff of this machine's (`UV_EXCLUDE_NEWER`, or `exclude-newer` in your own `uv.toml`: `$XDG_CONFIG_HOME/uv/uv.toml` or `~/.config/uv/uv.toml`, `/etc/uv/uv.toml`, or `UV_CONFIG_FILE`), leaves the lock and its record as they are. It says so when it would have seeded (`astro init`, or a start with no `uv.lock`); a start with a lock, which only compares, says nothing of a list it did not need. So does a list that cannot be read while the Airflow requirement still covers the release the lock was seeded beside. A seed needs `uv` on the `PATH`, and says so without it. A seed that fails, or is interrupted, leaves `pyproject.toml` and `uv.lock` exactly as they were, and the record and the group with them. A start does not try a seed again that failed, held none of the image's versions, or could not write its group, while `pyproject.toml`, `uv.lock`, the build and the Python are the same (for a day); `astro init` always tries.

To add a dependency, run `uv add <package>`. A running standalone Airflow picks the package up. A package the image ships locks at the image's version, since the lock already holds it. To take a different one, say `pandas>=3` where the image ships pandas 2, run `uv add 'pandas>=3'`: uv moves pandas, and only what pandas 3 needs moved with it. The image then installs your version over its own.

An older `astro` wrote the image's versions into `constraint-dependencies` itself, between a comment that began `Astro Runtime <build> (Python <version>) ships these versions` and one that began `End of what astro writes`, or wrote the seed record there. The next `astro init` or start takes them out and seeds the lock in their place; your own lines around them stay.

**The build the lock was seeded from is the build every image uses.** With no `runtime` and no Dockerfile, a Docker-mode start and `astro package astro` build `FROM` that build's `-base` image, the one the seed record names, not the newest of the series, so the image is the runtime your `uv.lock` was made for. A deploy uses it when the Deployment offers it, and otherwise the newest build the Deployment offers, with a warning naming both. Moving to a newer build is a step you take: `astro init` seeds the lock again from the newest build, the [`astro-runtime` group](#the-runtime-images-packages-the-astro-runtime-group) with it, and says so ("uv.lock: seeded from Astro Runtime 3.3-10 now, not 3.3-9"), and the images follow on their next build. A start does not move it: it says how many packages the lock holds apart from the image it would seed from now, and a Docker-mode start and `astro package astro` say when a newer build is available ("Astro Runtime 3.3-10 is available; this project builds on 3.3-9. Run astro init to move to it"). An `astro init` that seeds nothing (a cutoff, no list), or whose seed fails, still runs `uv lock` in a project with a `uv.lock`, so `astro init` is the one thing to run for a lock that is out of date; without uv it says to install it. The image runs the Python the lock was seeded for (the record names it), not the build's default, so it is the image the lock was made for. A pin moved to another release before the lock is seeded again leaves the image to the pin (by the catalog's word of the build's Airflow, not only the `+astro` pin beside it), and so does a `requires-python` that excludes the Python the lock was seeded for, or admits none of the Pythons the seeded build ships: the image then builds on the newest build of the series, with a warning, rather than failing. Any build, `docker build .` included, compares the image's Airflow with the lock's and says in its output when the lock was made for another runtime (see [`dockerfile`](#dockerfile)).

### The runtime image's packages: the `astro-runtime` group

The runtime image installs more than Airflow: providers, database drivers, `redis`, `gunicorn`, `pytest`, and with them pandas, numpy, kubernetes and the rest of what they pull in. A DAG that imports one works on a Deployment and in Docker mode without the project listing it. So that it works in standalone mode too, `astro init` and a standalone `astro local start` write the image's top-level packages, the ones nothing else it installs requires, into a dependency group of their own, and add it to `default-groups`, which `uv sync`, `uv run` and standalone mode install:

```toml
[dependency-groups]
astro-runtime = [
    # astro seeded uv.lock from Astro Runtime 3.3-9 (Python 3.14). Run astro init to seed it again.
    # The packages that image installs, so the local environment has what it has. astro writes them; to choose a version yourself, list the package in [project] dependencies.
    'aiofiles',
    'apache-airflow-providers-celery',
    …
]

[tool.uv]
default-groups = ['dev', 'astro-runtime']
```

- **Names only.** The seeded lock holds each one, and everything it pulls in, at the image's version, so the local environment has the image's packages at the image's versions. The group of a build the lock is seeded from is written together with the seed's record, once the seed has worked, so a seed that fails leaves the group, the record and `uv.lock` as they were. A group that gains a package seeds the lock again; one that only loses a package, because the project now lists it, does not. The image itself has them already, so the group changes nothing a build installs.
- **The list** is the one Astronomer publishes for the build and Python (`https://cdn.astronomer.io/runtime-roots/runtime-<build>-python-<python>.txt`; set `ASTRO_RUNTIME_ROOTS_URL` to read the same file names from a mirror, or to an address that does not answer to stop the request). For a build that does not publish one yet, uv works it out: one `uv pip compile` of the build's packages, held to its versions, whose annotations say which of them another one pulls in. Left out either way: Airflow, which the project lists; pip; and Astronomer's platform packages, every `astronomer-*` but `astronomer-providers` (the executor, logging, the FAB security manager, the version check, runtime extensions), which plug into Airflow itself rather than into a DAG.
- **Only where it installs.** The image builds a package from source when it has to, with system libraries a laptop may lack: `mysqlclient` publishes no wheel but Windows', and building it needs MySQL's client library. So uv decides, per package and per platform an Astro project locks for (Linux on x86_64 and aarch64, macOS on Apple silicon), whether it installs with everything it pulls in from wheels alone, on the Python the project runs: a `uv pip compile --only-binary` of the package, with the build's versions. What Airflow pulls in may build, since the project installs Airflow anyway. A package that publishes no wheel at all, for any platform (Windows included) or Python, is taken for pure Python and let build: `json-merge-patch`, which `apache-airflow-providers-google` pulls in, is one. One with wheels somewhere but not for a platform here is native, and holds the package that pulls it in out; one with wheels only for older Pythons is native too, not built for this Python yet, and is asked about again after a week. A package that installs everywhere is listed as it is, one that installs on some platforms with a marker naming exactly those, and one that installs on none is left out: `astro init` and a start that changes the group say which and give uv's reason (`apache-airflow-providers-mysql` pulls in `mysqlclient`, which has no wheel on Linux), and `uv add <name>` installs it anyway, on a machine where its build works. Docker mode and a deployment still have every package.
- **Kept per build.** Every answer, the list and each package's platforms, is of one build and Python and never changes, so it is kept in the CLI's cache. uv runs with your own configuration, so a mirror you resolve through serves it. The first start on a build asks uv about each package, a few seconds; later ones ask nothing. When uv cannot be asked (no network, or uv not installed), the group stays as it is, with a warning, and the next start asks again; a run that ends without an answer, uv finding no resolution or the three minutes it is given running out, is not tried again for an hour, from the same mirror.
- **A package you list yourself** in `dependencies`, an extra or another group leaves the group: the project installs it itself. It is still seeded at the image's version while your requirement admits it.
- **To go without the group**, take `astro-runtime` out of `default-groups`. The CLI keeps the group current and never puts it back in `default-groups`: it adds it there only when it creates the group, empty or not, and never deletes the group, so the choice stays. While it is out, the start says the group lists the image's packages rather than installs them. A project whose `default-groups` is `'all'` installs it anyway.
- **It follows the seed.** A seed is of one build and Python, and the group is that image's packages, read from the same list. Nothing to seed with empties the group: a runtime with no package list (Airflow 2 before 11.0.0), or a list of another Airflow release. A list that cannot be read, or an index that cannot be reached, keeps the group as it is. The group is never taken out: an empty one stays, under comments that say none of the image's packages apply, so a choice made in `default-groups` stays with it, and a project that never had a group gets one only to hold the record. A seed kept as it is keeps the group. The one exception is a cutoff, a `UV_EXCLUDE_NEWER` or the project's own `exclude-newer` (in `[tool.uv]` or its `uv.toml`): nothing is seeded and the group stays as it is, since it holds names alone, which any cutoff resolves. A `[dependency-groups]` written as an inline table cannot hold the group's comments, so it gets no group and no record, with a warning; the lock is seeded all the same, once: a start does not seed again for a group it cannot write.
- **Known limits.** A package that publishes only a source distribution is taken for pure Python and let build; a native one published so (none is in the current images: every such package in the `3.3-9` and `12.12.0` images was checked for compiled files) would fail to build on a laptop without its system libraries, and `uv add` of it there fails the same way. When the list is worked out rather than published, two top-level packages that each pull the other in are each read as pulled in, and both drop out of the group; `uv add` installs either.

### Private packages without a Dockerfile

`uv add git+https://github.com/acme/private-lib.git` adds `private-lib` to `dependencies` and its repository to `[tool.uv.sources]`. Standalone mode installs it through uv. A generated image and a check environment cannot read `[tool.uv.sources]`, so the CLI (in an image, its install helper) writes a dependency whose source is a git repository or a URL as a direct reference, `private-lib @ git+https://github.com/acme/private-lib.git@<rev>`, with the source's `rev`, `tag` or `branch` and its `subdirectory`. A version on the dependency is dropped, since a direct reference cannot carry one. A source limited by `marker`, `extra` or `group`, a `path` and a `workspace` member are left as the bare name.

To install from a private repository, give the build a `netrc` secret. The generated build mounts it as `/root/.netrc` while it installs your requirements, and git reads its credentials from there:

```toml
[project]
dependencies = ['apache-airflow==3.3.*', 'private-lib']

[tool.astro]
packages = ['git']
build-secrets = ['id=netrc,env=NETRC_CONTENT']

[tool.uv.sources]
private-lib = { git = "https://github.com/acme/private-lib.git", rev = "4bfeaf8" }
```

Set `NETRC_CONTENT` to a netrc entry, such as `machine github.com login x-access-token password <token>`. List `git` in [`packages`](#packages) if the runtime image does not have it. The mount is the generated build's own step, so it works on every runtime, including those whose own `ONBUILD` install never mounted it. It is written only when the build has the secret, and then needs BuildKit under Docker, or Podman 3.2 or later.

A source that names an index, `private-lib = { index = "acme" }` with a `[[tool.uv.index]]` named `acme`, reaches a generated image in one of two ways. With a `uv.lock`, the image installs the exact file the lock recorded for `private-lib`, by its URL on the index and its hash, so PyPI is never asked for it. Without one, it is a `--find-links` line for that package's page on the index (`<url>/private-lib/`), the way `astro local check` passes it, and the install reads the page besides PyPI rather than instead of it: a package with the same name on PyPI is a candidate too, which is how a dependency-confusion attack gets in. Commit a `uv.lock` for a project with a private index. Either way, only a source for a package the image installs counts: the Airflow sources `astro init` writes add nothing, since the runtime image already is Airflow. An index behind a login takes the same `netrc` secret.

`astro local check` builds its check environments outside the project, where uv cannot see the table, so it passes `constraint-dependencies` and the source index pages to them itself, and a check resolves the way the project does. A check against MWAA or Composer leaves out the `+astro` pins: those platforms run Apache's own Airflow.

```toml
[tool.uv]
constraint-dependencies = ['pandas<3']
```

A project on Airflow 3.1 whose lock was not seeded (locked before seeding existed, or under a `UV_EXCLUDE_NEWER` date) that fails to import with an error naming `ScalarAttributeImpl` has resolved SQLAlchemy 2.1, which the SQLAlchemy-Utils that Airflow 3.1 depends on does not support yet ([sqlalchemy-utils#800](https://github.com/kvesteri/sqlalchemy-utils/issues/800)): add `constraint-dependencies = ['sqlalchemy<2.1']` here, or move the project to Airflow 3.2 or later.

## A complete example

This mirrors the [`examples/etl-demo`](../examples/etl-demo) project — a real, runnable Astro project. The `[tool.ruff]`, `[tool.ty]`, and `[dependency-groups]` sections are standard Python tooling the CLI does not read; they are here to show that an Astro project is a normal Python project and its tools live in the same file. The CLI also ignores `[project] version`, which Python packaging uses but the manifest does not read.

```toml
[project]
name = 'etl-demo'
version = '0.1.0'
requires-python = '>=3.10'
dependencies = [
    "apache-airflow==3.1.*",  # the Airflow this project runs: the one place the version is stated
    "apache-airflow-providers-standard",  # Airflow 3's home for the classic operators (Bash, Python, Empty)
]

[tool.astro]
packages = ['libpq-dev']  # OS (apt) packages; installed in Docker mode, skipped with a warning in standalone mode
workspace = 'your-workspace-id'  # default workspace every deployment link inherits; replace with your real id

[tool.astro.deployments.dev]  # one table per Airflow the project talks to; replace the placeholder ids
deployment = 'your-dev-deployment-id'
default = true  # the query commands' fall-through, and the entry astro deploy highlights

[tool.astro.deployments.prod]
deployment = 'your-prod-deployment-id'

[dependency-groups]  # standard Python tooling from here down; the CLI does not read it
dev = ["ruff", "ty", "pytest"]  # uv sync installs this group by default

[tool.ruff]
line-length = 100
target-version = "py310"

[tool.ruff.lint]
select = ["E", "F", "I", "UP"]  # pycodestyle, pyflakes, import sorting, pyupgrade

[tool.ty.environment]
python-version = "3.10"

[tool.ty.rules]
invalid-argument-type = "ignore"  # Airflow's @task decorator returns XComArg, so a checker flags every TaskFlow chain
```
