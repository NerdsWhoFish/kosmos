mock_provider "google" {
  mock_data "google_project" {
    defaults = {
      number = "123456789012"
    }
  }

  mock_resource "google_service_account" {
    defaults = {
      email = "kosmos-mock@kosmos-test.iam.gserviceaccount.com"
      name  = "projects/kosmos-test/serviceAccounts/kosmos-mock@kosmos-test.iam.gserviceaccount.com"
    }
  }

  mock_resource "google_cloud_run_v2_service" {
    defaults = {
      uri = "https://kosmos-mock-123456789012.us-east1.run.app"
    }
  }
}

mock_provider "grafana" {}

run "bootstrap_defaults" {
  command = plan

  variables {
    project_id               = "kosmos-test"
    environment              = "test"
    integration_secret_value = "test-only-integration-secret-value"
  }

  assert {
    condition     = output.region == "us-east1"
    error_message = "default region must be us-east1"
  }

  assert {
    condition     = output.name_prefix == "kosmos-test"
    error_message = "name prefix must include the environment"
  }

  assert {
    condition     = output.deploy_service == false && output.service_url == null
    error_message = "Cloud Run must not deploy until an immutable image digest is supplied"
  }

  assert {
    condition     = output.attachments_bucket == "kosmos-test-attachments"
    error_message = "attachment bucket must derive from the project by default"
  }

  assert {
    condition     = output.firestore_database == "(default)"
    error_message = "Kosmos must use the project's default Firestore database"
  }

  assert {
    condition     = length(output.secret_ids) == 4
    error_message = "all runtime secret containers must be managed by the environment module"
  }

  assert {
    condition     = length(google_iam_workload_identity_pool.github.display_name) <= 32
    error_message = "the workload identity pool display name must fit GCP's limit"
  }

  assert {
    condition     = var.min_instances == 0 && var.max_instances == 3
    error_message = "Cloud Run defaults must preserve scale-to-zero and the cost cap"
  }

  assert {
    condition     = google_cloud_tasks_queue.jobs.rate_limits[0].max_concurrent_dispatches == 2
    error_message = "the async queue must keep a conservative near-free concurrency cap"
  }

  assert {
    condition     = length(google_firestore_index.pagination) == 4
    error_message = "filtered cursor pagination must ship its required Firestore indexes"
  }
}

run "production_service" {
  command = plan

  variables {
    project_id                  = "kosmos-production"
    environment                 = "production"
    image_digest                = "us-east1-docker.pkg.dev/kosmos-production/kosmos/kosmos@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    public_url                  = "https://cast.nerdswhofish.com"
    google_client_id            = "client.apps.googleusercontent.com"
    allowed_google_domains      = ["nerdswhofish.com", "theoutdoorprogrammer.com", "apollorion.com"]
    organization_id             = "nerds-who-fish"
    faro_url                    = "https://faro.example.com/collect"
    otel_exporter_otlp_endpoint = "https://otlp.example.com/otlp"
    billing_account_id          = "000000-000000-000000"
    budget_notification_email   = "owner@example.com"
    manage_grafana              = true
    grafana_faro_app_id         = "902"
    integration_secret_value    = "test-only-integration-secret-value"
  }

  assert {
    condition     = output.deploy_service
    error_message = "an immutable image digest must enable Cloud Run"
  }

  assert {
    condition     = length(google_cloud_run_v2_service.kosmos) == 1 && length(google_cloud_run_v2_service.jobs) == 1
    error_message = "production must create public web and private job services"
  }

  assert {
    condition     = google_cloud_run_v2_service.kosmos[0].template[0].scaling[0].min_instance_count == 0
    error_message = "production must scale to zero"
  }

  assert {
    condition     = google_cloud_run_v2_service.jobs[0].template[0].scaling[0].min_instance_count == 0 && google_cloud_run_v2_service.jobs[0].template[0].scaling[0].max_instance_count == 1
    error_message = "the private worker must scale to zero with a one-instance cost cap"
  }

  assert {
    condition     = google_cloud_run_v2_service.jobs[0].template[0].service_account == google_service_account.worker.email
    error_message = "the private worker must use its dedicated least-privilege identity"
  }

  assert {
    condition     = length([for env in google_cloud_run_v2_service.jobs[0].template[0].containers[0].env : env if env.name == "KOSMOS_SESSION_SECRET"]) == 0
    error_message = "the private worker must not receive the web session-signing secret"
  }

  assert {
    condition     = length(google_cloud_run_v2_service_iam_member.jobs) == 1 && google_cloud_run_v2_service_iam_member.jobs[0].member == "serviceAccount:${google_service_account.job_invoker.email}"
    error_message = "only the dedicated job identity may invoke the private worker"
  }

  assert {
    condition     = contains([for env in google_cloud_run_v2_service.jobs[0].template[0].containers[0].env : env.value if env.name == "KOSMOS_JOB_INVOKER_SERVICE_ACCOUNT"], google_service_account.job_invoker.email)
    error_message = "the worker must sign Cloud Tasks requests with the dedicated invoker identity"
  }

  assert {
    condition     = contains([for env in google_cloud_run_v2_service.kosmos[0].template[0].containers[0].env : env.value if env.name == "KOSMOS_JOB_TARGET_URL"], google_cloud_run_v2_service.jobs[0].uri)
    error_message = "the web service must enqueue tasks to the actual private worker URL"
  }

  assert {
    condition     = length([for env in google_cloud_run_v2_service.jobs[0].template[0].containers[0].env : env.value if env.name == "KOSMOS_JOB_TARGET_URL"]) == 0
    error_message = "the worker must derive its generated Cloud Run URL from the authenticated scheduler request"
  }

  assert {
    condition     = google_cloud_scheduler_job.integration_sync[0].schedule == "0 9-17 * * 1-5" && google_cloud_scheduler_job.integration_sync[0].time_zone == "America/New_York"
    error_message = "integration synchronization must run hourly from 9 through 5 on weekdays"
  }

  assert {
    condition     = google_cloud_scheduler_job.integration_sync[0].http_target[0].oidc_token[0].service_account_email == google_service_account.job_invoker.email
    error_message = "the scheduler must authenticate as the dedicated job invoker"
  }

  assert {
    condition     = google_cloud_run_v2_service.kosmos[0].template[0].scaling[0].max_instance_count == 3
    error_message = "production must retain the default cost cap"
  }

  assert {
    condition     = contains([for env in google_cloud_run_v2_service.kosmos[0].template[0].containers[0].env : env.value if env.name == "KOSMOS_ALLOWED_GOOGLE_DOMAINS"], "apollorion.com,nerdswhofish.com,theoutdoorprogrammer.com")
    error_message = "production must pass the sorted Google domain allowlist to Cloud Run"
  }

  assert {
    condition     = contains([for env in google_cloud_run_v2_service.kosmos[0].template[0].containers[0].env : env.value if env.name == "KOSMOS_ORGANIZATION_ID"], "nerds-who-fish")
    error_message = "production must pass the shared organization scope to Cloud Run"
  }

  assert {
    condition     = length(google_billing_budget.kosmos) == 1 && length(google_monitoring_notification_channel.billing_email) == 1
    error_message = "billing inputs must enable both the project budget and email channel"
  }

  assert {
    condition     = length(google_monitoring_alert_policy.server_errors) == 1
    error_message = "production must alert on Cloud Run server errors"
  }

  assert {
    condition     = length(google_monitoring_alert_policy.uptime) == 1 && length(google_monitoring_alert_policy.job_failures) == 1 && length(google_monitoring_alert_policy.scheduler_failures) == 1 && google_monitoring_alert_policy.queue_backlog.enabled
    error_message = "production must alert on downtime, job failures, scheduler failures, and queue backlog"
  }

  assert {
    condition     = length(google_monitoring_uptime_check_config.health) == 1 && google_monitoring_uptime_check_config.health[0].monitored_resource[0].labels.host == "cast.nerdswhofish.com"
    error_message = "production must monitor the public health endpoint"
  }


  assert {
    condition     = length(grafana_dashboard.kosmos) == 1 && length(grafana_rule_group.kosmos) == 1
    error_message = "production must manage the Kosmos Grafana dashboard and alert rule"
  }
}
