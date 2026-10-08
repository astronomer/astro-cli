# This project requires Composer environments to name a service account
# explicitly, so make a dedicated one and give it the Composer worker role.
resource "google_service_account" "composer" {
  account_id   = "orders-demo-composer"
  display_name = "orders-demo Composer environment"
  project      = var.project
}

resource "google_project_iam_member" "composer_worker" {
  project = var.project
  role    = "roles/composer.worker"
  member  = "serviceAccount:${google_service_account.composer.email}"
}
