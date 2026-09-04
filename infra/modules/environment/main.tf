locals {
  name_prefix             = "kosmos-${var.environment}"
  deploy_service          = var.image_digest != null
  public_host             = var.public_url == null ? null : regex("^https://([^/]+)", var.public_url)[0]
  attachments_bucket      = coalesce(var.attachments_bucket_name, "${var.project_id}-attachments")
  runtime_service_account = "${local.name_prefix}-runtime"
  worker_service_account  = "${local.name_prefix}-worker"
  release_service_account = "${local.name_prefix}-releaser"
  job_invoker_account     = "${local.name_prefix}-job-invoker"
  operations_runbook_url  = "https://github.com/NerdsWhoFish/kosmos/blob/main/docs/operations.md"
  labels = merge({
    application = "kosmos"
    environment = var.environment
    managed_by  = "opentofu"
  }, var.labels)
  secret_ids = {
    google_client_secret  = "${local.name_prefix}-google-client-secret"
    session_secret        = "${local.name_prefix}-session-secret"
    integration_secret    = "${local.name_prefix}-integration-secret"
    otel_exporter_headers = "${local.name_prefix}-otel-exporter-headers"
  }
  web_secret_environment = merge(
    {
      KOSMOS_SESSION_SECRET     = "session_secret"
      KOSMOS_INTEGRATION_SECRET = "integration_secret"
    },
    var.google_client_id == null ? {} : { GOOGLE_CLIENT_SECRET = "google_client_secret" },
    var.otel_exporter_otlp_endpoint == null ? {} : { OTEL_EXPORTER_OTLP_HEADERS = "otel_exporter_headers" },
  )
  worker_secret_environment = merge(
    { KOSMOS_INTEGRATION_SECRET = "integration_secret" },
    var.google_client_id == null ? {} : { GOOGLE_CLIENT_SECRET = "google_client_secret" },
    var.otel_exporter_otlp_endpoint == null ? {} : { OTEL_EXPORTER_OTLP_HEADERS = "otel_exporter_headers" },
  )
  base_environment_variables = merge(
    {
      KOSMOS_ENV             = var.environment
      KOSMOS_GCP_PROJECT     = var.project_id
      KOSMOS_ORGANIZATION_ID = var.organization_id
    },
    var.google_client_id == null ? {} : { GOOGLE_CLIENT_ID = var.google_client_id },
    var.otel_exporter_otlp_endpoint == null ? {} : { OTEL_EXPORTER_OTLP_ENDPOINT = var.otel_exporter_otlp_endpoint },
  )
  web_environment_variables = merge(
    local.base_environment_variables,
    { KOSMOS_ATTACHMENTS_BUCKET = local.attachments_bucket },
    var.public_url == null ? {} : { KOSMOS_PUBLIC_URL = var.public_url },
    length(var.allowed_google_domains) == 0 ? {} : { KOSMOS_ALLOWED_GOOGLE_DOMAINS = join(",", sort(tolist(var.allowed_google_domains))) },
    var.faro_url == null ? {} : { KOSMOS_FARO_URL = var.faro_url },
  )
  job_environment_variables = merge(local.base_environment_variables, {
    KOSMOS_PROCESS_ROLE   = "jobs"
    KOSMOS_TASKS_PROJECT  = var.project_id
    KOSMOS_TASKS_LOCATION = var.region
    KOSMOS_TASKS_QUEUE    = google_cloud_tasks_queue.jobs.name
  })
  pagination_indexes = {
    account_events = {
      collection = "events"
      fields = [
        { path = "kind", order = "ASCENDING" },
        { path = "occurredAt", order = "DESCENDING" },
        { path = "__name__", order = "DESCENDING" },
      ]
    }
    activities = {
      collection = "activities"
      fields = [
        { path = "contactId", order = "ASCENDING" },
        { path = "occurredAt", order = "DESCENDING" },
        { path = "__name__", order = "DESCENDING" },
      ]
    }
    attachments = {
      collection = "attachments"
      fields = [
        { path = "recordType", order = "ASCENDING" },
        { path = "recordId", order = "ASCENDING" },
        { path = "createdAt", order = "DESCENDING" },
        { path = "__name__", order = "DESCENDING" },
      ]
    }
    document_revisions = {
      collection = "documentRevisions"
      fields = [
        { path = "documentId", order = "ASCENDING" },
        { path = "createdAt", order = "DESCENDING" },
        { path = "__name__", order = "DESCENDING" },
      ]
    }
    leads = {
      collection = "contacts"
      fields = [
        { path = "status", order = "ASCENDING" },
        { path = "updatedAt", order = "DESCENDING" },
        { path = "__name__", order = "DESCENDING" },
      ]
    }
  }
  required_services = toset([
    "artifactregistry.googleapis.com",
    "billingbudgets.googleapis.com",
    "cloudtasks.googleapis.com",
    "cloudscheduler.googleapis.com",
    "firestore.googleapis.com",
    "gmail.googleapis.com",
    "iam.googleapis.com",
    "iamcredentials.googleapis.com",
    "logging.googleapis.com",
    "monitoring.googleapis.com",
    "people.googleapis.com",
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

resource "google_firestore_index" "pagination" {
  for_each = local.pagination_indexes

  project         = var.project_id
  database        = google_firestore_database.kosmos.name
  collection      = each.value.collection
  query_scope     = "COLLECTION"
  deletion_policy = "ABANDON"

  dynamic "fields" {
    for_each = each.value.fields
    content {
      field_path = fields.value.path
      order      = fields.value.order
    }
  }
}

resource "google_cloud_tasks_queue" "jobs" {
  project  = var.project_id
  name     = "${local.name_prefix}-jobs"
  location = var.region

  rate_limits {
    max_concurrent_dispatches = 1
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

resource "google_service_account" "worker" {
  project      = var.project_id
  account_id   = local.worker_service_account
  display_name = "Kosmos ${var.environment} worker"
}

resource "google_service_account" "job_invoker" {
  project      = var.project_id
  account_id   = local.job_invoker_account
  display_name = "Kosmos ${var.environment} job invoker"
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

resource "google_project_iam_member" "worker_firestore" {
  project = var.project_id
  role    = "roles/datastore.user"
  member  = "serviceAccount:${google_service_account.worker.email}"
}

resource "google_project_iam_member" "worker_tasks" {
  project = var.project_id
  role    = "roles/cloudtasks.enqueuer"
  member  = "serviceAccount:${google_service_account.worker.email}"
}

resource "google_service_account_iam_member" "runtime_job_invoker" {
  for_each = toset([
    google_service_account.runtime.email,
    google_service_account.worker.email,
  ])

  service_account_id = google_service_account.job_invoker.name
  role               = "roles/iam.serviceAccountUser"
  member             = "serviceAccount:${each.value}"
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

resource "google_secret_manager_secret_version" "integration" {
  secret                 = google_secret_manager_secret.runtime["integration_secret"].id
  secret_data_wo         = var.integration_secret_value
  secret_data_wo_version = var.integration_secret_version
}

resource "google_secret_manager_secret_iam_member" "runtime" {
  for_each = google_secret_manager_secret.runtime

  project   = var.project_id
  secret_id = each.value.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.runtime.email}"
}

resource "google_secret_manager_secret_iam_member" "worker" {
  for_each = local.worker_secret_environment

  project   = var.project_id
  secret_id = google_secret_manager_secret.runtime[each.value].secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.worker.email}"
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
        for_each = merge(local.web_environment_variables, {
          KOSMOS_PROCESS_ROLE                = "web"
          KOSMOS_TASKS_PROJECT               = var.project_id
          KOSMOS_TASKS_LOCATION              = var.region
          KOSMOS_TASKS_QUEUE                 = google_cloud_tasks_queue.jobs.name
          KOSMOS_JOB_TARGET_URL              = google_cloud_run_v2_service.jobs[0].uri
          KOSMOS_JOB_AUDIENCE                = google_cloud_run_v2_service.jobs[0].uri
          KOSMOS_JOB_INVOKER_SERVICE_ACCOUNT = google_service_account.job_invoker.email
        })
        content {
          name  = env.key
          value = env.value
        }
      }

      dynamic "env" {
        for_each = local.web_secret_environment
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
    google_secret_manager_secret_version.integration,
    google_storage_bucket_iam_member.runtime_attachments,
  ]
}

resource "google_cloud_run_v2_service" "jobs" {
  count = local.deploy_service ? 1 : 0

  project  = var.project_id
  name     = "${var.service_name}-jobs"
  location = var.region

  deletion_protection = true

  template {
    service_account = google_service_account.worker.email

    scaling {
      min_instance_count = 0
      max_instance_count = var.job_max_instances
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
        for_each = merge(local.job_environment_variables, {
          KOSMOS_JOB_INVOKER_SERVICE_ACCOUNT = google_service_account.job_invoker.email
        })
        content {
          name  = env.key
          value = env.value
        }
      }

      dynamic "env" {
        for_each = local.worker_secret_environment
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
    google_project_iam_member.worker_firestore,
    google_project_iam_member.worker_tasks,
    google_service_account_iam_member.runtime_job_invoker,
    google_secret_manager_secret_iam_member.worker,
    google_secret_manager_secret_version.integration,
  ]
}

resource "google_cloud_run_v2_service_iam_member" "jobs" {
  count = local.deploy_service ? 1 : 0

  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.jobs[0].name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.job_invoker.email}"
}

resource "google_cloud_scheduler_job" "integration_sync" {
  count = local.deploy_service ? 1 : 0

  project          = var.project_id
  region           = var.region
  name             = "${local.name_prefix}-integration-sync"
  description      = "Queue Gmail and Tiller synchronization during business hours"
  schedule         = var.integration_sync_schedule
  time_zone        = var.integration_sync_time_zone
  attempt_deadline = "180s"

  retry_config {
    retry_count          = 3
    min_backoff_duration = "30s"
    max_backoff_duration = "300s"
    max_doublings        = 3
  }

  http_target {
    uri         = "${google_cloud_run_v2_service.jobs[0].uri}/api/v1/jobs/schedule"
    http_method = "POST"
    body        = base64encode("{}")

    headers = {
      "Content-Type" = "application/json"
    }

    oidc_token {
      service_account_email = google_service_account.job_invoker.email
      audience              = google_cloud_run_v2_service.jobs[0].uri
    }
  }

  depends_on = [
    google_cloud_run_v2_service_iam_member.jobs,
    google_project_service.required,
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

  documentation {
    content   = "Kosmos web requests are returning server errors. Follow ${local.operations_runbook_url}."
    mime_type = "text/markdown"
  }

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

  lifecycle {
    create_before_destroy = true
  }

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
