# Terraform for the demo

Three independent stacks, one per platform the demo project ships to. Apply
and destroy each on its own; they share no state and no resources.

| Stack | Builds | Wall time | Rough cost while alive |
| --- | --- | --- | --- |
| [`astro/`](astro/) | Two Astro Deployments, dev and prod | 2-5 min | per your Astro contract |
| [`aws-mwaa/`](aws-mwaa/) | One MWAA environment, its VPC, bucket, and role | 30-60 min | about $0.40/hour (mw1.micro plus one NAT gateway) |
| [`gcp-composer/`](gcp-composer/) | One Composer 3 environment | 20-40 min | about $0.40/hour |

**These cost money for as long as they exist.** Nothing here scales to zero.
Run `terraform destroy` when the demo is over.

State is local: each stack writes `terraform.tfstate` in its own directory and
both it and `.terraform/` are gitignored. Those files are the only record of
what was built, so keep them until you destroy.

## What the outputs are for

Every stack's outputs name the manifest key they fill in. The demo project's
`pyproject.toml` ships with placeholders; `terraform output` tells you what to
put in their place.

| From | Output | Manifest key |
| --- | --- | --- |
| astro | `workspace_id` | `[tool.astro] workspace` |
| astro | `dev_deployment_id` | `[tool.astro.deployments.dev] deployment` |
| astro | `prod_deployment_id` | `[tool.astro.deployments.prod] deployment` |
| aws-mwaa | `environment_name` | `[tool.astro.deployments.prod-mwaa] environment` |
| aws-mwaa | `bucket_name` | `[tool.astro.targets.mwaa] bucket` |
| aws-mwaa | `region` | `[tool.astro.targets.mwaa] region` |
| gcp-composer | `environment_name` | `[tool.astro.deployments.prod-composer] environment` |
| gcp-composer | `project` | `[tool.astro.targets.composer] project` |
| gcp-composer | `location` | `[tool.astro.targets.composer] location` |

The astro stack also prints a `manifest_snippet` you can paste straight over
the placeholder links, and the two cloud stacks print the exact upload command
`astro package` will tell you to run, with the bucket or environment filled in.
