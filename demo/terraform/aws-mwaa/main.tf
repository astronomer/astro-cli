provider "aws" {
  region = var.region

  default_tags {
    tags = {
      Project   = "orders-demo"
      ManagedBy = "terraform"
      Purpose   = "astro-cli-v2-demo"
    }
  }
}

data "aws_caller_identity" "current" {}

data "aws_region" "current" {}

data "aws_availability_zones" "available" {
  state = "available"
}
