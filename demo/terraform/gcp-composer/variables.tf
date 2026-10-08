variable "project" {
  description = "GCP project to build the Composer environment in. No default on purpose: name your own."
  type        = string
}

variable "region" {
  description = "GCP region for the Composer environment."
  type        = string
  default     = "us-central1"
}

variable "name" {
  description = "Composer environment name. This is the value that goes in the manifest's composer link (environment = '...')."
  type        = string
  default     = "orders-demo-composer"
}

variable "image_version" {
  description = <<-EOT
    Composer image version: Composer 3 on the newest Airflow it offers.

    3.1.8 satisfies the demo project's pin of 3.1, so
    `astro local check --target composer` checks against exactly this and
    reports no version gap.
  EOT
  type        = string
  default     = "composer-3-airflow-3.1.8"
}
