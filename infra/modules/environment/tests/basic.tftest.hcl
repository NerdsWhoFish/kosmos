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

run "bootstrap_defaults" {
  command = plan

  variables {
    project_id  = "kosmos-test"
    environment = "test"
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
    condition     = length(output.secret_ids) == 3
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
}

run "production_service" {
  command = plan

  variables {
    project_id                  = "kosmos-production"
    environment                 = "production"
    image_digest                = "us-east1-docker.pkg.dev/kosmos-production/kosmos/kosmos@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    public_url                  = "https://cast.nerdswhofish.com"
    google_client_id            = "client.apps.googleusercontent.com"
    faro_url                    = "https://faro.example.com/collect"
    otel_exporter_otlp_endpoint = "https://otlp.example.com/otlp"
    billing_account_id          = "000000-000000-000000"
    budget_notification_email   = "owner@example.com"
  }

  assert {
    condition     = output.deploy_service
    error_message = "an immutable image digest must enable Cloud Run"
  }

  assert {
    condition     = length(google_cloud_run_v2_service.kosmos) == 1
    error_message = "production must create one Cloud Run service"
  }

  assert {
    condition     = google_cloud_run_v2_service.kosmos[0].template[0].scaling[0].min_instance_count == 0
    error_message = "production must scale to zero"
  }

  assert {
    condition     = google_cloud_run_v2_service.kosmos[0].template[0].scaling[0].max_instance_count == 3
    error_message = "production must retain the default cost cap"
  }

  assert {
    condition     = length(google_billing_budget.kosmos) == 1 && length(google_monitoring_notification_channel.billing_email) == 1
    error_message = "billing inputs must enable both the project budget and email channel"
  }
}
