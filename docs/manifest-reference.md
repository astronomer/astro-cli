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

It picks **the build**. Docker mode, `astro deploy` and `astro package` build `FROM runtime:3.3-8` instead of `runtime:3.3`, and the `[tool.uv]` pins that standalone mode installs from follow the Airflow release `3.3-8` carries, when the requirement covers it. It never stands in for the requirement: a requirement that pins no series is refused whether or not `runtime` is set, and to change the Airflow version you change the requirement. `astro init` and a conversion never write it.

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

Set it and **that file is the build**: the Airflow requirement, `packages` and your other dependencies stop describing the image, because the Dockerfile decides what goes in. Install your Python and OS packages inside the file itself. The Airflow requirement is still required and still read — standalone mode installs it, and it tells the CLI which Airflow generation to run (the service set differs between 2 and 3) — but it no longer picks the image. [`runtime`](#runtime) is refused beside it, since the `FROM` line picks the base.

The file's `FROM` and the requirement have to agree, or Docker mode runs one Airflow and standalone installs another. When the final stage's `FROM` names an Astro Runtime tag, it is compared with the requirement — by series for an Airflow 3 tag (`runtime:3.2-4` beside `apache-airflow==3.3.*`), by generation for an Airflow 2 one — and a disagreement is refused, naming both lines and the fix:

```
tool.astro.dockerfile: Dockerfile builds FROM astrocrpublic.azurecr.io/runtime:3.2-4, which is Airflow 3.2, and the requirement apache-airflow==3.3.* in [project] dependencies is Airflow 3.3: Docker mode would run one and standalone the other. To fix it, change the requirement to apache-airflow==3.2.*, or change the FROM line to a 3.3 runtime
```

It is refused wherever the project runs, in both modes: `astro local start` and `restart` (standalone included, since that is where the requirement is what runs), `astro local check`, `astro package` and `astro deploy`, and Astro Desktop, which offers the requirement change as a fix (`dockerfile_airflow_mismatch`). A `FROM` that cannot be compared is let through: one built from a build argument (`FROM ${BASE}`), one pinned by digest, one that is not an Astro Runtime image, and one with no tag.

Every path that builds an image honours it: `astro local start --docker`, `astro deploy`, `astro package`, and Astro Desktop. Standalone mode has no image and ignores it, so a project pinned to a Dockerfile gets no benefit from it there.

The build context is the whole project directory, and only `.dockerignore` (or a `<Dockerfile>.dockerignore` beside the file, which Docker reads instead) keeps anything out of it. An Astro Runtime base copies the rest into the image (its ONBUILD `COPY . .`), including the per-machine files Astro tools write: the virtualenv, `.env`, and standalone Airflow's database, config and logs under `.astro/standalone/`. When `astro init` keeps a Dockerfile, it creates that ignore file or adds each of these lines it does not already exclude, and leaves the lines already there alone:

```
.venv/
.env
.astro/standalone/
.astro/worktrees/
.astro/*.local.yaml
.astro/*.local.yml
.astro/otto/*.local.json
.astro/otto/mcp.json
plugins/fix_local_executor_pickle.py
```

If you added `dockerfile` to the manifest by hand, add the lines yourself. Until you do, every `astro` build from the Dockerfile warns and names the per-machine files it would copy in.

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

Without the flag, each of these commands reads `BUILD_SECRET_INPUT`, one secret per line, the way `astro deploy` does. Without either, they use [`build-secrets`](#build-secrets), so a project whose build always needs a secret can declare it once. The CLI passes on only the spec and Docker reads the value itself, so the CLI never writes the value to disk. For that reason, a flag or `BUILD_SECRET_INPUT` has to be given again on each later start or restart that builds the image. A generated image installs your dependencies through the runtime image, which mounts only a `netrc` secret, so a flag with any other id is refused there rather than accepted and dropped. A `BUILD_SECRET_INPUT` line with another id is dropped for a generated image, since a CI runner may set the variable for every job. `astro package` refuses it for a bucket target too. Standalone mode builds no image, so it warns and starts without the secret.

Before each of these commands builds, it reads the Dockerfile for `RUN --mount=type=secret` steps. For each secret id that no flag, `BUILD_SECRET_INPUT` line or `build-secrets` entry supplies, it prints a warning that names the id, the line and the flag. It is a warning and not a refusal, because a secret mount is optional unless it sets `required=true`. If the build then fails, the build output has usually pushed the warning out of sight, so the final error names the missing ids and the flag again.

A conversion sets this for you when it finds a Dockerfile it cannot carry — one doing more than naming a base image. A Dockerfile that only pins a runtime is retired instead, since the Airflow requirement now says the same thing, except in a project that deploys to Astro Private Cloud, where it is kept but not declared because APC's `astro deploy` still builds it (see [converting a 1.x project](install.md#step-4-check-the-conversion-and-finish-it)). A kept Dockerfile's `FROM` is where the conversion reads the version from, so the two agree; when `--airflow-version`, or the requirement an existing `pyproject.toml` already carries, names another Airflow than that `FROM`, the conversion refuses with the message above rather than write a project that would not start. A Dockerfile kept undeclared for APC is refused the same way, with a message of its own, since the project would deploy one Airflow and run another.

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

There are four kinds of link, and the kind follows from what the link sets:

- an **Astro** link sets `deployment`, the Deployment id;
- an **MWAA** link sets `target = 'mwaa'` and `environment`, the MWAA environment name;
- a **Composer** link sets `target = 'composer'` and `environment`, the Composer environment name;
- an **endpoint** link sets `url`, the address of an Airflow with no control plane to ask — an instance on a VM, a platform team's shared Airflow.

A link names coordinates (`deployment` or `environment`) or a `url`, never both.

Pick any name you like, with two rules: it cannot be empty, and it cannot be `local`, which always means the Airflow running on your own machine. Keys the table does not define are errors rather than silence — misspell `default` on the link you meant to be the default and the CLI says so, instead of quietly resolving to another link.

### `deployment`

The Astro Deployment id this link points at. Required on an Astro link, and an error on an MWAA or Composer link, which name an `environment` instead.

### `environment`

The environment name on the platform the link's `target` names — the MWAA environment, the Composer environment. Required on an MWAA or Composer link, and an error on any other kind, which has no such name.

### `url`

The base URL of an Airflow the CLI cannot look up, like `https://airflow.staging.corp.dev`. It must be a full `http` or `https` address, host included. A link with a `url` needs an `auth` table: nothing about a URL says how its Airflow checks callers.

A `url` must not carry a username or password (`https://admin:hunter2@airflow.corp.dev` is an error). That is a credential, and credentials are named, never written — use `auth = { method = 'basic', username-env = 'AIRFLOW_USER', password-env = 'AIRFLOW_PASSWORD' }`.

### `workspace`

The workspace an Astro Deployment lives in. Optional if `[tool.astro]` sets a default `workspace`; the link's own value wins when it sets one. An Astro link with no workspace at either level is an error. Only an Astro link has a workspace — MWAA, Composer and endpoint links are not in one, the `[tool.astro]` default does not reach them, and setting `workspace` on one is an error.

### `target`

The target to build and deploy this link with, and — for `mwaa` and `composer` — the platform the link points at. Optional: it falls back to the `[tool.astro]` default `target`, and then to the built-in `astro` when neither level names one.

A link's target must be `astro`, `mwaa`, or `composer`, spelled in lower case; anything else is an error, including a value inherited from `[tool.astro]`. (The package targets are a wider set — `astro package oss` builds an artifact for an Airflow there is nothing to link to.) Setting it to an empty string is an error too — an empty string means you meant something and got it wrong.

### `auth`

How the CLI proves itself to this link's Airflow, as an inline table: `auth = { method = '…', … }`. Where an Airflow is and how it checks callers are separate questions, so any method attaches to any kind of link — a self-hosted Airflow behind Google IAP is a `url` link with `method = 'google'`, an Airflow 3 under Keycloak a `url` link with `method = 'airflow-token'`.

The table is optional on every kind but the endpoint one, where there is no safe default. Left out, a link takes its kind's default: an Astro link `astro`, an MWAA link `aws`, a Composer link `google`.

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

Every field that carries a credential is a `*-env` name: it names the environment variable holding the value, never the value. No literal secret is legal in this file, and a field that would hold one does not exist. The variables themselves resolve the way every other value does — the shell, the project `.env`, `astro local env <noun> set` — so a teammate who is missing one is told which variable to set.

`exec`'s `command` is an array — the program and its arguments, as you would type them apart: `command = ['acme-airflow-token', '--profile', 'prod']`. The CLI runs the program directly and never through a shell, so a single string is an error rather than a quoting puzzle.

Anything else is an error: an unknown method, a field the method does not take, a missing credential field, half a credential pair, both credential pairs at once.

### `default`

Marks the project's default link. At most one link in the file may set it; two or more is an error. A project with a single link treats it as the default without the mark.

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

### How links resolve

The two levels fold together at parse time, so by the time the CLI acts on a link every value is already filled in. The rules: an Astro link's `workspace` is its own value if it set one, else the `[tool.astro]` default, and it must end up with one; a link's `target` is its own value, else the `[tool.astro]` default, else the built-in `astro`; a link's auth method is its own `auth` table if it has one, else its kind's default; and at most one link may carry `default = true`.

### Linking from the command line

`astro link add <name>` writes a link, `astro link remove <name>` removes one, and `astro link default <name>` moves the `default` mark (`--unset` clears it). Astro Desktop's link editor writes through the same code, so both refuse a link the next read would not load, and both leave the rest of the file, comments included, as it was.

In a terminal, a missing argument is asked for instead: `astro link add` with no `--deployment` lists the Astro Deployments in the project's workspace (else your current one) with the picker `astro deploy` uses, and names the link after the one chosen, lowercased with each run of other characters turned into a dash; `remove` and `default` list the project's links. A run that cannot be asked, with no terminal or with `--output json`, never prompts and names the missing argument instead.

```sh
astro link add prod --deployment your-prod-deployment-id
astro link add prod-mwaa --target mwaa --environment orders-prod --region eu-west-1
astro link add gcp --target composer --environment orders --project acme-data --location us-central1
astro link add staging --url https://airflow.staging.corp.dev --auth token --token-env STAGING_AIRFLOW_TOKEN
astro link add bespoke --url https://airflow.acme.internal --auth exec -- acme-airflow-token --profile prod
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

`astro init` writes a few settings into it that hold the project to the Airflow a deployment runs. A deployment runs Astronomer's build of Airflow, such as `3.3.2+astro.1`, which differs from PyPI's `3.3.2`. For the pin `apache-airflow==3.3.*` it writes:

```toml
[tool.uv]
environments = ["sys_platform == 'linux'", "sys_platform == 'darwin' and platform_machine == 'arm64'"]
constraint-dependencies = ['apache-airflow==3.3.2+astro.1', 'apache-airflow-task-sdk==1.3.2+astro.1']

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

- **The pins** are the ones the runtime image pins. The release is the one the runtime catalog says a deployment runs: the one the [`runtime`](#runtime) build carries when it is set, and otherwise the one the newest runtime of the requirement's series carries. The catalog does not name the build, so the newest `+astro` build of that release on the index is taken. The image also pins SQLAlchemy, but nothing published says to which release without pulling the image, so that line is not written.
- **The index is explicit**, so uv takes only the three packages the sources name from it. Every other package still comes from PyPI, or from the indexes you add.
- **`environments`** limits the lockfile to the platforms Airflow runs on locally. Without it, uv also locks for Intel macOS, where one package's marker (shap's `numba<0.63`) pulls numba and llvmlite back to releases that do not build. A project that names its own `environments` keeps them. On an Intel Mac, `astro local start` stops before uv and says so: run Docker mode with `--docker`, or add `"sys_platform == 'darwin' and platform_machine == 'x86_64'"` to the list. Windows is left out too: standalone does not run there, so a Docker-mode project on Windows that runs `uv sync`, `uv run` or `uv add` on the host needs `"sys_platform == 'win32'"` added.
- **`exclude-newer-package`** turns any `exclude-newer` cutoff off for the three packages, `false` for each. Astronomer's index shows upload times only in the text of its pages, which uv does not read, so under a cutoff, the project's own or a `UV_EXCLUDE_NEWER`, uv would treat every build as too new. This is why the CLI needs uv 0.9.25 or later.

A standalone `astro local start` keeps these settings current. When the pin or `runtime` moves, or Astronomer ships a newer build of the release, it rewrites them and says so. It leaves your other keys and your comments alone, and it never takes over an index named `astronomer` at another address, or a source you wrote for one of the three packages: it warns and writes nothing instead. When Astronomer's index has no build the requirement covers, it takes out the pins, the sources, the index and the `environments` it wrote, and Airflow comes from PyPI; the bare `apache-airflow-core` and `apache-airflow-task-sdk` stay, except on Airflow 2, which has neither. When the index cannot be read, it leaves the settings as they are, unless the pin has moved off the build's release, in which case it takes out everything it wrote, as for an index with no build.

To add a dependency, run `uv add <package>`. It keeps the pins, and a running standalone Airflow picks the package up.

### Private packages without a Dockerfile

`uv add git+https://github.com/acme/private-lib.git` adds `private-lib` to `dependencies` and its repository to `[tool.uv.sources]`. Standalone mode installs it through uv. A generated image and a check environment cannot read `[tool.uv.sources]`, so the CLI writes a dependency whose source is a git repository or a URL as a direct reference, `private-lib @ git+https://github.com/acme/private-lib.git@<rev>`, with the source's `rev`, `tag` or `branch` and its `subdirectory`. A version on the dependency is dropped, since a direct reference cannot carry one. A source limited by `marker`, `extra` or `group`, a `path` and a `workspace` member are left as the bare name.

To install from a private repository, give the build a `netrc` secret. The runtime image mounts it as `/root/.netrc` while it installs your requirements, and git reads its credentials from there:

```toml
[project]
dependencies = ['apache-airflow==3.3.*', 'private-lib']

[tool.astro]
packages = ['git']
build-secrets = ['id=netrc,env=NETRC_CONTENT']

[tool.uv.sources]
private-lib = { git = "https://github.com/acme/private-lib.git", rev = "4bfeaf8" }
```

Set `NETRC_CONTENT` to a netrc entry, such as `machine github.com login x-access-token password <token>`. List `git` in [`packages`](#packages) if the runtime image does not have it. Runtime images older than the `netrc` mount install without the secret; when such a build fails, the error names the runtime image and says so.

`astro local check` builds its check environments outside the project, where uv cannot see the table, so it passes `constraint-dependencies` and the source index pages to them itself, and a check resolves the way the project does. A check against MWAA or Composer leaves out the `+astro` pins: those platforms run Apache's own Airflow.

```toml
[tool.uv]
constraint-dependencies = ['pandas<3']
```

A project on Airflow 3.1 that fails to import with an error naming `ScalarAttributeImpl` has resolved SQLAlchemy 2.1, which the SQLAlchemy-Utils that Airflow 3.1 depends on does not support yet ([sqlalchemy-utils#800](https://github.com/kvesteri/sqlalchemy-utils/issues/800)): add `constraint-dependencies = ['sqlalchemy<2.1']` here, or move the project to Airflow 3.2 or later.

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
