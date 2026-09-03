provider "google" {
  project = var.project_id
}

resource "google_project_service" "firestore" {
  project            = var.project_id
  service            = "firestore.googleapis.com"
  disable_on_destroy = false
}

resource "google_firestore_database" "kosmos" {
  project                 = var.project_id
  name                    = var.database_name
  location_id             = var.location_id
  type                    = "FIRESTORE_NATIVE"
  concurrency_mode        = "OPTIMISTIC"
  delete_protection_state = "DELETE_PROTECTION_ENABLED"

  depends_on = [google_project_service.firestore]
}
