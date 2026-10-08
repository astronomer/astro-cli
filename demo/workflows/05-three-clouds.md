# 5. Ship the same project to three clouds

**The story.** One `pyproject.toml`, no Dockerfile, three platforms. The CLI
tells you what will break on each one *before* you upload, then builds each
platform the artifact it actually eats.

**Shows:** `astro local check --target mwaa/composer`, `astro package` for all
targets, v2 `astro deploy` and the manifest's deployment links, the four link
kinds and the auth axis.

**Needs:** Docker for the image targets (`astro package`, `astro package astro`,
`astro deploy`). The `mwaa` and `composer` targets need no Docker. Network for
the MWAA constraints resolution. An Astro login for the deploy step. Real
environments come from [`../terraform/`](../terraform/).

---

## The committed inventory

`demo/project/pyproject.toml` names five Airflows and holds not one credential:

```toml
[tool.astro.deployments.dev]            # an Astro link: a Deployment id
deployment = 'your-dev-deployment-id'

[tool.astro.deployments.prod]
deployment = 'your-prod-deployment-id'
default = true                          # the query commands' fall-through; deploy only highlights it

[tool.astro.deployments.prod-mwaa]      # an MWAA link: an environment name
target = 'mwaa'
environment = 'orders-demo-mwaa'

[tool.astro.deployments.prod-composer]  # a Composer link
target = 'composer'
environment = 'orders-demo-composer'

[tool.astro.deployments.staging]        # an endpoint link: a bare URL
url = 'https://airflow.staging.example.com'
auth = { method = 'token', token-env = 'STAGING_AIRFLOW_TOKEN' }
```

Four kinds of link, and the kind follows from what the link sets. Auth is a
separate axis: any of `astro / aws / google / basic / token / airflow-token /
exec / none` attaches to any kind, and a link that says nothing takes its
kind's default — Astro links `astro`, MWAA `aws`, Composer `google`.
[Workflow 4](04-point-at-any-airflow.md) reads these same links; this one
ships to them.

`token-env` names the environment variable holding the token, never the token.
No field that could hold a literal secret exists in this file, so a teammate
who clones the repo gets the whole inventory and no credentials at all.

Fill the placeholder ids from the terraform outputs — every stack's README
says which output goes in which key, and `terraform output manifest_snippet`
in the astro stack prints the block ready to paste.

## Check before you upload

```sh
astro local check --target mwaa,composer
```

```
[mwaa] reusing the cached check environment for Airflow 3.0.6
[mwaa] resolving dependencies against MWAA's constraints for Airflow 3.0.6
[composer] reusing the cached check environment for Airflow 3.1.8
== mwaa ==
checking against Airflow 3.0.6 (mapped down from the manifest pin 3.1)
note: mwaa does not offer Airflow 3.1; checking against 3.0.6, the closest
      version it runs — a 3.1-only feature would break there
no DAG findings
constraints: dependencies resolve against MWAA's constraints file
check passed for mwaa: 3 DAGs, 0 errors, 0 warnings

== composer ==
checking against Airflow 3.1.8
note: composer pins its own Python; checking with the default interpreter,
      so a Python-version-specific issue may not show
no DAG findings
check passed for composer: 3 DAGs, 0 errors, 0 warnings
```

**This is the money shot.** It builds a scratch virtualenv with the Airflow
version each platform *actually runs*, parses your DAGs inside it, and for
MWAA also resolves your dependencies against MWAA's published constraints file
with uv. Seconds on your laptop, versus a 20-to-30-minute MWAA environment
update that fails.

The version mapping is the honest part. The project pins `3.1`. Composer runs
3.1.8, so it checks against exactly that. MWAA offers no 3.1 at all, so it
maps **down** to 3.0.6 — the closest version MWAA does run — and says so,
because a lower version is what you would end up on. A pin below everything
the platform offers is an error, not a silent pass.

The scratch environments are cached by Airflow version, Python version, and a
hash of your dependencies, so the first check pays the uv install and later
ones do not. The output says which of the two is happening.

Exit codes match the plain check: `0` clean, `1` findings, `2` an operational
problem. Any target failing fails the whole run, which is what CI wants.

## Build all three artifacts

### Astro — an image

```sh
astro package
```

```
target: astro
image:  astro-package/orders-demo:3.1-18-61063d6
runtime: 3.1-18

Deploy it with:
  astro deploy prod --image-name astro-package/orders-demo:3.1-18-61063d6
```

No Dockerfile anywhere. The CLI writes a one-line `FROM
astrocrpublic.azurecr.io/runtime:<version>` plus a `requirements.txt` and a
`packages.txt` from the manifest, and the runtime image's built-in steps
install them. The tag is content-addressed over the inputs that change the
image, so the same manifest gives the same tag and a rebuild is a cache hit.

That last line is the CI story: one job packages, uploads `--save image.tar`,
and another job loads it and runs `astro deploy prod --image-name <tag>`. The image
`astro package` builds and the image `astro deploy` builds are the same
artifact from the same code.

### MWAA — an S3-shaped tree

```sh
astro package mwaa
```

```
target: mwaa
tree:   /…/demo/project/dist/mwaa
deps:   /…/demo/project/dist/mwaa/requirements.txt

warnings:
  - the manifest pins Airflow "3.1", which MWAA does not list (MWAA offers:
    3.2.1, 3.0.6, 2.11.0, 2.10.3, 2.10.1, 2.9.2, 2.8.1, 2.7.2);
    requirements.txt carries a commented constraint template instead of a
    pinned one
  - MWAA installs no OS packages from this artifact; the manifest's packages
    (libpq-dev) are skipped. Ship them another way (a startup script, a
    plugins wheel, or a custom image where the platform allows one).

Upload it with:
  aws s3 sync /…/demo/project/dist/mwaa/ s3://your-mwaa-bucket/
  Point the environment at the new requirements.txt object version (MWAA
  console, or `aws mwaa update-environment --requirements-s3-object-version <ver>`).
```

`dist/mwaa/` holds `dags/`, a `requirements.txt` with `apache-airflow` dropped
(MWAA supplies Airflow and rejects a pin of it), a `plugins.zip` when
`plugins/` holds real files, and an `ENV_SETUP.md` listing every value
`[tool.astro.env]` declares — MWAA has no bucket file for those, so the
artifact tells you what to set rather than pretending:

```markdown
# Environment setup for orders-demo

MWAA does not read this project's [tool.astro.env] section. Set these on the
environment before your DAGs run.

## Environment variables
- [ ] LOG_LEVEL (has a default)
- [ ] ORDERS_API_URL (from workspace)
- [ ] WAREHOUSE_URI (required)
…
```

The warnings never fail the build. A version MWAA does not offer, OS packages
with nowhere to go — the `dags/` and `requirements.txt` are correct without
them, and you may know better than a version list that moves a few times a
year.

### Composer — a GCS-shaped tree

```sh
astro package composer
```

```
target: composer
tree:   /…/demo/project/dist/composer
deps:   /…/demo/project/dist/composer/composer-requirements.txt

Upload it with:
  gcloud composer environments storage dags import --environment=<env> --location=<region> --source=…/dist/composer/dags
  gcloud composer environments update <env> --location=<region> --update-pypi-packages-from-file …/dist/composer/composer-requirements.txt
```

Two commands, not one, and that is deliberate. Composer syncs `dags/` and
`plugins/` from its bucket but installs PyPI packages on the *environment*, so
one bucket file cannot carry the dependencies. The file is named
`composer-requirements.txt` rather than `requirements.txt` to make that
obvious, with the command that reads it in a header comment. Run only the
first and you ship DAGs that cannot import their libraries.

The terraform stacks print both commands with the environment already filled
in: `terraform output dags_import_command` and `update_deps_command`.

## Deploy to Astro

```sh
astro login
astro deploy
```

Deploy always asks. With no link named it lists the deployable links and waits
for an answer; `default = true` only highlights `prod`, and Enter takes the
highlighted entry:

```
Deploy to which deployment?
 #     NAME     WHERE                                        PRESELECTED BY
 1     dev      astro deployment your-dev-deployment-id
 2     prod     astro deployment your-prod-deployment-id     default = true

> [2]
```

The label names what moved the cursor, because three different things can:
`default = true` is the committed manifest, `ASTRO_DEPLOYMENT` is a
variable exported in this shell, `astro use` is your own selection. They rank
in that reverse order — env, then selection, then marker — the same as the query
commands.

Name one on the command line to skip the question: `astro deploy dev`, or
`astro deploy --deployment dev`. In CI, where nobody is there to answer, naming
it is required — an `astro use` selection or `ASTRO_DEPLOYMENT` never decides a deploy. `--dags`
ships only the `dags/` folder and needs no Docker at all; `--image` ships only
the image.

Against the shipped placeholder ids you get:

```
Building your project image, this can take a few minutes...
Error: deployment with id your-prod-deployment-id not found
```

which is the manifest doing its job — replace the placeholders with the
terraform outputs first. A name no link declares is sent as a Deployment id,
the way `astro deploy <deployment id>` has always worked, so it fails the same
way.

## The last mile, told straight

**Astro is one command.** `astro deploy` builds the image from the manifest,
checks the runtime version against the Deployment, pushes to the Deployment's
registry, uploads the DAGs, and rolls it out.

**MWAA and Composer are not, yet.** `astro package` builds the right artifact
and prints the exact upload command, and you run it:

```sh
astro package mwaa
aws s3 sync dist/mwaa/ s3://$(cd ../terraform/aws-mwaa && terraform output -raw bucket_name)/

astro package composer
eval "$(cd ../terraform/gcp-composer && terraform output -raw dags_import_command)"
eval "$(cd ../terraform/gcp-composer && terraform output -raw update_deps_command)"
```

And `astro deploy` turns away a link that is not an Astro link, by name and by
kind, listing the ones it could have meant:

```sh
astro deploy prod-mwaa
```

```
Error: link "prod-mwaa" is mwaa, and astro deploy ships to Astro Deployments,
       astro links: dev, prod
```

The same for `prod-composer` (`is composer`) and `staging` (`is endpoint`).
That refusal is the honest current behaviour and it is deliberate: before the
guard landed, `astro deploy prod-mwaa` resolved the link, found no
Deployment id on it, fell through to the unlinked flow, and offered to ship
your DAGs to whichever Astro Deployment you picked. Refusing beats that.

Making `astro deploy` actually ride an MWAA or Composer link — package, then
sync, then update the environment, under your own AWS or Google credentials —
is not yet supported. Until it is, the last mile is the one command the CLI
prints for you.

## A warning you will see, and should

Once you have run the DAGs locally, `include/` holds the SQLite warehouse and
the generated report, and both package targets say:

```
- include/ has files but is not shipped: MWAA and Composer have no include/
  folder, so a DAG that imports from it will break. Move shared modules under
  dags/.
```

That is correct and worth leaving in the demo. This project already keeps its
shared helper in `dags/warehouse.py` for exactly that reason; `include/` here
is only output. Astro ships `include/`; the two managed platforms do not.
