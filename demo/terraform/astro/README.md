# astro — two Astro Deployments for the demo

Creates the `dev` and `prod` Deployments the demo project's manifest links to.

**Cost: whatever your Astro contract says.** Two STANDARD deployments with a
SMALL scheduler and workers that scale from zero. Destroy them when the demo
is over — `terraform destroy` here deletes both Deployments, so only run it
when you mean to.

**Time: 2-5 minutes.** Much faster than the two cloud stacks.

## Auth

The provider reads `ASTRO_API_TOKEN` from the environment. Never put a token
in a `.tf` file.

Mint a workspace-scoped token with the CLI you are already logged into:

```sh
astro workspace token create --name orders-demo-tf --role WORKSPACE_OWNER \
  --workspace <your-workspace-id> --clean-output
export ASTRO_API_TOKEN=<the token it prints>
```

`WORKSPACE_OWNER` is what creating Deployments needs. A `WORKSPACE_OPERATOR`
token is enough to adopt Deployments that already exist.

## Run it

`organization_id` and `workspace_id` have no defaults — name your own:

```sh
astro organization list      # organization_id
astro workspace list         # workspace_id

terraform init
terraform apply -var organization_id=<org> -var workspace_id=<ws>
terraform output
```

Tear it down:

```sh
terraform destroy -var organization_id=<org> -var workspace_id=<ws>
```

### Which control plane

`host` defaults to `https://api.astronomer.io`; override it for a different
control plane, and mint the token against that plane too — a token from one
plane will not authenticate against another.

## What it builds

Two STANDARD Deployments, `orders-demo-dev` and `orders-demo-prod`, on the
ASTRO executor with DAG deploy on and one default worker queue that scales
from zero to ten. Both read the same settings from one `locals` block, so
they differ only by name. The workspace is read through a data source, not
created.

## Where the outputs go

```sh
terraform output manifest_snippet     # paste this straight into pyproject.toml
terraform output dev_deployment_id    # → [tool.astro.deployments.dev] deployment
terraform output prod_deployment_id   # → [tool.astro.deployments.prod] deployment
terraform output workspace_id         # → [tool.astro] workspace
```

`manifest_snippet` prints the whole block ready to paste over the placeholders
in `demo/project/pyproject.toml`:

```toml
[tool.astro]
workspace = 'clw…'

[tool.astro.deployments.dev]
deployment = 'clm…'

[tool.astro.deployments.prod]
deployment = 'cln…'
default = true
```

`prod` carries `default = true`, so the query commands fall through to it and
`astro deploy` highlights it in the prompt. Deploy always asks, so the marker
never ships on its own.

## Adopting Deployments you already have

If the Deployments exist — made by hand or by the CLI — adopt them instead of
creating new ones. Add an `imports.tf`:

```hcl
import {
  id = "<existing dev deployment id>"
  to = astro_deployment.dev
}

import {
  id = "<existing prod deployment id>"
  to = astro_deployment.prod
}
```

Then `terraform apply`. Make the attributes in `deployments.tf` match what the
Deployments already have, or the first plan after import will want to change
them.
