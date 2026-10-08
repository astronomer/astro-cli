# Each output names where it goes in demo/project/pyproject.toml.

output "workspace_name" {
  description = "Workspace the deployments live in."
  value       = data.astro_workspace.demo.name
}

output "workspace_id" {
  description = "Workspace id. Goes in [tool.astro] workspace, the default every astro link inherits."
  value       = var.workspace_id
}

output "dev_deployment_id" {
  description = "Goes in [tool.astro.deployments.dev] deployment."
  value       = astro_deployment.dev.id
}

output "prod_deployment_id" {
  description = "Goes in [tool.astro.deployments.prod] deployment."
  value       = astro_deployment.prod.id
}

output "manifest_snippet" {
  description = "Paste this over the placeholder links in demo/project/pyproject.toml."
  value       = <<-EOT
    [tool.astro]
    workspace = '${var.workspace_id}'

    [tool.astro.deployments.dev]
    deployment = '${astro_deployment.dev.id}'

    [tool.astro.deployments.prod]
    deployment = '${astro_deployment.prod.id}'
    default = true
  EOT
}
