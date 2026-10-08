# aws-mwaa — one MWAA environment for the demo

Builds an Amazon MWAA environment the demo project can ship DAGs to, plus the
VPC, S3 bucket, and execution role MWAA needs.

**Cost: about $0.40/hour while it exists** — roughly $0.20 for the `mw1.micro`
environment and about the same for the single NAT gateway. Nothing scales to
zero. Destroy it when the demo is over.

**Time: 30-60 minutes to apply.** MWAA is slow to build. Start it before you
need it.

## Auth

Ordinary AWS credentials from the standard chain — env vars, a profile, SSO,
an instance role. Whichever you use, have it active before you run terraform:

```sh
export AWS_PROFILE=<your-profile>
aws sts get-caller-identity      # confirm you are who you think you are
```

You need permission to create VPCs, NAT gateways, S3 buckets, IAM roles, and
MWAA environments.

## Run it

```sh
terraform init
terraform apply
terraform output
```

Change the region, environment name, Airflow version, or environment class in
`variables.tf`, or pass them with `-var`:

```sh
terraform apply -var region=eu-west-1 -var name=orders-demo-eu
```

Tear it down when you are finished:

```sh
terraform destroy
```

## What it builds

- A small VPC from the `terraform-aws-modules/vpc` module: two private and two
  public subnets across two availability zones, one NAT gateway. MWAA requires
  two private subnets in different AZs; the single NAT keeps the bill down.
- A versioned private S3 bucket, prefixed `orders-demo-mwaa-`. MWAA requires
  versioning, and reads `dags/` and `requirements.txt` from it. Terraform seeds
  both so the environment has something valid to start from.
- An execution role with the documented MWAA minimum policy, scoped to this
  environment, its bucket, its log groups, and the celery queues.
- The environment itself: `mw1.micro`, one scheduler, one worker, one
  webserver, all five log streams on, and `PUBLIC_ONLY` webserver access so
  the UI opens without a bastion.

## Where the outputs go

```sh
terraform output environment_name   # → [tool.astro.deployments.prod-mwaa] environment
terraform output bucket_name        # → [tool.astro.targets.mwaa] bucket, as s3://<this>
terraform output region             # → [tool.astro.targets.mwaa] region
terraform output webserver_url      # the Airflow UI
terraform output sync_command       # the exact aws s3 sync to run after astro package mwaa
```

In `demo/project/pyproject.toml` that becomes:

```toml
[tool.astro.deployments.prod-mwaa]
target = 'mwaa'
environment = 'orders-demo-mwaa'          # environment_name

[tool.astro.targets.mwaa]
region = 'us-east-1'                      # region
bucket = 's3://orders-demo-mwaa-a1b2c3'   # s3:// + bucket_name
```

## The Airflow version gap, on purpose

This stack builds Airflow **3.2.1**, the newest MWAA offers. The demo project
pins **3.1**, which MWAA does not offer at all. That is not an oversight — it
is the thing `astro local check --target mwaa` exists to catch:

```
checking against Airflow 3.0.6 (mapped down from the manifest pin 3.1)
note: mwaa does not offer Airflow 3.1; checking against 3.0.6, the closest
      version it runs — a 3.1-only feature would break there
```

The check maps *down* to the closest version MWAA runs, because a lower
version is what would actually run there if you picked one. Set
`-var airflow_version=3.0.6` to build what the check checks, or pin the
manifest at `3.2` to match what this stack builds — either way you now know,
on your laptop, in seconds.
