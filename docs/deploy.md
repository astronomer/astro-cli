# `astro deploy` and `astro package`

A project here is a directory whose `pyproject.toml` has a `[tool.astro]` table (see the [manifest reference](manifest-reference.md)). This page describes how such a project deploys to an Astro Deployment, and how `astro package` builds the artifact Astro, MWAA or Cloud Composer consumes without shipping it.

Terms used below:

- An **image deploy** ships the project's code and dependencies as a container image. A **dags-only deploy** ships only `dags/` as a tarball and leaves the running image in place. A **both** deploy (the default) does both.
- The **transport** is the set of calls that talk to Astro: create the deploy, push the image to the Deployment's registry, upload the DAG tarball, finalize.
- The **runtime image** is Astronomer's Airflow base image (`astrocrpublic.azurecr.io/runtime`). Each tag has a `-base` twin in the same repository, the same image without the `ONBUILD` steps that install a 1.x project's `requirements.txt` and `packages.txt`. A generated build starts `FROM` the `-base` tag and writes those steps out itself.

## What deploys: a `pyproject.toml` project, or an image already built

v2 builds and deploys only projects with a `pyproject.toml`, on Astro and on Astro Private Cloud. Nobody has to move to v2, and Astro CLI 1.x keeps deploying the projects it made, so v2 has no path for the 1.x layout (a `Dockerfile` and `.astro/config.yaml`).

`astro deploy` looks at the working directory. A `pyproject.toml` with a `[tool.astro]` table is deployed, even if the table fails to validate (the deploy then reports the manifest error). A Dockerfile is not evidence of a 1.x project: a project with a manifest may declare one (`[tool.astro] dockerfile`). The routing is in [`cmd/astro/deploy.go`](../cmd/astro/deploy.go).

Anywhere else, one walk decides (`utils.Locate`, used by `astro deploy`, `astro remote deploy`, APC's deploy, and the bundle-path check of `astro deploy bundle`). It is the `astro dev` stub's walk (`scaffold.Find1xProject`), so the deploy refuses as 1.x exactly where the local commands' advice says a 1.x project is. From the working directory up, at each directory: a `pyproject.toml` that is a project's stops it (`scaffold.HasManifest`): one whose `[tool.astro]` loads, or one that fails to validate, to parse or to be read, so the deploy reports why. Only a `pyproject.toml` that parses and has no `[tool.astro]` (a monorepo root's `[tool.ruff]`) is passed over; a broken one cannot be told apart from a broken manifest, so it stops the walk with its parse error, a 1.x checkout's included. Else a 1.x project stops it (`scaffold.Is1xProject`): a Dockerfile and a `.astro/config.yaml` that is not the CLI's own settings file (one with a `project` key, or no `context`, `contexts` or `telemetry`), so a stray Dockerfile in the home directory beside the global config is not one, while a home directory that is a 1.x project is. Else the walk goes on up. Each directory's `pyproject.toml` is loaded once, and the deploy uses the manifest the walk loaded rather than reading the file again.

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

Under an Astro Private Cloud (APC) context, `astro deploy` at a `pyproject.toml` project's root builds the project and ships it to an APC Deployment, Airflow 2 (from 2.7) or Airflow 3, as `astro local` runs them. It is the same deploy as on Astro wherever nothing is APC's own: the target resolution in [`internal/deploy`](../internal/deploy/deploy.go), the checks before a build, the build at linux/amd64 and the runtime label read in [`internal/deploybuild`](../internal/deploybuild/doc.go), and every image action on one engine through `pkg/imagebuild`. What only APC knows is in [`internal/platform/apc/deploy/manifest.go`](../internal/platform/apc/deploy/manifest.go): Houston, its registry and tag scheme, its runtime list, and how DAGs reach a Deployment.

- **Which Deployment.** The argument or `--deployment` (which must agree), each an `apc` link on the context's domain or a Deployment id, as on Astro (see [Selecting the Deployment](#3-selecting-the-deployment)): links of the project's `[tool.astro.deployments]` with `target = 'apc'` (see [Astro Private Cloud links](manifest-reference.md#astro-private-cloud-links)). An interactive deploy that names none asks, offering those links with the `default = true` one preselected, then the workspace's Deployments no link names; picking one of those saves it as an `apc` link. A run that cannot ask (no terminal, or `--output json`) is refused with kind `input_required`, naming the links it could have named. An astro link is refused under an APC context, and an `apc` link on another domain is refused with the command that reaches it: `astro context switch <domain>` when this machine has a login there, else `astro login <domain>`. The workspace is `--workspace` (its older spelling `--workspace-id` is hidden and must agree), else the link's own, else the context's; `[tool.astro] workspace` is an Astro workspace and is never read. The `project.deployment` a 1.x project's `.astro/config.yaml` saved (`--save`) is not read, and `astro config set project.deployment` says it was removed.
- **The image.** A generated build starts from the runtime the project's `[tool.astro] runtime` names, else from the newest runtime build Houston lists for the Airflow pin (its `runtimeReleases` query, for the Deployment's cluster and, from Houston 2.0.0, its workspace and the Deployment, whose overrides of Airflow 3, its minimum runtime and listing every runtime apply), so the image carries a runtime Houston accepts. With nothing listed, an Airflow 3 pin builds on its series tag, and an Airflow 2 one on the newest runtime of the public catalog; with that unreadable too the deploy is refused, naming `[tool.astro] runtime`. A declared Dockerfile is the build, as on Astro, and its `FROM` may be a mirror of Astro Runtime (a repository named `runtime` or `astro-runtime` on any registry). The image carries the project's files as on Astro, and the labels Houston reads: `io.astronomer.skip.revision=true` and `io.astronomer.deploy.revision.description=<--description>`. `--build-secret` and the manifest's `build-secrets` reach a declared Dockerfile's build, with Astro's refusals.
- **The runtime rules, before the push.** Houston checks a pushed image's runtime only after the push, where the deploy never sees its answer (houston-api's `validateRuntimeVersionForUpdateImage` and `validateRuntimeVersion`). So the deploy refuses, before the build where the project says the runtime and again on the built image's label: a downgrade from the Deployment's current runtime, as Houston does when it has one (an Airflow 3 runtime is newer than every Airflow 2 one, so Airflow 3 to 2 is one); an Airflow 3 runtime where the platform lists no Airflow 3 build for this Deployment (`Airflow 3 is not enabled for this Deployment`: ask the platform admin to enable it, or pin Airflow 2); and a runtime newer than the newest the platform lists for this Deployment, naming the list. That list can stop below Houston's own ceiling (Airflow 3 off at the Deployment's scope, its newest build yanked, a build not yet stable), so the last refusal can turn away a runtime Houston would take. Any other runtime the list does not hold deploys with a warning: the list holds the newest build of each Airflow version, while Houston takes any build it knows (an older build of a listed Airflow, a deprecated or yanked one), and drops after the push only one it does not. Airflow 2 to 3 is an upgrade Houston allows from any Airflow 2 runtime, and the deploy says so as a warning. A Houston that lists nothing (before 0.29.0, or a list that cannot be read) holds the image to the downgrade rule alone. A prebuilt image (`--image-name`) is held to its label, and one already in a registry (`--remote`) to `--runtime-version`, before the push or the mutation that points the Deployment at it, more loosely (`deploybuild` `CheckPrebuiltListed`): only a downgrade is refused, as Houston refuses one, saying to rebuild that image `FROM` a newer runtime. An Airflow 3 runtime where the platform lists no Airflow 3 build, and one newer than the newest it lists, are warned of and deployed, since Houston may take them (its ceiling for an image update includes Airflow 3), as is any other runtime it does not list. Astro CLI 1.x deployed a prebuilt image whatever it carried, asking first at most, and earlier v2 builds asked too (`--yes` answered it); no deploy asks now, and `--yes` is no flag of APC's `astro deploy`.
- **The push.** To the Deployment's own registry from Houston 1.0.0, `registry.<domain>` before it, as `<release>/airflow:<next tag>`, with the context's token carried by the push alone (see [A push's credential lasts one push](#1-image-deploys)): nothing is logged in to the user's store. A prebuilt image is pushed the same way, under the same tag whatever its own: Houston deploys only a push to `<release>/airflow`, skips one tagged `latest` and does not roll out a tag it has deployed before. A BYO registry takes the push on the engine's own login, then the image by the `updateDeploymentImage` mutation with the runtime its label carries and, from Houston 0.36.0, `--description`; a prebuilt image keeps its own tag there, as Astro CLI 1.x pushed it, and one named without a tag takes `<release>-<next tag>`. `apc.sha_as_tag` is honoured.
- **A prebuilt image** (`--image-name <ref>`) is the project's deploy with that image instead of a build: no build, no checks before one, and its runtime read from its label (one without the label is refused as not Astro Runtime). Everything else is a build's: the Deployment resolution, the runtime rules, the push and the DAGs. Outside any project it deploys too, alone: the target is a Deployment id (there are no links without a manifest; a name the workspace does not hold, in no shape a Houston id has, is told that a link name resolves only from the project's directory) or the workspace's pick, and no `dags/` goes with it, so the deploy says what that leaves the Deployment: an `image` Deployment runs the image's DAGs (unless `show_warnings` is off; not refused, as `--image` is from a project), and one that takes Dag uploads, or may, keeps the Dags it had, which is said whatever `show_warnings` is. From a project to an `image` Deployment it warns that the Deployment runs only the DAGs the image carries (unless `show_warnings` is off).
- **An image already in a registry** (`--image-name <ref> --remote --runtime-version <version>`) is neither pulled, built nor pushed: Houston's `updateDeploymentImage` mutation points the Deployment at the reference, with `--runtime-version` (Houston requires it, and there is no local label to read it from) and, from Houston 0.36.0, `--description`. The Deployment resolves as a build's does (links, ids, the default preselected, `input_required`, a link on another domain), `--runtime-version` is held to the prebuilt image's runtime rules above (only a downgrade refused), and the DAGs follow as a pushed image's do, from a project and from outside one. The cluster config is read only to place the DAGs: one that cannot be read does not stop the deploy, and an upload that may have been due is said not to have happened. The mutation needs Houston 0.29.2 or newer and a platform that has turned it on; one that has not is told the setting a platform admin turns it on with (`deployments.deploymentImagesRegistry.updateDeploymentImageEndpoint.enabled` from Houston 2.0.0, `deployments.enableUpdateDeploymentImageEndpoint` before it). `--remote` without `--image-name` or without `--runtime-version` is refused before anything is asked; `--runtime-version` without `--remote` is taken, as 1.x took it, with a warning that it does nothing.
- **The DAGs.** An `image` Deployment runs the DAGs inside the image, so none are uploaded (`dags_in_image` in `-o json`), and a generated image whose `.dockerignore` leaves out every DAG file is refused. A `dag_deploy` Deployment on a cluster that takes DAG-only deploys gets the project's `dags/` uploaded beside the image. A `git_sync` or `volume` Deployment takes its DAGs from there: the image ships alone, with a warning saying so. `--image` leaves the Deployment's DAGs as they are, so it is refused for an `image` Deployment. `--dags` uploads the DAGs alone, with no build and no container engine. An empty `dags/` is a warning, not a question. A Deployment whose DAG source Houston did not say (before 0.29.0) is uploaded to, and one that turns out to take no upload ships the image alone, with a warning. An upload that fails after the push fails the deploy, naming the image pushed and `astro deploy <deployment> --dags`, which uploads the DAGs alone.
- **`-o json`** publishes `{deployment, link, workspace, type, image, dags_in_image, runtime_version, url, warnings}`, `type` spelled as Astro's: `image-and-dag`, `image-only` (`--image`, an image deployed alone from outside a project, or a deploy whose Deployment took no DAGs from it) or `dag-only`. `link` is set whenever a link named the target, `--image-name` and `--remote` included.
- **Flags.** As on Astro: the argument, `--deployment`, `--workspace`, `--dags`, `--image`, `--image-name`, `--description`, `--build-secret`, `-o`; and APC's own `--remote` and `--runtime-version`, which Astro has no equivalent of (an Astro Deployment takes images only from Astro's registry). `--force` and `--prompt` (`-p`) are accepted, hidden and read by nothing, as on Astro: there is no uncommitted-changes check, and the deploy asks whenever no Deployment is named, so to choose from the list, name none. There is no `--wait`: Astro CLI 1.x's APC deploy had none. `--save` and `--no-cache` are removed flags. With `--dags`, `--image-name` and `--remote` are refused, in `astro deploy`'s words: `--dags deploys only your Dags; drop --image-name` (and `--remote`, which is refused with `--dags` on its own too).

What only APC does with a prebuilt image is in [`internal/platform/apc/deploy`](../internal/platform/apc/deploy/manifest.go): the push and its tag, and [`registry_image.go`](../internal/platform/apc/deploy/registry_image.go)'s mutation for an image already in a registry. Which Deployment, and the runtime rules, are the shared ones.

**In or below a project in the 1.x layout every deploy is refused**, `--image-name` and `--dags` included: with the words Astro's deploy uses, `… uses the Astro CLI 1.x layout (a Dockerfile and .astro/config.yaml), and Astro CLI v2 deploys only pyproject.toml projects. Convert it with astro init, or deploy it with Astro CLI 1.x` (kind `no_project`): `astro init` converts a 1.x project under an APC context as under Astro. Below a `pyproject.toml` project's root, the deploy is told to run from the root, and outside any project a deploy with nothing to build is kind `no_project`, as on Astro.

## 1. Image deploys

The image is built by [`pkg/imagebuild`](../pkg/imagebuild/imagebuild.go), the same builder Docker-mode `astro local start` and `astro package astro` use, so the three produce the same image from the same manifest. `imagebuild.ForManifest` picks the build:

- **Generated (the common case).** A Dockerfile `FROM astrocrpublic.azurecr.io/runtime:<tag>-base` that installs `packages.txt` as root, then `requirements.txt` as root, then copies the context in owned by `astro`: the runtime's `ONBUILD` steps, written out (`imagebuild.generatedDockerfile`). Each install runs the runtime's own script (`install-system-packages`, `install-python-dependencies`, which applies the runtime's constraints file), or, on Airflow 2 runtimes before 11.0.0, which ship no scripts, the inline command their `ONBUILD` ran. When the build has a `netrc` secret, the Python install mounts it as `/root/.netrc` (`RUN --mount=type=secret,id=netrc,target=/root/.netrc,required=false`), on every runtime, including those whose own `ONBUILD` never mounted it. That step needs BuildKit under Docker, and Buildah 1.21 or later under Podman (Podman 3.2, the first to vendor it). Without the secret the step is a plain `RUN`, which the legacy builder and an older Podman run too. The `requirements.txt` holds `[project] dependencies` without `apache-airflow`/`apache-airflow-core` (`manifest.WithoutAirflow`), after a `--find-links` line for the index page of each of those dependencies that a `[tool.uv.sources]` entry routes to an index (`manifest.UV.IndexPagesFor`). The Airflow sources `astro init` writes name packages the image does not install, so they add no line, and a build host that cannot reach Astronomer's index still builds; the `packages.txt` holds `[tool.astro] packages`, plus `git` when a dependency is a git direct reference (a `[tool.uv.sources]` git source becomes `name @ git+https://…`, and the runtime image ships no git for uv to fetch it with). `scripts/check-runtime-base-tags.sh` checks that every `-base` tag the CLI can pick is published. The tag the rest of this section picks (`runtime:3.3`, `runtime:3.3-8-python-3.13`, `astro-runtime:12.6.0`) names the runtime; the build pulls its `-base` twin from the same repository, so a registry mirror or an egress allow-list that serves only the runtime tags has to carry the `-base` tags too. A pull-through cache fetches them on demand; a mirror that replicates a list of tags needs them added. The tag is `[tool.astro] runtime` when set, else the series of the Airflow requirement (`runtime:3.3`). When `[project] requires-python` does not admit that build's default Python, the tag is instead the exact build's Python flavor, `runtime:3.3-8-python-3.13`, for the newest Python the build ships that it admits; with no `runtime` set, the build is the catalog's newest of the series. A `requires-python` admitting none of the build's Pythons is refused before the build, and without a readable catalog the default Python is kept (`imagebuild.RuntimeImageForPython`). The Python is `runtimeversions.ProjectPython`'s choice, which a standalone environment of the same manifest runs too (`imagebuild.StandalonePython`) (see [`requires-python`](manifest-reference.md#requires-python)). An Airflow 2 pin (from 2.7, as Docker mode) builds `FROM quay.io/astronomer/astro-runtime:<build>-base`, since Airflow 2 images are tagged by runtime version: `[tool.astro] runtime` when set, else the newest runtime the catalog lists for the pin, yanked builds skipped. With no `runtime` and no readable catalog (a fresh fetch, else a stale cached copy) there is no base to name, and the deploy is refused before the build, naming `[tool.astro] runtime`. Airflow 2 builds list no Python, so the image runs its runtime's default.

  An Astro deploy picks the build from what Astro offers first. With no `[tool.astro] runtime`, the base is the newest stable or deprecated runtime the deployment options (`GetDeploymentOptions`, for the organization, the same list the runtime check reads) offer whose Airflow the pin covers, for either Airflow generation, so the image carries a runtime Astro takes; only when it offers none does the rule above run. A preview channel is never picked unless named in `runtime`, and a bare `apache-airflow==3.*` is refused as everywhere else rather than resolved to the newest offered series. An offered Airflow 3 build that ships no Python `requires-python` admits gives way to the rule above, which can pick a newer build of the series that does. This is the rule APC's deploy follows over Houston's runtime list: both are a `deploybuild.RuntimeOffer` composed by `deploybuild.OfferedBase`, each platform implementing the offer over its own API. `astro package astro` has no Deployment to ask, so it builds by the rule above, and can build a newer build of the series than a deploy of the same project.

  The image a deploy or `astro package astro` generates also carries the project's own files in `/usr/local/airflow`: `include/`, `plugins/`, the modules `dags/` imports, everything a DAG can reach on Astro. A deploy sends `dags/` alone beside the image, so without them a DAG that imports `include` fails there. They are copied by the generated Dockerfile's trailing `COPY . .`, after both installs, so the dependency layers keep their cache. Left out at any depth: `.git`, `.venv`, `venv`, `__pycache__`, `node_modules` and the tool caches, and every linked worktree of the project's own repository inside it (one an agent checked out under `.claude/worktrees/`, say): a directory whose `.git` is a file whose `gitdir:` points under that repository's `<common git dir>/worktrees/`. Only files are read for it; git is not run. A build that leaves any out says so in one plain line, `left out N git worktrees of this repository, in <places>`, summarized as the gitignored warning is. Anything else with a `.git` (a clone, a submodule, a worktree of another repository) is the project's code as far as the image is concerned, and is carried; at the root, the project's own `requirements.txt` and `packages.txt` in any case (the generated ones win). A project's `.dockerignore` applies, during the copy, so an excluded directory is never read. After it come `imagebuild.ShippedImageIgnores`, which no `!` line of the project's can undo, the same rules `astro init` writes into a declared Dockerfile's `.dockerignore`. They are two lists in `pkg/airflowrt`:

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
4. Fetch the Deployment and check it can take this deploy: CI/CD enforcement, DAG deploys enabled for an `--image` deploy, then that it is not hibernating (refused with the wake-up step, kind `deployment_hibernating`, after the two refusals waking would not clear; Astro itself refuses only when the deploy is created, after the build), and the runtimes it offers. A both deploy to a Deployment with DAG deploys off puts the DAGs inside the image (see [DAG deploys off](#dag-deploys-off)).
5. Check the planned runtime before the build. The planned runtime is `[tool.astro] runtime`, else the base's tag without any `-python-X.Y` (the series, or the exact build a Python flavor names), or for a declared Dockerfile the runtime its final `FROM` names (`airflowrt.ReadDeclaredBase`). An exact version gets the full rules: no downgrade, a version the Deployment offers, and the Airflow 2→3 floor. A series is refused only when it is older than the Deployment's, or the Deployment offers no build of it. A downgrade error names the line to change. This makes a bad pin fail in seconds rather than after a long build.
6. Build for `linux/amd64` and print "Building your project image, this can take a few minutes...".
7. Read `io.astronomer.docker.runtime.version` off the built image and run the runtime rules again on it. The label is the authority, because the requirement states what the project intends and the label states what the image is.
8. Create the deploy, log the container engine in to the Deployment's registry as user `cli` with the session token and push the image, upload the DAG tarball for a both deploy to a Deployment with DAG deploys on, finalize, and wait for health with `--wait`.

**A deploy to Astro requires an Astro Runtime base.** An image without the runtime label is refused, never deployed on the requirement's word:

- `--image-name`: `image "<ref>" is not based on Astro Runtime (the … label is missing); build it FROM the Astro runtime`
- a declared Dockerfile: `the image built from <Dockerfile> is missing the … label, so it is not based on Astro Runtime; build it FROM an Astro Runtime image`

This is the one limit on a declared Dockerfile. It builds and runs locally on any base, but deploys to Astro only on a runtime base.

`--image-name <ref>` deploys a prebuilt local image and skips the build. It is how CI deploys an image `astro package` built (see [The Astro target](#the-astro-target)).

### Dag deploys off

A Deployment with Dag deploys off takes no Dag upload, so the image is the only way Dags reach it. A default deploy there ships the image alone, with the Dags inside it, as the 1.x CLI did: the generated image carries `dags/` with the rest of the project, the deploy is created as `IMAGE_AND_DAG` and finalized without a tarball, and nothing is uploaded. The summary says so (`Deployed image (tag …) to …, with the Dags inside the image: its Dag deploys are off, so no Dags were uploaded.`), and `-o json` adds `"dags_in_image": true`.

A remote-execution Deployment runs its Dags on its remote agents. With Dag deploys off, a default deploy uploads nothing and holds the image to nothing about Dags, as the 1.x CLI uploaded none there; the summary says `It runs remote execution with Dag deploys off, so no Dags were uploaded.`, and `-o json` adds `"remote_execution": true`. With Dag deploys on, it uploads them like any other Deployment.

An image deploy refuses a Remote Execution agent image, one that sets `ASTRO_AGENT_CLIENT_*` variables: it would run as the Deployment's scheduler and crash-loop, since the agent's entrypoint runs only its own components. The refusal names `astro remote deploy`, which is where an agent image goes. The check reads variable names, so an image that is not an agent's could trip it; `show_warnings` false skips it, as in Astro CLI 1.x. The refusal does not suggest that, because the switch applies to every deploy and also turns off the APC deploy's checks and warnings and the confirmation before turning Dag deploys off.

| Deployment / flags | what ships |
| --- | --- |
| Dag deploys on (remote execution or not), default | the image (which carries `dags/` too), and the Dags uploaded beside it |
| Dag deploys off, default, generated build | the image with the Dags inside; refused when the project's `.dockerignore` leaves out every `.py` file under `dags/`, since the Deployment would run none |
| Dag deploys off, default, declared Dockerfile | the image as the Dockerfile builds it; a warning when its ignore file leaves out every Dag, since its own steps may put Dags in the image |
| Dag deploys off, default, `--image-name` | the prebuilt image, with a warning that the Deployment runs the Dags inside it |
| Dag deploys off, `--image` (with or without `--image-name`), in a project | refused, as in 1.x: `--image` leaves the running Dags in place, and there they are the image's |
| Dag deploys off, `--image-name` outside any project | the prebuilt image alone, with the warning above that the Deployment runs the Dags inside it; not refused, since no project's Dags are left stale |
| remote execution, Dag deploys off, default | the image; no Dags uploaded, and a `.dockerignore` dropping them is not refused |
| remote execution, Dag deploys off, `--image` | the image, as an image-only deploy |
| Dag deploys off, `--dags` | refused |

### Gitignored files and credentials

An image a deploy or `astro package astro` builds is pushed to a registry, so before either builds, `shipcheck.Survey` lists what the image would carry, once per command (`imagebuild.SurveyProject`: what a generated build copies, or a declared Dockerfile's context under its ignore file), and asks git about it. The same listing feeds the declared Dockerfile's local-files warning and, through the transport, the DAG check above. It runs for a generated build and a declared Dockerfile alike, and not for `--dags` or `--image-name`, which build nothing. A declared Dockerfile is held to a generated build's rule: everything in its build context counts as shipped, whatever the Dockerfile copies.

| what the image would carry | outcome |
| --- | --- |
| a file git ignores | warned about with a count and the top-level places they are under, not the files ("5,510 files … under `.claude/` and `.astro/`", at most three places, then how many more), with the ignore file as the fix. Generated output such as a dbt `target/` ships on purpose often enough that this is a warning |
| a file that looks like a credential and that git ignores, or that was never added | **refused**, naming it |
| a file that looks like a credential and that git tracks (committed or staged) | goes ahead: the project's choice |
| anything, where git cannot answer (not installed, an unsafe owner, another failure) | warned about, with git's reason and the files that look like credentials; the build goes ahead, so a CI deploy does not break on committed files |
| a declared Dockerfile's context carrying `.git` | warned about, with `**/.git` as the fix; **refused** when its `config` holds an `extraheader`, or a URL (a remote or an `insteadOf`) whose userinfo has a password or starts with a known token prefix (`ghp_`, `gho_`, `ghu_`, `ghs_`, `ghr_`, `github_pat_`, `glpat-`, `x-access-token`, `oauth2`). A plain user (`ssh://git@…`, `https://user@…`) is fine, and an opaque token of an unknown kind used as a bare user is not detected |
| a declared Dockerfile's context carrying a linked worktree of the project's own repository | its files count as carried, so its ignored files are warned about and one that looks like credentials **refuses** the build. The refusal says the file sits in another git checkout and names that checkout's directory to add to the ignore file, since git there may track the file while this project's does not. A generated build leaves such worktrees out. Every reason a build is refused for is in the one refusal |
| a generated build's file reached through a link out of the project | not asked of git; **refused** when its path in the project or where the link leads looks like a credential (`dockercfg/config.json → ~/.docker/config.json`), whether or not the project is a git repository, and even when the target is committed elsewhere: the fix is one ignore-file line |
| a symlink in a declared Dockerfile's context | ships as a link, so it is never refused |

- The patterns are one table, `shipcheck.LooksSecret`: `*.pem`, `*.key`, `*.p12`, `*.p8`, `*.pfx`, `*.jks`, `*.keystore`, `*.ppk`, `id_rsa*`, `id_ed25519*`, `id_ecdsa*`, `id_dsa*`, `*credentials*`, `*-key.json`, `*_key.json`, `service-account*.json`, `sa-*.json`, `client_secret*.json`, `token.json`, `.netrc`, `.npmrc`, `.pypirc`, `.pgpass`, `*.kubeconfig`, `kubeconfig`, `*.tfvars`, `*.tfstate`, `.env*`, `*.env`, `*.env.*`, `config.json` under `.docker/`, and anything under `.aws/` or `.ssh/`. A bare `*secret*` is not one: too many modules are named for it.
- Git is asked once per repository: the project's, and each submodule or nested clone, about its own files. A nested repository its parent ignores whole counts as ignored.
- A generated build carries a link as what it points at. Git refuses a path beyond a symbolic link, so a file reached through a link that stays inside the project is asked about at its real path (`include/shared -> ../libs/shared` is asked about as `libs/shared/…`), and one whose link cannot be resolved is asked about as its outermost link. The survey records the links it goes through, and only files under them are resolved. A declared build carries a link as a link, so git is asked about the link itself.
- Outside a git work tree there is nothing to ask, and nothing is said.
- A declared build's survey reads the ignore file its engine reads: under podman a `.containerignore` first, as podman and buildah do (docker never reads one); then `<Dockerfile>.dockerignore`, else `.dockerignore`. The refusal names that file as the fix. The engine is podman when `<engine> --version` says so, which counts the podman-docker shim, or, when it does not answer, when its binary is named podman; a deploy resolves its engine once, when a build first needs it, and its checks and its build share it.

## 2. Dags-only deploys

`astro deploy --dags` tars `<project>/dags`, uploads it to the URL the create-deploy call returns, and finalizes. It reads the Deployment's runtime version and type from the API, not from the manifest, because the DAGs must fit the image already running. That version decides symlink validation (Airflow 3), the monitoring DAG's Airflow major version, and whether files sit under `dags/` or at the bundle root (`--no-dags-base-dir`). It never builds an image and never needs Docker. A hibernating Deployment refuses the create-deploy call, before anything is uploaded, and the deploy says how to wake it, with kind `deployment_hibernating`, as an image deploy does before its build; a bundle deploy is refused the same way.

## 3. Selecting the Deployment

The manifest's `[tool.astro.deployments.<name>]` links name the Deployments a project ships to (see the [manifest reference](manifest-reference.md#toolastrodeploymentsname)). Under an Astro context `astro deploy` ships only to Astro links. A named MWAA, Composer, endpoint or Astro Private Cloud (`apc`) link is refused: `link "<name>" is <kind>, and astro deploy ships to Astro Deployments`. Use `astro package` for MWAA and Composer. An `apc` link is never offered by the prompt and never preselected, even as the project's default. Under an Astro Private Cloud context the same rules hold with `apc` links on the context's domain in their place (see [Astro Private Cloud](#astro-private-cloud)).

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
an image deploy needs Docker, but no running container engine was found. Start Docker and try again, or run 'astro deploy --dags' to deploy just your Dags (no Docker needed). Server-side builds are coming
```

The check runs for `--image-name` too. A dags-only deploy never needs Docker, and neither do the `mwaa` and `composer` package targets. `astro package astro` refuses the same way. A server-side build, where the CLI uploads the context and the control plane builds, is the long-term answer for users without Docker and is not built.

**One engine does every image step.** The build, the label reads, the registry login and the push all run on the engine resolved once for the deploy: `container.binary` for the working directory's project (a 1.x project's `.astro/config.yaml` over the global setting), else Docker or Podman found on `PATH`, with a Podman machine's connection env. `astro remote deploy` and an image deploy to Astro Private Cloud resolve it the same way, as does the registry login `astro login` makes for Astro Private Cloud. Each step is the engine's own command, through `pkg/imagebuild`. A login is `<engine> login <registry> -u <user> --password-stdin`, so the token is never on a command line. A push the registry refuses (a 403, or a login it rejects) fails with advice on clearing cached credentials.

**A push's credential lasts one push.** A push that carries the session token (an image deploy to Astro, and an image deploy to Astro Private Cloud's own registry, `--image-name` included) runs no login. The token is written straight into one file made for that push, `config.json` (owner-only) in an owner-only temporary directory, in the format Docker's `config.json` and Podman's `auth.json` share, and the push is pointed at it every way an engine looks: `DOCKER_CONFIG` for Docker, `REGISTRY_AUTH_FILE` and `--authfile` for Podman. Of the user's own Docker config only what picks the daemon is read: its `currentContext`, written into the file, and its `contexts/` directory, linked beside it (or copied, on Windows without the symlink privilege, the copy removed with the directory), so the same daemon answers. Their credential helpers, keychain and stored logins are neither read nor written for the push. It is deleted when the push ends, success or failure, and a push also clears any such directory over an hour old that a killed push left. Nothing is left behind, and two deploys pushing at once with different tokens never share a file. A registry that refuses the token is reported as that, with the registry's own words, not as stale cached logins. The tag, the build and the label reads stay on the user's own config. A push with no credential of its own, such as to a BYO registry on Astro Private Cloud, uses the user's own config and whatever login it holds. Two logins still go to the user's store, as Astro CLI 1.x left them: Astro Private Cloud's registry login (`astro login`), and `astro remote deploy`'s login to the base image registry, which the build's pull of `Dockerfile.client`'s base needs (a build cannot run on a throwaway config: Docker's builder, buildx, is found through the user's config directory).

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

**Warnings come last.** In text, `astro deploy` and `astro package` hold the warnings and notes they find as they go and print them on stderr after the build's output, just above the result, where a long build cannot scroll them away. A build or deploy that fails prints them before its error. Under `--output json` they print as they are found, and the result carries them in `warnings`.

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
