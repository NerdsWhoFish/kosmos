variable "grafana_frontend_error_labels" {
  description = "Additional frontend alert labels, including severity or investigation routing. Service and environment remain module-owned."
  type        = map(string)
  default     = {}
  nullable    = false

  validation {
    condition     = alltrue([for key in keys(var.grafana_frontend_error_labels) : !contains(["service", "environment"], key)])
    error_message = "Frontend alert labels must not override service or environment."
  }
}

variable "grafana_frontend_error_contact_point" {
  description = "Optional existing direct contact point for frontend errors. Null retains notification-policy routing."
  type        = string
  default     = null
  nullable    = true

  validation {
    condition     = var.grafana_frontend_error_contact_point == null || try(trimspace(var.grafana_frontend_error_contact_point) != "", false)
    error_message = "The frontend error contact point must be null or a nonempty existing name."
  }
}
