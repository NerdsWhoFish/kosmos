variable "otel_exporter_headers_secrets" {
  description = "Optional existing same-project web and jobs header secrets pinned to numeric versions for an atomic destination cutover."
  type = map(object({
    secret_id = string
    version   = string
  }))
  default  = {}
  nullable = false

  validation {
    condition = length(var.otel_exporter_headers_secrets) == 0 || (
      toset(keys(var.otel_exporter_headers_secrets)) == toset(["web", "jobs"]) &&
      var.otel_exporter_otlp_endpoint != null &&
      alltrue([for ref in values(var.otel_exporter_headers_secrets) :
        can(regex("^[A-Za-z0-9_-]{1,255}$", ref.secret_id)) &&
        can(regex("^[1-9][0-9]*$", ref.version))
      ])
    )
    error_message = "Provide both web and jobs same-project secret IDs with numeric versions and an enabled endpoint, or leave the map empty."
  }
}

locals {
  otel_header_references = merge({
    for component in ["web", "jobs"] : component => {
      secret_id = google_secret_manager_secret.runtime["otel_exporter_headers"].secret_id
      version   = "latest"
    }
  }, var.otel_exporter_headers_secrets)
}

# Retain the old grants because existing revisions still reference that secret.
resource "google_secret_manager_secret_iam_member" "external_otel_headers" {
  for_each = var.otel_exporter_headers_secrets

  project   = var.project_id
  secret_id = each.value.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${each.key == "web" ? google_service_account.runtime.email : google_service_account.worker.email}"
}
