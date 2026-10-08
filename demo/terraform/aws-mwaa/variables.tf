variable "region" {
  description = "AWS region for the MWAA stack."
  type        = string
  default     = "us-east-1"
}

variable "name" {
  description = "MWAA environment name. This is the value that goes in the manifest's mwaa link (environment = '...')."
  type        = string
  default     = "orders-demo-mwaa"
}

variable "airflow_version" {
  description = <<-EOT
    Apache Airflow version for MWAA. 3.2.1 is the newest MWAA offers.

    Note this does not match the demo project's pin of 3.1 — MWAA has no 3.1.
    That gap is the point of `astro local check --target mwaa`, which maps the
    pin down to 3.0.6 (the closest version MWAA runs) and says so. Pin the
    manifest at whatever you build here if you want the two to line up.
  EOT
  type        = string
  default     = "3.2.1"
}

variable "environment_class" {
  description = "MWAA environment class. mw1.micro is the smallest and cheapest."
  type        = string
  default     = "mw1.micro"
}

variable "vpc_cidr" {
  description = "CIDR block for the MWAA VPC."
  type        = string
  default     = "10.20.0.0/16"
}
