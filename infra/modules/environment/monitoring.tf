locals {
  operational_notification_channels = concat(
    tolist(var.monitoring_notification_channels),
    google_monitoring_notification_channel.billing_email[*].name,
  )
}

resource "google_monitoring_alert_policy" "uptime" {
  count = local.deploy_service && var.public_url != null ? 1 : 0

  project      = var.project_id
  display_name = "Kosmos ${var.environment} unavailable"
  combiner     = "OR"
  enabled      = true

  documentation {
    content   = "The public Kosmos health endpoint is unavailable. Follow ${local.operations_runbook_url}."
    mime_type = "text/markdown"
  }

  conditions {
    display_name = "Public health check failed"

    condition_threshold {
      filter          = "resource.type = \"uptime_url\" AND metric.type = \"monitoring.googleapis.com/uptime_check/check_passed\" AND metric.labels.check_id = \"${google_monitoring_uptime_check_config.health[0].uptime_check_id}\""
      comparison      = "COMPARISON_LT"
      threshold_value = 1
      duration        = "120s"

      aggregations {
        alignment_period   = "60s"
        per_series_aligner = "ALIGN_FRACTION_TRUE"
      }

      trigger {
        count = 1
      }
    }
  }

  notification_channels = local.operational_notification_channels

  alert_strategy {
    auto_close = "1800s"
  }
}

resource "google_monitoring_alert_policy" "job_failures" {
  count = local.deploy_service ? 1 : 0

  project      = var.project_id
  display_name = "Kosmos ${var.environment} background job failures"
  combiner     = "OR"
  enabled      = true

  documentation {
    content   = "A private Kosmos integration worker returned a retryable server error. Follow ${local.operations_runbook_url}."
    mime_type = "text/markdown"
  }

  conditions {
    display_name = "Private worker returned 5xx"

    condition_threshold {
      filter          = "resource.type = \"cloud_run_revision\" AND metric.type = \"run.googleapis.com/request_count\" AND resource.labels.service_name = \"${google_cloud_run_v2_service.jobs[0].name}\" AND metric.labels.response_code_class = \"5xx\""
      comparison      = "COMPARISON_GT"
      threshold_value = 0
      duration        = "0s"

      aggregations {
        alignment_period   = "300s"
        per_series_aligner = "ALIGN_RATE"
      }
    }
  }

  notification_channels = local.operational_notification_channels

  alert_strategy {
    auto_close = "1800s"
  }
}

resource "google_monitoring_alert_policy" "queue_backlog" {
  project      = var.project_id
  display_name = "Kosmos ${var.environment} background queue backlog"
  combiner     = "OR"
  enabled      = true

  documentation {
    content   = "Kosmos integration tasks have remained queued. Follow ${local.operations_runbook_url}."
    mime_type = "text/markdown"
  }

  conditions {
    display_name = "Cloud Tasks queue depth remained high"

    condition_threshold {
      filter          = "resource.type = \"cloud_tasks_queue\" AND metric.type = \"cloudtasks.googleapis.com/queue/depth\" AND resource.labels.queue_id = \"${google_cloud_tasks_queue.jobs.name}\" AND resource.labels.location = \"${var.region}\""
      comparison      = "COMPARISON_GT"
      threshold_value = 10
      duration        = "600s"

      aggregations {
        alignment_period   = "300s"
        per_series_aligner = "ALIGN_MAX"
      }
    }
  }

  notification_channels = local.operational_notification_channels

  alert_strategy {
    auto_close = "1800s"
  }

  depends_on = [google_project_service.required]
}

resource "google_monitoring_alert_policy" "scheduler_failures" {
  count = local.deploy_service ? 1 : 0

  project      = var.project_id
  display_name = "Kosmos ${var.environment} integration scheduler failures"
  combiner     = "OR"
  enabled      = true

  documentation {
    content   = "Cloud Scheduler could not start a Kosmos integration pass. Follow ${local.operations_runbook_url}."
    mime_type = "text/markdown"
  }

  conditions {
    display_name = "Cloud Scheduler could not dispatch integration work"

    condition_matched_log {
      filter = "resource.type = \"cloud_scheduler_job\" AND resource.labels.job_id = \"${google_cloud_scheduler_job.integration_sync[0].name}\" AND severity >= ERROR"
    }
  }

  notification_channels = local.operational_notification_channels

  alert_strategy {
    auto_close = "1800s"

    notification_rate_limit {
      period = "300s"
    }
  }
}
