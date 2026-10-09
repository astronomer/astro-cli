# `astro deploy` and `astro package`

A project here is a directory whose `pyproject.toml` has a `[tool.astro]` table (see the [manifest reference](manifest-reference.md)). This page describes how such a project deploys to an Astro Deployment, and how `astro package` builds the artifact Astro, MWAA or Cloud Composer consumes without shipping it.

Terms used below:

- An **image deploy** ships the project's code and dependencies as a container image. A **dags-only deploy** ships only `dags/` as a tarball and leaves the running image in place. A **both** deploy (the default) does both.
- The **transport** is the set of calls that talk to Astro: create the deploy, push the image to the Deployment's registry, upload the DAG tarball, finalize.
- The **runtime image** is Astronomer's Airflow base image (`astrocrpublic.azurecr.io/runtime`). Building `FROM` it runs its `ONBUILD` steps, which install the project's `requirements.txt` and `packages.txt`.

## Routing: a project or a 1.x project

`astro deploy` looks at the working directory. A `pyproject.toml` with a `[tool.astro]` table takes the manifest path, even if the table fails to validate (the deploy then reports the manifest error). Anything else takes the 1.x path, `deploy.Deploy` in `internal/platform/astro/deploy`, which is unchanged: it deploys a project made by Astro CLI 1.x (a 1.x project). A Dockerfile is not evidence of a 1.x project: a project with a manifest may declare one (`[tool.astro] dockerfile`). The check is `HasManifest` in [`internal/project`](../internal/project/project.go), and the fork is in [`cmd/astro/deploy.go`](../cmd/astro/deploy.go).

The manifest path lives beside the 1.x path in the same package (`manifest.go`, `manifest_image.go`) and in [`internal/deploy`](../internal/deploy/deploy.go). It calls the 1.x path's transport pieces directly instead of wrapping its `Deploy()`, which prints, rewrites `.dockerignore` and assumes the 1.x layout.

## 1. Image deploys

The image is built by [`pkg/imagebuild`](../pkg/imagebuild/imagebuild.go), the same builder Docker-mode `astro local start` and `astro package astro` use, so the three produce the same image from the same manifest. `imagebuild.ForManifest` picks the build:

- **Generated (the common case).** A one-line `FROM astrocrpublic.azurecr.io/runtime:<tag>` Dockerfile, plus a `requirements.txt` holding `[project] dependencies` without `apache-airflow`/`apache-airflow-core` (`manifest.WithoutAirflow`) and a `packages.txt` holding `[tool.astro] packages`. A deploy and a package then copy the project in over it (see [What the image carries](#what-the-image-carries)). The tag is `[tool.astro] runtime` when set, else the series of the Airflow requirement (`runtime:3.3`). When `[project] requires-python` does not admit that build's default Python, the tag is instead the exact build's Python flavor, `runtime:3.3-8-python-3.13`, for the newest Python the build ships that it admits; with no `runtime` set, the build is the catalog's newest of the series. A `requires-python` admitting none of the build's Pythons is refused before the build, and without a readable catalog the default Python is kept (`imagebuild.RuntimeImageForPython`). The Python is `runtimeversions.ProjectPython`'s choice, which a standalone environment of the same manifest runs too (`imagebuild.StandalonePython`) (see [`requires-python`](manifest-reference.md#requires-python)). Deploy builds only Airflow 3 bases from a generated build.
- **Declared Dockerfile.** When `[tool.astro] dockerfile` is set, that file is the build, with the project directory as context. No base is resolved.

### What the image carries

A deploy and `astro package astro` ship the project in the image, the way the 1.x CLI built it: with the **project directory as the build context**, so Docker applies the project's `.dockerignore` and its own symlink rules (a symlink is copied as a link, and nothing outside the project can be reached). Which DAGs go in depends on where they run:

| | the project (`plugins/`, `include/`, `tests/`, top-level code such as `utils/`) | `dags/` |
| --- | --- | --- |
| deploy to a Deployment with DAG deploys | in the image | uploaded as the DAG tarball, not in the image |
| deploy to a Deployment without DAG deploys | in the image | in the image; nothing is uploaded |
| deploy to a Deployment with remote execution (DAG deploys on or off) | in the image | not in the image, and not uploaded |
| `--image-name` (a prebuilt image) | as built | whatever the image carries; uploaded too to a Deployment with DAG deploys |
| declared Dockerfile | as its `COPY` lines decide | as its `COPY` lines decide; uploaded to a Deployment with DAG deploys |
| `astro package astro` | in the image | in the image |
| Docker-mode `astro local start`, Astro Desktop | not copied: mounted into the containers | not copied: mounted |

This is the 1.x path's rule (`buildImage` and `buildImageWithoutDags` in `internal/platform/astro/deploy/deploy.go`): the image is the project, with `dags/` left out for a Deployment that takes DAG deploys or runs remote execution, and an image deploy uploads no DAGs under remote execution.

How it is built. The runtime image's `ONBUILD` triggers read `requirements.txt` and `packages.txt` from the main build context and end in `COPY . .`, so the main context of the first build is the small generated one, as it has always been, tagged `<image>:<tag>-deps`. A second build starts `FROM` that image (whose triggers have run, so none fire again) with the project as its context and runs `COPY --chown=astro:0 . .`. Its ignore file is written beside the generated Dockerfile as `<Dockerfile>.dockerignore`, which BuildKit reads instead of the context's `.dockerignore` (Podman is pointed at it with `--ignorefile`), so the project's file is read and never rewritten. It holds the project's `.dockerignore`, then the rules the CLI always adds, which a `!` in the project's file cannot undo: `**/.venv`, `**/.env`, `**/__pycache__`, `**/*.pyc`, `.git`, `.astro`, `plugins/fix_local_executor_pickle.py`, and the project's own top-level `requirements.txt` and `packages.txt` (the generated ones are what the image installed, and they stay). Then `dags/`, when the DAGs are not to be in the image. The `-deps` tag is removed after the second build. The copy is the last layer, after the dependency installs, so editing a DAG or a plugin reinstalls nothing.

**An ignore file that leaves `dags/` out is refused when the DAGs are to be built in.** For a Deployment without DAG deploys, a `.dockerignore` rule that leaves out `dags/` (or everything in it) would ship an image with no DAGs to a Deployment that runs only the image's. The 1.x path removed a `dags/` line from the project's `.dockerignore` before every image deploy; this one edits nothing and refuses instead, for a generated build and a declared Dockerfile alike: `found a dags entry in the project's .dockerignore. …` `astro package astro` builds anyway and warns.

The text summary says where the DAGs went: uploaded (with the bundle version), built into the image by this deploy, left to the image a prebuilt `--image-name` or a declared Dockerfile carries, or not deployed because the Deployment runs remote execution.

Before anything is built, the deploy checks the project:

- the Dockerfile's `FROM` agrees with the Airflow requirement (`scaffold.CheckDockerfileAirflow`);
- the build secrets resolve, and the local files a declared Dockerfile would copy into the image are warned about;
- a `[tool.astro] runtime` build agrees with the requirement, against the runtime catalog.

The pipeline, in order (`DeployManifestImage` in [`manifest_image.go`](../internal/platform/astro/deploy/manifest_image.go)):

1. Check for a running container engine. Without one the deploy stops (see [Without Docker](#5-without-docker)).
2. Log in with the login for the project's host (see [Selecting the Deployment](#3-selecting-the-deployment)).
3. Collect git metadata (see [Command surface](#6-command-surface)).
4. Fetch the Deployment and check it can take this deploy: CI/CD enforcement, and the runtimes it offers. Its DAG deploy setting decides whether `dags/` goes into the image (see [What the image carries](#what-the-image-carries)). `--image` to a Deployment without DAG deploys is refused, as on the 1.x path: its DAGs are inside the image, so an image-only deploy cannot leave them in place. Deploy without `--image` to ship the image with `dags/` in it, or enable DAG deploys (`astro deployment update <id> --dag-deploy enable`).
5. Check the planned runtime before the build. The planned runtime is `[tool.astro] runtime`, else the base's tag without any `-python-X.Y` (the series, or the exact build a Python flavor names), or for a declared Dockerfile the runtime its final `FROM` names (`airflowrt.ReadDeclaredBase`). An exact version gets the full rules: no downgrade, a version the Deployment offers, and the Airflow 2→3 floor. A series is refused only when it is older than the Deployment's, or the Deployment offers no build of it. A downgrade error names the line to change. This makes a bad pin fail in seconds rather than after a long build.
6. Build for `linux/amd64` and print "Building your project image, this can take a few minutes...".
7. Read `io.astronomer.docker.runtime.version` off the built image and run the runtime rules again on it. The label is the authority, because the requirement states what the project intends and the label states what the image is.
8. Create the deploy, push the image as user `cli` with the session token, upload the DAG tarball for a both deploy to a Deployment with DAG deploys and no remote execution, finalize, and wait for health with `--wait`.

**A deploy to Astro requires an Astro Runtime base.** An image without the runtime label is refused, never deployed on the requirement's word:

- `--image-name`: `image "<ref>" is not based on Astro Runtime (the … label is missing); build it FROM the Astro runtime`
- a declared Dockerfile: `the image built from <Dockerfile> is missing the … label, so it is not based on Astro Runtime; build it FROM an Astro Runtime image`

This is the one limit on a declared Dockerfile. It builds and runs locally on any base, but deploys to Astro only on a runtime base.

`--image-name <ref>` deploys a prebuilt local image and skips the build. It is how CI deploys an image `astro package` built (see [The Astro target](#the-astro-target)).

## 2. Dags-only deploys

`astro deploy --dags` tars `<project>/dags`, uploads it to the URL the create-deploy call returns, and finalizes. A Deployment without DAG deploys has nowhere to upload to, so it is refused there; a plain `astro deploy` ships the DAGs inside the image instead. It reads the Deployment's runtime version and type from the API, not from the manifest, because the DAGs must fit the image already running. That version decides symlink validation (Airflow 3), the monitoring DAG's Airflow major version, and whether files sit under `dags/` or at the bundle root (`--no-dags-base-dir`). It never builds an image and never needs Docker.

## 3. Selecting the Deployment

The manifest's `[tool.astro.deployments.<name>]` links name the Deployments a project ships to (see the [manifest reference](manifest-reference.md#toolastrodeploymentsname)). `astro deploy` ships only to Astro links. A named MWAA, Composer or endpoint link is refused: `link "<name>" is <kind>, and astro deploy ships to Astro Deployments`. Use `astro package` for those platforms.

**Deploy never picks for you.** Shipping code is too consequential to decide from a marker, a pinned selection or an exported variable, so:

- **The argument or `--deployment` names a link or an id.** `astro deploy prod` ships to the `prod` link. A value no link declares is used as a Deployment id, so `astro deploy <id>` (what astronomer/deploy-action runs) keeps working. Naming both with different values is an error. There is no `-d` shorthand for `--deployment`: on this command `-d` means `--dags`.
- **With nothing named, deploy asks.** The prompt (on stderr) is the CLI's numbered picker table of the project's Astro links. `ASTRO_DEPLOYMENT`, then your `astro use` selection, then the `default = true` link preselect the highlighted entry, labelled in its `PRESELECTED BY` column with whichever put it there (`ASTRO_DEPLOYMENT`, `astro use`, `default = true`), and the prompt shows its number (`> [2]`). Enter takes it. None of them can skip the question, and the prompt does not change your `astro use` selection. A project with exactly one link preselects it as its default. An answer is a link name or a number, exactly as listed, names first, so a link called `2` selects itself. Three bad answers give up.
- **A run that cannot be asked fails.** With no terminal or with `--output json`, a deploy that names nothing stops: `a deploy must name the deployment it ships to: astro deploy <name> or --deployment <name>. …`. Under `--output json` it is the error object with kind `input_required` (see [Prompts](architecture.md#prompts)).
- **A project that links nothing** needs a workspace (`--workspace`, else the current context), then runs the 1.x path's workspace flow: pick a Deployment, or create one (`deployment.GetDeployment`). A run that cannot be asked must pass `--deployment`.
- **A project whose links are all non-Astro** is refused, naming what it declares, rather than offered an unrelated Deployment.

**Workspace.** `--workspace`, else the link's workspace (its own, or the `[tool.astro]` default it inherits), else the current context's. `--workspace-id`, the older spelling, still works and is hidden from help; given with `--workspace`, the two must agree.

**The project's host picks the login.** A project that names an Astro host (`[tool.astro] domain`, else `ASTRO_DOMAIN`) deploys with the login stored for that host, even while the CLI is switched to another one. That login is used for every step: the Deployment lookup, the deploy record, the registry push, the DAG upload, finalize and `--wait`, whether the Deployment was named by link or by id. With no login for that host, the deploy stops before it builds and names `astro login <domain>`. A project that names no host, or names the current one, uses the current context. The unlinked workspace flow lists Deployments with the current context only, so on another host it refuses and asks for `--deployment <id>`. The same login rule is in [workspace-link.md](workspace-link.md#deployment-links).

## 4. `astro package`

`astro package [target]` builds the artifact one Airflow platform consumes. It never ships: it reads the manifest and project files, needs no account and no link, and touches the network only to pull a base image. That makes it the build stage of a CI pipeline. The default target is `astro`.

Each target implements one interface in [`internal/pack`](../internal/pack/pack.go), so adding a platform is adding a file. A target:

- checks the manifest against its platform and **warns** rather than fails where the platform cannot honor part of it (an Airflow pin it does not offer, OS packages, an `include/` folder with files), because the artifact is still valid;
- builds an artifact of a named kind: an `image`, a `tree` (a directory laid out for a bucket sync) or a `bundle`;
- writes it to a predictable place and reports the exact next command to ship it.

| target | kind | status |
| --- | --- | --- |
| `astro` | image | built |
| `mwaa` | tree | built |
| `composer` | tree | built |
| `oss` | — | registered; errors `the "oss" package target is registered but not built yet` |

An unknown target lists the known ones.

### The Astro target

The image build from [section 1](#1-image-deploys), without the transport: resolve the base, build `linux/amd64` (`--platform` overrides), read the runtime label, stop. It needs Docker.

A generated image carries the project, `dags/` included (see [What the image carries](#what-the-image-carries)). `dags/` goes in because a package cannot know the DAG mode of the Deployment it will be deployed to with `--image-name`: one without DAG deploys runs the DAGs in the image, and a default deploy to one with them uploads the working directory's `dags/` as well, which replaces the image's.

- **Tag:** `astro-package/<project name>:<runtime>-<hash>`, plus a moving `astro-package/<project name>:latest`. The 7-character hash covers the base, the platform, the dependencies, the OS packages, what a generated build copies from the project (each path the build's ignore rules leave in, with its kind, its executable bit and its contents or link target; other mode bits, which a checkout or umask changes, do not count, and an excluded directory is not read), and a declared Dockerfile's path and contents, so the same inputs give the same tag. `--tag <ref>` replaces the name and writes no `:latest`.
- **`--save <file>.tar`** also runs `docker save`, for uploading the image as a CI artifact.
- **Declared Dockerfile without a runtime label:** the image is still produced, with a warning that `astro deploy` will refuse it.
- **Declared env values** are not in the image. When the manifest declares any, the build warns and lists them: set them on the Deployment.
- **`--build-secret`** works as it does for `astro deploy` (see [`build-secrets`](manifest-reference.md#build-secrets)).

The CI story is two jobs. Package: `astro package --save image.tar` and upload the file (or `--tag <registry>/…` and `docker push`). Deploy: `docker load < image.tar` (or pull), then `astro deploy prod --image-name astro-package/<name>:<tag>`. Package-then-deploy and a direct deploy build the same image from the same code.

### The `mwaa` target

MWAA builds its environment from an S3 prefix, so the target writes a tree to `dist/mwaa` (`--out-dir` overrides; the directory is wiped first):

- `dags/`, copied as-is.
- `requirements.txt`: MWAA's `--constraint "https://raw.githubusercontent.com/apache/airflow/constraints-<airflow>/constraints-<python>.txt"` line for the matched version, then the dependencies without Airflow. When MWAA does not offer the pinned Airflow, the file carries a commented constraint template and the versions to pick from, with a warning.
- `plugins.zip`, only when `plugins/` holds real files (a lone `.gitkeep` does not count).
- `ENV_SETUP.md`, only when `[tool.astro.env]` declares values: the checklist of what to set on the environment, each marked `(from workspace)`, `(has a default)`, `(optional)` or `(required)`.

The next steps it prints: `aws s3 sync dist/mwaa/ s3://<bucket>/` (the bucket from `[tool.astro.targets.mwaa] bucket`, with or without `s3://`, else a placeholder), then `aws mwaa update-environment` with the new `requirements.txt` object version, and the `plugins.zip` one when plugins ship. `--save <file>.zip` also writes the tree to one zip.

### The `composer` target

Composer reads `dags/` and `plugins/` from its bucket, but takes PyPI packages as an environment setting, so the tree at `dist/composer` makes the two-part hand-off explicit:

- `dags/`, copied as-is.
- `plugins/`, as a folder, only when it holds real files.
- `composer-requirements.txt`: the dependencies without Airflow and with no constraint line, since Composer resolves against its own image. A header comment names the command that reads it.
- `ENV_SETUP.md`, as for MWAA.

The next steps it prints: `gcloud composer environments storage dags import --environment=<env> --location=<region> --source=…/dags` (and `… plugins import` when plugins ship), then `gcloud composer environments update <env> --location=<region> --update-pypi-packages-from-file …/composer-requirements.txt`.

### Pre-flight: `astro local check --target`

`astro local check --target <name>` checks the project against the Airflow a platform actually runs, before an upload. It maps the manifest's Airflow pin to a version the target offers (from [`pkg/platformversions`](../pkg/platformversions)), builds a scratch virtualenv with that Airflow and the project's dependencies, and runs the same DAG checks as a plain `astro local check`: import errors, duplicate DAG ids, slow parses.

- **Targets:** `astro` (the plain check), `mwaa`, `composer`. The flag repeats and takes a comma list.
- **Mapping:** a pin the platform offers resolves to it. A pin it does not offer maps *down* to the closest lower version, with a note naming both, because that is what would run there. A pin below everything it offers is an error.
- **MWAA constraints:** `--target mwaa` also resolves the dependencies with uv against MWAA's published constraints for the matched version. A conflict fails the target with uv's message. When the constraints cannot be fetched, that step is skipped with a note and the DAG checks still stand. Composer has no published constraints, so it has no such step.
- **Python:** MWAA's Airflow 3 runs Python 3.12 and the check provisions it. Composer does not publish its Python, so the check uses the default interpreter and notes the skew.
- **Cache:** scratch environments live under the astro cache directory (`check-venvs/`), keyed by Airflow, Python and the full requirement set. The check prints whether it is reusing one or provisioning one. Unused ones are removed after 14 days.
- **Exit codes:** `0` clean, `1` findings (or an MWAA constraints conflict), `2` an operational problem. With several targets the worst code wins.
- **`--output json`:** one object per target per line: `target`, `airflow_checked`, `mapped_from`, `notes`, `constraints`, `findings`, `dags`, `errors`, `warnings`, `error`.

## 5. Without Docker

An image deploy needs a container engine (the `container.binary` setting picks Docker or Podman). Without one, `astro deploy` stops before any transport work:

```
an image deploy needs Docker, but no running container engine was found. Start Docker and try again, or run 'astro deploy --dags' to deploy just your DAGs (no Docker needed). Server-side builds are coming
```

The check runs for `--image-name` too. A dags-only deploy never needs Docker, and neither do the `mwaa` and `composer` package targets. `astro package astro` refuses the same way. A server-side build, where the CLI uploads the context and the control plane builds, is the long-term answer for users without Docker and is not built.

## 6. Command surface

Both commands follow the CLI's output rules: text by default, `--output json` from the same value (see [Output](architecture.md#output)).

```
astro deploy [LINK-NAME | DEPLOYMENT-ID] [flags]
  --deployment <name|id>  the link name or Deployment id to deploy to
  --workspace <id>        the workspace (overrides the link's and the context's)
  -d, --dags              deploy only dags/ (no image build, no Docker)
  --image                 deploy only the image, leave DAGs untouched (needs DAG deploys)
  -i, --image-name <ref>  deploy a prebuilt local image; skips the build
  --no-dags-base-dir      put DAG files at the bundle root rather than under dags/
  --description <text>    description recorded on the deploy
  --build-secret <spec>   a build secret, repeatable (not with --dags or --image-name)
  -w, --wait              wait for the Deployment to report healthy
  --wait-time <dur>       how long --wait waits (default 5m)
  --output text|json

astro package [TARGET] [flags]    # astro (default), mwaa, composer, oss
  --save <file>           also write the artifact to one file (.tar for astro, .zip for a tree)
  --tag <ref>             image reference for the astro target
  --platform <p>          build platform for the astro target (default linux/amd64)
  --out-dir <dir>         tree directory for bucket targets (default dist/<target>)
  --build-secret <spec>   a build secret, repeatable (astro target only)
  -o, --output text|json
```

The 1.x-only flags (`--save`, `--pytest`, `--env`, `--test`, `--parse`, `--deployment-name`, `--dags-path`, `--dag-bundle-name`) are refused on the manifest path with the alternative to use. `--force` and `--prompt` are accepted and do nothing.

**Git metadata.** With the `deploy.git_metadata` setting on (the default), the deploy records the HEAD commit on Astro, and the commit message becomes the description when `--description` is not given. `commit_url` is set only for a GitHub remote. A project with uncommitted changes to tracked files records no commit and prints `note: the project has uncommitted changes, so this deploy records no git commit` on stderr. `--image-name` records none.

**`astro deploy --output json`** prints one object when the deploy finishes. It streams no progress events, so the object is a result like any other (see [Output](architecture.md#output)): indented and colored on a terminal, one line when piped. Its shape is pinned by `cmd/astro/testdata/schema/deploy.json`.

```json
{
  "deployment": "clx…",
  "link": "prod",
  "workspace": "clw…",
  "type": "image-and-dag",
  "image_tag": "deploy-2026-07-23T18-40",
  "dag_bundle_version": "3-1690000000",
  "runtime_version": "3.1-2",
  "url": "https://cloud.astronomer.io/…",
  "git": {
    "commit_sha": "0123abc…",
    "branch": "main",
    "commit_url": "https://github.com/…/commit/0123abc…"
  }
}
```

`deployment`, `workspace` and `type` (`dag-only`, `image-only` or `image-and-dag`) are always present. The rest are omitted when they do not apply: a dags-only deploy has no `image_tag`, an image-only deploy no `dag_bundle_version`, nor does an `image-and-dag` deploy that uploaded none (a Deployment without DAG deploys, or with remote execution), a Deployment named by id no `link`, a deploy with no recorded commit no `git`.

**`astro package --output json`** streams the build's log lines as `{"event":"log",…}` objects, then prints one result object:

```json
{
  "target": "astro",
  "kind": "image",
  "image": "astro-package/my-project:3.1-2-a1b2c3d",
  "runtime_version": "3.1-2",
  "saved_path": "image.tar"
}
```

`target` and `kind` are always present. An image carries `image`, `runtime_version` and, with `--save`, `saved_path`. A tree carries `tree_path`, `deps_file` and `next_steps`. A bundle carries `bundle_path`. `size`, when present, is the saved file's size in bytes, and `warnings` lists the target's warnings, including undeclared local env values that will not follow the project.
