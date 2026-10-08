# The two Deployments the manifest links to: dev and prod. Same shape, so the
# settings live once in a local and each resource reads them.

locals {
  deployment_defaults = {
    type                    = "STANDARD"
    executor                = "ASTRO"
    cloud_provider          = var.cloud_provider
    region                  = var.region
    scheduler_size          = "SMALL"
    default_task_pod_cpu    = "0.25"
    default_task_pod_memory = "0.5Gi"
    resource_quota_cpu      = "10"
    resource_quota_memory   = "20Gi"
    is_high_availability    = false
    is_development_mode     = false
    is_cicd_enforced        = false
    is_dag_deploy_enabled   = true
  }

  worker_queues = [
    {
      name               = "default"
      is_default         = true
      astro_machine      = "A5"
      min_worker_count   = 0
      max_worker_count   = 10
      worker_concurrency = 1
    },
  ]
}

resource "astro_deployment" "dev" {
  name         = "${var.name_prefix}-dev"
  description  = "orders-demo, dev. Linked from pyproject.toml as [tool.astro.deployments.dev]."
  workspace_id = var.workspace_id

  type                    = local.deployment_defaults.type
  executor                = local.deployment_defaults.executor
  cloud_provider          = local.deployment_defaults.cloud_provider
  region                  = local.deployment_defaults.region
  scheduler_size          = local.deployment_defaults.scheduler_size
  default_task_pod_cpu    = local.deployment_defaults.default_task_pod_cpu
  default_task_pod_memory = local.deployment_defaults.default_task_pod_memory
  resource_quota_cpu      = local.deployment_defaults.resource_quota_cpu
  resource_quota_memory   = local.deployment_defaults.resource_quota_memory
  is_high_availability    = local.deployment_defaults.is_high_availability
  is_development_mode     = local.deployment_defaults.is_development_mode
  is_cicd_enforced        = local.deployment_defaults.is_cicd_enforced
  is_dag_deploy_enabled   = local.deployment_defaults.is_dag_deploy_enabled

  contact_emails        = []
  environment_variables = []
  worker_queues         = local.worker_queues
}

resource "astro_deployment" "prod" {
  name         = "${var.name_prefix}-prod"
  description  = "orders-demo, prod. Linked from pyproject.toml as [tool.astro.deployments.prod], the default link."
  workspace_id = var.workspace_id

  type                    = local.deployment_defaults.type
  executor                = local.deployment_defaults.executor
  cloud_provider          = local.deployment_defaults.cloud_provider
  region                  = local.deployment_defaults.region
  scheduler_size          = local.deployment_defaults.scheduler_size
  default_task_pod_cpu    = local.deployment_defaults.default_task_pod_cpu
  default_task_pod_memory = local.deployment_defaults.default_task_pod_memory
  resource_quota_cpu      = local.deployment_defaults.resource_quota_cpu
  resource_quota_memory   = local.deployment_defaults.resource_quota_memory
  is_high_availability    = local.deployment_defaults.is_high_availability
  is_development_mode     = local.deployment_defaults.is_development_mode
  is_cicd_enforced        = local.deployment_defaults.is_cicd_enforced
  is_dag_deploy_enabled   = local.deployment_defaults.is_dag_deploy_enabled

  contact_emails        = []
  environment_variables = []
  worker_queues         = local.worker_queues
}
