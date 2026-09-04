variable "project_id" {
  description = "Existing GCP project ID for this Kosmos environment."
  type        = string

  validation {
    condition     = length(trimspace(var.project_id)) > 0
    error_message = "project_id must not be empty."
  }
}

variable "environment" {
  description = "Kosmos environment."
  type        = string

  validation {
    condition     = contains(["test", "production"], var.environment)
    error_message = "environment must be test or production."
  }
}

variable "organization_id" {
  description = "Stable organization scope used for shared workspace records."
  type        = string
  default     = "default"

  validation {
    condition     = can(regex("^[a-z0-9][a-z0-9-]{1,62}$", var.organization_id))
    error_message = "organization_id must contain 2 to 63 lowercase letters, numbers, or hyphens."
  }
}

variable "region" {
  description = "Cloud Run and Artifact Registry region."
  type        = string
  default     = "us-east1"

  validation {
    condition     = startswith(var.region, "us-")
    error_message = "Kosmos regions must initially be US regions."
  }
}

variable "firestore_location" {
  description = "Firestore location. Treat this as immutable after database creation."
  type        = string
  default     = "nam5"

  validation {
    condition     = startswith(var.firestore_location, "us-") || startswith(var.firestore_location, "nam")
    error_message = "firestore_location must be a US regional or North America multi-region location."
  }
}

variable "image_digest" {
  description = "Immutable Kosmos container reference. Null creates bootstrap resources without Cloud Run."
  type        = string
  default     = null
  nullable    = true

  validation {
    condition     = var.image_digest == null || can(regex("@sha256:[0-9a-f]{64}$", var.image_digest))
    error_message = "image_digest must be null or an immutable @sha256 reference."
  }
}

variable "integration_secret_value" {
  description = "Encryption and signing key for provider tokens and attachment links. Supplied write-only and never persisted in state."
  type        = string
  sensitive   = true
  ephemeral   = true

  validation {
    condition     = length(var.integration_secret_value) >= 32
    error_message = "integration_secret_value must contain at least 32 characters."
  }
}

variable "integration_secret_version" {
  description = "Monotonic write-only version used to rotate the integration secret."
  type        = number
  default     = 1

  validation {
    condition     = var.integration_secret_version >= 1 && floor(var.integration_secret_version) == var.integration_secret_version
    error_message = "integration_secret_version must be a positive integer."
  }
}

variable "service_name" {
  description = "Cloud Run service name."
  type        = string
  default     = "kosmos"

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{0,48}[a-z0-9]$", var.service_name))
    error_message = "service_name must be a valid Cloud Run service name between 2 and 50 characters."
  }
}

variable "public_url" {
  description = "Canonical public HTTPS origin used by OAuth and browser links."
  type        = string
  default     = null
  nullable    = true

  validation {
    condition     = var.public_url == null || can(regex("^https://[^/]+(?:/.*)?$", var.public_url))
    error_message = "public_url must be null or an absolute HTTPS URL."
  }
}

variable "google_client_id" {
  description = "Public Google OAuth web client ID."
  type        = string
  default     = null
  nullable    = true
}

variable "allowed_google_domains" {
  description = "Verified Google email domains allowed to receive a Kosmos session. Production deployments must provide at least one."
  type        = set(string)
  default     = []

  validation {
    condition     = alltrue([for domain in var.allowed_google_domains : can(regex("^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$", domain))])
    error_message = "allowed_google_domains must contain lowercase DNS domain names."
  }
}

variable "faro_url" {
  description = "Optional Grafana Faro collector URL returned through runtime configuration."
  type        = string
  default     = null
  nullable    = true

  validation {
    condition     = var.faro_url == null || can(regex("^https://", var.faro_url))
    error_message = "faro_url must be null or an HTTPS URL."
  }
}

variable "otel_exporter_otlp_endpoint" {
  description = "Optional HTTPS OTLP endpoint for backend traces and logs."
  type        = string
  default     = null
  nullable    = true

  validation {
    condition     = var.otel_exporter_otlp_endpoint == null || can(regex("^https://[^/?#]+(?:/[^?#]*)?$", var.otel_exporter_otlp_endpoint))
    error_message = "otel_exporter_otlp_endpoint must be null or an absolute HTTPS URL without a query string or fragment."
  }
}

variable "github_repository" {
  description = "GitHub owner/name trusted to publish Quill release images through OIDC."
  type        = string
  default     = "NerdsWhoFish/kosmos"

  validation {
    condition     = can(regex("^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$", var.github_repository))
    error_message = "github_repository must be owner/name."
  }
}

variable "min_instances" {
  description = "Cloud Run minimum instances. The near-free idle-cost invariant requires zero."
  type        = number
  default     = 0

  validation {
    condition     = var.min_instances == 0
    error_message = "Kosmos requires min_instances = 0 unless the architecture decision is changed."
  }
}

variable "max_instances" {
  description = "Hard Cloud Run cost and surge cap."
  type        = number
  default     = 3

  validation {
    condition     = var.max_instances >= 1 && var.max_instances <= 25
    error_message = "max_instances must be between 1 and 25."
  }
}

variable "job_max_instances" {
  description = "Hard concurrency and cost cap for the private background worker."
  type        = number
  default     = 1

  validation {
    condition     = var.job_max_instances >= 1 && var.job_max_instances <= 3
    error_message = "job_max_instances must be between 1 and 3."
  }
}

variable "integration_sync_schedule" {
  description = "Cron schedule for queueing Gmail and Tiller synchronization."
  type        = string
  default     = "0 9-17 * * 1-5"
}

variable "integration_sync_time_zone" {
  description = "IANA time zone used for the integration synchronization schedule."
  type        = string
  default     = "America/New_York"
}

variable "container_cpu" {
  description = "Cloud Run CPU allocation."
  type        = string
  default     = "1"
}

variable "container_memory" {
  description = "Cloud Run memory allocation."
  type        = string
  default     = "512Mi"
}

variable "allow_unauthenticated" {
  description = "Allow public Cloud Run invocation while application authentication protects private routes."
  type        = bool
  default     = true
}

variable "uptime_check_enabled" {
  description = "Run the public health probe and evaluate its availability alert."
  type        = bool
  default     = true
}

variable "attachments_bucket_name" {
  description = "Globally unique private attachment bucket name. Null derives it from project_id."
  type        = string
  default     = null
  nullable    = true

  validation {
    condition     = var.attachments_bucket_name == null || can(regex("^[a-z0-9][a-z0-9._-]{1,61}[a-z0-9]$", var.attachments_bucket_name))
    error_message = "attachments_bucket_name must be null or a valid Cloud Storage bucket name."
  }
}

variable "attachment_retention_days" {
  description = "Minimum retention period for attachments and receipts."
  type        = number
  default     = 30

  validation {
    condition     = var.attachment_retention_days >= 1
    error_message = "attachment_retention_days must be at least one."
  }
}

variable "billing_account_id" {
  description = "Optional billing account ID used to create a project-scoped budget."
  type        = string
  default     = null
  nullable    = true
}

variable "monthly_budget_usd" {
  description = "Monthly alerts-only project budget in whole USD."
  type        = number
  default     = 5

  validation {
    condition     = var.monthly_budget_usd >= 1
    error_message = "monthly_budget_usd must be at least one."
  }
}

variable "budget_alert_thresholds" {
  description = "Current-spend percentages that trigger budget alerts."
  type        = set(number)
  default     = [0.5, 0.8, 1.0]

  validation {
    condition     = alltrue([for threshold in var.budget_alert_thresholds : threshold > 0 && threshold <= 1])
    error_message = "budget_alert_thresholds values must be greater than zero and at most one."
  }
}

variable "budget_notification_email" {
  description = "Optional email address that receives billing-budget alerts."
  type        = string
  default     = null
  nullable    = true

  validation {
    condition     = var.budget_notification_email == null || can(regex("^[^@\\s]+@[^@\\s]+\\.[^@\\s]+$", var.budget_notification_email))
    error_message = "budget_notification_email must be null or a valid email address."
  }
}

variable "monitoring_notification_channels" {
  description = "Existing Monitoring notification-channel resource names for operational alerts."
  type        = set(string)
  default     = []
}

variable "manage_grafana" {
  description = "Manage the Kosmos Grafana dashboard and application alert rules."
  type        = bool
  default     = false
}

variable "grafana_faro_app_id" {
  description = "Grafana Frontend Observability application ID used by dashboards and alert rules."
  type        = string
  default     = null
  nullable    = true

  validation {
    condition     = !var.manage_grafana || (var.grafana_faro_app_id != null && can(regex("^[0-9]+$", var.grafana_faro_app_id)))
    error_message = "grafana_faro_app_id must be a numeric application ID when manage_grafana is enabled."
  }
}

variable "grafana_logs_datasource_uid" {
  description = "Grafana Loki datasource UID used by dashboards and alert rules."
  type        = string
  default     = "grafanacloud-logs"
}

variable "labels" {
  description = "Additional labels applied to resources that support labels."
  type        = map(string)
  default     = {}
}
