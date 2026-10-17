# `astro deploy` and `astro package`

A project here is a directory whose `pyproject.toml` has a `[tool.astro]` table (see the [manifest reference](manifest-reference.md)). This page describes how such a project deploys to an Astro Deployment, and how `astro package` builds the artifact Astro, MWAA or Cloud Composer consumes without shipping it.

Terms used below:

- An **image deploy** ships the project's code and dependencies as a container image. A **dags-only deploy** ships only `dags/` as a tarball and leaves the running image in place. A **both** deploy (the default) does both.
- The **transport** is the set of calls that talk to Astro: create the deploy, push the image to the Deployment's registry, upload the DAG tarball, finalize.
- The **runtime image** is Astronomer's Airflow base image (`astrocrpublic.azurecr.io/runtime`). Building `FROM` it runs its `ONBUILD` steps, which install the project's `requirements.txt` and `packages.txt`.

## What deploys: a `pyproject.toml` project, or an image already built

v2 builds and deploys only projects with a `pyproject.toml`, on Astro and on Astro Private Cloud. Nobody has to move to v2, and Astro CLI 1.x keeps deploying the projects it made, so v2 has no path for the 1.x layout (a `Dockerfile` and `.astro/config.yaml`).

`astro deploy` looks at the working directory. A `pyproject.toml` with a `[tool.astro]` table is deployed, even if the table fails to validate (the deploy then reports the manifest error). A Dockerfile is not evidence of a 1.x project: a project with a manifest may declare one (`[tool.astro] dockerfile`). The routing is in [`cmd/astro/deploy.go`](../cmd/astro/deploy.go).

Anywhere else, one walk decides (`utils.Locate`, used by `astro deploy`, `astro remote deploy`, APC's deploy, and the bundle-path check of `astro deploy bundle`). It is `astro init`'s walk (`scaffold.Find1xProject`), so the deploy refuses as 1.x exactly where init and the local commands' advice say a 1.x project is. From the working directory up, at each directory: a `pyproject.toml` that is a project's stops it (`scaffold.HasManifest`): one whose `[tool.astro]` loads, or one that fails to validate, to parse or to be read, so the deploy reports why. Only a `pyproject.toml` that parses and has no `[tool.astro]` (a monorepo root's `[tool.ruff]`) is passed over; a broken one cannot be told apart from a broken manifest, so it stops the walk with its parse error, a 1.x checkout's included. Else a 1.x project stops it (`scaffold.Is1xProject`): a Dockerfile and a `.astro/config.yaml` that is not the CLI's own settings file (one with a `project` key, or no `context`, `contexts` or `telemetry`), so a stray Dockerfile in the home directory beside the global config is not one, while a home directory that is a 1.x project is. Else the walk goes on up. Each directory's `pyproject.toml` is loaded once, and the deploy uses the manifest the walk loaded rather than reading the file again.

| the walk finds | `astro deploy` |
| --- | --- |
| a 1.x project, here or above | refused: `this project uses the Astro CLI 1.x layout (a Dockerfile and .astro/config.yaml), and Astro CLI v2 deploys only pyproject.toml projects. Convert it with astro init, or deploy it with Astro CLI 1.x`. From below it, it names the project: `this directory is inside a project at <dir> that uses …. Convert it with astro init in <dir>, …` |
| a `pyproject.toml` project above | refused: `this directory is inside the project at <dir>. Run the deploy from the project directory, <dir>` |
| nothing | `--image-name` deploys (below); anything else is refused: `this is not an Astro project directory. Change to an Astro project directory, or run astro init to make this one an Astro project` (in the home directory, only the advice to change directory) |

Every refusal is reported under `--output json` as the error object with kind `no_project`, and exit 1. `--image-name` is refused in and below a 1.x project like every other mode: a prebuilt image deployed from a 1.x checkout would leave that project's DAGs stale with nothing said.

**`--image-name <ref>` outside any project** deploys an image already built on this machine, alone: an image-only deploy, with no `dags/` and no git commit. The target is the argument or `--deployment`, a Deployment id (there are no links without a manifest), or the workspace's Deployments to pick from; a run that cannot ask says to pass the id as the argument or with `--deployment`. The login is the current context's. The deploy warns, on stderr and in the result's `warnings`: a Deployment that takes DAG deploys keeps the DAGs it had, and one that does not now runs the DAGs inside the image, if any. At a `pyproject.toml` project's root, `--image-name` is the project's deploy with a prebuilt image, `dags/` included as usual.

**`astro deploy bundle [PATH]`** ships a directory of its own, PATH or the working directory, as a non-DAG bundle: a dbt project, or any other files the DAGs read, mounted at a path in Airflow. It carries no DAGs, and it is not a project deploy: PATH must be a directory, and one in or below any project the walk finds is refused. **`--delete`** removes the bundle mounted at a path instead. It replaces Astro CLI 1.x's `astro dbt deploy` and `astro dbt delete` and the 1.x `astro deploy --non-dags`, which did the same upload with different defaults and Deployment selection. `astro deployment bundle` is something else: it lists and changes the bundle registrations on a Deployment, and uploads nothing.

Because `bundle` is a subcommand, `astro deploy bundle` never means a Deployment called bundle. No new link may be called `bundle`: `scaffold.SaveLink` refuses to add one for every writer (`astro link add` among them, and Astro Desktop), and a deploy that links a picked Deployment names it otherwise. A link already called `bundle` saves again as any other does, a link already called `bundle` is deployed to with `astro deploy --deployment bundle`.

- **The type** decides the defaults. The types are a table in [`internal/platform/astro/deploy/bundletype.go`](../internal/platform/astro/deploy/bundletype.go), each with how to detect it, how to check a bundle of it, how to name it, and the directory its bundles mount below by default; adding a type is adding an entry. With no `--type`, the first type that detects PATH is used; else, given `--mount-path`, the type whose default it has the shape of (its directory and one name below it, after cleaning: `/usr/local/airflow/dbt/<name>` is `dbt`); else `none`. A deploy and a delete of the same bundle so record the same type. Today the table has one entry, `dbt`: a directory holding `dbt_project.yml`, named by the project's `name` and mounted by default at `/usr/local/airflow/dbt/<name>`. Any other `--type` (and `none`) is a free-form label the deploy records, with no defaults, so it needs `--mount-path`. A default mount path is said on stderr.
- **PATH is checked only when the mount path is derived from it.** Without `--mount-path`, `--type dbt` needs a dbt project at PATH. With `--mount-path`, `--type dbt` labels any directory, as 1.x's `--non-dags-bundle-type dbt` did.
- **Delete** uses `--mount-path` as given, or the mount path PATH's type would deploy to, so `astro deploy bundle --delete` in a dbt project removes that project. With neither, it is a usage error. Its type is found as the deploy's is.
- **The login** is chosen as `astro deploy <id>` from the same directory chooses it. In a `pyproject.toml` project in the working directory whose manifest loads, the project's login (for its Astro host) is used for everything: Deployment ids, link names and the workspace picker. Anywhere else, or when the manifest does not load, the current context is used for everything.
- **The target** is `--deployment`: a link of the project in the working directory, else a Deployment id, else the name of a Deployment in `--workspace` (or the project's `[tool.astro] workspace`, or the login's). The name lookup is this command's, as 1.x's `astro dbt deploy --deployment-name` had one; `astro deploy` takes a link or an id. A name that matches no Deployment, or several (listed by id), is a usage error, as a mistyped link name is. It has no `-d`: on `astro deploy`, `-d` is `--dags`. Without it, the Deployments of `--workspace` (or the context's) are offered, but only to a run that can be asked and only on the current context's host. A run under `--output json` or without a terminal fails with kind `input_required` and `pass --deployment with a Deployment id, a Deployment name, or a link name`. A project on another host is told to pass `--deployment <id>` or switch context.
- **`--wait`** waits for the Deployment to take the deploy or the delete, under the same login. In text the result prints first and the wait's progress goes to stderr; under json the one object comes after the wait, with `waited` and, when it failed, `wait_error`, and the run exits 1.
- **Output:** `--output json` publishes one object, pinned by `cmd/astro/testdata/schema/deploy-bundle.json` and, for `--delete`, `deploy-bundle-delete.json`. Both carry `bundle_type`, `mount_path`, `deployment` and `deploy_id`, and `bundle_name` when the mount path is the one the type derives from PATH (a dbt project's `name`; a mount path of its own names no bundle); the deploy adds `bundle_path`, `bundle_version` and `git`. 1.x's `astro dbt deploy -o json` keys `project` and `project_path` are `bundle_name` and `bundle_path` here, beside `bundle_type`. What the deploy prints on the way (the git note, the default mount path) goes to stderr there.

**Flags that cannot all apply** are refused as usage errors before anything is read: `--wait-time` without `--wait`, `--no-dags-base-dir` with a deploy that ships no DAGs (`--image`, or `--image-name` outside a project), and `--dags` with `--image` or `--image-name` (the message names the ones given).

`astro remote deploy`, which builds and pushes a Remote Execution client image, follows the same walk: it builds at a `pyproject.toml` project's root, refuses in or below a 1.x project and below a project's root with the messages above, and with `--image-name` pushes an image already built from outside any project.

The deploy code lives in `internal/platform/astro/deploy` (`manifest.go`, `manifest_image.go`, and the bundle and Remote Execution client paths beside them) and in [`internal/deploy`](../internal/deploy/deploy.go). The build half that is not about Astro (the checks before a build, the engine check, the build at linux/amd64 and the read of its runtime label, and the runtime rules the image is held to) is in [`internal/deploybuild`](../internal/deploybuild/doc.go), so a deploy to another control plane builds and judges an image the same way.

### Astro Private Cloud

v2 does not build projects for Astro Private Cloud yet. Support for `pyproject.toml` projects there is planned. A project can already link APC Deployments (`target = 'apc'`, written by `astro link add` under an APC context; see [Astro Private Cloud links](manifest-reference.md#astro-private-cloud-links)), but `astro deploy` does not read those links yet: under an APC context it takes a Deployment id as before. Until then, `astro deploy` under an APC context follows the same walk and the same `--image-name` rule:

- **a deploy that would build the project**, one without `--image-name`, is refused: `Astro CLI v2 cannot build and deploy projects to Astro Private Cloud yet: use Astro CLI 1.x for now.` At a `pyproject.toml` project's root it adds that support is coming, and that `--dags` uploads the project's DAGs and an image built with `astro package` deploys with `--image-name`; that is a usage error (exit 2), a mode v2 does not have. Outside any project it is kind `no_project` (exit 1), as on Astro;
- **in or below a project in the 1.x layout every deploy is refused**, `--image-name` and `--dags` included: `… uses the Astro CLI 1.x layout (a Dockerfile and .astro/config.yaml), which Astro CLI v2 does not deploy to Astro Private Cloud. Deploy it with Astro CLI 1.x` (kind `no_project`). It does not suggest `astro init`, which refuses to convert a project under an APC context. Below a `pyproject.toml` project's root, the deploy is told to run from the root;
- **`--image-name <ref>`** (with `--remote`, an image already in a registry) deploys at a project's root, where the project's `dags/` then goes to a Deployment that takes DAG uploads, and outside any project, where the image ships alone: no `dags/` is uploaded, and when the Deployment takes DAG uploads (or what it takes could not be read) the deploy says its DAGs were not updated, whatever `show_warnings` is;
- **`--dags`** uploads a `pyproject.toml` project's DAGs, from the project's root; with `--image-name` it is refused, in `astro deploy`'s words: `--dags deploys only your DAGs; drop --image-name`.

APC's uncommitted-changes check (`--force` passes it) holds only for a deploy at a project's root: an image deployed alone from outside any project reads nothing from the directory.

The target is the argument, or the Deployment picked from the workspace's. The `project.deployment` a 1.x project's `.astro/config.yaml` saved (`--save`) is not read, and `astro config set project.deployment` says it was removed.

## 1. Image deploys

The image is built by [`pkg/imagebuild`](../pkg/imagebuild/imagebuild.go), the same builder Docker-mode `astro local start` and `astro package astro` use, so the three produce the same image from the same manifest. `imagebuild.ForManifest` picks the build:

- **Generated (the common case).** A one-line `FROM astrocrpublic.azurecr.io/runtime:<tag>` Dockerfile, plus a `requirements.txt` holding `[project] dependencies` without `apache-airflow`/`apache-airflow-core` (`manifest.WithoutAirflow`) and a `packages.txt` holding `[tool.astro] packages`, plus `git` when a dependency is a git direct reference (a `[tool.uv.sources]` git source becomes `name @ git+https://…`, and the runtime image ships no git for uv to fetch it with). The tag is `[tool.astro] runtime` when set, else the series of the Airflow requirement (`runtime:3.3`). When `[project] requires-python` does not admit that build's default Python, the tag is instead the exact build's Python flavor, `runtime:3.3-8-python-3.13`, for the newest Python the build ships that it admits; with no `runtime` set, the build is the catalog's newest of the series. A `requires-python` admitting none of the build's Pythons is refused before the build, and without a readable catalog the default Python is kept (`imagebuild.RuntimeImageForPython`). The Python is `runtimeversions.ProjectPython`'s choice, which a standalone environment of the same manifest runs too (`imagebuild.StandalonePython`) (see [`requires-python`](manifest-reference.md#requires-python)). Deploy builds only Airflow 3 bases from a generated build.

  The image a deploy or `astro package astro` generates also carries the project's own files in `/usr/local/airflow`: `include/`, `plugins/`, the modules `dags/` imports, everything a DAG can reach on Astro. A deploy sends `dags/` alone beside the image, so without them a DAG that imports `include` fails there. They are copied by the runtime image's trailing `ONBUILD COPY . .`, after both installs, so the dependency layers keep their cache. Left out at any depth: `.git`, `.venv`, `venv`, `__pycache__`, `node_modules` and the tool caches; at the root, the project's own `requirements.txt` and `packages.txt` in any case (the generated ones win). A project's `.dockerignore` applies, during the copy, so an excluded directory is never read. After it come `imagebuild.ShippedImageIgnores`, which no `!` line of the project's can undo, the same rules `astro init` writes into a declared Dockerfile's `.dockerignore`. They are two lists in `pkg/airflowrt`:

  - `ShippedImageRules`, files no shipped image carries. At any depth: `.env`, `.env.*` and `.envrc` (local values, their per-stage variants, direnv's exports), and `airflow_settings.yaml` (1.x's local connections and variables, in cleartext). At the root only: `airflow.db`, `airflow.cfg` and `logs/`, what an Airflow run with the project as its home leaves there. The image's `AIRFLOW_HOME` is the project root, so a shipped `airflow.cfg` would replace the runtime's own; below the root these names are the project's own data. 1.x's default `.dockerignore` also had `astro`, the CLI binary; it is not here, because a root `astro/` can be a Python package (astro-sdk imports as `astro`). These are not added to `.gitignore`: some are committed on purpose, and git is the project's business.
  - `LocalFileRules`, the per-machine files Astro tools write (`.astro/standalone/`, `.astro/worktrees/`, the `*.local.*` files, Otto's `mcp.json`, the Airflow 2 pickle-fix plugin), which `.gitignore` also gets. A symlink is carried as what it points at, inside the project or out of it, since a link copied as a link would dangle in the image. The project's files are part of `astro package astro`'s content-addressed tag, so editing one moves the tag. Docker-mode `astro local start` runs the project mounted, so its image holds the dependencies alone (`imagebuild.ForLocalManifest`).
- **Declared Dockerfile.** When `[tool.astro] dockerfile` is set, that file is the build, with the project directory as context. No base is resolved.

Before anything is built, the deploy checks the project:

- the Dockerfile's `FROM` agrees with the Airflow requirement (`scaffold.CheckDockerfileAirflow`);
- the build secrets resolve, and the local files a declared Dockerfile would copy into the image are warned about;
- the image would carry no gitignored file that looks like a credential (see [Gitignored files and credentials](#gitignored-files-and-credentials));
- a `[tool.astro] runtime` build agrees with the requirement, against the runtime catalog.

The pipeline, in order (`DeployManifestImage` in [`manifest_image.go`](../internal/platform/astro/deploy/manifest_image.go)):

1. Check for a running container engine. Without one the deploy stops (see [Without Docker](#5-without-docker)).
2. Log in with the login for the project's host (see [Selecting the Deployment](#3-selecting-the-deployment)).
3. Collect git metadata (see [Command surface](#6-command-surface)).
4. Fetch the Deployment and check it can take this deploy: CI/CD enforcement, DAG deploys enabled for an `--image` deploy, and the runtimes it offers. A both deploy to a Deployment with DAG deploys off puts the DAGs inside the image (see [DAG deploys off](#dag-deploys-off)).
5. Check the planned runtime before the build. The planned runtime is `[tool.astro] runtime`, else the base's tag without any `-python-X.Y` (the series, or the exact build a Python flavor names), or for a declared Dockerfile the runtime its final `FROM` names (`airflowrt.ReadDeclaredBase`). An exact version gets the full rules: no downgrade, a version the Deployment offers, and the Airflow 2→3 floor. A series is refused only when it is older than the Deployment's, or the Deployment offers no build of it. A downgrade error names the line to change. This makes a bad pin fail in seconds rather than after a long build.
6. Build for `linux/amd64` and print "Building your project image, this can take a few minutes...".
7. Read `io.astronomer.docker.runtime.version` off the built image and run the runtime rules again on it. The label is the authority, because the requirement states what the project intends and the label states what the image is.
8. Create the deploy, log the container engine in to the Deployment's registry as user `cli` with the session token and push the image, upload the DAG tarball for a both deploy to a Deployment with DAG deploys on, finalize, and wait for health with `--wait`.

**A deploy to Astro requires an Astro Runtime base.** An image without the runtime label is refused, never deployed on the requirement's word:

- `--image-name`: `image "<ref>" is not based on Astro Runtime (the … label is missing); build it FROM the Astro runtime`
- a declared Dockerfile: `the image built from <Dockerfile> is missing the … label, so it is not based on Astro Runtime; build it FROM an Astro Runtime image`

This is the one limit on a declared Dockerfile. It builds and runs locally on any base, but deploys to Astro only on a runtime base.

`--image-name <ref>` deploys a prebuilt local image and skips the build. It is how CI deploys an image `astro package` built (see [The Astro target](#the-astro-target)).

### DAG deploys off

A Deployment with DAG deploys off takes no DAG upload, so the image is the only way DAGs reach it. A default deploy there ships the image alone, with the DAGs inside it, as the 1.x CLI did: the generated image carries `dags/` with the rest of the project, the deploy is created as `IMAGE_AND_DAG` and finalized without a tarball, and nothing is uploaded. The summary says so (`Deployed image (tag …) to …, with the DAGs inside the image: its DAG deploys are off, so no DAGs were uploaded.`), and `-o json` adds `"dags_in_image": true`.

A remote-execution Deployment runs its DAGs on its remote agents. With DAG deploys off, a default deploy uploads nothing and holds the image to nothing about DAGs, as the 1.x CLI uploaded none there; the summary says `It runs remote execution with DAG deploys off, so no DAGs were uploaded.`, and `-o json` adds `"remote_execution": true`. With DAG deploys on, it uploads them like any other Deployment.

| Deployment / flags | what ships |
| --- | --- |
| DAG deploys on (remote execution or not), default | the image (which carries `dags/` too), and the DAGs uploaded beside it |
| DAG deploys off, default, generated build | the image with the DAGs inside; refused when the project's `.dockerignore` leaves out every `.py` file under `dags/`, since the Deployment would run none |
| DAG deploys off, default, declared Dockerfile | the image as the Dockerfile builds it; a warning when its ignore file leaves out every DAG, since its own steps may put DAGs in the image |
| DAG deploys off, default, `--image-name` | the prebuilt image, with a warning that the Deployment runs the DAGs inside it |
| DAG deploys off, `--image` (with or without `--image-name`), in a project | refused, as in 1.x: `--image` leaves the running DAGs in place, and there they are the image's |
| DAG deploys off, `--image-name` outside any project | the prebuilt image alone, with the warning above that the Deployment runs the DAGs inside it; not refused, since no project's DAGs are left stale |
| remote execution, DAG deploys off, default | the image; no DAGs uploaded, and a `.dockerignore` dropping them is not refused |
| remote execution, DAG deploys off, `--image` | the image, as an image-only deploy |
| DAG deploys off, `--dags` | refused |

### Gitignored files and credentials

An image a deploy or `astro package astro` builds is pushed to a registry, so before either builds, `shipcheck.Survey` lists what the image would carry, once per command (`imagebuild.SurveyProject`: what a generated build copies, or a declared Dockerfile's context under its ignore file), and asks git about it. The same listing feeds the declared Dockerfile's local-files warning and, through the transport, the DAG check above. It runs for a generated build and a declared Dockerfile alike, and not for `--dags` or `--image-name`, which build nothing. A declared Dockerfile is held to a generated build's rule: everything in its build context counts as shipped, whatever the Dockerfile copies.

| what the image would carry | outcome |
| --- | --- |
| a file git ignores | warned about: ten named, the rest counted, with the ignore file as the fix. Generated output such as a dbt `target/` ships on purpose often enough that this is a warning |
| a file that looks like a credential and that git ignores, or that was never added | **refused**, naming it |
| a file that looks like a credential and that git tracks (committed or staged) | goes ahead: the project's choice |
| anything, where git cannot answer (not installed, an unsafe owner, another failure) | warned about, with git's reason and the files that look like credentials; the build goes ahead, so a CI deploy does not break on committed files |
| a declared Dockerfile's context carrying `.git` | warned about, with `**/.git` as the fix; **refused** when its `config` holds an `extraheader`, or a URL (a remote or an `insteadOf`) whose userinfo has a password or starts with a known token prefix (`ghp_`, `gho_`, `ghu_`, `ghs_`, `ghr_`, `github_pat_`, `glpat-`, `x-access-token`, `oauth2`). A plain user (`ssh://git@…`, `https://user@…`) is fine, and an opaque token of an unknown kind used as a bare user is not detected |
| a generated build's file reached through a link out of the project | not asked of git; **refused** when its path in the project or where the link leads looks like a credential (`dockercfg/config.json → ~/.docker/config.json`), whether or not the project is a git repository, and even when the target is committed elsewhere: the fix is one ignore-file line |
| a symlink in a declared Dockerfile's context | ships as a link, so it is never refused |

- The patterns are one table, `shipcheck.LooksSecret`: `*.pem`, `*.key`, `*.p12`, `*.p8`, `*.pfx`, `*.jks`, `*.keystore`, `*.ppk`, `id_rsa*`, `id_ed25519*`, `id_ecdsa*`, `id_dsa*`, `*credentials*`, `*-key.json`, `*_key.json`, `service-account*.json`, `sa-*.json`, `client_secret*.json`, `token.json`, `.netrc`, `.npmrc`, `.pypirc`, `.pgpass`, `*.kubeconfig`, `kubeconfig`, `*.tfvars`, `*.tfstate`, `.env*`, `*.env`, `*.env.*`, `config.json` under `.docker/`, and anything under `.aws/` or `.ssh/`. A bare `*secret*` is not one: too many modules are named for it.
- Git is asked once per repository: the project's, and each submodule or nested clone, about its own files. A nested repository its parent ignores whole counts as ignored.
- A generated build carries a link as what it points at. Git refuses a path beyond a symbolic link, so a file reached through a link that stays inside the project is asked about at its real path (`include/shared -> ../libs/shared` is asked about as `libs/shared/…`), and one whose link cannot be resolved is asked about as its outermost link. The survey records the links it goes through, and only files under them are resolved. A declared build carries a link as a link, so git is asked about the link itself.
- Outside a git work tree there is nothing to ask, and nothing is said.
- A declared build's survey reads the ignore file its engine reads: under podman a `.containerignore` first, as podman and buildah do (docker never reads one); then `<Dockerfile>.dockerignore`, else `.dockerignore`. The refusal names that file as the fix. The engine is podman when `<engine> --version` says so, which counts the podman-docker shim, or, when it does not answer, when its binary is named podman; a deploy resolves its engine once, when a build first needs it, and its checks and its build share it.

## 2. Dags-only deploys

`astro deploy --dags` tars `<project>/dags`, uploads it to the URL the create-deploy call returns, and finalizes. It reads the Deployment's runtime version and type from the API, not from the manifest, because the DAGs must fit the image already running. That version decides symlink validation (Airflow 3), the monitoring DAG's Airflow major version, and whether files sit under `dags/` or at the bundle root (`--no-dags-base-dir`). It never builds an image and never needs Docker.

## 3. Selecting the Deployment

The manifest's `[tool.astro.deployments.<name>]` links name the Deployments a project ships to (see the [manifest reference](manifest-reference.md#toolastrodeploymentsname)). `astro deploy` ships only to Astro links. A named MWAA, Composer, endpoint or Astro Private Cloud (`apc`) link is refused: `link "<name>" is <kind>, and astro deploy ships to Astro Deployments`. Use `astro package` for MWAA and Composer. An `apc` link is never offered by the prompt and never preselected, even as the project's default.

**Deploy never picks for you.** Shipping code is too consequential to decide from a marker, a pinned selection or an exported variable, so:

- **The argument or `--deployment` names a link or an id.** `astro deploy prod` ships to the `prod` link. A value no link declares is used as a Deployment id, so `astro deploy <id>` (what astronomer/deploy-action runs) keeps working. Naming both with different values is an error. There is no `-d` shorthand for `--deployment`: on this command `-d` means `--dags`.
- **With nothing named, deploy asks.** The prompt (on stderr) is the CLI's numbered picker table: the project's Astro links first, the `default = true` link on top and the rest by name, then the workspace's Deployments that no link points at yet. Each link's row ends in `linked`. The project's default link (`default = true`, or a project's only link) is preselected: it is highlighted, its row reads `linked, default`, and the prompt shows its number (`> [1]`). Enter takes it, and it never skips the question. Nothing else preselects: `ASTRO_DEPLOYMENT` and your `astro use` selection choose which Airflow the query commands read, and where code ships is the project's, so the CLI and Astro Desktop highlight the same entry. When `ASTRO_DEPLOYMENT` is exported and names something other than the default, the prompt is preceded by a one-line note on stderr saying the variable does not steer it, since it once moved the cursor. An answer is a name or a number, exactly as listed, names first, so a link called `2` selects itself. Three bad answers give up.
- **Picking a Deployment that is not linked yet links it.** The project's links end in `linked`; these rows carry no marker, and are listed under the name the link will get: the Deployment's name as `astro link add` spells it (`Orders Prod (EU)` is `orders-prod-eu`), else its id (also for a name of digits alone, which would shadow a row number), with `-2`, `-3` appended past a name already taken. The link is written to `pyproject.toml` once the deploy succeeds, and the deploy says so on stderr; a deploy that fails or is interrupted writes none, and a link that cannot be written is a warning on a deploy that still succeeded. The Deployments listed are the workspace's: `--workspace`, else `[tool.astro] workspace`, else the login's. When they cannot be listed, or the list takes over 15 seconds, the prompt warns and offers the links alone.
- **A run that cannot be asked fails.** With no terminal or with `--output json`, a deploy that names nothing stops: `a deploy must name the deployment it ships to: astro deploy <name> or --deployment <name>. …`. Under `--output json` it is the error object with kind `input_required` (see [Prompts](architecture.md#prompts)).
- **A project that links nothing** needs a workspace (`--workspace`, else `[tool.astro] workspace`, else the current context), then runs the workspace flow `astro deployment` commands use: pick a Deployment, or create one (`deployment.GetDeployment`). What it picks is linked as above. A run that cannot be asked must pass `--deployment`.
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

- **Tag:** `astro-package/<project name>:<runtime>-<hash>`, plus a moving `astro-package/<project name>:latest`. The 7-character hash covers the base, the platform, the dependencies, the OS packages and a declared Dockerfile's path and contents, so the same inputs give the same tag. `--tag <ref>` replaces the name and writes no `:latest`.
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

**One engine does every image step.** The build, the label reads, the registry login and the push all run on the engine resolved once for the deploy: `container.binary` for the working directory's project (a 1.x project's `.astro/config.yaml` over the global setting), else Docker or Podman found on `PATH`, with a Podman machine's connection env. `astro remote deploy` and an image deploy to Astro Private Cloud resolve it the same way, as does the registry login `astro login` makes for Astro Private Cloud. Each step is the engine's own command, through `pkg/imagebuild`: the login is `<engine> login <registry> -u <user> --password-stdin`, so the token is never on a command line, and the credential stays in the engine's credential store as any `docker login` leaves it. A push the registry refuses with a 403 fails with advice on clearing cached credentials.

## 6. Command surface

Both commands follow the CLI's output rules: text by default, `--output json` from the same value (see [Output](architecture.md#output)).

```
astro deploy [LINK-NAME | DEPLOYMENT-ID] [flags]
  --deployment <name|id>  the link name or Deployment id to deploy to
  --workspace <id>        the workspace (overrides the link's and the context's)
  -d, --dags              deploy only dags/ (no image build, no Docker)
  --image                 deploy only the image, leave DAGs untouched
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

The flags only a 1.x project's deploy read are gone: `--save` (`-s`), `--pytest`, `--env` (`-e`), `--test` (`-t`), `--parse`, `--deployment-name` (`-n`), `--dags-path` and `--dag-bundle-name`, and on Astro Private Cloud `--save` and `--no-cache`. A run passing one fails as a usage error naming what replaced it (run the tests with `uv run pytest` first, check the DAGs with `astro local check`, name the Deployment with `--deployment`, and so on; see [Removed 1.x commands and flags](architecture.md#removed-1x-commands-and-flags)). `--force` (`-f`) and `--prompt` (`-p`) are accepted, hidden and read by nothing: astronomer/deploy-action passes `--force` on every deploy, and the deploy always asks unless a Deployment is named, which is what `--prompt` asked for. `--non-dags` and its `--non-dags-*` flags are gone too, for `astro deploy bundle` (see above).

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

`deployment`, `workspace` and `type` (`dag-only`, `image-only` or `image-and-dag`) are always present. The rest are omitted when they do not apply: a dags-only deploy has no `image_tag`, an image-only deploy no `dag_bundle_version`, a Deployment named by id no `link`, a deploy with no recorded commit no `git`.

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
