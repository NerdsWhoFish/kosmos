variable "project_id" {
  type        = string
  description = "GCP project that owns the Kosmos deployment."
}

variable "region" {
  type        = string
  description = "Cloud Run and Artifact Registry region."
  default     = "us-east1"
}

variable "service_name" {
  type        = string
  description = "Cloud Run service name."
  default     = "kosmos"
}

variable "image" {
  type        = string
  description = "Immutable container image digest to deploy. Null bootstraps infrastructure without creating Cloud Run."
  default     = null
  nullable    = true
}

variable "github_repository" {
  type        = string
  description = "GitHub owner/name allowed to publish release images through OIDC."
  default     = "NerdsWhoFish/kosmos"
}

variable "runtime_service_account_id" {
  type        = string
  description = "Stable service account ID for the Cloud Run runtime."
  default     = "kosmos-runtime"
}

variable "min_instance_count" {
  type        = number
  description = "Minimum Cloud Run instances. Keep zero for near-free idle cost."
  default     = 0
}

variable "max_instance_count" {
  type        = number
  description = "Maximum Cloud Run instances."
  default     = 10
}

variable "container_cpu" {
  type        = string
  description = "Cloud Run CPU allocation."
  default     = "1"
}

variable "container_memory" {
  type        = string
  description = "Cloud Run memory allocation."
  default     = "512Mi"
}

variable "environment" {
  type        = string
  description = "Deployment environment label."
  default     = "production"
}

variable "public_url" {
  type        = string
  description = "Canonical public URL used by OAuth callbacks and externally visible links."
  default     = null
  nullable    = true
}

variable "allow_unauthenticated" {
  type        = bool
  description = "Whether Cloud Run receives public invocations. Put admin auth at the application and edge layers."
  default     = true
}

variable "secret_environment" {
  type        = map(string)
  description = "Map of container environment variable names to Secret Manager secret names."
  default     = {}
}

variable "environment_variables" {
  type        = map(string)
  description = "Non-secret environment variables for the service."
  default     = {}
}
