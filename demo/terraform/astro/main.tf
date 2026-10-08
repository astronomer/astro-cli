provider "astro" {
  organization_id = var.organization_id

  # Which control plane. Defaults to production; see variables.tf for the
  # dev-plane value.
  host = var.host

  # The token comes from the ASTRO_API_TOKEN env var. Never put it in a .tf
  # file — the provider reads the variable itself.
}

# The workspace already exists; read it, don't create it.
data "astro_workspace" "demo" {
  id = var.workspace_id
}
