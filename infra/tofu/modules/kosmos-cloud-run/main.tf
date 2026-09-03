provider "google" {
  project = var.project_id
  region  = var.region
}

resource "google_project_service" "required" {
  for_each = toset([
    "artifactregistry.googleapis.com",
    "run.googleapis.com",
    "secretmanager.googleapis.com",
  ])

  project            = var.project_id
  service            = each.value
  disable_on_destroy = false
}

resource "google_artifact_registry_repository" "kosmos" {
  project       = var.project_id
  location      = var.region
  repository_id = var.service_name
  description   = "Immutable Kosmos container images"
  format        = "DOCKER"

  depends_on = [google_project_service.required]
}

resource "google_service_account" "runtime" {
  project      = var.project_id
  account_id   = var.runtime_service_account_id
  display_name = "Kosmos Cloud Run runtime"
}

resource "google_secret_manager_secret_iam_member" "runtime" {
  for_each = var.secret_names

  project   = var.project_id
  secret_id = each.value
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.runtime.email}"
}

resource "google_cloud_run_v2_service" "kosmos" {
  project  = var.project_id
  name     = var.service_name
  location = var.region

  deletion_protection = true

  template {
    service_account = google_service_account.runtime.email

    scaling {
      min_instance_count = var.min_instance_count
      max_instance_count = var.max_instance_count
    }

    containers {
      image = var.image

      resources {
        limits = {
          cpu    = var.container_cpu
          memory = var.container_memory
        }
        cpu_idle = true
      }

      env {
        name  = "KOSMOS_ENV"
        value = var.environment
      }

      dynamic "env" {
        for_each = var.environment_variables
        content {
          name  = env.key
          value = env.value
        }
      }

      dynamic "env" {
        for_each = var.secret_names
        content {
          name = upper(replace(env.value, "-", "_"))
          value_source {
            secret_key_ref {
              secret  = "projects/${var.project_id}/secrets/${env.value}"
              version = "latest"
            }
          }
        }
      }
    }
  }

  depends_on = [
    google_project_service.required,
    google_secret_manager_secret_iam_member.runtime,
  ]
}

resource "google_cloud_run_v2_service_iam_member" "public" {
  count = var.allow_unauthenticated ? 1 : 0

  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.kosmos.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}
