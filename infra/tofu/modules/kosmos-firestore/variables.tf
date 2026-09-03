variable "project_id" {
  type        = string
  description = "GCP project that owns the Firestore database."
}

variable "location_id" {
  type        = string
  description = "Firestore location, selected to match the application region."
  default     = "nam5"
}

variable "database_name" {
  type        = string
  description = "Firestore database name. Use (default) for the first database in a project."
  default     = "(default)"
}
