provider "google" {
  project = var.project_id
}

resource "google_project_service" "storage" {
  project            = var.project_id
  service            = "storage.googleapis.com"
  disable_on_destroy = false
}

resource "google_storage_bucket" "attachments" {
  project                     = var.project_id
  name                        = var.bucket_name
  location                    = var.location
  uniform_bucket_level_access = true
  public_access_prevention    = "enforced"
  force_destroy               = false

  retention_policy {
    retention_period = var.retention_days * 86400
  }

  versioning { enabled = true }

  depends_on = [google_project_service.storage]
}
