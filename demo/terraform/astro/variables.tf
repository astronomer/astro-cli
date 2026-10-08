variable "organization_id" {
  description = "Astro organization id. Find it with `astro organization list`."
  type        = string
}

variable "workspace_id" {
  description = "Astro workspace the deployments go in. Find it with `astro workspace list`."
  type        = string
}

variable "host" {
  description = "Astro API host. Defaults to https://api.astronomer.io; override it for a different control plane, and mint the token against that plane."
  type        = string
  default     = "https://api.astronomer.io"
}

variable "name_prefix" {
  description = "Prefix for the two Deployment names."
  type        = string
  default     = "orders-demo"
}

variable "cloud_provider" {
  description = "Cloud the STANDARD deployments run on: AWS, AZURE, or GCP."
  type        = string
  default     = "AWS"
}

variable "region" {
  description = "Region for the STANDARD deployments, in the cloud provider's own spelling."
  type        = string
  default     = "us-east-1"
}
