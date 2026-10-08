# Each output names where it goes in demo/project/pyproject.toml.

output "environment_name" {
  description = "MWAA environment name. Goes in [tool.astro.deployments.prod-mwaa] environment."
  value       = aws_mwaa_environment.this.name
}

output "bucket_name" {
  description = "S3 bucket holding the MWAA source (dags/, requirements.txt). Goes in [tool.astro.targets.mwaa] bucket, as s3://<this>."
  value       = aws_s3_bucket.mwaa.id
}

output "region" {
  description = "Region the environment runs in. Goes in [tool.astro.targets.mwaa] region."
  value       = var.region
}

output "webserver_url" {
  description = "Airflow webserver URL. Open this to watch the DAGs after an upload."
  value       = "https://${aws_mwaa_environment.this.webserver_url}"
}

output "airflow_version" {
  description = "Airflow version running on the environment. Compare it with [tool.astro] airflow in the manifest."
  value       = aws_mwaa_environment.this.airflow_version
}

output "environment_class" {
  description = "MWAA environment class."
  value       = aws_mwaa_environment.this.environment_class
}

output "sync_command" {
  description = "The upload `astro package mwaa` tells you to run, with the bucket filled in."
  value       = "aws s3 sync dist/mwaa/ s3://${aws_s3_bucket.mwaa.id}/"
}
