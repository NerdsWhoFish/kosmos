provider "google" {
  project = var.project_id
  region  = var.region
}

locals {
  deploy_service = var.image != null
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

resource "google_iam_workload_identity_pool" "github" {
  project                   = var.project_id
  workload_identity_pool_id = "${var.service_name}-github"
  display_name              = "Kosmos GitHub releases"
  depends_on                = [google_project_service.required]
}

resource "google_iam_workload_identity_pool_provider" "github" {
  project                            = var.project_id
  workload_identity_pool_id          = google_iam_workload_identity_pool.github.workload_identity_pool_id
  workload_identity_pool_provider_id = "github-oidc"
  display_name                       = "GitHub Actions OIDC"

  oidc { issuer_uri = "https://token.actions.githubusercontent.com" }

  attribute_mapping = {
    "google.subject"       = "assertion.sub"
    "attribute.repository" = "assertion.repository"
  }
  attribute_condition = "attribute.repository == \"${var.github_repository}\""
}

resource "google_service_account" "releaser" {
  project      = var.project_id
  account_id   = "${var.service_name}-releaser"
  display_name = "Kosmos release publisher"
}

resource "google_artifact_registry_repository_iam_member" "releaser" {
  project    = var.project_id
  location   = var.region
  repository = google_artifact_registry_repository.kosmos.repository_id
  role       = "roles/artifactregistry.writer"
  member     = google_service_account.releaser.member
}

resource "google_service_account_iam_member" "releaser" {
  service_account_id = google_service_account.releaser.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/${google_iam_workload_identity_pool.github.name}/attribute.repository/${var.github_repository}"
}

resource "google_secret_manager_secret_iam_member" "runtime" {
  for_each = var.secret_names

  project   = var.project_id
  secret_id = each.value
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.runtime.email}"
}

resource "google_cloud_run_v2_service" "kosmos" {
  count = local.deploy_service ? 1 : 0

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
  count = var.allow_unauthenticated && local.deploy_service ? 1 : 0

  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.kosmos[0].name
  role     = "roles/run.invoker"
  member   = "allUsers"
}
