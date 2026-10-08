resource "google_composer_environment" "this" {
  name    = var.name
  region  = var.region
  project = var.project

  labels = {
    project    = "orders-demo"
    managed-by = "terraform"
    purpose    = "astro-cli-v2-demo"
  }

  config {
    software_config {
      image_version = var.image_version
    }

    node_config {
      service_account = google_service_account.composer.email
    }

    # Smallest preset. Workers are capped at one so the scratch env stays cheap.
    environment_size = "ENVIRONMENT_SIZE_SMALL"

    workloads_config {
      scheduler {
        cpu        = 0.5
        memory_gb  = 2
        storage_gb = 1
        count      = 1
      }
      triggerer {
        cpu       = 0.5
        memory_gb = 2
        count     = 1
      }
      web_server {
        cpu        = 0.5
        memory_gb  = 2
        storage_gb = 1
      }
      worker {
        cpu        = 0.5
        memory_gb  = 2
        storage_gb = 1
        min_count  = 1
        max_count  = 1
      }
    }
  }

  timeouts {
    create = "60m"
    update = "60m"
    delete = "30m"
  }

  depends_on = [google_project_iam_member.composer_worker]
}
