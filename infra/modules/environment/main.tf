locals {
  name_prefix             = "kosmos-${var.environment}"
  deploy_service          = var.image_digest != null
  public_host             = var.public_url == null ? null : regex("^https://([^/]+)", var.public_url)[0]
  attachments_bucket      = coalesce(var.attachments_bucket_name, "${var.project_id}-attachments")
  runtime_service_account = "${local.name_prefix}-runtime"
  release_service_account = "${local.name_prefix}-releaser"
  labels = merge({
    application = "kosmos"
    environment = var.environment
    managed_by  = "opentofu"
  }, var.labels)
  secret_ids = {
    google_client_secret  = "${local.name_prefix}-google-client-secret"
    session_secret        = "${local.name_prefix}-session-secret"
    otel_exporter_headers = "${local.name_prefix}-otel-exporter-headers"
  }
  secret_environment = merge(
    { KOSMOS_SESSION_SECRET = "session_secret" },
    var.google_client_id == null ? {} : { GOOGLE_CLIENT_SECRET = "google_client_secret" },
    var.otel_exporter_otlp_endpoint == null ? {} : { OTEL_EXPORTER_OTLP_HEADERS = "otel_exporter_headers" },
  )
  environment_variables = merge(
    {
      KOSMOS_ENV                = var.environment
      KOSMOS_GCP_PROJECT        = var.project_id
      KOSMOS_ORGANIZATION_ID    = var.organization_id
      KOSMOS_ATTACHMENTS_BUCKET = local.attachments_bucket
    },
    var.public_url == null ? {} : { KOSMOS_PUBLIC_URL = var.public_url },
    var.google_client_id == null ? {} : { GOOGLE_CLIENT_ID = var.google_client_id },
    length(var.allowed_google_domains) == 0 ? {} : { KOSMOS_ALLOWED_GOOGLE_DOMAINS = join(",", sort(tolist(var.allowed_google_domains))) },
    var.faro_url == null ? {} : { KOSMOS_FARO_URL = var.faro_url },
    var.otel_exporter_otlp_endpoint == null ? {} : { OTEL_EXPORTER_OTLP_ENDPOINT = var.otel_exporter_otlp_endpoint },
  )
  required_services = toset([
    "artifactregistry.googleapis.com",
    "billingbudgets.googleapis.com",
    "cloudtasks.googleapis.com",
    "firestore.googleapis.com",
    "iam.googleapis.com",
    "iamcredentials.googleapis.com",
    "logging.googleapis.com",
    "monitoring.googleapis.com",
    "run.googleapis.com",
    "secretmanager.googleapis.com",
    "storage.googleapis.com",
    "sts.googleapis.com",
  ])
}

resource "google_project_service" "required" {
  for_each = local.required_services

  project            = var.project_id
  service            = each.value
  disable_on_destroy = false
}

data "google_project" "current" {
  project_id = var.project_id

  depends_on = [google_project_service.required]
}

resource "google_artifact_registry_repository" "kosmos" {
  project       = var.project_id
  location      = var.region
  repository_id = "kosmos"
  description   = "Immutable Kosmos application artifacts"
  format        = "DOCKER"
  labels        = local.labels

  depends_on = [google_project_service.required]
}

resource "google_firestore_database" "kosmos" {
  project                           = var.project_id
  name                              = "(default)"
  location_id                       = var.firestore_location
  type                              = "FIRESTORE_NATIVE"
  concurrency_mode                  = "OPTIMISTIC"
  point_in_time_recovery_enablement = var.environment == "production" ? "POINT_IN_TIME_RECOVERY_ENABLED" : "POINT_IN_TIME_RECOVERY_DISABLED"
  delete_protection_state           = "DELETE_PROTECTION_ENABLED"

  depends_on = [google_project_service.required]
}

resource "google_cloud_tasks_queue" "jobs" {
  project  = var.project_id
  name     = "${local.name_prefix}-jobs"
  location = var.region

  rate_limits {
    max_concurrent_dispatches = 2
    max_dispatches_per_second = 1
  }

  retry_config {
    max_attempts       = 5
    max_retry_duration = "3600s"
    min_backoff        = "5s"
    max_backoff        = "300s"
    max_doublings      = 5
  }

  depends_on = [google_project_service.required]
}

resource "google_storage_bucket" "attachments" {
  project                     = var.project_id
  name                        = local.attachments_bucket
  location                    = upper(var.region)
  uniform_bucket_level_access = true
  public_access_prevention    = "enforced"
  force_destroy               = false
  labels                      = local.labels

  retention_policy {
    retention_period = var.attachment_retention_days * 86400
  }

  versioning {
    enabled = true
  }

  lifecycle_rule {
    condition {
      days_since_noncurrent_time = var.attachment_retention_days
    }
    action {
      type = "Delete"
    }
  }

  depends_on = [google_project_service.required]
}

resource "google_service_account" "runtime" {
  project      = var.project_id
  account_id   = local.runtime_service_account
  display_name = "Kosmos ${var.environment} runtime"
}

resource "google_project_iam_member" "runtime_firestore" {
  project = var.project_id
  role    = "roles/datastore.user"
  member  = "serviceAccount:${google_service_account.runtime.email}"
}

resource "google_project_iam_member" "runtime_tasks" {
  project = var.project_id
  role    = "roles/cloudtasks.enqueuer"
  member  = "serviceAccount:${google_service_account.runtime.email}"
}

resource "google_storage_bucket_iam_member" "runtime_attachments" {
  bucket = google_storage_bucket.attachments.name
  role   = "roles/storage.objectAdmin"
  member = "serviceAccount:${google_service_account.runtime.email}"
}

resource "google_iam_workload_identity_pool" "github" {
  project                   = var.project_id
  workload_identity_pool_id = "${local.name_prefix}-github"
  display_name              = "Kosmos ${var.environment} GitHub"

  depends_on = [google_project_service.required]
}

resource "google_iam_workload_identity_pool_provider" "github" {
  project                            = var.project_id
  workload_identity_pool_id          = google_iam_workload_identity_pool.github.workload_identity_pool_id
  workload_identity_pool_provider_id = "github-oidc"
  display_name                       = "GitHub Actions OIDC"

  oidc {
    issuer_uri = "https://token.actions.githubusercontent.com"
  }

  attribute_mapping = {
    "google.subject"       = "assertion.sub"
    "attribute.repository" = "assertion.repository"
  }
  attribute_condition = "attribute.repository == \"${var.github_repository}\""
}

resource "google_service_account" "releaser" {
  project      = var.project_id
  account_id   = local.release_service_account
  display_name = "Kosmos ${var.environment} release publisher"
}

resource "google_artifact_registry_repository_iam_member" "releaser" {
  project    = var.project_id
  location   = var.region
  repository = google_artifact_registry_repository.kosmos.repository_id
  role       = "roles/artifactregistry.writer"
  member     = "serviceAccount:${google_service_account.releaser.email}"
}

resource "google_service_account_iam_member" "releaser" {
  service_account_id = google_service_account.releaser.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/${google_iam_workload_identity_pool.github.name}/attribute.repository/${var.github_repository}"
}

resource "google_secret_manager_secret" "runtime" {
  for_each = local.secret_ids

  project   = var.project_id
  secret_id = each.value
  labels    = local.labels

  replication {
    auto {}
  }

  depends_on = [google_project_service.required]
}

resource "google_secret_manager_secret_iam_member" "runtime" {
  for_each = google_secret_manager_secret.runtime

  project   = var.project_id
  secret_id = each.value.secret_id
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
      min_instance_count = var.min_instances
      max_instance_count = var.max_instances
    }

    containers {
      image = var.image_digest

      resources {
        limits = {
          cpu    = var.container_cpu
          memory = var.container_memory
        }
        cpu_idle = true
      }

      dynamic "env" {
        for_each = local.environment_variables
        content {
          name  = env.key
          value = env.value
        }
      }

      dynamic "env" {
        for_each = local.secret_environment
        content {
          name = env.key
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.runtime[env.value].secret_id
              version = "latest"
            }
          }
        }
      }
    }
  }

  depends_on = [
    google_project_iam_member.runtime_firestore,
    google_secret_manager_secret_iam_member.runtime,
    google_storage_bucket_iam_member.runtime_attachments,
  ]
}

resource "google_cloud_run_v2_service_iam_member" "public" {
  count = local.deploy_service && var.allow_unauthenticated ? 1 : 0

  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.kosmos[0].name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

resource "google_monitoring_notification_channel" "billing_email" {
  count = var.billing_account_id != null && var.budget_notification_email != null ? 1 : 0

  project      = var.project_id
  display_name = "Kosmos ${var.environment} billing"
  type         = "email"
  enabled      = true

  labels = {
    email_address = var.budget_notification_email
  }

  depends_on = [google_project_service.required]
}

resource "google_billing_budget" "kosmos" {
  count = var.billing_account_id != null ? 1 : 0

  billing_account = var.billing_account_id
  display_name    = "Kosmos ${var.environment}"

  budget_filter {
    projects = ["projects/${data.google_project.current.number}"]
  }

  amount {
    specified_amount {
      currency_code = "USD"
      units         = tostring(var.monthly_budget_usd)
    }
  }

  dynamic "threshold_rules" {
    for_each = var.budget_alert_thresholds
    content {
      threshold_percent = threshold_rules.value
    }
  }

  all_updates_rule {
    monitoring_notification_channels = google_monitoring_notification_channel.billing_email[*].name
    enable_project_level_recipients  = true
  }

  deletion_policy = "PREVENT"

  depends_on = [google_project_service.required]
}

resource "google_monitoring_alert_policy" "server_errors" {
  count = local.deploy_service ? 1 : 0

  project      = var.project_id
  display_name = "Kosmos ${var.environment} HTTP 5xx"
  combiner     = "OR"
  enabled      = true

  conditions {
    display_name = "Cloud Run returned 5xx"

    condition_threshold {
      filter          = "resource.type = \"cloud_run_revision\" AND metric.type = \"run.googleapis.com/request_count\" AND resource.labels.service_name = \"${google_cloud_run_v2_service.kosmos[0].name}\" AND metric.labels.response_code_class = \"5xx\""
      comparison      = "COMPARISON_GT"
      threshold_value = 0
      duration        = "0s"

      aggregations {
        alignment_period   = "300s"
        per_series_aligner = "ALIGN_RATE"
      }
    }
  }

  notification_channels = concat(tolist(var.monitoring_notification_channels), google_monitoring_notification_channel.billing_email[*].name)

  alert_strategy {
    auto_close = "1800s"
  }

  depends_on = [google_project_service.required]
}

resource "google_monitoring_uptime_check_config" "health" {
  count = local.deploy_service && var.public_url != null ? 1 : 0

  project      = var.project_id
  display_name = "Kosmos ${var.environment} health"
  timeout      = "10s"
  period       = "300s"

  http_check {
    path         = "/api/v1/health"
    port         = 443
    use_ssl      = true
    validate_ssl = true
  }

  monitored_resource {
    type = "uptime_url"
    labels = {
      project_id = var.project_id
      host       = local.public_host
    }
  }

  depends_on = [google_project_service.required]
}
