# gcp-composer — one Cloud Composer 3 environment for the demo

Builds a Composer 3 environment the demo project can ship DAGs to, plus the
service account it runs as.

**Cost: about $0.40/hour while it exists.** Composer charges for the
environment whether or not anything runs on it. Destroy it when the demo is
over.

**Time: 20-40 minutes to apply.** Start it before you need it.

## Auth

Terraform's google provider uses your Application Default Credentials. Set
them up once:

```sh
gcloud auth application-default login
gcloud config set project <your-project>
```

Your account needs Composer Administrator, Service Account Admin, and Project
IAM Admin on the project, and the Composer API enabled:

```sh
gcloud services enable composer.googleapis.com --project <your-project>
```

## Run it

`project` has no default — name your own:

```sh
terraform init
terraform apply -var project=<your-gcp-project>
terraform output
```

Region, environment name, and image version are in `variables.tf`. Tear it
down when you are finished:

```sh
terraform destroy -var project=<your-gcp-project>
```

## What it builds

- A service account, `orders-demo-composer`, with `roles/composer.worker`.
  Composer environments must name a service account explicitly in many
  projects, so this stack makes a dedicated one rather than leaning on the
  default compute account.
- The environment itself: Composer 3 on `composer-3-airflow-3.1.8`, the
  smallest size preset, workers capped at one. Composer makes its own GCS
  bucket; the bucket and the Airflow URL come out as outputs.

## Where the outputs go

```sh
terraform output environment_name      # → [tool.astro.deployments.prod-composer] environment
terraform output project               # → [tool.astro.targets.composer] project
terraform output location              # → [tool.astro.targets.composer] location
terraform output airflow_uri           # the Airflow UI
terraform output dags_import_command   # the exact gcloud import to run after astro package composer
terraform output update_deps_command   # the dependency step that goes with it
```

In `demo/project/pyproject.toml` that becomes:

```toml
[tool.astro.deployments.prod-composer]
target = 'composer'
environment = 'orders-demo-composer'   # environment_name

[tool.astro.targets.composer]
project = 'your-gcp-project'           # project
location = 'us-central1'               # location
```

## The Airflow versions line up here

Composer 3 offers Airflow 3.1.8, which satisfies the demo project's pin of
`3.1`. So `astro local check --target composer` checks against exactly what
this stack builds and reports no version gap — unlike the MWAA stack next
door, where the pin has nowhere to land. One command, two honest answers:

```
== composer ==
checking against Airflow 3.1.8
note: composer pins its own Python; checking with the default interpreter,
      so a Python-version-specific issue may not show
```

## Dependencies do not live in the bucket

Composer syncs `dags/` and `plugins/` from GCS but installs PyPI packages on
the environment. `astro package composer` writes both halves and prints both
commands: a `storage dags import` for the bucket and an `environments update
--update-pypi-packages-from-file` for the dependencies. Running only the first
ships DAGs that cannot import their libraries.
