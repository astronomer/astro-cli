# Each output names where it goes in demo/project/pyproject.toml.

output "environment_name" {
  description = "Composer environment name. Goes in [tool.astro.deployments.prod-composer] environment."
  value       = google_composer_environment.this.name
}

output "project" {
  description = "GCP project the environment lives in. Goes in [tool.astro.targets.composer] project."
  value       = var.project
}

output "location" {
  description = "Region the environment lives in. Goes in [tool.astro.targets.composer] location."
  value       = var.region
}

output "gcs_bucket" {
  description = "GCS bucket Composer made for dags, plugins, and data."
  value       = replace(google_composer_environment.this.config[0].dag_gcs_prefix, "/dags", "")
}

output "dag_gcs_prefix" {
  description = "GCS prefix Composer syncs dags from. The gcloud import command below writes here for you."
  value       = google_composer_environment.this.config[0].dag_gcs_prefix
}

output "airflow_uri" {
  description = "Airflow web UI URL. Open this to watch the DAGs after an upload."
  value       = google_composer_environment.this.config[0].airflow_uri
}

output "image_version" {
  description = "Composer image version, Airflow version included. Compare it with [tool.astro] airflow in the manifest."
  value       = google_composer_environment.this.config[0].software_config[0].image_version
}

output "dags_import_command" {
  description = "The upload `astro package composer` tells you to run, with the environment filled in."
  value       = "gcloud composer environments storage dags import --environment=${google_composer_environment.this.name} --location=${var.region} --source=dist/composer/dags"
}

output "update_deps_command" {
  description = "The dependency step `astro package composer` tells you to run, with the environment filled in."
  value       = "gcloud composer environments update ${google_composer_environment.this.name} --location=${var.region} --update-pypi-packages-from-file dist/composer/composer-requirements.txt"
}
